package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskToggle(t *testing.T) {
	task := &Task{
		FilePath:    "test.md",
		LineNumber:  1,
		RawLine:     "- [ ] Test task",
		Done:        false,
		Description: "Test task",
	}

	// Toggle to done
	task.Toggle()
	if !task.Done {
		t.Error("Expected task to be done after toggle")
	}
	if !task.Modified {
		t.Error("Expected task to be marked as modified")
	}
	if !strings.Contains(task.RawLine, "[x]") {
		t.Errorf("Expected RawLine to contain [x], got: %s", task.RawLine)
	}
	if !strings.Contains(task.RawLine, "✅") {
		t.Errorf("Expected RawLine to contain done date emoji, got: %s", task.RawLine)
	}

	// Toggle back to not done
	task.Toggle()
	if task.Done {
		t.Error("Expected task to be not done after second toggle")
	}
	if !strings.Contains(task.RawLine, "[ ]") {
		t.Errorf("Expected RawLine to contain [ ], got: %s", task.RawLine)
	}
	if strings.Contains(task.RawLine, "✅") {
		t.Errorf("Expected RawLine to not contain done date emoji, got: %s", task.RawLine)
	}
}

func TestTaskToggleWithExistingMetadata(t *testing.T) {
	task := &Task{
		FilePath:    "test.md",
		LineNumber:  1,
		RawLine:     "- [ ] Test task 📅 2025-01-15 ⏫",
		Done:        false,
		Description: "Test task 📅 2025-01-15 ⏫",
	}

	task.Toggle()
	if !strings.Contains(task.RawLine, "📅 2025-01-15") {
		t.Errorf("Expected due date to be preserved, got: %s", task.RawLine)
	}
	if !strings.Contains(task.RawLine, "⏫") {
		t.Errorf("Expected priority to be preserved, got: %s", task.RawLine)
	}
	if !strings.Contains(task.RawLine, "✅") {
		t.Errorf("Expected done date to be added, got: %s", task.RawLine)
	}
}

func TestTaskToggleWithIndentation(t *testing.T) {
	task := &Task{
		FilePath:    "test.md",
		LineNumber:  1,
		RawLine:     "  - [ ] Indented task",
		Done:        false,
		Description: "Indented task",
	}

	task.Toggle()
	if !strings.HasPrefix(task.RawLine, "  - [x]") {
		t.Errorf("Expected indentation to be preserved, got: %s", task.RawLine)
	}
}

func TestParseFile(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")

	content := `# Test File

- [ ] Task one
- [x] Task two (done)
- [ ] Task three 📅 2025-01-15

Some text here.

  - [ ] Indented task
`
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tasks, err := parseFile(testFile)
	if err != nil {
		t.Fatalf("parseFile failed: %v", err)
	}

	if len(tasks) != 4 {
		t.Errorf("Expected 4 tasks, got %d", len(tasks))
	}

	// Check first task
	if tasks[0].Done {
		t.Error("First task should not be done")
	}
	if tasks[0].Description != "Task one" {
		t.Errorf("First task description wrong: %s", tasks[0].Description)
	}
	if tasks[0].LineNumber != 3 {
		t.Errorf("First task line number wrong: %d", tasks[0].LineNumber)
	}

	// Check second task (done)
	if !tasks[1].Done {
		t.Error("Second task should be done")
	}

	// Check indented task
	if tasks[3].Description != "Indented task" {
		t.Errorf("Indented task description wrong: %s", tasks[3].Description)
	}
}

func TestParseQueryFile(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		content     string
		wantNotDone bool
		wantErr     bool
	}{
		{
			name: "not done filter",
			content: "# Query\n\n```tasks\nnot done\n```\n",
			wantNotDone: true,
			wantErr:     false,
		},
		{
			name: "no filter",
			content: "# Query\n\n```tasks\ndue today\n```\n",
			wantNotDone: false,
			wantErr:     false,
		},
		{
			name: "multiple filters including not done",
			content: "# Query\n\n```tasks\nnot done\ndue today\ngroup by folder\n```\n",
			wantNotDone: true,
			wantErr:     false,
		},
		{
			name:        "no tasks block",
			content:     "# Just markdown\n\nNo tasks block here.",
			wantNotDone: false,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile := filepath.Join(tmpDir, tt.name+".md")
			err := os.WriteFile(testFile, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			gotNotDone, err := parseQueryFile(testFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseQueryFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotNotDone != tt.wantNotDone {
				t.Errorf("parseQueryFile() = %v, want %v", gotNotDone, tt.wantNotDone)
			}
		})
	}
}

