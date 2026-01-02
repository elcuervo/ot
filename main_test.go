package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
			name:        "not done filter",
			content:     "# Query\n\n```tasks\nnot done\n```\n",
			wantNotDone: true,
			wantErr:     false,
		},
		{
			name:        "no filter",
			content:     "# Query\n\n```tasks\ndue today\n```\n",
			wantNotDone: false,
			wantErr:     false,
		},
		{
			name:        "multiple filters including not done",
			content:     "# Query\n\n```tasks\nnot done\ndue today\ngroup by folder\n```\n",
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

func TestParseDueDate(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantDate    string
		wantNil     bool
	}{
		{
			name:        "task with due date",
			description: "Morning standup 📅 2025-12-29",
			wantDate:    "2025-12-29",
			wantNil:     false,
		},
		{
			name:        "task without due date",
			description: "Simple task without date",
			wantDate:    "",
			wantNil:     true,
		},
		{
			name:        "task with due date and priority",
			description: "Important task 📅 2025-01-15 ⏫",
			wantDate:    "2025-01-15",
			wantNil:     false,
		},
		{
			name:        "task with multiple emojis",
			description: "Task 🔁 every day 📅 2025-06-01 ✅ 2025-05-01",
			wantDate:    "2025-06-01",
			wantNil:     false,
		},
		{
			name:        "task with only completion date should have no due date",
			description: "Completed task ✅ 2025-05-01",
			wantDate:    "",
			wantNil:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDueDate(tt.description)
			if tt.wantNil {
				if got != nil {
					t.Errorf("Expected nil, got %v", got)
				}
			} else {
				if got == nil {
					t.Error("Expected non-nil date")
				} else if got.Format("2006-01-02") != tt.wantDate {
					t.Errorf("Got %s, want %s", got.Format("2006-01-02"), tt.wantDate)
				}
			}
		})
	}
}

func TestParseQueryFileDateFilters(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name            string
		content         string
		wantFilterCount int
		wantFirstField  string
		wantFirstOp     string
		wantFirstDate   string
	}{
		{
			name:            "due today",
			content:         "```tasks\nnot done\ndue today\n```\n",
			wantFilterCount: 1,
			wantFirstField:  "due",
			wantFirstOp:     "on",
			wantFirstDate:   "today",
		},
		{
			name:            "due tomorrow",
			content:         "```tasks\ndue tomorrow\n```\n",
			wantFilterCount: 1,
			wantFirstField:  "due",
			wantFirstOp:     "on",
			wantFirstDate:   "tomorrow",
		},
		{
			name:            "due before specific date",
			content:         "```tasks\ndue before 2025-12-31\n```\n",
			wantFilterCount: 1,
			wantFirstField:  "due",
			wantFirstOp:     "before",
			wantFirstDate:   "2025-12-31",
		},
		{
			name:            "due after specific date",
			content:         "```tasks\ndue after 2025-01-01\n```\n",
			wantFilterCount: 1,
			wantFirstField:  "due",
			wantFirstOp:     "after",
			wantFirstDate:   "2025-01-01",
		},
		{
			name:            "due on specific date",
			content:         "```tasks\ndue on 2025-06-15\n```\n",
			wantFilterCount: 1,
			wantFirstField:  "due",
			wantFirstOp:     "on",
			wantFirstDate:   "2025-06-15",
		},
		{
			name:            "no date filter",
			content:         "```tasks\nnot done\n```\n",
			wantFilterCount: 0,
		},
		{
			name:            "scheduled today",
			content:         "```tasks\nscheduled today\n```\n",
			wantFilterCount: 1,
			wantFirstField:  "scheduled",
			wantFirstOp:     "on",
			wantFirstDate:   "today",
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

			if len(query.DateFilters) != tt.wantFilterCount {
				t.Errorf("DateFilters count = %d, want %d", len(query.DateFilters), tt.wantFilterCount)
			}

			if tt.wantFilterCount > 0 {
				f := query.DateFilters[0]
				if f.Field != tt.wantFirstField {
					t.Errorf("Field = %q, want %q", f.Field, tt.wantFirstField)
				}
				if f.Operator != tt.wantFirstOp {
					t.Errorf("Operator = %q, want %q", f.Operator, tt.wantFirstOp)
				}
				if f.Date != tt.wantFirstDate {
					t.Errorf("Date = %q, want %q", f.Date, tt.wantFirstDate)
				}
			}
		})
	}
}

