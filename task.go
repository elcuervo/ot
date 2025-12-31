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
	checkboxRe = regexp.MustCompile(`^(\s*-\s*)\[([ xX])\](.*)$`)
	doneRe     = regexp.MustCompile(`\s*✅\s*\d{4}-\d{2}-\d{2}`)
	taskRe     = regexp.MustCompile(`^\s*-\s*\[([ xX])\]\s*(.*)$`)
	dueDateRe  = regexp.MustCompile(`📅\s*(\d{4}-\d{2}-\d{2})`)
)

// Task represents a single task from a markdown file
type Task struct {
	FilePath    string
	LineNumber  int
	RawLine     string
	Done        bool
	Description string
	Modified    bool
	DueDate     *time.Time
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
	content := matches[3]

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

	prefix := matches[1]
	checkbox := "[ ]"
	if t.Done {
		checkbox = "[x]"
	}

	t.RawLine = fmt.Sprintf("%s%s %s", prefix, checkbox, t.Description)
}

// scanVault recursively finds all .md files in a directory
func scanVault(vaultPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
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

// editorFinishedMsg is sent when the external editor closes
type editorFinishedMsg struct {
	err  error
	task *Task
}

// openInEditor opens the task file in an external editor at the correct line
func openInEditor(task *Task) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	lineArg := fmt.Sprintf("+%d", task.LineNumber)
	c := exec.Command(editor, lineArg, task.FilePath)

	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err: err, task: task}
	})
}
