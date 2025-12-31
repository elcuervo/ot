package main

import (
	"bufio"
	"cmp"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/savioxavier/termlink"
)

//go:embed VERSION
var version string

const (
	defaultWindowHeight = 24
	defaultWindowWidth  = 80
	reservedUILines     = 5 // title(1) + padding(1) + newline(1) + help margin(1) + help(1)
	minVisibleHeight    = 3
)

var (
	checkboxRe      = regexp.MustCompile(`^(\s*-\s*)\[([ xX])\](.*)$`)
	doneRe          = regexp.MustCompile(`\s*✅\s*\d{4}-\d{2}-\d{2}`)
	taskRe          = regexp.MustCompile(`^\s*-\s*\[([ xX])\]\s*(.*)$`)
	dueDateRe       = regexp.MustCompile(`📅\s*(\d{4}-\d{2}-\d{2})`)
	blockRe         = regexp.MustCompile("(?s)```tasks\\s*\\n(.+?)```")
	headerRe        = regexp.MustCompile(`(?m)^##\s+(.+)$`)
	groupByFuncRe   = regexp.MustCompile(`group by function task\.file\.(\w+)`)
	groupBySimpleRe = regexp.MustCompile(`group by (\w+)`)
	dateFilterRe    = regexp.MustCompile(`(due|scheduled|done)\s+((?:today|tomorrow|yesterday)(?:\s+or\s+(?:today|tomorrow|yesterday))*|before\s+\S+|after\s+\S+|on\s+\S+(?:\s+or\s+\S+)*)`)
	mdLinkRe        = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

// Task represents a single task from a markdown file
type Task struct {
	FilePath    string     // Source file path
	LineNumber  int        // 1-indexed line number
	RawLine     string     // Original line content
	Done        bool       // Is the task completed?
	Description string     // Task text without checkbox
	Modified    bool       // Has this task been modified?
	DueDate     *time.Time // 📅 due date (nil if none)
}

// Toggle switches the task between done and not done
func (t *Task) Toggle() {
	t.Done = !t.Done
	t.Modified = true
	t.updateRawLine()
}

// updateRawLine rebuilds the raw line based on current state
func (t *Task) updateRawLine() {
	matches := checkboxRe.FindStringSubmatch(t.RawLine)
	if matches == nil {
		return
	}

	prefix := matches[1]  // "- " or "  - " etc
	content := matches[3] // Everything after checkbox

	// Remove existing done date if present
	content = doneRe.ReplaceAllString(content, "")

	if t.Done {
		doneDate := time.Now().Format("2006-01-02")
		t.RawLine = fmt.Sprintf("%s[x]%s ✅ %s", prefix, content, doneDate)
	} else {
		t.RawLine = fmt.Sprintf("%s[ ]%s", prefix, content)
	}
}

// rebuildRawLine rebuilds the raw line with a new description
func (t *Task) rebuildRawLine() {
	matches := checkboxRe.FindStringSubmatch(t.RawLine)
	if matches == nil {
		return
	}

	prefix := matches[1] // "- " or "  - " etc
	checkbox := "[ ]"
	if t.Done {
		checkbox = "[x]"
	}

	t.RawLine = fmt.Sprintf("%s%s %s", prefix, checkbox, t.Description)
}

// renderWithLinks converts markdown links [text](url) to clickable terminal hyperlinks
func renderWithLinks(text string) string {
	return mdLinkRe.ReplaceAllStringFunc(text, func(match string) string {
		parts := mdLinkRe.FindStringSubmatch(match)
		if len(parts) == 3 {
			linkText := parts[1]
			url := parts[2]
			return termlink.Link(linkText, url)
		}
		return match
	})
}

// scanVault recursively finds all .md files in a directory
func scanVault(vaultPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}

		// Collect .md files
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// parseDueDate extracts due date from task description (📅 YYYY-MM-DD)
func parseDueDate(description string) *time.Time {
	matches := dueDateRe.FindStringSubmatch(description)
	if matches == nil {
		return nil
	}
	date, err := time.Parse("2006-01-02", matches[1])
	if err != nil {
		return nil
	}
	return &date
}

// parseFile extracts tasks from a markdown file
func parseFile(filePath string) ([]*Task, error) {
	file, err := os.Open(filePath)

	if err != nil {
		return nil, err
	}

	defer file.Close()

	var tasks []*Task

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		matches := taskRe.FindStringSubmatch(line)

		if matches != nil {
			status := strings.ToLower(matches[1])
			description := strings.TrimSpace(matches[2])

			tasks = append(tasks, &Task{
				FilePath:    filePath,
				LineNumber:  lineNum,
				RawLine:     line,
				Done:        status == "x",
				Description: description,
				DueDate:     parseDueDate(description),
			})
		}
	}

	return tasks, scanner.Err()
}

// DateFilter represents a date-based filter
type DateFilter struct {
	Field    string // "due", "scheduled", "done"
	Operator string // "today", "before", "after", "on"
	Date     string // "today", "tomorrow", or YYYY-MM-DD
	Dates    []string
}

// Query represents parsed query options
type Query struct {
	Name        string // Section name (from ## header before the block)
	NotDone     bool
	GroupBy     string       // "folder", "filename", or ""
	DateFilters []DateFilter // date-based filters
}

type Config struct {
	DefaultProfile string             `toml:"default_profile"`
	Profiles       map[string]Profile `toml:"profiles"`
}

type Profile struct {
	Vault  string `toml:"vault"`
	Query  string `toml:"query"`
	Editor string `toml:"editor"` // "external" or "inline", defaults to external
}

type ResolvedProfile struct {
	Name       string
	VaultPath  string
	QueryPath  string
	EditorMode string // "external" or "inline"
}

type ProfileError struct {
	Profile string
	Field   string
	Err     error
}

func (e *ProfileError) Error() string {
	if e.Profile == "" {
		return fmt.Sprintf("config: %s: %v", e.Field, e.Err)
	}

	if e.Field == "" {
		return fmt.Sprintf("profile %q: %v", e.Profile, e.Err)
	}

	return fmt.Sprintf("profile %q: %s: %v", e.Profile, e.Field, e.Err)
}

func (e *ProfileError) Unwrap() error {
	return e.Err
}

var (
	ErrEmptyPath    = errors.New("path is empty")
	ErrPathNotExist = errors.New("path does not exist")
	ErrNotDirectory = errors.New("path is not a directory")
)