func TestMatchDateFilter(t *testing.T) {
	// Use fixed dates for testing
	parseDate := func(s string) *time.Time {
		d, _ := time.Parse("2006-01-02", s)
		return &d
	}

	tests := []struct {
		name   string
		task   *Task
		filter DateFilter
		want   bool
	}{
		{
			name:   "task on target date",
			task:   &Task{DueDate: parseDate("2025-12-29")},
			filter: DateFilter{Field: "due", Operator: "on", Date: "2025-12-29"},
			want:   true,
		},
		{
			name:   "task not on target date",
			task:   &Task{DueDate: parseDate("2025-12-30")},
			filter: DateFilter{Field: "due", Operator: "on", Date: "2025-12-29"},
			want:   false,
		},
		{
			name:   "task before target date",
			task:   &Task{DueDate: parseDate("2025-12-28")},
			filter: DateFilter{Field: "due", Operator: "before", Date: "2025-12-29"},
			want:   true,
		},
		{
			name:   "task not before target date",
			task:   &Task{DueDate: parseDate("2025-12-29")},
			filter: DateFilter{Field: "due", Operator: "before", Date: "2025-12-29"},
			want:   false,
		},
		{
			name:   "task after target date",
			task:   &Task{DueDate: parseDate("2025-12-30")},
			filter: DateFilter{Field: "due", Operator: "after", Date: "2025-12-29"},
			want:   true,
		},
		{
			name:   "task not after target date",
			task:   &Task{DueDate: parseDate("2025-12-29")},
			filter: DateFilter{Field: "due", Operator: "after", Date: "2025-12-29"},
			want:   false,
		},
		{
			name:   "nil task date",
			task:   &Task{DueDate: nil},
			filter: DateFilter{Field: "due", Operator: "on", Date: "2025-12-29"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchDateFilter(tt.task, tt.filter)
			if got != tt.want {
				t.Errorf("matchDateFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty string", input: "", want: ""},
		{name: "absolute path", input: "/usr/bin", want: "/usr/bin"},
		{name: "tilde only", input: "~", want: home},
		{name: "tilde with path", input: "~/Documents", want: filepath.Join(home, "Documents")},
		{name: "whitespace trimmed", input: "  /path  ", want: "/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("expandPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("expandPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveVaultPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "absolute path unchanged", input: "/vault", want: "/vault"},
		{name: "relative becomes absolute", input: "vault", want: filepath.Join(home, "vault")},
		{name: "tilde path", input: "~/vault", want: filepath.Join(home, "vault")},
		{name: "empty stays empty", input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveVaultPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveVaultPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("resolveVaultPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveQueryPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name    string
		query   string
		vault   string
		want    string
		wantErr bool
	}{
		{name: "absolute query unchanged", query: "/queries/q.md", vault: "/vault", want: "/queries/q.md"},
		{name: "relative joins vault", query: "queries/q.md", vault: "/vault", want: "/vault/queries/q.md"},
		{name: "tilde query expands", query: "~/q.md", vault: "/vault", want: filepath.Join(home, "q.md")},
		{name: "empty vault uses relative", query: "q.md", vault: "", want: "q.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveQueryPath(tt.query, tt.vault)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveQueryPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("resolveQueryPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateProfile(t *testing.T) {
	tests := []struct {
		name     string
		profile  Profile
		wantErr  bool
		errField string
	}{
		{name: "valid profile", profile: Profile{Vault: "/v", Query: "q.md"}, wantErr: false},
		{name: "empty vault", profile: Profile{Vault: "", Query: "q.md"}, wantErr: true, errField: "vault"},
		{name: "whitespace vault", profile: Profile{Vault: "  ", Query: "q.md"}, wantErr: true, errField: "vault"},
		{name: "empty query", profile: Profile{Vault: "/v", Query: ""}, wantErr: true, errField: "query"},
		{name: "both empty", profile: Profile{}, wantErr: true, errField: "vault"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProfile("test", tt.profile)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProfile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errField != "" {
				var pe *ProfileError
				if errors.As(err, &pe) && pe.Field != tt.errField {
					t.Errorf("error field = %q, want %q", pe.Field, tt.errField)
				}
			}
		})
	}
}

func TestSelectProfile(t *testing.T) {
	tests := []struct {
		name        string
		profileFlag string
		cfg         Config
		wantName    string
		wantNil     bool
		wantErr     bool
	}{
		{
			name:        "explicit flag",
			profileFlag: "work",
			cfg:         Config{Profiles: map[string]Profile{"work": {Vault: "/v", Query: "q"}}},
			wantName:    "work",
		},
		{
			name:        "default profile",
			profileFlag: "",
			cfg:         Config{DefaultProfile: "home", Profiles: map[string]Profile{"home": {Vault: "/v", Query: "q"}}},
			wantName:    "home",
		},
		{
			name:        "no profile",
			profileFlag: "",
			cfg:         Config{},
			wantNil:     true,
		},
		{
			name:        "flag profile not found",
			profileFlag: "missing",
			cfg:         Config{Profiles: map[string]Profile{"work": {}}},
			wantErr:     true,
		},
		{
			name:        "default profile not found",
			profileFlag: "",
			cfg:         Config{DefaultProfile: "missing", Profiles: map[string]Profile{}},
			wantErr:     true,
		},
		{
			name:        "flag with no profiles map",
			profileFlag: "work",
			cfg:         Config{},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, profile, err := selectProfile(tt.profileFlag, tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("selectProfile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantNil && profile != nil {
				t.Errorf("selectProfile() profile = %v, want nil", profile)
				return
			}
			if !tt.wantNil && !tt.wantErr && name != tt.wantName {
				t.Errorf("selectProfile() name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestResolveProfilePaths(t *testing.T) {
	tmpDir := t.TempDir()
	vaultDir := filepath.Join(tmpDir, "vault")
	os.MkdirAll(vaultDir, 0755)

	fileAsVault := filepath.Join(tmpDir, "file.txt")
	os.WriteFile(fileAsVault, []byte("not a dir"), 0644)

	tests := []struct {
		name     string
		profile  Profile
		wantErr  bool
		errField string
	}{
		{
			name:    "valid profile",
			profile: Profile{Vault: vaultDir, Query: "tasks.md"},
			wantErr: false,
		},
		{
			name:     "non-existent vault",
			profile:  Profile{Vault: filepath.Join(tmpDir, "nonexistent"), Query: "tasks.md"},
			wantErr:  true,
			errField: "vault",
		},
		{
			name:     "vault is file",
			profile:  Profile{Vault: fileAsVault, Query: "tasks.md"},
			wantErr:  true,
			errField: "vault",
		},
		{
			name:     "empty vault",
			profile:  Profile{Vault: "", Query: "tasks.md"},
			wantErr:  true,
			errField: "vault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := resolveProfilePaths("test", tt.profile)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveProfilePaths() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errField != "" {
				var pe *ProfileError
				if errors.As(err, &pe) && pe.Field != tt.errField {
					t.Errorf("error field = %q, want %q", pe.Field, tt.errField)
				}
			}
			if !tt.wantErr && resolved == nil {
				t.Error("resolveProfilePaths() returned nil without error")
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "valid config",
			cfg:     Config{DefaultProfile: "work", Profiles: map[string]Profile{"work": {Vault: "/v", Query: "q"}}},
			wantErr: false,
		},
		{
			name:    "no default profile",
			cfg:     Config{Profiles: map[string]Profile{"work": {Vault: "/v", Query: "q"}}},
			wantErr: false,
		},
		{
			name:    "missing default profile",
			cfg:     Config{DefaultProfile: "missing", Profiles: map[string]Profile{"work": {}}},
			wantErr: true,
		},
		{
			name:    "empty config",
			cfg:     Config{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContainsGlob(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"simple/path", false},
		{"path/to/file.md", false},
		{"path/*/file.md", true},
		{"path/**/file.md", true},
		{"path/?.md", true},
		{"path/[abc].md", true},
		{"~/vault", false},
		{"projects/*/todo.md", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := containsGlob(tt.path)
			if got != tt.want {
				t.Errorf("containsGlob(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestResolveQuery(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a query file
	queryFile := filepath.Join(tmpDir, "query.md")
	os.WriteFile(queryFile, []byte("```tasks\nnot done\ndue today\n```\n"), 0644)

	tests := []struct {
		name      string
		input     string
		vaultPath string
		wantLen   int
		wantErr   bool
	}{
		{
			name:      "inline query not done",
			input:     "not done",
			vaultPath: tmpDir,
			wantLen:   1,
			wantErr:   false,
		},
		{
			name:      "inline query due today",
			input:     "due today",
			vaultPath: tmpDir,
			wantLen:   1,
			wantErr:   false,
		},
		{
			name:      "query file path",
			input:     queryFile,
			vaultPath: tmpDir,
			wantLen:   1,
			wantErr:   false,
		},
		{
			name:      "relative query file",
			input:     "query.md",
			vaultPath: tmpDir,
			wantLen:   1,
			wantErr:   false,
		},
		{
			name:      "nonexistent file treated as inline",
			input:     "nonexistent.md",
			vaultPath: tmpDir,
			wantLen:   1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, err := resolveQuery(tt.input, tt.vaultPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(queries) != tt.wantLen {
				t.Errorf("resolveQuery() returned %d queries, want %d", len(queries), tt.wantLen)
			}
		})
	}
}

func TestParseInlineQuery(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantNotDone bool
		wantGroupBy string
	}{
		{
			name:        "not done",
			input:       "not done",
			wantNotDone: true,
			wantGroupBy: "",
		},
		{
			name:        "due today",
			input:       "due today",
			wantNotDone: false,
			wantGroupBy: "",
		},
		{
			name:        "not done with group by",
			input:       "not done\ngroup by folder",
			wantNotDone: true,
			wantGroupBy: "folder",
		},
		{
			name:        "empty string",
			input:       "",
			wantNotDone: false,
			wantGroupBy: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, err := parseInlineQuery(tt.input)
			if err != nil {
				t.Fatalf("parseInlineQuery() error = %v", err)
			}
			if len(queries) != 1 {
				t.Fatalf("parseInlineQuery() returned %d queries, want 1", len(queries))
			}
			q := queries[0]
			if q.NotDone != tt.wantNotDone {
				t.Errorf("NotDone = %v, want %v", q.NotDone, tt.wantNotDone)
			}
			if q.GroupBy != tt.wantGroupBy {
				t.Errorf("GroupBy = %q, want %q", q.GroupBy, tt.wantGroupBy)
			}
		})
	}
}
