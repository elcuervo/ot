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
	FilePath    string // Source file path
	LineNumber  int    // 1-indexed line number
	RawLine     string // Original line content
	Done        bool   // Is the task completed?
	Description string // Task text without checkbox
	Modified    bool   // Has this task been modified?
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
			})
		}
	}

	return tasks, scanner.Err()
}

// Query represents parsed query options
type Query struct {
	NotDone bool
	GroupBy string // "folder", "filename", or ""
}

// parseQueryFile checks if the query file contains "not done" filter (simple version)
func parseQueryFile(filePath string) (bool, error) {
	query, err := parseQueryFileExtended(filePath)
	if err != nil {
		return false, err
	}
	return query.NotDone, nil
}

// parseQueryFileExtended parses the query file and extracts all supported options
// It combines settings from all ```tasks blocks in the file
func parseQueryFileExtended(filePath string) (*Query, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Find all ```tasks blocks
	blockRe := regexp.MustCompile("(?s)```tasks\\s*\\n(.+?)```")
	matches := blockRe.FindAllStringSubmatch(string(content), -1)
	if matches == nil {
		return nil, fmt.Errorf("no ```tasks block found in %s", filePath)
	}

	query := &Query{}
	groupByFuncRe := regexp.MustCompile(`group by function task\.file\.(\w+)`)
	groupBySimpleRe := regexp.MustCompile(`group by (\w+)`)

	// Combine settings from all blocks
	for _, match := range matches {
		queryContent := match[1]

		// Parse filters
		if strings.Contains(queryContent, "not done") {
			query.NotDone = true
		}

		// Parse group by - supports both simple and function syntax
		// Simple: "group by folder", "group by filename"
		// Function: "group by function task.file.folder"
		if query.GroupBy == "" {
			if funcMatch := groupByFuncRe.FindStringSubmatch(queryContent); funcMatch != nil {
				query.GroupBy = funcMatch[1]
			} else if simpleMatch := groupBySimpleRe.FindStringSubmatch(queryContent); simpleMatch != nil {
				// Skip "function" as a group by value
				if simpleMatch[1] != "function" {
					query.GroupBy = simpleMatch[1]
				}
			}
		}
	}

	return query, nil
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

// TUI Model
type model struct {
	groups    []TaskGroup
	tasks     []*Task // Flat list for navigation
	cursor    int
	vaultPath string
	groupBy   string
	quitting  bool
	err       error
}

func newModel(groups []TaskGroup, vaultPath string, groupBy string) model {
	// Create flat task list for navigation
	var tasks []*Task
	for _, g := range groups {
		tasks = append(tasks, g.Tasks...)
	}

	return model{
		groups:    groups,
		tasks:     tasks,
		vaultPath: vaultPath,
		groupBy:   groupBy,
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
		for _, group := range m.groups {
			// Show group header if grouping is active
			if m.groupBy != "" && group.Name != "" {
				b.WriteString(groupStyle.Render(fmt.Sprintf("## %s", group.Name)) + "\n")
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
				if m.groupBy != "filename" {
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

	// Help
	help := helpStyle.Render("↑/k up • ↓/j down • space/enter toggle (saves immediately) • q quit")
	b.WriteString("\n" + help)

	return b.String()
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
		fmt.Println("  not done        Show only incomplete tasks")
		fmt.Println("  group by folder Group tasks by folder")
		fmt.Println("  group by filename Group tasks by filename")
		fmt.Println("\nExample:")
		fmt.Println("  mt --vault ~/obsidian-vault query.md")
		os.Exit(1)
	}

	queryFile := args[0]

	if *vaultPath == "" {
		fmt.Println("Error: --vault flag is required")
		os.Exit(1)
	}

	// Parse query file
	query, err := parseQueryFileExtended(queryFile)
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

	// Apply filter
	var filteredTasks []*Task
	for _, task := range allTasks {
		if query.NotDone && task.Done {
			continue // Skip done tasks if "not done" filter is active
		}
		filteredTasks = append(filteredTasks, task)
	}

	if len(filteredTasks) == 0 {
		fmt.Println("No tasks found matching the query.")
		os.Exit(0)
	}

	// Group tasks
	groups := groupTasks(filteredTasks, query.GroupBy, *vaultPath)

	// List mode (non-interactive)
	if *listOnly {
		fmt.Printf("Found %d task(s):\n\n", len(filteredTasks))
		for _, group := range groups {
			if query.GroupBy != "" && group.Name != "" {
				fmt.Printf("## %s\n", group.Name)
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
			if query.GroupBy != "" {
				fmt.Println()
			}
		}
		os.Exit(0)
	}

	// Run TUI
	p := tea.NewProgram(newModel(groups, *vaultPath, query.GroupBy), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