func validateProfile(name string, p Profile) error {
	if strings.TrimSpace(p.Vault) == "" {
		return &ProfileError{Profile: name, Field: "vault", Err: ErrEmptyPath}
	}

	if strings.TrimSpace(p.Query) == "" {
		return &ProfileError{Profile: name, Field: "query", Err: ErrEmptyPath}
	}

	return nil
}

func validateVaultExists(name, vaultPath string) error {
	info, err := os.Stat(vaultPath)

	if err != nil {
		if os.IsNotExist(err) {
			return &ProfileError{Profile: name, Field: "vault", Err: fmt.Errorf("%w: %s", ErrPathNotExist, vaultPath)}
		}

		return &ProfileError{Profile: name, Field: "vault", Err: err}
	}

	if !info.IsDir() {
		return &ProfileError{Profile: name, Field: "vault", Err: fmt.Errorf("%w: %s", ErrNotDirectory, vaultPath)}
	}

	return nil
}

func validateConfig(cfg Config) error {
	if cfg.DefaultProfile != "" && cfg.Profiles != nil {
		if _, ok := cfg.Profiles[cfg.DefaultProfile]; !ok {
			return &ProfileError{Field: "default_profile", Err: fmt.Errorf("profile %q not found", cfg.DefaultProfile)}
		}
	}

	return nil
}

func selectProfile(profileFlag string, cfg Config) (string, *Profile, error) {
	if profileFlag != "" {
		if cfg.Profiles == nil {
			return "", nil, &ProfileError{Profile: profileFlag, Err: errors.New("no profiles defined in config")}
		}

		p, ok := cfg.Profiles[profileFlag]

		if !ok {
			return "", nil, &ProfileError{Profile: profileFlag, Err: errors.New("profile not found")}
		}

		return profileFlag, &p, nil
	}

	if cfg.DefaultProfile != "" {
		if cfg.Profiles == nil {
			return "", nil, &ProfileError{Field: "default_profile", Err: fmt.Errorf("profile %q not found", cfg.DefaultProfile)}
		}

		p, ok := cfg.Profiles[cfg.DefaultProfile]

		if !ok {
			return "", nil, &ProfileError{Field: "default_profile", Err: fmt.Errorf("profile %q not found", cfg.DefaultProfile)}
		}

		return cfg.DefaultProfile, &p, nil
	}

	return "", nil, nil
}

func resolveProfilePaths(name string, p Profile) (*ResolvedProfile, error) {
	if err := validateProfile(name, p); err != nil {
		return nil, err
	}

	vaultPath, err := resolveVaultPath(p.Vault)

	if err != nil {
		return nil, &ProfileError{Profile: name, Field: "vault", Err: err}
	}

	vaultPath = filepath.Clean(vaultPath)
	resolved, err := filepath.EvalSymlinks(vaultPath)
	if err == nil {
		vaultPath = resolved
	}

	if err := validateVaultExists(name, vaultPath); err != nil {
		return nil, err
	}

	queryPath, err := resolveQueryPath(p.Query, vaultPath)

	if err != nil {
		return nil, &ProfileError{Profile: name, Field: "query", Err: err}
	}

	queryPath = filepath.Clean(queryPath)

	return &ResolvedProfile{Name: name, VaultPath: vaultPath, QueryPath: queryPath, EditorMode: p.Editor}, nil
}

// parseQueryFile checks if the query file contains "not done" filter (simple version)
func parseQueryFile(filePath string) (bool, error) {
	queries, err := parseAllQueryBlocks(filePath)

	if err != nil {
		return false, err
	}

	if len(queries) == 0 {
		return false, nil
	}

	return queries[0].NotDone, nil
}

// parseQueryFileExtended parses the first query block (for backwards compatibility)
func parseQueryFileExtended(filePath string) (*Query, error) {
	queries, err := parseAllQueryBlocks(filePath)

	if err != nil {
		return nil, err
	}

	if len(queries) == 0 {
		return nil, fmt.Errorf("no ```tasks block found in %s", filePath)
	}

	return queries[0], nil
}

// parseAllQueryBlocks parses all ```tasks blocks from a file
func parseAllQueryBlocks(filePath string) ([]*Query, error) {
	content, err := os.ReadFile(filePath)

	if err != nil {
		return nil, err
	}

	matches := blockRe.FindAllStringSubmatchIndex(string(content), -1)

	if matches == nil {
		return nil, fmt.Errorf("no ```tasks block found in %s", filePath)
	}

	headers := headerRe.FindAllStringSubmatchIndex(string(content), -1)

	var queries []*Query

	for _, match := range matches {
		blockStart := match[0]
		queryContent := string(content[match[2]:match[3]])

		// Find the nearest ## header before this block
		sectionName := ""

		for _, header := range headers {
			headerEnd := header[1]
			if headerEnd < blockStart {
				sectionName = string(content[header[2]:header[3]])
			} else {
				break
			}
		}

		query := parseQueryContent(queryContent)
		query.Name = sectionName
		queries = append(queries, query)
	}

	return queries, nil
}

// parseQueryContent parses the content of a single ```tasks block
func parseQueryContent(queryContent string) *Query {
	query := &Query{}

	if strings.Contains(queryContent, "not done") {
		query.NotDone = true
	}

	// Parse date filters
	dateMatches := dateFilterRe.FindAllStringSubmatch(queryContent, -1)

	for _, dm := range dateMatches {
		field := dm[1]   // due, scheduled, done
		operand := dm[2] // today, tomorrow, before X, after X, on X

		var op, date string
		var dates []string
		switch {
		case strings.HasPrefix(operand, "before "):
			op = "before"
			date = strings.TrimSpace(strings.TrimPrefix(operand, "before "))
		case strings.HasPrefix(operand, "after "):
			op = "after"
			date = strings.TrimSpace(strings.TrimPrefix(operand, "after "))
		case strings.HasPrefix(operand, "on "):
			op = "on"
			dates = splitOrDates(strings.TrimSpace(strings.TrimPrefix(operand, "on ")))
		default:
			op = "on"
			dates = splitOrDates(operand)
		}

		if len(dates) == 1 {
			date = dates[0]
			dates = nil
		}

		query.DateFilters = append(query.DateFilters, DateFilter{
			Field:    field,
			Operator: op,
			Date:     date,
			Dates:    dates,
		})
	}

	// Parse group by - supports both simple and function syntax
	if funcMatch := groupByFuncRe.FindStringSubmatch(queryContent); funcMatch != nil {
		query.GroupBy = funcMatch[1]
	} else if simpleMatch := groupBySimpleRe.FindStringSubmatch(queryContent); simpleMatch != nil {
		if simpleMatch[1] != "function" {
			query.GroupBy = simpleMatch[1]
		}
	}

	return query
}