func TestScanVault(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test structure
	os.MkdirAll(filepath.Join(tmpDir, "notes"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "projects"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, ".obsidian"), 0755) // Should be skipped

	os.WriteFile(filepath.Join(tmpDir, "root.md"), []byte("# Root"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "notes", "note1.md"), []byte("# Note 1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "notes", "note2.md"), []byte("# Note 2"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "projects", "project.md"), []byte("# Project"), 0644)
	os.WriteFile(filepath.Join(tmpDir, ".obsidian", "config.md"), []byte("config"), 0644) // Should be skipped
	os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("text file"), 0644)          // Should be skipped

	files, err := scanVault(tmpDir)
	if err != nil {
		t.Fatalf("scanVault failed: %v", err)
	}

	if len(files) != 4 {
		t.Errorf("Expected 4 .md files, got %d: %v", len(files), files)
	}

	// Check that hidden dir files are not included
	for _, f := range files {
		if strings.Contains(f, ".obsidian") {
			t.Errorf("Hidden directory file should not be included: %s", f)
		}
		if strings.HasSuffix(f, ".txt") {
			t.Errorf("Non-md file should not be included: %s", f)
		}
	}
}

func TestSaveTask(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")

	content := `# Test File

- [ ] Task one
- [ ] Task two
- [ ] Task three
`
	err := os.WriteFile(testFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Parse and modify a task
	tasks, _ := parseFile(testFile)
	tasks[1].Toggle() // Toggle "Task two"

	// Save the task
	err = saveTask(tasks[1])
	if err != nil {
		t.Fatalf("saveTask failed: %v", err)
	}

	// Read the file and verify
	saved, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	lines := strings.Split(string(saved), "\n")
	if !strings.Contains(lines[3], "[x]") {
		t.Errorf("Expected line 4 to contain [x], got: %s", lines[3])
	}
	if !strings.Contains(lines[3], "✅") {
		t.Errorf("Expected line 4 to contain done date, got: %s", lines[3])
	}

	// Other lines should be unchanged
	if !strings.Contains(lines[2], "[ ]") {
		t.Errorf("Expected line 3 to be unchanged, got: %s", lines[2])
	}
	if !strings.Contains(lines[4], "[ ]") {
		t.Errorf("Expected line 5 to be unchanged, got: %s", lines[4])
	}
}

func TestParseQueryFileGroupBy(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		content     string
		wantGroupBy string
	}{
		{
			name:        "group by folder",
			content:     "```tasks\nnot done\ngroup by folder\n```\n",
			wantGroupBy: "folder",
		},
		{
			name:        "group by filename",
			content:     "```tasks\nnot done\ngroup by filename\n```\n",
			wantGroupBy: "filename",
		},
		{
			name:        "group by function task.file.folder",
			content:     "```tasks\ngroup by function task.file.folder\nnot done\n```\n",
			wantGroupBy: "folder",
		},
		{
			name:        "group by function task.file.filename",
			content:     "```tasks\ngroup by function task.file.filename\nnot done\n```\n",
			wantGroupBy: "filename",
		},
		{
			name:        "no grouping",
			content:     "```tasks\nnot done\n```\n",
			wantGroupBy: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile := filepath.Join(tmpDir, tt.name+".md")
			err := os.WriteFile(testFile, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			query, err := parseQueryFileExtended(testFile)
			if err != nil {
				t.Fatalf("parseQueryFileExtended failed: %v", err)
			}

			if query.GroupBy != tt.wantGroupBy {
				t.Errorf("GroupBy = %q, want %q", query.GroupBy, tt.wantGroupBy)
			}
		})
	}
}

func TestGroupTasksByFolder(t *testing.T) {
	tasks := []*Task{
		{FilePath: "/vault/notes/daily.md", Description: "Task 1"},
		{FilePath: "/vault/projects/work.md", Description: "Task 2"},
		{FilePath: "/vault/notes/weekly.md", Description: "Task 3"},
		{FilePath: "/vault/projects/home.md", Description: "Task 4"},
	}

	groups := groupTasks(tasks, "folder", "/vault")

	if len(groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(groups))
	}

	// Check that tasks are grouped correctly
	notesCount := 0
	projectsCount := 0
	for _, g := range groups {
		if g.Name == "notes" {
			notesCount = len(g.Tasks)
		}
		if g.Name == "projects" {
			projectsCount = len(g.Tasks)
		}
	}

	if notesCount != 2 {
		t.Errorf("Expected 2 tasks in notes, got %d", notesCount)
	}
	if projectsCount != 2 {
		t.Errorf("Expected 2 tasks in projects, got %d", projectsCount)
	}
}
