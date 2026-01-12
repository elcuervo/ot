package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	checkboxRe  = regexp.MustCompile(`^(\s*-\s*)\[([ xX])\](.*)$`)
	doneRe      = regexp.MustCompile(`\s*✅\s*\d{4}-\d{2}-\d{2}`)
	doneDateRe  = regexp.MustCompile(`✅\s*(\d{4}-\d{2}-\d{2})`)
	taskRe      = regexp.MustCompile(`^\s*-\s*\[([ xX])\]\s*(.*)$`)
	dueDateRe   = regexp.MustCompile(`📅\s*(\d{4}-\d{2}-\d{2})`)
	schedDateRe = regexp.MustCompile(`⏳\s*(\d{4}-\d{2}-\d{2})`)
	priorityRe  = regexp.MustCompile(`[🔺⏫🔼🔽⏬]`)
)

// Priority levels (lower value = higher priority)
const (
	PriorityHighest = iota + 1
	PriorityHigh
	PriorityMedium
	PriorityNormal
	PriorityLow
	PriorityLowest
)

var priorityEmojis = map[int]string{
	PriorityHighest: "🔺",
	PriorityHigh:    "⏫",
	PriorityMedium:  "🔼",
	PriorityNormal:  "",
	PriorityLow:     "🔽",
	PriorityLowest:  "⏬",
}

var emojiToPriority = map[string]int{
	"🔺": PriorityHighest,
	"⏫": PriorityHigh,
	"🔼": PriorityMedium,
	"🔽": PriorityLow,
	"⏬": PriorityLowest,
}

// Task represents a single task from a markdown file
type Task struct {
	FilePath      string
	LineNumber    int
	RawLine       string
	Done          bool
	Description   string
	Modified      bool
	DueDate       *time.Time
	ScheduledDate *time.Time
	DoneDate      *time.Time
	Priority      int
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

	prefix := matches[1]
	if t.Done {
		now := time.Now()
		t.DoneDate = &now
	} else {
		t.DoneDate = nil
	}

	t.rebuildRawLineWithPrefix(prefix)
}

// rebuildRawLine rebuilds the raw line with a new description
func (t *Task) rebuildRawLine() {
	matches := checkboxRe.FindStringSubmatch(t.RawLine)
	if matches == nil {
		return
	}

	prefix := matches[1]
	t.rebuildRawLineWithPrefix(prefix)
}

func (t *Task) rebuildRawLineWithPrefix(prefix string) {
	checkbox := "[ ]"
	if t.Done {
		checkbox = "[x]"
	}

	desc := strings.TrimSpace(t.Description)
	line := fmt.Sprintf("%s%s", prefix, checkbox)
	if desc != "" {
		line = fmt.Sprintf("%s %s", line, desc)
	} else {
		line = line + " "
	}

	if t.Done && t.DoneDate != nil {
		line = fmt.Sprintf("%s ✅ %s", line, t.DoneDate.Format("2006-01-02"))
	}

	t.RawLine = line
}

// scanVault recursively finds all .md files in a directory
func scanVault(vaultPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && strings.HasPrefix(info.Name(), ".") && path != vaultPath {
			return filepath.SkipDir
		}

		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// parseDueDate extracts due date from task description
func parseDueDate(description string) *time.Time {
	matches := dueDateRe.FindStringSubmatch(description)
	if matches == nil {
		return nil
	}
	date, err := time.ParseInLocation("2006-01-02", matches[1], time.Local)
	if err != nil {
		return nil
	}
	return &date
}

// parseScheduledDate extracts scheduled date from task description
func parseScheduledDate(description string) *time.Time {
	matches := schedDateRe.FindStringSubmatch(description)
	if matches == nil {
		return nil
	}
	date, err := time.ParseInLocation("2006-01-02", matches[1], time.Local)
	if err != nil {
		return nil
	}
	return &date
}

// parseDoneDate extracts completion date from task description
func parseDoneDate(description string) *time.Time {
	matches := doneDateRe.FindStringSubmatch(description)
	if matches == nil {
		return nil
	}
	date, err := time.ParseInLocation("2006-01-02", matches[1], time.Local)
	if err != nil {
		return nil
	}
	return &date
}

func normalizeDescription(description string) (string, *time.Time) {
	doneDate := parseDoneDate(description)
	clean := strings.TrimSpace(doneRe.ReplaceAllString(description, ""))
	return clean, doneDate
}

// DisplayDescription returns the description with completion date appended when present.
func (t *Task) DisplayDescription() string {
	desc := strings.TrimSpace(t.Description)
	if t.Done && t.DoneDate != nil {
		if desc == "" {
			return fmt.Sprintf("✅ %s", t.DoneDate.Format("2006-01-02"))
		}
		return fmt.Sprintf("%s ✅ %s", desc, t.DoneDate.Format("2006-01-02"))
	}
	return desc
}

// parsePriority extracts priority from task description
func parsePriority(description string) int {
	match := priorityRe.FindString(description)
	if match == "" {
		return PriorityNormal
	}
	if priority, ok := emojiToPriority[match]; ok {
		return priority
	}
	return PriorityNormal
}

// SetPriority updates the task's priority
func (t *Task) SetPriority(priority int) {
	if priority < PriorityHighest {
		priority = PriorityHighest
	}
	if priority > PriorityLowest {
		priority = PriorityLowest
	}

	// Remove existing priority emoji from description
	t.Description = strings.TrimSpace(priorityRe.ReplaceAllString(t.Description, ""))

	// Add new priority emoji if not normal
	if emoji := priorityEmojis[priority]; emoji != "" {
		t.Description = t.Description + " " + emoji
	}

	t.Priority = priority
	t.Modified = true
	t.rebuildRawLine()
}

// CyclePriorityUp increases priority (towards highest)
func (t *Task) CyclePriorityUp() {
	t.SetPriority(t.Priority - 1)
}

// CyclePriorityDown decreases priority (towards lowest)
func (t *Task) CyclePriorityDown() {
	t.SetPriority(t.Priority + 1)
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
			rawDescription := strings.TrimSpace(matches[2])
			description, doneDate := normalizeDescription(rawDescription)
			if status != "x" {
				doneDate = nil
			}

			tasks = append(tasks, &Task{
				FilePath:      filePath,
				LineNumber:    lineNum,
				RawLine:       line,
				Done:          status == "x",
				Description:   description,
				DueDate:       parseDueDate(description),
				ScheduledDate: parseScheduledDate(description),
				DoneDate:      doneDate,
				Priority:      parsePriority(description),
			})
		}
	}

	return tasks, scanner.Err()
}

