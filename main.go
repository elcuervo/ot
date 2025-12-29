package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	// Find the checkbox pattern and replace it
	checkboxRe := regexp.MustCompile(`^(\s*-\s*)\[([ xX])\](.*)$`)
	matches := checkboxRe.FindStringSubmatch(t.RawLine)
	if matches == nil {
		return
	}

	prefix := matches[1]  // "- " or "  - " etc
	content := matches[3] // Everything after checkbox

	// Remove existing done date if present
	doneRe := regexp.MustCompile(`\s*✅\s*\d{4}-\d{2}-\d{2}`)
	content = doneRe.ReplaceAllString(content, "")

	if t.Done {
		// Add done date
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
	dueDateRe := regexp.MustCompile(`📅\s*(\d{4}-\d{2}-\d{2})`)
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

	// Match task lines: - [ ] or - [x] or - [X]
	taskRe := regexp.MustCompile(`^\s*-\s*\[([ xX])\]\s*(.*)$`)

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
}

// Query represents parsed query options
type Query struct {
	Name        string       // Section name (from ## header before the block)
	NotDone     bool
	GroupBy     string       // "folder", "filename", or ""
	DateFilters []DateFilter // date-based filters
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

	// Find all ```tasks blocks with their positions
	blockRe := regexp.MustCompile("(?s)```tasks\\s*\\n(.+?)```")
	matches := blockRe.FindAllStringSubmatchIndex(string(content), -1)
	if matches == nil {
		return nil, fmt.Errorf("no ```tasks block found in %s", filePath)
	}

	// Find all ## headers with their positions
	headerRe := regexp.MustCompile(`(?m)^##\s+(.+)$`)
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

	groupByFuncRe := regexp.MustCompile(`group by function task\.file\.(\w+)`)
	groupBySimpleRe := regexp.MustCompile(`group by (\w+)`)

	// Date filter patterns - matches: "due today", "due before tomorrow", "due after 2024-01-01"
	// Supports: due, scheduled, done
	// Operators: today (shorthand), before, after, on
	dateFilterRe := regexp.MustCompile(`(due|scheduled|done)\s+(today|tomorrow|before\s+\S+|after\s+\S+|on\s+\S+)`)

	// Parse filters
	if strings.Contains(queryContent, "not done") {
		query.NotDone = true
	}

	// Parse date filters
	dateMatches := dateFilterRe.FindAllStringSubmatch(queryContent, -1)
	for _, dm := range dateMatches {
		field := dm[1]   // due, scheduled, done
		operand := dm[2] // today, tomorrow, before X, after X, on X

		var op, date string
		if operand == "today" || operand == "tomorrow" {
			op = "on"
			date = operand
		} else {
			parts := strings.SplitN(operand, " ", 2)
			op = parts[0]
			if len(parts) > 1 {
				date = parts[1]
			}
		}

		query.DateFilters = append(query.DateFilters, DateFilter{
			Field:    field,
			Operator: op,
			Date:     date,
		})
	}

	// Parse group by - supports both simple and function syntax
	// Simple: "group by folder", "group by filename"
	// Function: "group by function task.file.folder"
	if funcMatch := groupByFuncRe.FindStringSubmatch(queryContent); funcMatch != nil {
		query.GroupBy = funcMatch[1]
	} else if simpleMatch := groupBySimpleRe.FindStringSubmatch(queryContent); simpleMatch != nil {
		// Skip "function" as a group by value
		if simpleMatch[1] != "function" {
			query.GroupBy = simpleMatch[1]
		}
	}

	return query
}