// startOfDay returns the time truncated to midnight UTC
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// relPath returns the relative path from basePath, or the original if that fails
func relPath(basePath, filePath string) string {
	if rel, err := filepath.Rel(basePath, filePath); err == nil {
		return rel
	}
	return filePath
}

// calculateVisibleRange returns start/end indices for visible lines,
// keeping cursorLineIdx visible and roughly centered.
func calculateVisibleRange(cursorLineIdx int, lineHeights []int, visibleHeight int) (startLine, endLine int) {
	totalLines := len(lineHeights)

	if totalLines == 0 {
		return 0, 0
	}

	// Calculate total height and cursor position in rendered lines
	totalHeight := 0
	cursorPos := 0

	for i, h := range lineHeights {
		if i < cursorLineIdx {
			cursorPos += h
		}
		totalHeight += h
	}

	// If everything fits, show all
	if totalHeight <= visibleHeight {
		return 0, totalLines
	}

	// Target: center cursor in visible area
	targetStart := cursorPos - visibleHeight/2

	if targetStart < 0 {
		targetStart = 0
	}

	// Find startLine from target position
	pos := 0

	for i, h := range lineHeights {
		if pos >= targetStart {
			startLine = i
			break
		}
		pos += h
	}

	// Find endLine that fits in visibleHeight
	rendered := 0

	for i := startLine; i < totalLines; i++ {
		if rendered+lineHeights[i] > visibleHeight {
			break
		}

		rendered += lineHeights[i]
		endLine = i + 1
	}

	// Ensure cursor is visible (may need to scroll down)
	if cursorLineIdx >= endLine {
		endLine = cursorLineIdx + 1
		// Recalculate startLine from end
		rendered = 0

		for i := endLine - 1; i >= 0; i-- {
			if rendered+lineHeights[i] > visibleHeight {
				startLine = i + 1
				break
			}

			rendered += lineHeights[i]
			startLine = i
		}
	}

	// Don't leave empty space at bottom
	rendered = 0

	for i := startLine; i < totalLines; i++ {
		rendered += lineHeights[i]
	}

	for startLine > 0 && rendered < visibleHeight {
		startLine--
		rendered += lineHeights[startLine]
	}

	// Final endLine calculation
	rendered = 0
	endLine = startLine

	for i := startLine; i < totalLines; i++ {
		if rendered+lineHeights[i] > visibleHeight {
			break
		}
		rendered += lineHeights[i]
		endLine = i + 1
	}

	// Safety: always include cursor
	if cursorLineIdx >= endLine {
		endLine = cursorLineIdx + 1
	}

	return startLine, endLine
}

// resolveDate converts relative date strings to actual dates (in UTC for comparison)
func resolveDate(dateStr string) time.Time {
	today := startOfDay(time.Now())

	switch dateStr {
	case "today":
		return today
	case "tomorrow":
		return today.AddDate(0, 0, 1)
	case "yesterday":
		return today.AddDate(0, 0, -1)
	default:
		// Try to parse as YYYY-MM-DD (already in UTC from time.Parse)
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			return parsed
		}
		return today
	}
}

func splitOrDates(value string) []string {
	parts := strings.Split(value, " or ")

	var dates []string

	for _, part := range parts {
		part = strings.TrimSpace(part)

		if part != "" {
			dates = append(dates, part)
		}
	}
	return dates
}

// matchDateFilter checks if a task matches a date filter
func matchDateFilter(task *Task, filter DateFilter) bool {
	var taskDate *time.Time

	switch filter.Field {
	case "due":
		taskDate = task.DueDate
	// Add more fields as needed (scheduled, done, etc.)
	default:
		return true // Unknown field, don't filter
	}

	// If task has no date for this field
	if taskDate == nil {
		return false // Tasks without dates don't match date filters
	}

	targetDate := resolveDate(filter.Date)
	taskDateOnly := startOfDay(*taskDate)

	if len(filter.Dates) > 0 {
		for _, date := range filter.Dates {
			target := resolveDate(date)

			switch filter.Operator {
			case "on":
				if taskDateOnly.Equal(target) {
					return true
				}
			case "before":
				if taskDateOnly.Before(target) {
					return true
				}
			case "after":
				if taskDateOnly.After(target) {
					return true
				}
			}
		}

		return false
	}

	switch filter.Operator {
	case "on":
		return taskDateOnly.Equal(targetDate)
	case "before":
		return taskDateOnly.Before(targetDate)
	case "after":
		return taskDateOnly.After(targetDate)
	default:
		return true
	}
}

// matchAllDateFilters checks if a task matches all date filters
func matchAllDateFilters(task *Task, filters []DateFilter) bool {
	for _, filter := range filters {
		if !matchDateFilter(task, filter) {
			return false
		}
	}

	return true
}

// OrderedMap maintains insertion order for keys with comparable constraint
type OrderedMap[K cmp.Ordered, V any] struct {
	data  map[K]V
	order []K
}

// NewOrderedMap creates a new OrderedMap
func NewOrderedMap[K cmp.Ordered, V any]() *OrderedMap[K, V] {
	return &OrderedMap[K, V]{
		data: make(map[K]V),
	}
}

// Set adds or updates a key-value pair, tracking insertion order
func (m *OrderedMap[K, V]) Set(key K, value V) {
	if _, exists := m.data[key]; !exists {
		m.order = append(m.order, key)
	}

	m.data[key] = value
}

// Get retrieves a value by key
func (m *OrderedMap[K, V]) Get(key K) (V, bool) {
	v, ok := m.data[key]
	return v, ok
}

// Keys returns keys in insertion order
func (m *OrderedMap[K, V]) Keys() []K {
	return m.order
}

// TaskGroup represents a group of tasks
type TaskGroup struct {
	Name  string
	Tasks []*Task
}

// groupTasks groups tasks by the specified field
func groupTasks(tasks []*Task, groupBy string, vaultPath string) []TaskGroup {
	if groupBy == "" {
		return []TaskGroup{{Name: "", Tasks: tasks}}
	}

	groups := NewOrderedMap[string, []*Task]()

	for _, task := range tasks {
		var key string
		rel := relPath(vaultPath, task.FilePath)

		switch groupBy {
		case "folder":
			key = filepath.Dir(rel)
			if key == "." {
				key = "/"
			}
		case "filename":
			key = filepath.Base(task.FilePath)
		default:
			key = ""
		}

		existing, _ := groups.Get(key)
		groups.Set(key, append(existing, task))
	}

	result := make([]TaskGroup, 0, len(groups.Keys()))

	for _, name := range groups.Keys() {
		tasks, _ := groups.Get(name)
		result = append(result, TaskGroup{
			Name:  name,
			Tasks: tasks,
		})
	}

	return result
}