// saveTask writes the modified task back to its source file
func saveTask(task *Task) error {
	content, err := os.ReadFile(task.FilePath)

	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")

	if task.LineNumber > 0 && task.LineNumber <= len(lines) {
		lines[task.LineNumber-1] = task.RawLine
	}

	tempPath := task.FilePath + ".tmp"
	err = os.WriteFile(tempPath, []byte(strings.Join(lines, "\n")), 0644)

	if err != nil {
		return err
	}

	return os.Rename(tempPath, task.FilePath)
}

// deleteTask removes a task line from its source file
func deleteTask(task *Task) error {
	content, err := os.ReadFile(task.FilePath)

	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")

	if task.LineNumber > 0 && task.LineNumber <= len(lines) {
		lines = append(lines[:task.LineNumber-1], lines[task.LineNumber:]...)
	}

	tempPath := task.FilePath + ".tmp"
	err = os.WriteFile(tempPath, []byte(strings.Join(lines, "\n")), 0644)

	if err != nil {
		return err
	}

	return os.Rename(tempPath, task.FilePath)
}

// restoreTaskLine inserts a line back into the file at the specified line number
func restoreTaskLine(filePath string, lineNumber int, line string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")

	insertAt := lineNumber - 1
	if insertAt < 0 {
		insertAt = 0
	}
	if insertAt > len(lines) {
		insertAt = len(lines)
	}

	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertAt]...)
	newLines = append(newLines, line)
	newLines = append(newLines, lines[insertAt:]...)

	tempPath := filePath + ".tmp"
	err = os.WriteFile(tempPath, []byte(strings.Join(newLines, "\n")), 0644)
	if err != nil {
		return err
	}

	return os.Rename(tempPath, filePath)
}

// addTask inserts a new task line after the reference task in its source file
func addTask(refTask *Task, description string) (*Task, error) {
	content, err := os.ReadFile(refTask.FilePath)

	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	cleanDesc, _ := normalizeDescription(description)
	newLine := "- [ ] " + cleanDesc

	// Insert after the reference task's line
	insertAt := refTask.LineNumber
	if insertAt > len(lines) {
		insertAt = len(lines)
	}

	// Insert the new line
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertAt]...)
	newLines = append(newLines, newLine)
	newLines = append(newLines, lines[insertAt:]...)

	tempPath := refTask.FilePath + ".tmp"
	err = os.WriteFile(tempPath, []byte(strings.Join(newLines, "\n")), 0644)

	if err != nil {
		return nil, err
	}

	if err := os.Rename(tempPath, refTask.FilePath); err != nil {
		return nil, err
	}

	return &Task{
		FilePath:    refTask.FilePath,
		LineNumber:  insertAt + 1,
		RawLine:     newLine,
		Done:        false,
		Description: cleanDesc,
		Priority:    PriorityNormal,
	}, nil
}

// addEmptyTask inserts an empty task line after the reference task and returns it
func addEmptyTask(refTask *Task) (*Task, error) {
	return addTask(refTask, "")
}

// openNewTaskInEditor creates an empty task and opens it in an external editor
func openNewTaskInEditor(refTask *Task) tea.Cmd {
	newTask, err := addEmptyTask(refTask)
	if err != nil {
		return func() tea.Msg {
			return editorFinishedMsg{err: err, task: nil}
		}
	}

	return openInEditor(newTask)
}

// editorFinishedMsg is sent when the external editor closes
type editorFinishedMsg struct {
	err  error
	task *Task
}

func splitCommandLine(command string) []string {
	var args []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	for _, r := range command {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' && !inSingle {
			escaped = true
			continue
		}

		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
				continue
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
				continue
			}
		case ' ', '\t', '\n':
			if !inSingle && !inDouble {
				if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
				continue
			}
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// openInEditor opens the task file in an external editor at the correct line
func openInEditor(task *Task) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	lineArg := fmt.Sprintf("+%d", task.LineNumber)
	parts := splitCommandLine(editor)
	if len(parts) == 0 {
		parts = []string{"vi"}
	}
	args := append(parts[1:], lineArg, task.FilePath)
	c := exec.Command(parts[0], args...)

	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err: err, task: task}
	})
}

// createTasksFile creates a tasks.md file with an empty task in the current directory
func createTasksFile() error {
	filename := "tasks.md"

	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("%s already exists", filename)
	}

	content := "# Tasks\n\n- [ ] \n"
	return os.WriteFile(filename, []byte(content), 0644)
}