// resolveDate converts relative date strings to actual dates (in UTC for comparison)
func resolveDate(dateStr string) time.Time {
	now := time.Now()
	// Use UTC for consistent comparison with parsed dates
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

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
	// Normalize to UTC for comparison (task dates are already UTC from time.Parse)
	taskDateOnly := time.Date(taskDate.Year(), taskDate.Month(), taskDate.Day(), 0, 0, 0, 0, time.UTC)

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

	groups := make(map[string][]*Task)
	var order []string

	for _, task := range tasks {
		var key string
		relPath, _ := filepath.Rel(vaultPath, task.FilePath)

		switch groupBy {
		case "folder":
			key = filepath.Dir(relPath)
			if key == "." {
				key = "/"
			}
		case "filename":
			key = filepath.Base(task.FilePath)
		default:
			key = ""
		}

		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], task)
	}

	result := make([]TaskGroup, 0, len(order))
	for _, name := range order {
		result = append(result, TaskGroup{
			Name:  name,
			Tasks: groups[name],
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
	Name   string       // Section name (from ## header)
	Query  *Query       // The query for this section
	Groups []TaskGroup  // Grouped task results
	Tasks  []*Task      // Flat task list for this section
}

// TUI Model
type model struct {
	sections  []QuerySection
	tasks     []*Task // Flat list of all tasks for navigation
	cursor    int
	vaultPath string
	quitting  bool
	err       error
}

func newModel(sections []QuerySection, vaultPath string) model {
	// Create flat task list for navigation across all sections
	var tasks []*Task
	for i := range sections {
		for _, g := range sections[i].Groups {
			tasks = append(tasks, g.Tasks...)
		}
		sections[i].Tasks = tasks[len(tasks)-len(sections[i].Tasks):] // Update section's flat list
	}

	// Rebuild sections' flat task lists
	for i := range sections {
		var sectionTasks []*Task
		for _, g := range sections[i].Groups {
			sectionTasks = append(sectionTasks, g.Tasks...)
		}
		sections[i].Tasks = sectionTasks
	}

	// Build global flat list
	tasks = nil
	for _, s := range sections {
		tasks = append(tasks, s.Tasks...)
	}

	return model{
		sections:  sections,
		tasks:     tasks,
		vaultPath: vaultPath,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
		}
	}

	return m, nil
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170")).
			MarginBottom(1)

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

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err)
	}

	if m.quitting {
		return "Goodbye!\n"
	}

	var b strings.Builder

	// Title
	title := titleStyle.Render(fmt.Sprintf("mt - Tasks from %s", m.vaultPath))
	b.WriteString(title + "\n")

	// Task list
	if len(m.tasks) == 0 {
		b.WriteString("\nNo tasks found.\n")
	} else {
		taskIndex := 0
		for _, section := range m.sections {
			// Show section header if there's a name
			if section.Name != "" {
				b.WriteString(sectionStyle.Render(fmt.Sprintf("## %s", section.Name)) + "\n")
			}

			if len(section.Tasks) == 0 {
				b.WriteString(fileStyle.Render("  (no matching tasks)") + "\n")
				continue
			}

			for _, group := range section.Groups {
				// Show group header if grouping is active
				if section.Query.GroupBy != "" && group.Name != "" {
					b.WriteString(groupStyle.Render(fmt.Sprintf("  ### %s", group.Name)) + "\n")
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
						relPath := task.FilePath
						if rel, err := filepath.Rel(m.vaultPath, task.FilePath); err == nil {
							relPath = rel
						}
						fileInfo = fileStyle.Render(fmt.Sprintf(" (%s:%d)", relPath, task.LineNumber))
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

					b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, line, fileInfo))
					taskIndex++
				}
			}
		}
	}

	// Help
	help := helpStyle.Render("↑/k up • ↓/j down • space/enter toggle (saves immediately) • q quit")
	b.WriteString("\n" + help)

	return b.String()
}

// filterTasks applies a query's filters to a task list
func filterTasks(allTasks []*Task, query *Query) []*Task {
	var filtered []*Task
	for _, task := range allTasks {
		// Skip done tasks if "not done" filter is active
		if query.NotDone && task.Done {
			continue
		}
		// Apply date filters
		if len(query.DateFilters) > 0 && !matchAllDateFilters(task, query.DateFilters) {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered
}

func main() {
	// Parse flags
	vaultPath := flag.String("vault", "", "Path to Obsidian vault")
	listOnly := flag.Bool("list", false, "List tasks without TUI (non-interactive)")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: mt <query-file.md> --vault <path>")
		fmt.Println("\nOptions:")
		fmt.Println("  --vault <path>  Path to Obsidian vault (required)")
		fmt.Println("  --list          List tasks without TUI")
		fmt.Println("\nSupported query filters:")
		fmt.Println("  not done              Show only incomplete tasks")
		fmt.Println("  due today             Tasks due today")
		fmt.Println("  due before <date>     Tasks due before date")
		fmt.Println("  due after <date>      Tasks due after date")
		fmt.Println("  group by folder       Group tasks by folder")
		fmt.Println("  group by filename     Group tasks by filename")
		fmt.Println("\nDate values: today, tomorrow, yesterday, or YYYY-MM-DD")
		fmt.Println("\nExample:")
		fmt.Println("  mt --vault ~/obsidian-vault query.md")
		os.Exit(1)
	}

	queryFile := args[0]

	if *vaultPath == "" {
		fmt.Println("Error: --vault flag is required")
		os.Exit(1)
	}

	// Parse all query blocks from the file
	queries, err := parseAllQueryBlocks(queryFile)
	if err != nil {
		fmt.Printf("Error parsing query file: %v\n", err)
		os.Exit(1)
	}

	// Scan vault
	files, err := scanVault(*vaultPath)
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
		groups := groupTasks(filtered, query.GroupBy, *vaultPath)

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
				fmt.Printf("## %s\n", section.Name)
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
					relPath := task.FilePath
					if rel, err := filepath.Rel(*vaultPath, task.FilePath); err == nil {
						relPath = rel
					}
					checkbox := "[ ]"
					if task.Done {
						checkbox = "[x]"
					}
					fmt.Printf("%s %s (%s:%d)\n", checkbox, task.Description, relPath, task.LineNumber)
				}
			}
			fmt.Println()
		}
		os.Exit(0)
	}

	// Run TUI
	p := tea.NewProgram(newModel(sections, *vaultPath), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