// saveTask writes the modified task back to its source file
func saveTask(task *Task) error {
	// Read the entire file
	content, err := os.ReadFile(task.FilePath)

	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")

	// Update the specific line (1-indexed to 0-indexed)
	if task.LineNumber > 0 && task.LineNumber <= len(lines) {
		lines[task.LineNumber-1] = task.RawLine
	}

	// Write back atomically
	tempPath := task.FilePath + ".tmp"
	err = os.WriteFile(tempPath, []byte(strings.Join(lines, "\n")), 0644)

	if err != nil {
		return err
	}

	return os.Rename(tempPath, task.FilePath)
}

// startEdit initiates editing for a task - either external or inline based on config
func (m *model) startEdit(task *Task) tea.Cmd {
	// Check if we should use inline editor
	useInline := m.editorMode == "inline"

	// If not explicitly set to inline, check if $EDITOR is available
	if !useInline && m.editorMode != "external" {
		// Default behavior: use external if $EDITOR is set, otherwise inline
		if os.Getenv("EDITOR") == "" {
			useInline = true
		}
	}

	if useInline {
		// Enter inline edit mode - only edit the description
		m.editing = true
		m.editingTask = task
		m.editInput = task.Description
		m.editCursor = len(task.Description)
		return nil
	}

	// Use external editor
	return openInEditor(task)
}

// openInEditor opens the task file in an external editor at the correct line
func openInEditor(task *Task) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi" // fallback
	}

	// Build command with line number argument
	// Most editors support +LINE syntax (vim, nvim, nano, emacs, code, etc.)
	lineArg := fmt.Sprintf("+%d", task.LineNumber)
	c := exec.Command(editor, lineArg, task.FilePath)

	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err: err, task: task}
	})
}

// editorFinishedMsg is sent when the external editor closes
type editorFinishedMsg struct {
	err  error
	task *Task
}

// QuerySection represents a section with its query and results
type QuerySection struct {
	Name   string      // Section name (from ## header)
	Query  *Query      // The query for this section
	Groups []TaskGroup // Grouped task results
	Tasks  []*Task     // Flat task list for this section
}

// TUI Model
type model struct {
	sections     []QuerySection
	tasks        []*Task // Flat list of all tasks for navigation
	cursor       int
	vaultPath    string
	titleName    string
	queryFile    string   // Path to query file for refresh
	queries      []*Query // Parsed queries for refresh
	quitting     bool
	err          error
	windowHeight int // Terminal height for scrolling
	windowWidth  int // Terminal width
	aboutOpen    bool

	// Search state
	searching        bool             // Whether search mode is active
	searchQuery      string           // Current search input
	searchNavigating bool             // Whether navigating results (vs typing)
	filteredTasks    []*Task          // Tasks matching search query
	taskToSection    map[*Task]string // Map task to its section name for search
	taskToGroup      map[*Task]string // Map task to its group name for search

	// Editor state
	editorMode  string // "external" or "inline" from config
	editing     bool   // Whether inline edit mode is active
	editingTask *Task  // Task being edited inline
	editInput   string // Current edit buffer
	editCursor  int    // Cursor position in edit buffer
}

func newModel(sections []QuerySection, vaultPath string, titleName string, queryFile string, queries []*Query, editorMode string) model {
	// Build flat task list from all sections and task-to-section/group maps
	var tasks []*Task
	taskToSection := make(map[*Task]string)
	taskToGroup := make(map[*Task]string)
	for _, s := range sections {
		for _, g := range s.Groups {
			for _, task := range g.Tasks {
				tasks = append(tasks, task)
				taskToSection[task] = s.Name
				taskToGroup[task] = g.Name
			}
		}
	}

	return model{
		sections:      sections,
		tasks:         tasks,
		vaultPath:     vaultPath,
		titleName:     titleName,
		queryFile:     queryFile,
		queries:       queries,
		windowHeight:  defaultWindowHeight,
		windowWidth:   defaultWindowWidth,
		taskToSection: taskToSection,
		taskToGroup:   taskToGroup,
		editorMode:    editorMode,
	}
}

func (m model) Init() tea.Cmd {
	return tea.WindowSize()
}

