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

// parseQueryFile checks if the query file contains "not done" filter
func parseQueryFile(filePath string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, err
	}

	// Find ```tasks block
	blockRe := regexp.MustCompile("(?s)```tasks\\s*\\n(.+?)```")
	matches := blockRe.FindStringSubmatch(string(content))
	if matches == nil {
		return false, fmt.Errorf("no ```tasks block found in %s", filePath)
	}

	queryContent := matches[1]
	return strings.Contains(queryContent, "not done"), nil
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
	tasks     []*Task
	cursor    int
	vaultPath string
	quitting  bool
	err       error
}

func newModel(tasks []*Task, vaultPath string) model {
	return model{
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
			// Save all modified tasks
			for _, task := range m.tasks {
				if task.Modified {
					if err := saveTask(task); err != nil {
						m.err = err
					}
				}
			}
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
				m.tasks[m.cursor].Toggle()
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
)

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err)
	}

	if m.quitting {
		modified := 0
		for _, t := range m.tasks {
			if t.Modified {
				modified++
			}
		}
		if modified > 0 {
			return fmt.Sprintf("Saved %d modified task(s). Goodbye!\n", modified)
		}
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
		for i, task := range m.tasks {
			cursor := "  "
			if m.cursor == i {
				cursor = cursorStyle.Render("> ")
			}

			// Build checkbox
			checkbox := "[ ]"
			if task.Done {
				checkbox = "[x]"
			}

			// Get relative path for display
			relPath := task.FilePath
			if rel, err := filepath.Rel(m.vaultPath, task.FilePath); err == nil {
				relPath = rel
			}
			fileInfo := fileStyle.Render(fmt.Sprintf("(%s:%d)", relPath, task.LineNumber))

			// Format line
			var line string
			if task.Done {
				line = doneStyle.Render(fmt.Sprintf("%s %s", checkbox, task.Description))
			} else {
				line = fmt.Sprintf("%s %s", checkbox, task.Description)
			}

			// Highlight if selected
			if m.cursor == i {
				line = selectedStyle.Render(line)
			}

			// Mark modified tasks
			modified := ""
			if task.Modified {
				modified = " *"
			}

			b.WriteString(fmt.Sprintf("%s%s %s%s\n", cursor, line, fileInfo, modified))
		}
	}

	// Help
	help := helpStyle.Render("↑/k up • ↓/j down • space/enter toggle • q quit (auto-saves)")
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
	filterNotDone, err := parseQueryFile(queryFile)
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
		if filterNotDone && task.Done {
			continue // Skip done tasks if "not done" filter is active
		}
		filteredTasks = append(filteredTasks, task)
	}

	if len(filteredTasks) == 0 {
		fmt.Println("No tasks found matching the query.")
		os.Exit(0)
	}

	// List mode (non-interactive)
	if *listOnly {
		fmt.Printf("Found %d task(s):\n\n", len(filteredTasks))
		for _, task := range filteredTasks {
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
		os.Exit(0)
	}

	// Run TUI
	p := tea.NewProgram(newModel(filteredTasks, *vaultPath), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
