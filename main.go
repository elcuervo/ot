package main

import (
	"bufio"
	"cmp"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultWindowHeight = 24
	defaultWindowWidth  = 80
	reservedUILines     = 4 // title(1) + newline(1) + help margin(1) + help(1)
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
	Vault string `toml:"vault"`
	Query string `toml:"query"`
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
	queryFile    string   // Path to query file for refresh
	queries      []*Query // Parsed queries for refresh
	quitting     bool
	err          error
	windowHeight int // Terminal height for scrolling
	windowWidth  int // Terminal width
}

func newModel(sections []QuerySection, vaultPath string, queryFile string, queries []*Query) model {
	// Build flat task list from all sections
	var tasks []*Task
	for _, s := range sections {
		for _, g := range s.Groups {
			tasks = append(tasks, g.Tasks...)
		}
	}

	return model{
		sections:     sections,
		tasks:        tasks,
		vaultPath:    vaultPath,
		queryFile:    queryFile,
		queries:      queries,
		windowHeight: defaultWindowHeight,
		windowWidth:  defaultWindowWidth,
	}
}

func (m model) Init() tea.Cmd {
	return tea.WindowSize()
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

	// Rebuild flat task list
	var tasks []*Task
	for _, s := range sections {
		tasks = append(tasks, s.Tasks...)
	}

	m.sections = sections
	m.tasks = tasks

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

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

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
		}
	}

	return m, nil
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170"))

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
			Foreground(lipgloss.Color("99")).
			MarginTop(1)
)

var sectionStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("205")).
	MarginTop(1)

var countStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("245"))

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

	// Title
	title := titleStyle.Render(fmt.Sprintf("ot - Tasks from %s", m.vaultPath))
	b.WriteString(title + "\n")

	// Task list
	if len(m.tasks) == 0 {
		b.WriteString("\nNo tasks found.\n")
	} else {
		// Build all lines first
		var lines []viewLine
		taskIndex := 0

		for _, section := range m.sections {
			// Show section header with task count
			if section.Name != "" {
				count := len(section.Tasks)
				countText := countStyle.Render(fmt.Sprintf(" (%d)", count))
				lines = append(lines, viewLine{
					content:   sectionStyle.Render(fmt.Sprintf("## %s", section.Name)) + countText,
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

			for _, group := range section.Groups {
				// Show group header if grouping is active
				if section.Query.GroupBy != "" && group.Name != "" {
					lines = append(lines, viewLine{
						content:   groupStyle.Render(fmt.Sprintf("  ### %s", group.Name)),
						taskIndex: -1,
					})
				}

				for _, task := range group.Tasks {
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
					if task.Done {
						line = doneStyle.Render(fmt.Sprintf("%s %s", checkbox, task.Description))
					} else {
						line = fmt.Sprintf("%s %s", checkbox, task.Description)
					}

					// Highlight if selected
					if m.cursor == taskIndex {
						line = selectedStyle.Render(line)
					}

					lines = append(lines, viewLine{
						content:   fmt.Sprintf("%s%s%s", cursor, line, fileInfo),
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
		helpText := "↑/k up • ↓/j down • space/enter toggle • r refresh • q quit"
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
		help := helpStyle.Render("↑/k up • ↓/j down • space/enter toggle • r refresh • q quit")
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
	flag.Parse()

	args := flag.Args()
	cfg, cfgPath, err := loadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	var profile *Profile
	if *profileName != "" {
		if cfg.Profiles == nil {
			fmt.Printf("Error: profile %q not found (no profiles in %s)\n", *profileName, cfgPath)
			os.Exit(1)
		}
		p, ok := cfg.Profiles[*profileName]
		if !ok {
			fmt.Printf("Error: profile %q not found in %s\n", *profileName, cfgPath)
			os.Exit(1)
		}
		profile = &p
	} else if cfg.DefaultProfile != "" {
		p, ok := cfg.Profiles[cfg.DefaultProfile]
		if !ok {
			fmt.Printf("Error: default_profile %q not found in %s\n", cfg.DefaultProfile, cfgPath)
			os.Exit(1)
		}
		profile = &p
	}

	var queryFile string
	queryFromProfile := false
	if len(args) > 0 {
		queryFile = args[0]
	} else if profile != nil {
		queryFile = profile.Query
		queryFromProfile = true
	}

	var resolvedVault string
	vaultFromProfile := false
	if profile != nil {
		resolvedVault = profile.Vault
		vaultFromProfile = true
	}
	if *vaultPath != "" {
		resolvedVault = *vaultPath
		vaultFromProfile = false
	}

	if resolvedVault != "" {
		var err error
		if vaultFromProfile {
			resolvedVault, err = resolveVaultPath(resolvedVault)
		} else {
			resolvedVault, err = expandPath(resolvedVault)
		}
		if err != nil {
			fmt.Printf("Error expanding vault path: %v\n", err)
			os.Exit(1)
		}
	}
	if queryFile != "" {
		var err error
		if queryFromProfile {
			queryFile, err = resolveQueryPath(queryFile, resolvedVault)
		} else {
			queryFile, err = expandPath(queryFile)
		}
		if err != nil {
			fmt.Printf("Error expanding query path: %v\n", err)
			os.Exit(1)
		}
	}

	if resolvedVault != "" {
		resolvedVault = filepath.Clean(resolvedVault)
	}
	if queryFile != "" {
		queryFile = filepath.Clean(queryFile)
	}
	if resolvedVault != "" {
		resolved, err := filepath.EvalSymlinks(resolvedVault)
		if err != nil {
			fmt.Printf("Error resolving vault path: %v\n", err)
			os.Exit(1)
		}
		resolvedVault = resolved
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
	p := tea.NewProgram(newModel(sections, resolvedVault, queryFile, queries), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