// filterBySearch filters tasks based on search query (matches task description, section name, or group name)
func (m *model) filterBySearch() {
	if m.searchQuery == "" {
		m.filteredTasks = nil
		return
	}

	query := strings.ToLower(m.searchQuery)
	var filtered []*Task
	seen := make(map[*Task]bool)

	for _, task := range m.tasks {
		// Skip duplicates
		if seen[task] {
			continue
		}

		// Match against task description
		if strings.Contains(strings.ToLower(task.Description), query) {
			filtered = append(filtered, task)
			seen[task] = true
			continue
		}

		// Match against section/category name
		sectionName := m.taskToSection[task]
		if strings.Contains(strings.ToLower(sectionName), query) {
			filtered = append(filtered, task)
			seen[task] = true
			continue
		}

		// Match against group name (folder/filename)
		groupName := m.taskToGroup[task]
		if strings.Contains(strings.ToLower(groupName), query) {
			filtered = append(filtered, task)
			seen[task] = true
		}
	}

	m.filteredTasks = filtered

	// Reset cursor to valid range
	if m.cursor >= len(filtered) {
		m.cursor = len(filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// activeTasks returns the current task list (filtered or all)
func (m *model) activeTasks() []*Task {
	if m.searching && m.searchQuery != "" {
		return m.filteredTasks
	}
	return m.tasks
}

// refresh re-scans the vault and rebuilds sections from the stored queries
func (m *model) refresh() {
	// Re-parse query file to pick up any changes
	queries, err := parseAllQueryBlocks(m.queryFile)
	if err != nil {
		m.err = err
		return
	}

	m.queries = queries

	// Re-scan vault
	files, err := scanVault(m.vaultPath)

	if err != nil {
		m.err = err
		return
	}

	// Re-parse all files for tasks
	var allTasks []*Task

	for _, file := range files {
		tasks, err := parseFile(file)
		if err != nil {
			continue
		}

		allTasks = append(allTasks, tasks...)
	}

	// Rebuild sections
	var sections []QuerySection

	for _, query := range m.queries {
		filtered := filterTasks(allTasks, query)
		groups := groupTasks(filtered, query.GroupBy, m.vaultPath)

		sections = append(sections, QuerySection{
			Name:   query.Name,
			Query:  query,
			Groups: groups,
			Tasks:  filtered,
		})
	}

	// Rebuild flat task list and task-to-section/group maps
	var tasks []*Task
	taskToSection := make(map[*Task]string)
	taskToGroup := make(map[*Task]string)
	for _, s := range sections {
		for _, g := range s.Groups {
			for _, task := range g.Tasks {
				tasks = append(tasks, task)
				taskToSection[task] = s.Name
				taskToGroup[task] = g.Name
			}
		}
	}

	m.sections = sections
	m.tasks = tasks
	m.taskToSection = taskToSection
	m.taskToGroup = taskToGroup

	// Re-apply search filter if active
	if m.searching && m.searchQuery != "" {
		m.filterBySearch()
	}

	// Adjust cursor if needed
	if m.cursor >= len(m.tasks) {
		m.cursor = len(m.tasks) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowHeight = msg.Height
		m.windowWidth = msg.Width

	case editorFinishedMsg:
		// External editor closed - refresh to pick up any changes
		if msg.err != nil {
			m.err = msg.err
		}
		m.refresh()
		return m, nil

	case tea.KeyMsg:
		if m.aboutOpen {
			switch msg.String() {
			case "esc", "ctrl+[", "q", "?":
				m.aboutOpen = false
				return m, nil
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}

		// Handle inline edit mode
		if m.editing {
			switch msg.String() {
			case "esc", "ctrl+[":
				// Cancel edit
				m.editing = false
				m.editingTask = nil
				m.editInput = ""
				m.editCursor = 0
				return m, nil

			case "enter":
				// Save edit - update description and rebuild raw line
				if m.editingTask != nil && m.editInput != m.editingTask.Description {
					m.editingTask.Description = m.editInput
					m.editingTask.Modified = true
					// Rebuild raw line with new description
					m.editingTask.rebuildRawLine()
					if err := saveTask(m.editingTask); err != nil {
						m.err = err
					}
				}
				m.editing = false
				m.editingTask = nil
				m.editInput = ""
				m.editCursor = 0
				m.refresh()
				return m, nil

			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit

			case "backspace":
				if m.editCursor > 0 {
					m.editInput = m.editInput[:m.editCursor-1] + m.editInput[m.editCursor:]
					m.editCursor--
				}
				return m, nil

			case "delete":
				if m.editCursor < len(m.editInput) {
					m.editInput = m.editInput[:m.editCursor] + m.editInput[m.editCursor+1:]
				}
				return m, nil

			case "left":
				if m.editCursor > 0 {
					m.editCursor--
				}
				return m, nil

			case "right":
				if m.editCursor < len(m.editInput) {
					m.editCursor++
				}
				return m, nil

			case "home", "ctrl+a":
				m.editCursor = 0
				return m, nil

			case "end", "ctrl+e":
				m.editCursor = len(m.editInput)
				return m, nil

			default:
				// Insert character at cursor position
				if len(msg.String()) == 1 {
					m.editInput = m.editInput[:m.editCursor] + msg.String() + m.editInput[m.editCursor:]
					m.editCursor++
				}
				return m, nil
			}
		}

		if msg.String() == "?" {
			m.aboutOpen = true
			return m, nil
		}

		// Handle search mode input
		if m.searching {
			// Navigation mode within search (after pressing Enter)
			if m.searchNavigating {
				switch msg.String() {
				case "esc", "ctrl+[", "/", "q":
					// Exit search mode entirely
					m.searching = false
					m.searchNavigating = false
					m.searchQuery = ""
					m.filteredTasks = nil
					m.cursor = 0
					return m, nil

				case "backspace":
					// Go back to typing mode
					m.searchNavigating = false
					return m, nil

				case "up", "k":
					if m.cursor > 0 {
						m.cursor--
					}
					return m, nil

				case "down", "j":
					tasks := m.activeTasks()
					if m.cursor < len(tasks)-1 {
						m.cursor++
					}
					return m, nil

				case "ctrl+c":
					m.quitting = true
					return m, tea.Quit

				case "enter", " ", "x":
					// Toggle task in search mode
					tasks := m.activeTasks()
					if len(tasks) > 0 && m.cursor < len(tasks) {
						task := tasks[m.cursor]
						task.Toggle()
						if err := saveTask(task); err != nil {
							m.err = err
						}
					}
					return m, nil

				case "e":
					// Edit task
					tasks := m.activeTasks()
					if len(tasks) > 0 && m.cursor < len(tasks) {
						task := tasks[m.cursor]
						return m, m.startEdit(task)
					}
					return m, nil
				}
				return m, nil
			}

			// Typing mode - all keys are input except special ones
			switch msg.String() {
			case "esc", "ctrl+[":
				// Exit search mode
				m.searching = false
				m.searchQuery = ""
				m.filteredTasks = nil
				m.cursor = 0
				return m, nil

			case "enter":
				// Switch to navigation mode if we have results
				if len(m.filteredTasks) > 0 {
					m.searchNavigating = true
				} else if m.searchQuery == "" {
					// Empty search, exit
					m.searching = false
				}
				return m, nil

			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
					m.filterBySearch()
				}
				return m, nil

			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit

			case "up":
				if m.cursor > 0 {
					m.cursor--
				}
				return m, nil

			case "down":
				tasks := m.activeTasks()
				if m.cursor < len(tasks)-1 {
					m.cursor++
				}
				return m, nil

			default:
				// Add character to search query (only printable characters)
				if len(msg.String()) == 1 {
					m.searchQuery += msg.String()
					m.filterBySearch()
				}
				return m, nil
			}
		}

		// Normal mode keybindings
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "/":
			// Enter search mode
			m.searching = true
			m.searchQuery = ""
			m.filteredTasks = nil
			m.cursor = 0

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.tasks)-1 {
				m.cursor++
			}

		case "enter", " ", "x":
			if len(m.tasks) > 0 {
				task := m.tasks[m.cursor]
				task.Toggle()
				// Save immediately
				if err := saveTask(task); err != nil {
					m.err = err
				}
			}

		case "g":
			m.cursor = 0

		case "G":
			if len(m.tasks) > 0 {
				m.cursor = len(m.tasks) - 1
			}

		case "r":
			m.refresh()

		case "e":
			// Edit task
			if len(m.tasks) > 0 {
				task := m.tasks[m.cursor]
				return m, m.startEdit(task)
			}
		}
	}

	return m, nil
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170"))

	titleNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("99"))

	searchModeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(lipgloss.Color("160")).
			Padding(0, 1)

	resultsModeStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("231")).
				Background(lipgloss.Color("214")).
				Padding(0, 1)

	editCursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("231")).
			Background(lipgloss.Color("212"))

	aboutStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("white"))

	aboutBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("241")).
			Padding(1, 2)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true)

	doneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Strikethrough(true)

	fileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212"))

	groupStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("99"))
)

var sectionStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("205")).
	MarginTop(1)

var countStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("245"))

var searchStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("212")).
	Bold(true)

var matchStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("214")).
	Bold(true)

var searchInputStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("170"))

// viewLine represents a renderable line with its associated task index (or -1 for non-task lines)
type viewLine struct {
	content   string
	taskIndex int // -1 for header lines, >= 0 for task lines
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err)
	}

	if m.quitting {
		return "Goodbye!\n"
	}

	var b strings.Builder
	if m.aboutOpen {
		versionLine := fmt.Sprintf("ot %s", strings.TrimSpace(version))
		creditLine := "created with ☠️ by elcuervo"
		helpLine := "press esc or q to close"
		contentWidth := lipgloss.Width(versionLine)
		if width := lipgloss.Width(creditLine); width > contentWidth {
			contentWidth = width
		}
		if width := lipgloss.Width(helpLine); width > contentWidth {
			contentWidth = width
		}
		centered := lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center)
		aboutText := aboutStyle.Render(centered.Render(versionLine) + "\n" + centered.Render(creditLine))
		aboutHelp := helpStyle.Render(centered.Render(helpLine))
		box := aboutBoxStyle.Render(aboutText + "\n" + aboutHelp)

		return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, box)
	}

	// Edit popup (centered modal)
	if m.editing && m.editingTask != nil {
		titleLine := aboutStyle.Render("Edit Task")

		// Build the input line with cursor
		checkbox := "[ ] "
		if m.editingTask.Done {
			checkbox = "[x] "
		}

		var inputLine strings.Builder
		inputLine.WriteString(checkbox)
		inputLine.WriteString(m.editInput[:m.editCursor])
		if m.editCursor < len(m.editInput) {
			inputLine.WriteString(editCursorStyle.Render(string(m.editInput[m.editCursor])))
			inputLine.WriteString(m.editInput[m.editCursor+1:])
		} else {
			inputLine.WriteString(editCursorStyle.Render(" "))
		}

		// Calculate box width - use most of window but cap it
		maxWidth := m.windowWidth - 8
		if maxWidth > 60 {
			maxWidth = 60
		}
		if maxWidth < 20 {
			maxWidth = 20
		}

		helpLine := "enter save • esc cancel"
		contentWidth := lipgloss.Width(inputLine.String())
		if contentWidth < lipgloss.Width(titleLine) {
			contentWidth = lipgloss.Width(titleLine)
		}
		if contentWidth < lipgloss.Width(helpLine) {
			contentWidth = lipgloss.Width(helpLine)
		}
		if contentWidth > maxWidth {
			contentWidth = maxWidth
		}

		centered := lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Left)
		editContent := titleLine + "\n\n" + centered.Render(inputLine.String())
		editHelp := helpStyle.Render(helpLine)
		box := aboutBoxStyle.Render(editContent + "\n\n" + editHelp)

		return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, box)
	}

	// Title
	titlePrefix := titleStyle.Render("ot - Tasks from ")
	titleName := titleNameStyle.Render(m.titleName)
	modeLabel := ""

	if m.searching {
		if m.searchNavigating {
			modeLabel = resultsModeStyle.Render("results")
		} else {
			modeLabel = searchModeStyle.Render("search")
		}
	}

	titleLine := titlePrefix + titleName
	if modeLabel != "" {
		titleLine += " " + modeLabel
	}
	b.WriteString(titleLine + "\n")

	// Search bar (if in search mode)
	if m.searching {
		searchLabel := searchStyle.Render("/")
		searchInput := searchInputStyle.Render(m.searchQuery)
		if m.searchNavigating {
			// No cursor when navigating
			b.WriteString(searchLabel + searchInput + "\n")
		} else {
			// Show cursor when typing
			cursorChar := searchStyle.Render("_")
			b.WriteString(searchLabel + searchInput + cursorChar + "\n")
		}
	} else {
		b.WriteString("\n")
	}

	// Task list
	if len(m.tasks) == 0 {
		b.WriteString("\nNo tasks found.\n")
	} else if m.searching && m.searchQuery != "" {
		// Search results view
		tasks := m.activeTasks()

		if len(tasks) == 0 {
			b.WriteString(fileStyle.Render("  No matching tasks\n"))
		} else {
			var lines []viewLine

			query := strings.ToLower(m.searchQuery)

			for i, task := range tasks {
				cursor := "  "
				if m.cursor == i {
					cursor = cursorStyle.Render("> ")
				}

				checkbox := "[ ]"
				if task.Done {
					checkbox = "[x]"
				}

				// Determine what matched for visual feedback
				sectionName := m.taskToSection[task]
				groupName := m.taskToGroup[task]
				descLower := strings.ToLower(task.Description)

				var matchInfo string
				if strings.Contains(descLower, query) {
					// Matched description - no extra indicator needed
					matchInfo = ""
				} else if strings.Contains(strings.ToLower(sectionName), query) {
					// Matched section name
					matchInfo = matchStyle.Render(fmt.Sprintf("→%s ", sectionName))
				} else if strings.Contains(strings.ToLower(groupName), query) {
					// Matched group name
					matchInfo = matchStyle.Render(fmt.Sprintf("→%s ", groupName))
				}

				// Show section context
				sectionInfo := ""
				if sectionName != "" && matchInfo == "" {
					sectionInfo = countStyle.Render(fmt.Sprintf("[%s] ", sectionName))
				}
				fileInfo := fileStyle.Render(fmt.Sprintf(" (%s:%d)", relPath(m.vaultPath, task.FilePath), task.LineNumber))

				var line string
				desc := renderWithLinks(task.Description)
				if task.Done {
					line = doneStyle.Render(fmt.Sprintf("%s %s", checkbox, desc))
				} else {
					line = fmt.Sprintf("%s %s", checkbox, desc)
				}

				if m.cursor == i {
					line = selectedStyle.Render(line)
				}

				lines = append(lines, viewLine{
					content:   fmt.Sprintf("%s%s%s%s%s", cursor, matchInfo, sectionInfo, line, fileInfo),
					taskIndex: i,
				})
			}

			// Calculate visible window height (extra line for search bar)
			visibleHeight := m.windowHeight - reservedUILines - 1
			if visibleHeight < minVisibleHeight {
				visibleHeight = minVisibleHeight
			}

			lineHeights := make([]int, len(lines))
			totalRenderedLines := 0
			for i, line := range lines {
				height := 1 + strings.Count(line.content, "\n")
				lineHeights[i] = height
				totalRenderedLines += height
			}

			startLine, endLine := calculateVisibleRange(m.cursor, lineHeights, visibleHeight)

			for i := startLine; i < endLine; i++ {
				b.WriteString(lines[i].content + "\n")
			}

			// Search mode help - different text for typing vs navigating
			var helpText string
			if m.searchNavigating {
				helpText = "↑/k up • ↓/j down • enter/space toggle • e edit • backspace edit query • esc/q exit • ? about"
			} else {
				helpText = "type to search • ↑/↓ navigate • enter select • esc cancel • ? about"
			}
			matchInfo := fmt.Sprintf("[%d matches]", len(tasks))
			padding := m.windowWidth - len(helpText) - len(matchInfo) - 1
			if padding < 2 {
				padding = 2
			}
			helpText = helpText + strings.Repeat(" ", padding) + matchInfo
			help := helpStyle.Render(helpText)
			b.WriteString("\n" + help)
		}
	} else {
		// Normal view - Build all lines first
		var lines []viewLine
		taskIndex := 0

		for _, section := range m.sections {
			// Show section header with task count
			if section.Name != "" {
				count := len(section.Tasks)
				countText := countStyle.Render(fmt.Sprintf(" (%d)", count))
				lines = append(lines, viewLine{
					content:   sectionStyle.Render(fmt.Sprintf("# %s", section.Name)) + countText,
					taskIndex: -1,
				})
			}

			if len(section.Tasks) == 0 {
				lines = append(lines, viewLine{
					content:   fileStyle.Render("  (no matching tasks)"),
					taskIndex: -1,
				})

				continue
			}

			firstGroup := true

			for _, group := range section.Groups {
				// Show group header if grouping is active
				if section.Query.GroupBy != "" && group.Name != "" {
					if !firstGroup {
						lines = append(lines, viewLine{
							content:   "",
							taskIndex: -1,
						})
					}

					count := len(group.Tasks)
					countText := countStyle.Render(fmt.Sprintf(" (%d)", count))
					lines = append(lines, viewLine{
						content:   groupStyle.Render(fmt.Sprintf("  ## %s", group.Name)) + countText,
						taskIndex: -1,
					})

					firstGroup = false
				}

				for _, task := range group.Tasks {
					indent := ""

					if section.Query.GroupBy != "" && group.Name != "" {
						indent = "  "
					}

					cursor := "  "

					if m.cursor == taskIndex {
						cursor = cursorStyle.Render("> ")
					}

					// Build checkbox
					checkbox := "[ ]"

					if task.Done {
						checkbox = "[x]"
					}

					// Get relative path for display (only show if not grouping by filename)
					fileInfo := ""

					if section.Query.GroupBy != "filename" {
						fileInfo = fileStyle.Render(fmt.Sprintf(" (%s:%d)", relPath(m.vaultPath, task.FilePath), task.LineNumber))
					} else {
						fileInfo = fileStyle.Render(fmt.Sprintf(" (:%d)", task.LineNumber))
					}

					// Format line
					var line string
					desc := renderWithLinks(task.Description)

					if task.Done {
						line = doneStyle.Render(fmt.Sprintf("%s %s", checkbox, desc))
					} else {
						line = fmt.Sprintf("%s %s", checkbox, desc)
					}

					// Highlight if selected
					if m.cursor == taskIndex {
						line = selectedStyle.Render(line)
					}

					lines = append(lines, viewLine{
						content:   fmt.Sprintf("%s%s%s%s", indent, cursor, line, fileInfo),
						taskIndex: taskIndex,
					})

					taskIndex++
				}
			}
		}

		// Calculate visible window height
		visibleHeight := m.windowHeight - reservedUILines

		if visibleHeight < minVisibleHeight {
			visibleHeight = minVisibleHeight
		}

		// Build line heights (accounting for lipgloss margins which add newlines)
		lineHeights := make([]int, len(lines))
		totalRenderedLines := 0

		for i, line := range lines {
			height := 1 + strings.Count(line.content, "\n")
			lineHeights[i] = height
			totalRenderedLines += height
		}

		// Find which line contains the cursor
		cursorLineIdx := 0

		for i, line := range lines {
			if line.taskIndex == m.cursor {
				cursorLineIdx = i
				break
			}
		}

		// Calculate visible range
		startLine, endLine := calculateVisibleRange(cursorLineIdx, lineHeights, visibleHeight)

		// Render visible lines
		for i := startLine; i < endLine; i++ {
			b.WriteString(lines[i].content + "\n")
		}

		// Build help line with scroll indicator on the right
		helpText := "↑/k up • ↓/j down • space/enter toggle • e edit • / search • r refresh • q quit • ? about"

		if totalRenderedLines > visibleHeight {
			scrollInfo := fmt.Sprintf("[%d-%d of %d]", startLine+1, endLine, len(lines))
			// Calculate padding to right-align scroll info
			padding := m.windowWidth - len(helpText) - len(scrollInfo) - 1
			if padding < 2 {
				padding = 2
			}
			helpText = helpText + strings.Repeat(" ", padding) + scrollInfo
		}
		help := helpStyle.Render(helpText)
		b.WriteString("\n" + help)
	}

	if len(m.tasks) == 0 {
		// Help for empty state
		help := helpStyle.Render("↑/k up • ↓/j down • space/enter toggle • e edit • / search • r refresh • q quit • ? about")
		b.WriteString("\n" + help)
	}

	return b.String()
}

// Filter returns elements from slice that satisfy the predicate
func Filter[T any](slice []T, predicate func(T) bool) []T {
	var result []T
	for _, v := range slice {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}

// filterTasks applies a query's filters to a task list
func filterTasks(allTasks []*Task, query *Query) []*Task {
	return Filter(allTasks, func(task *Task) bool {
		if query.NotDone && task.Done {
			return false
		}
		if len(query.DateFilters) > 0 && !matchAllDateFilters(task, query.DateFilters) {
			return false
		}
		return true
	})
}

func configPath() (string, error) {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(homeDir, ".config")
	}
	return filepath.Join(configDir, "ot", "config.toml"), nil
}

func loadConfig() (Config, string, error) {
	path, err := configPath()

	if err != nil {
		return Config{}, "", err
	}

	data, err := os.ReadFile(path)

	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, path, nil
		}

		return Config{}, path, err
	}

	var cfg Config

	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, path, err
	}

	return cfg, path, nil
}

func expandPath(value string) (string, error) {
	value = strings.TrimSpace(value)

	if value == "" {
		return value, nil
	}

	expanded := os.ExpandEnv(value)

	if !strings.HasPrefix(expanded, "~") {
		return expanded, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	if expanded == "~" {
		return homeDir, nil
	}

	if strings.HasPrefix(expanded, "~/") {
		return filepath.Join(homeDir, expanded[2:]), nil
	}

	if strings.HasPrefix(expanded, "~\\") {
		return filepath.Join(homeDir, expanded[2:]), nil
	}

	return expanded, nil
}

func resolveVaultPath(value string) (string, error) {
	expanded, err := expandPath(value)

	if err != nil {
		return "", err
	}

	if expanded == "" || filepath.IsAbs(expanded) {
		return expanded, nil
	}

	homeDir, err := os.UserHomeDir()

	if err != nil {
		return "", err
	}

	return filepath.Join(homeDir, expanded), nil
}

func resolveQueryPath(value, vault string) (string, error) {
	expanded, err := expandPath(value)

	if err != nil {
		return "", err
	}

	if expanded == "" || filepath.IsAbs(expanded) || vault == "" {
		return expanded, nil
	}

	return filepath.Join(vault, expanded), nil
}

func main() {
	// Parse flags
	vaultPath := flag.String("vault", "", "Path to Obsidian vault")
	listOnly := flag.Bool("list", false, "List tasks without TUI (non-interactive)")
	profileName := flag.String("profile", "", "Profile name from config (optional)")
	showVersion := flag.Bool("version", false, "Show version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("ot version %s\n", strings.TrimSpace(version))
		os.Exit(0)
	}

	args := flag.Args()
	cfg, cfgPath, err := loadConfig()

	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	var resolvedVault, queryFile, titleName, editorMode string

	// Try profile-based resolution
	name, profile, err := selectProfile(*profileName, cfg)

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if profile != nil {
		resolved, err := resolveProfilePaths(name, *profile)

		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		resolvedVault = resolved.VaultPath
		queryFile = resolved.QueryPath
		titleName = name
		editorMode = resolved.EditorMode
	}

	// CLI overrides
	if *vaultPath != "" {
		expanded, err := expandPath(*vaultPath)

		if err != nil {
			fmt.Printf("Error expanding vault path: %v\n", err)
			os.Exit(1)
		}

		resolvedVault = filepath.Clean(expanded)

		if resolved, err := filepath.EvalSymlinks(resolvedVault); err == nil {
			resolvedVault = resolved
		}

		titleName = filepath.Base(resolvedVault)
	}

	if len(args) > 0 {
		expanded, err := expandPath(args[0])

		if err != nil {
			fmt.Printf("Error expanding query path: %v\n", err)
			os.Exit(1)
		}

		queryFile = filepath.Clean(expanded)
	}

	if titleName == "" && resolvedVault != "" {
		titleName = filepath.Base(resolvedVault)
	}

	if queryFile == "" || resolvedVault == "" {
		fmt.Println("Usage: ot <query-file.md> --vault <path>")
		fmt.Println("\nOptions:")
		fmt.Println("  --vault <path>  Path to Obsidian vault (required)")
		fmt.Println("  --list          List tasks without TUI")
		fmt.Println("  --profile <name>  Use profile from config")
		fmt.Println("\nSupported query filters:")
		fmt.Println("  not done              Show only incomplete tasks")
		fmt.Println("  due today             Tasks due today")
		fmt.Println("  due today or tomorrow Tasks due today or tomorrow")
		fmt.Println("  due before <date>     Tasks due before date")
		fmt.Println("  due after <date>      Tasks due after date")
		fmt.Println("  group by folder       Group tasks by folder")
		fmt.Println("  group by filename     Group tasks by filename")
		fmt.Println("\nDate values: today, tomorrow, yesterday, or YYYY-MM-DD")
		fmt.Println("\nExample:")
		fmt.Println("  ot --vault ~/obsidian-vault query.md")

		if cfgPath != "" {
			fmt.Println("\nConfig:")
			fmt.Printf("  %s\n", cfgPath)
			fmt.Println("  Define profiles with vault/query and set default_profile to skip flags.")
		}

		os.Exit(1)
	}

	// Parse all query blocks from the file
	queries, err := parseAllQueryBlocks(queryFile)

	if err != nil {
		fmt.Printf("Error parsing query file: %v\n", err)
		os.Exit(1)
	}

	// Scan vault
	files, err := scanVault(resolvedVault)

	if err != nil {
		fmt.Printf("Error scanning vault: %v\n", err)
		os.Exit(1)
	}

	// Parse all files for tasks
	var allTasks []*Task
	for _, file := range files {
		tasks, err := parseFile(file)
		if err != nil {
			fmt.Printf("Warning: could not parse %s: %v\n", file, err)
			continue
		}
		allTasks = append(allTasks, tasks...)
	}

	// Process each query block into a section
	var sections []QuerySection

	totalTasks := 0

	for _, query := range queries {
		filtered := filterTasks(allTasks, query)
		groups := groupTasks(filtered, query.GroupBy, resolvedVault)

		sections = append(sections, QuerySection{
			Name:   query.Name,
			Query:  query,
			Groups: groups,
			Tasks:  filtered,
		})

		totalTasks += len(filtered)
	}

	if totalTasks == 0 {
		fmt.Println("No tasks found matching any query.")
		os.Exit(0)
	}

	// List mode (non-interactive)
	if *listOnly {
		fmt.Printf("Found %d task(s):\n\n", totalTasks)
		for _, section := range sections {
			if section.Name != "" {
				fmt.Printf("## %s (%d)\n", section.Name, len(section.Tasks))
			}

			if len(section.Tasks) == 0 {
				fmt.Println("(no matching tasks)")
				fmt.Println()
				continue
			}

			for _, group := range section.Groups {
				if section.Query.GroupBy != "" && group.Name != "" {
					fmt.Printf("### %s\n", group.Name)
				}

				for _, task := range group.Tasks {
					checkbox := "[ ]"

					if task.Done {
						checkbox = "[x]"
					}

					fmt.Printf("%s %s (%s:%d)\n", checkbox, task.Description, relPath(resolvedVault, task.FilePath), task.LineNumber)
				}
			}
			fmt.Println()
		}

		os.Exit(0)
	}

	// Run TUI
	p := tea.NewProgram(newModel(sections, resolvedVault, titleName, queryFile, queries, editorMode), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
