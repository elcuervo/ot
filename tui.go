package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultWindowHeight  = 24
	defaultWindowWidth   = 80
	minVisibleHeight     = 3
	maxInputWidth        = 70
	minInputWidth        = 30
	prioritySaveDebounce = 500 * time.Millisecond
	cursorCharacter      = ">"
)

type prioritySaveMsg struct {
	key  string
	at   time.Time
	task *Task
}

// OperationType represents the type of operation that can be undone
type OperationType int

const (
	OpToggle OperationType = iota
	OpDelete
	OpPriorityChange
)

// UndoEntry represents a single undoable operation
type UndoEntry struct {
	Type             OperationType
	Timestamp        time.Time
	FilePath         string
	LineNumber       int
	DeletedLine      string // For deletion undo
	PreviousPriority int    // For priority undo
	WasDone          bool   // For toggle undo
}

const maxUndoStackSize = 50

// ProfileTab holds per-profile state for tabbed mode
type ProfileTab struct {
	Profile   *ResolvedProfile
	Sections  []QuerySection
	Tasks     []*Task
	Cursor    int
	Cache     *TaskCache
	Watcher   *Watcher
	Debouncer *Debouncer
	Queries   []*Query
}

// model is the BubbleTea model
type model struct {
	// Tab mode (when multiple profiles)
	tabsEnabled bool
	tabs        []ProfileTab
	activeTab   int

	sections     []QuerySection
	tasks        []*Task
	cursor       int
	vaultPath    string
	titleName    string
	queryFile    string
	queries      []*Query
	quitting     bool
	err          error
	windowHeight int
	windowWidth  int
	aboutOpen    bool
	viewport     viewport.Model

	searching        bool
	searchQuery      string
	searchNavigating bool
	filteredTasks    []*Task
	taskToSection    map[*Task]string
	taskToGroup      map[*Task]string

	editorMode  string
	editing     bool
	editingTask *Task
	textInput   textinput.Model

	deleting     bool
	deletingTask *Task

	adding      bool
	addingRef   *Task
	addingInput textinput.Model

	// File watching and caching
	cache             *TaskCache
	watcher           *Watcher
	debouncer         *Debouncer
	selfModifiedFiles map[string]time.Time

	// Undo stack for all operations
	undoStack []UndoEntry

	// Debounced priority saves (keyed by taskKey)
	prioritySavePending map[string]time.Time
}

func newModel(sections []QuerySection, vaultPath string, titleName string, queryFile string, queries []*Query, editorMode string, cache *TaskCache, watcher *Watcher, debouncer *Debouncer) model {
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
		sections:            sections,
		tasks:               tasks,
		vaultPath:           vaultPath,
		titleName:           titleName,
		queryFile:           queryFile,
		queries:             queries,
		windowHeight:        defaultWindowHeight,
		windowWidth:         defaultWindowWidth,
		viewport:            viewport.New(defaultWindowWidth, defaultWindowHeight),
		taskToSection:       taskToSection,
		taskToGroup:         taskToGroup,
		editorMode:          editorMode,
		cache:               cache,
		watcher:             watcher,
		debouncer:           debouncer,
		selfModifiedFiles:   make(map[string]time.Time),
		undoStack:           make([]UndoEntry, 0),
		prioritySavePending: make(map[string]time.Time),
	}
}

func newModelWithTabs(tabs []ProfileTab) model {
	if len(tabs) == 0 {
		return model{
			windowHeight:        defaultWindowHeight,
			windowWidth:         defaultWindowWidth,
			viewport:            viewport.New(defaultWindowWidth, defaultWindowHeight),
			selfModifiedFiles:   make(map[string]time.Time),
			undoStack:           make([]UndoEntry, 0),
			prioritySavePending: make(map[string]time.Time),
		}
	}

	// Build task mappings for first tab
	taskToSection := make(map[*Task]string)
	taskToGroup := make(map[*Task]string)
	firstTab := tabs[0]
	for _, s := range firstTab.Sections {
		for _, g := range s.Groups {
			for _, task := range g.Tasks {
				taskToSection[task] = s.Name
				taskToGroup[task] = g.Name
			}
		}
	}

	queryFile := ""
	if firstTab.Profile.QueryIsFile {
		queryFile = firstTab.Profile.Query
	}

	return model{
		tabsEnabled:         true,
		tabs:                tabs,
		activeTab:           0,
		sections:            firstTab.Sections,
		tasks:               firstTab.Tasks,
		cursor:              firstTab.Cursor,
		vaultPath:           firstTab.Profile.VaultPath,
		titleName:           firstTab.Profile.Name,
		queryFile:           queryFile,
		queries:             firstTab.Queries,
		windowHeight:        defaultWindowHeight,
		windowWidth:         defaultWindowWidth,
		viewport:            viewport.New(defaultWindowWidth, defaultWindowHeight),
		taskToSection:       taskToSection,
		taskToGroup:         taskToGroup,
		editorMode:          firstTab.Profile.EditorMode,
		cache:               firstTab.Cache,
		watcher:             firstTab.Watcher,
		debouncer:           firstTab.Debouncer,
		selfModifiedFiles:   make(map[string]time.Time),
		undoStack:           make([]UndoEntry, 0),
		prioritySavePending: make(map[string]time.Time),
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.WindowSize()}
	if m.tabsEnabled {
		// Start watchers for all tabs
		for _, tab := range m.tabs {
			if tab.Watcher != nil {
				cmds = append(cmds, tab.Watcher.WatchCmd())
			}
		}
	} else if m.watcher != nil {
		cmds = append(cmds, m.watcher.WatchCmd())
	}
	return tea.Batch(cmds...)
}

func (m *model) switchTab(newTab int) {
	if !m.tabsEnabled || newTab < 0 || newTab >= len(m.tabs) || newTab == m.activeTab {
		return
	}

	// Save current tab state
	m.tabs[m.activeTab].Cursor = m.cursor
	m.tabs[m.activeTab].Sections = m.sections
	m.tabs[m.activeTab].Tasks = m.tasks

	// Switch to new tab
	m.activeTab = newTab
	tab := m.tabs[newTab]

	// Load new tab state
	m.sections = tab.Sections
	m.tasks = tab.Tasks
	m.cursor = tab.Cursor
	m.vaultPath = tab.Profile.VaultPath
	m.titleName = tab.Profile.Name
	m.queries = tab.Queries
	m.editorMode = tab.Profile.EditorMode
	m.cache = tab.Cache
	m.watcher = tab.Watcher
	m.debouncer = tab.Debouncer

	if tab.Profile.QueryIsFile {
		m.queryFile = tab.Profile.Query
	} else {
		m.queryFile = ""
	}

	// Rebuild task mappings for new tab
	m.taskToSection = make(map[*Task]string)
	m.taskToGroup = make(map[*Task]string)
	for _, s := range m.sections {
		for _, g := range s.Groups {
			for _, task := range g.Tasks {
				m.taskToSection[task] = s.Name
				m.taskToGroup[task] = g.Name
			}
		}
	}

	// Reset search state
	m.searching = false
	m.searchNavigating = false
	m.searchQuery = ""
	m.filteredTasks = nil

	// Ensure cursor is in bounds
	if m.cursor >= len(m.tasks) {
		if len(m.tasks) > 0 {
			m.cursor = len(m.tasks) - 1
		} else {
			m.cursor = 0
		}
	}
}

func (m model) renderTabBar() string {
	var tabs []string
	sep := tabSeparatorStyle.Render(" │ ")

	for i, tab := range m.tabs {
		name := tab.Profile.Name
		count := len(tab.Tasks)
		label := fmt.Sprintf("%s (%d)", name, count)

		if i == m.activeTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}

	return strings.Join(tabs, sep)
}

func (m model) renderHelpBar(rightInfo string) string {
	return m.renderFooterRight(rightInfo, true)
}

func (m model) renderFooterRight(rightInfo string, applyInfoStyle bool) string {
	if rightInfo == "" {
		return helpBarStyle.Width(m.windowWidth).Render("")
	}

	rightPart := rightInfo
	if applyInfoStyle {
		rightPart = helpBarInfoStyle.Render(rightInfo)
	}
	spacing := m.windowWidth - lipgloss.Width(rightPart)
	if spacing < 0 {
		spacing = 0
	}

	return helpBarStyle.Width(m.windowWidth).Render(strings.Repeat(" ", spacing) + rightPart)
}

func (m model) renderFooterSplit(left, right string) string {
	if left == "" && right == "" {
		return helpBarStyle.Width(m.windowWidth).Render("")
	}
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	spacing := m.windowWidth - leftWidth - rightWidth
	if spacing < 0 {
		spacing = 0
	}

	if right == "" {
		gap := helpBarStyle.Render(strings.Repeat(" ", spacing))
		return left + gap
	}

	gap := helpBarStyle.Render(strings.Repeat(" ", spacing))
	return left + gap + right
}

func (m model) buildViewport(lines []viewLine, cursorLineIdx int, contentHeight int) (string, int, int, int) {
	if contentHeight < minVisibleHeight {
		contentHeight = minVisibleHeight
	}

	width := m.windowWidth
	if width <= 0 {
		width = defaultWindowWidth
	}

	vp := m.viewport
	vp.Width = width
	vp.Height = contentHeight

	if len(lines) == 0 {
		vp.SetContent("")
		view := lipgloss.NewStyle().Width(width).Height(contentHeight).Render(vp.View())
		return normalizeViewHeight(view, contentHeight), 0, 0, 0
	}

	contentLines := make([]string, len(lines))
	lineHeights := make([]int, len(lines))
	totalRenderedLines := 0

	for i, line := range lines {
		contentLines[i] = line.content
		height := 1 + strings.Count(line.content, "\n")
		lineHeights[i] = height
		totalRenderedLines += height
	}

	if cursorLineIdx < 0 {
		cursorLineIdx = 0
	}
	if cursorLineIdx >= len(lines) {
		cursorLineIdx = len(lines) - 1
	}

	startLine := 0
	endLine := len(lines)
	startRow := 0

	if totalRenderedLines > contentHeight {
		startLine, endLine = calculateVisibleRange(cursorLineIdx, lineHeights, contentHeight)
		for i := 0; i < startLine; i++ {
			startRow += lineHeights[i]
		}
	}

	vp.SetContent(strings.Join(contentLines, "\n"))
	vp.YOffset = startRow

	view := lipgloss.NewStyle().Width(width).Height(contentHeight).Render(vp.View())
	return normalizeViewHeight(view, contentHeight), startLine, endLine, totalRenderedLines
}

func normalizeViewHeight(view string, height int) string {
	if height <= 0 {
		return ""
	}

	lines := strings.Split(view, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func buildFooterView(lines []string, height int) string {
	if height <= 0 {
		return ""
	}
	if len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	if height > len(lines) {
		padding := make([]string, height-len(lines))
		lines = append(padding, lines...)
	}
	return strings.Join(lines, "\n")
}

func (m *model) filterBySearch() {
	if m.searchQuery == "" {
		m.filteredTasks = nil
		return
	}

	query := strings.ToLower(m.searchQuery)
	var filtered []*Task
	seen := make(map[*Task]bool)

	for _, task := range m.tasks {
		if seen[task] {
			continue
		}

		if strings.Contains(strings.ToLower(task.Description), query) {
			filtered = append(filtered, task)
			seen[task] = true
			continue
		}

		sectionName := m.taskToSection[task]
		if strings.Contains(strings.ToLower(sectionName), query) {
			filtered = append(filtered, task)
			seen[task] = true
			continue
		}

		groupName := m.taskToGroup[task]
		if strings.Contains(strings.ToLower(groupName), query) {
			filtered = append(filtered, task)
			seen[task] = true
		}
	}

	m.filteredTasks = filtered

	m.clampCursor(len(filtered))
}

func (m *model) activeTasks() []*Task {
	if m.searching && m.searchQuery != "" {
		return m.filteredTasks
	}
	return m.tasks
}

// taskKey returns a unique key for a task based on file path and line number
func taskKey(task *Task) string {
	return fmt.Sprintf("%s:%d", task.FilePath, task.LineNumber)
}

// pushUndo adds an entry to the undo stack
func (m *model) pushUndo(entry UndoEntry) {
	entry.Timestamp = time.Now()
	m.undoStack = append(m.undoStack, entry)
	if len(m.undoStack) > maxUndoStackSize {
		m.undoStack = m.undoStack[len(m.undoStack)-maxUndoStackSize:]
	}
}

// popUndo removes and returns the most recent undo entry
func (m *model) popUndo() *UndoEntry {
	if len(m.undoStack) == 0 {
		return nil
	}
	entry := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	return &entry
}

// isRecentlyToggled checks if a task has a recent undo entry (for keeping it visible)
func (m *model) isRecentlyToggled(task *Task) bool {
	for _, entry := range m.undoStack {
		// Only consider toggle operations - delete entries have stale line numbers
		// after the file is modified, and priority changes don't affect visibility
		if entry.Type == OpToggle && entry.FilePath == task.FilePath && entry.LineNumber == task.LineNumber {
			return true
		}
	}
	return false
}

// undoLastOperation undoes the most recent operation
func (m *model) undoLastOperation() {
	entry := m.popUndo()
	if entry == nil {
		return
	}

	switch entry.Type {
	case OpToggle:
		m.undoToggle(entry)
	case OpDelete:
		m.undoDelete(entry)
	case OpPriorityChange:
		m.undoPriorityChange(entry)
	}
}

// undoToggle restores a task's previous toggle state
func (m *model) undoToggle(entry *UndoEntry) {
	for _, task := range m.tasks {
		if task.FilePath == entry.FilePath && task.LineNumber == entry.LineNumber {
			task.Toggle()
			if err := saveTask(task); err != nil {
				m.err = err
			} else {
				m.selfModifiedFiles[task.FilePath] = time.Now()
			}
			return
		}
	}
}

// undoDelete restores a deleted task line
func (m *model) undoDelete(entry *UndoEntry) {
	if err := restoreTaskLine(entry.FilePath, entry.LineNumber, entry.DeletedLine); err != nil {
		m.err = err
	} else {
		m.selfModifiedFiles[entry.FilePath] = time.Now()
	}
	m.refresh()
}

// undoPriorityChange restores a task's previous priority
func (m *model) undoPriorityChange(entry *UndoEntry) {
	for _, task := range m.tasks {
		if task.FilePath == entry.FilePath && task.LineNumber == entry.LineNumber {
			task.SetPriority(entry.PreviousPriority)
			if err := saveTask(task); err != nil {
				m.err = err
			} else {
				m.selfModifiedFiles[task.FilePath] = time.Now()
			}
			return
		}
	}
}

// filterTasksWithRecent applies query filters but keeps recently toggled tasks visible
func (m *model) filterTasksWithRecent(allTasks []*Task, query *Query) []*Task {
	return Filter(allTasks, func(task *Task) bool {
		// Date filters always apply - a task must match the date criteria
		// regardless of whether it was recently toggled
		if len(query.DateFilters) > 0 && !matchAllDateFilters(task, query.DateFilters) {
			return false
		}
		// Recently toggled tasks bypass the "not done" filter (for undo capability)
		// but must still match date filters above
		if m.isRecentlyToggled(task) {
			return true
		}
		// Apply normal "not done" filtering
		if query.NotDone && task.Done {
			return false
		}
		return true
	})
}

func (m *model) refresh() {
	m.refreshWithCache()
}

func (m *model) refreshWithCache() {
	// If we have a query file, re-parse it; otherwise reuse existing queries
	if m.queryFile != "" {
		queries, err := parseAllQueryBlocks(m.queryFile)
		if err != nil {
			m.err = err
			return
		}
		m.queries = queries
	}
	// For inline queries, m.queries is already set and doesn't change

	files, err := scanVault(m.vaultPath)

	if err != nil {
		m.err = err
		return
	}

	var allTasks []*Task

	for _, file := range files {
		// Try cache first
		if m.cache != nil {
			if tasks, ok := m.cache.Get(file); ok {
				allTasks = append(allTasks, tasks...)
				continue
			}
		}

		// Parse and cache
		tasks, err := parseFile(file)
		if err != nil {
			continue
		}

		if m.cache != nil {
			m.cache.Set(file, tasks)
		}

		allTasks = append(allTasks, tasks...)
	}

	var sections []QuerySection

	for _, query := range m.queries {
		filtered := m.filterTasksWithRecent(allTasks, query)
		groups := groupTasks(filtered, query.GroupBy, query.SortBy, m.vaultPath)

		sections = append(sections, QuerySection{
			Name:   query.Name,
			Query:  query,
			Groups: groups,
			Tasks:  filtered,
		})
	}

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

	if m.searching && m.searchQuery != "" {
		m.filterBySearch()
	}

	m.clampCursor(len(m.tasks))

	// Sync current tab state so tab bar counters are updated
	if m.tabsEnabled && m.activeTab >= 0 && m.activeTab < len(m.tabs) {
		m.tabs[m.activeTab].Sections = m.sections
		m.tabs[m.activeTab].Tasks = m.tasks
		m.tabs[m.activeTab].Cursor = m.cursor
	}
}

func (m *model) useInlineEditor() bool {
	if m.editorMode == "inline" {
		return true
	}
	if m.editorMode == "external" {
		return false
	}
	return os.Getenv("EDITOR") == ""
}

func (m *model) inputWidth() int {
	return max(minInputWidth, min(maxInputWidth, m.windowWidth-10))
}

func (m *model) clampCursor(length int) {
	m.cursor = max(0, min(m.cursor, length-1))
}

func (m *model) toggleAndSave(task *Task) {
	m.pushUndo(UndoEntry{
		Type:       OpToggle,
		FilePath:   task.FilePath,
		LineNumber: task.LineNumber,
		WasDone:    task.Done,
	})
	task.Toggle()
	if err := saveTask(task); err != nil {
		m.err = err
		m.popUndo() // Rollback on error
		return
	}
	m.selfModifiedFiles[task.FilePath] = time.Now()
}

func (m *model) schedulePrioritySave(task *Task) tea.Cmd {
	key := taskKey(task)
	at := time.Now()
	m.prioritySavePending[key] = at
	return tea.Tick(prioritySaveDebounce, func(time.Time) tea.Msg {
		return prioritySaveMsg{key: key, at: at, task: task}
	})
}

func (m *model) setPriorityDebounced(task *Task, priority int) tea.Cmd {
	if task.Priority != priority {
		m.pushUndo(UndoEntry{
			Type:             OpPriorityChange,
			FilePath:         task.FilePath,
			LineNumber:       task.LineNumber,
			PreviousPriority: task.Priority,
		})
	}
	task.SetPriority(priority)
	return m.schedulePrioritySave(task)
}

func (m *model) cyclePriorityUpDebounced(task *Task) tea.Cmd {
	m.pushUndo(UndoEntry{
		Type:             OpPriorityChange,
		FilePath:         task.FilePath,
		LineNumber:       task.LineNumber,
		PreviousPriority: task.Priority,
	})
	task.CyclePriorityUp()
	return m.schedulePrioritySave(task)
}

func (m *model) cyclePriorityDownDebounced(task *Task) tea.Cmd {
	m.pushUndo(UndoEntry{
		Type:             OpPriorityChange,
		FilePath:         task.FilePath,
		LineNumber:       task.LineNumber,
		PreviousPriority: task.Priority,
	})
	task.CyclePriorityDown()
	return m.schedulePrioritySave(task)
}

func (m *model) startEdit(task *Task) tea.Cmd {
	if m.useInlineEditor() {
		m.editing = true
		m.editingTask = task
		m.textInput = textinput.New()
		m.textInput.SetValue(task.Description)
		m.textInput.Focus()
		m.textInput.CursorEnd()
		m.textInput.CharLimit = 500
		return nil
	}
	return openInEditor(task)
}

func (m *model) startAdd(refTask *Task) tea.Cmd {
	if m.useInlineEditor() {
		m.adding = true
		m.addingRef = refTask
		m.addingInput = textinput.New()
		m.addingInput.Placeholder = "New task description..."
		m.addingInput.Focus()
		m.addingInput.CharLimit = 500
		return nil
	}
	return openNewTaskInEditor(refTask)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowHeight = msg.Height
		m.windowWidth = msg.Width
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height

	case editorFinishedMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		m.refresh()
		return m, nil

	case FileChangeMsg:
		// Skip self-triggered changes (within 500ms)
		if t, ok := m.selfModifiedFiles[msg.Path]; ok && time.Since(t) < 500*time.Millisecond {
			delete(m.selfModifiedFiles, msg.Path)
			if m.watcher != nil {
				return m, m.watcher.WatchCmd()
			}
			return m, nil
		}

		// Invalidate cache and trigger debounced refresh
		if m.cache != nil {
			m.cache.Invalidate(msg.Path)
		}
		if m.debouncer != nil {
			m.debouncer.Trigger()
		}
		if m.watcher != nil {
			return m, m.watcher.WatchCmd()
		}
		return m, nil

	case DebouncedRefreshMsg:
		m.refreshWithCache()
		return m, nil

	case prioritySaveMsg:
		latest, ok := m.prioritySavePending[msg.key]
		if !ok || !latest.Equal(msg.at) {
			return m, nil
		}
		delete(m.prioritySavePending, msg.key)
		if err := saveTask(msg.task); err != nil {
			m.err = err
		} else {
			m.selfModifiedFiles[msg.task.FilePath] = time.Now()
		}
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

		if m.editing {
			switch msg.String() {
			case "esc", "ctrl+[":
				m.editing = false
				m.editingTask = nil
				return m, nil

			case "enter":
				newValue := m.textInput.Value()
				if m.editingTask != nil && newValue != m.editingTask.Description {
					m.editingTask.Description = newValue
					m.editingTask.Modified = true
					m.editingTask.rebuildRawLine()
					if err := saveTask(m.editingTask); err != nil {
						m.err = err
					} else {
						m.selfModifiedFiles[m.editingTask.FilePath] = time.Now()
					}
				}
				m.editing = false
				m.editingTask = nil
				m.refresh()
				return m, nil

			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit

			default:
				var cmd tea.Cmd
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}

		if m.deleting {
			switch msg.String() {
			case "y", "Y", "enter", "d", "D":
				if m.deletingTask != nil {
					m.pushUndo(UndoEntry{
						Type:        OpDelete,
						FilePath:    m.deletingTask.FilePath,
						LineNumber:  m.deletingTask.LineNumber,
						DeletedLine: m.deletingTask.RawLine,
					})
					filePath := m.deletingTask.FilePath
					if err := deleteTask(m.deletingTask); err != nil {
						m.err = err
						m.popUndo() // Rollback on error
					} else {
						m.selfModifiedFiles[filePath] = time.Now()
					}
				}
				m.deleting = false
				m.deletingTask = nil
				m.refresh()
				return m, nil

			case "n", "N", "q", "esc", "ctrl+[":
				m.deleting = false
				m.deletingTask = nil
				return m, nil

			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}

		if m.adding {
			switch msg.String() {
			case "esc", "ctrl+[":
				m.adding = false
				m.addingRef = nil
				return m, nil

			case "enter":
				newValue := strings.TrimSpace(m.addingInput.Value())
				if m.addingRef != nil && newValue != "" {
					if _, err := addTask(m.addingRef, newValue); err != nil {
						m.err = err
					} else {
						m.selfModifiedFiles[m.addingRef.FilePath] = time.Now()
					}
				}
				m.adding = false
				m.addingRef = nil
				m.refresh()
				return m, nil

			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit

			default:
				var cmd tea.Cmd
				m.addingInput, cmd = m.addingInput.Update(msg)
				return m, cmd
			}
		}

		if msg.String() == "?" {
			m.aboutOpen = true
			return m, nil
		}

		if m.searching {
			if m.searchNavigating {
				switch msg.String() {
				case "esc", "ctrl+[", "/", "q":
					m.searching = false
					m.searchNavigating = false
					m.searchQuery = ""
					m.filteredTasks = nil
					m.cursor = 0
					return m, nil

				case "backspace":
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
					tasks := m.activeTasks()
					if len(tasks) > 0 && m.cursor < len(tasks) {
						m.toggleAndSave(tasks[m.cursor])
					}
					return m, nil

				case "e":
					tasks := m.activeTasks()
					if len(tasks) > 0 && m.cursor < len(tasks) {
						task := tasks[m.cursor]
						return m, m.startEdit(task)
					}
					return m, nil

				case "d":
					tasks := m.activeTasks()
					if len(tasks) > 0 && m.cursor < len(tasks) {
						m.deleting = true
						m.deletingTask = tasks[m.cursor]
					}
					return m, nil

				case "a", "n":
					tasks := m.activeTasks()
					if len(tasks) > 0 && m.cursor < len(tasks) {
						task := tasks[m.cursor]
						return m, m.startAdd(task)
					}
					return m, nil

				case "u":
					m.undoLastOperation()
					return m, nil
				}
				return m, nil
			}

			switch msg.String() {
			case "esc", "ctrl+[":
				m.searching = false
				m.searchQuery = ""
				m.filteredTasks = nil
				m.cursor = 0
				return m, nil

			case "enter":
				if len(m.filteredTasks) > 0 {
					m.searchNavigating = true
				} else if m.searchQuery == "" {
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
				if len(msg.String()) == 1 {
					m.searchQuery += msg.String()
					m.filterBySearch()
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "/":
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
				m.toggleAndSave(m.tasks[m.cursor])
			}

		case "g":
			m.cursor = 0

		case "G":
			if len(m.tasks) > 0 {
				m.cursor = len(m.tasks) - 1
			}

		case "r":
			// Clear undo stack so done tasks are hidden
			m.undoStack = make([]UndoEntry, 0)
			m.refresh()

		case "u":
			m.undoLastOperation()

		case "e":
			if len(m.tasks) > 0 {
				task := m.tasks[m.cursor]
				return m, m.startEdit(task)
			}

		case "d":
			if len(m.tasks) > 0 {
				m.deleting = true
				m.deletingTask = m.tasks[m.cursor]
			}

		case "a", "n":
			if len(m.tasks) > 0 {
				task := m.tasks[m.cursor]
				return m, m.startAdd(task)
			}

		case "+":
			if len(m.tasks) > 0 {
				task := m.tasks[m.cursor]
				return m, m.cyclePriorityUpDebounced(task)
			}

		case "-":
			if len(m.tasks) > 0 {
				task := m.tasks[m.cursor]
				return m, m.cyclePriorityDownDebounced(task)
			}

		case "!":
			if len(m.tasks) > 0 {
				task := m.tasks[m.cursor]
				return m, m.setPriorityDebounced(task, PriorityHighest)
			}

		case "0":
			if len(m.tasks) > 0 {
				task := m.tasks[m.cursor]
				return m, m.setPriorityDebounced(task, PriorityNormal)
			}

		case "tab":
			if m.tabsEnabled && len(m.tabs) > 1 {
				m.switchTab((m.activeTab + 1) % len(m.tabs))
			}

		case "shift+tab":
			if m.tabsEnabled && len(m.tabs) > 1 {
				newTab := m.activeTab - 1
				if newTab < 0 {
					newTab = len(m.tabs) - 1
				}
				m.switchTab(newTab)
			}
		}
	}

	return m, nil
}

// viewLine represents a renderable line with its associated task index
type viewLine struct {
	content   string
	taskIndex int
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err)
	}

	if m.quitting {
		return "Goodbye!\n"
	}

	if m.aboutOpen {
		sha := strings.TrimSpace(buildSHA)
		if sha == "" {
			sha = "unknown"
		}

		versionLine := fmt.Sprintf("ot v%s (%s)", strings.TrimSpace(version), sha)

		type helpItem struct {
			keys string
			desc string
		}
		type helpSection struct {
			title string
			items []helpItem
		}

		sectionsFull := []helpSection{
			{title: "Navigation", items: []helpItem{
				{keys: "↑/k", desc: "move up"},
				{keys: "↓/j", desc: "move down"},
				{keys: "g", desc: "top"},
				{keys: "G", desc: "bottom"},
			}},
			{title: "Tasks", items: []helpItem{
				{keys: "enter/space/x", desc: "toggle done"},
				{keys: "a/n", desc: "add after"},
				{keys: "e", desc: "edit"},
				{keys: "d", desc: "delete"},
				{keys: "u", desc: "undo"},
				{keys: "r", desc: "refresh"},
			}},
			{title: "Priority", items: []helpItem{
				{keys: "+", desc: "increase"},
				{keys: "-", desc: "decrease"},
				{keys: "!", desc: "highest"},
				{keys: "0", desc: "normal"},
			}},
			{title: "Search", items: []helpItem{
				{keys: "/", desc: "start search"},
				{keys: "type", desc: "filter"},
				{keys: "enter", desc: "lock results"},
				{keys: "↑/↓", desc: "move"},
				{keys: "backspace", desc: "edit query"},
				{keys: "esc", desc: "exit"},
			}},
			{title: "General", items: []helpItem{
				{keys: "?", desc: "help"},
				{keys: "q/ctrl+c", desc: "quit"},
			}},
		}

		sectionsCompact := []helpSection{
			{title: "Navigation", items: []helpItem{
				{keys: "↑/k", desc: "up"},
				{keys: "↓/j", desc: "down"},
				{keys: "g", desc: "top"},
				{keys: "G", desc: "bottom"},
			}},
			{title: "Tasks", items: []helpItem{
				{keys: "enter/space/x", desc: "toggle"},
				{keys: "a/n", desc: "add"},
				{keys: "e", desc: "edit"},
				{keys: "d", desc: "delete"},
				{keys: "u", desc: "undo"},
			}},
			{title: "Priority", items: []helpItem{
				{keys: "+", desc: "up"},
				{keys: "-", desc: "down"},
				{keys: "!", desc: "top"},
				{keys: "0", desc: "normal"},
			}},
			{title: "Search", items: []helpItem{
				{keys: "/", desc: "search"},
				{keys: "esc", desc: "exit"},
			}},
			{title: "General", items: []helpItem{
				{keys: "?", desc: "help"},
				{keys: "q/ctrl+c", desc: "quit"},
			}},
		}

		sectionsTiny := []helpSection{
			{title: "Navigation", items: []helpItem{
				{keys: "↑/k", desc: "up"},
				{keys: "↓/j", desc: "down"},
			}},
			{title: "Tasks", items: []helpItem{
				{keys: "enter/space", desc: "toggle"},
				{keys: "a", desc: "add"},
				{keys: "e", desc: "edit"},
			}},
			{title: "Search", items: []helpItem{
				{keys: "/", desc: "search"},
			}},
			{title: "General", items: []helpItem{
				{keys: "?", desc: "help"},
				{keys: "q", desc: "quit"},
			}},
		}

		if m.tabsEnabled && len(m.tabs) > 1 {
			sectionsFull = append(sectionsFull, helpSection{
				title: "Tabs",
				items: []helpItem{
					{keys: "tab", desc: "next"},
					{keys: "shift+tab", desc: "prev"},
				},
			})
			sectionsCompact = append(sectionsCompact, helpSection{
				title: "Tabs",
				items: []helpItem{
					{keys: "tab", desc: "next"},
					{keys: "shift+tab", desc: "prev"},
				},
			})
		}

		type helpRenderMode struct {
			sections       []helpSection
			showByline     bool
			showFooter     bool
			headerGapLines int
			maxCols        int
			sectionSpacing int
			itemsPerLine   int
		}

		renderHelpBody := func(width int, bodyHeight int, mode helpRenderMode) (string, bool) {
			if width <= 0 {
				width = 1
			}
			const gap = 4
			const minColWidth = 32
			maxCols := min(3, mode.maxCols)

			possibleCols := 1
			for c := maxCols; c >= 2; c-- {
				if width >= c*minColWidth+gap*(c-1) {
					possibleCols = c
					break
				}
			}

			maxKeyWidth := 0
			for _, sec := range mode.sections {
				for _, item := range sec.items {
					maxKeyWidth = max(maxKeyWidth, lipgloss.Width(item.keys))
				}
			}

			type layoutConfig struct {
				cols           int
				colWidths      []int
				itemsPerLine   int
				sectionSpacing int
			}
			type layoutResult struct {
				cfg         layoutConfig
				assignments [][]helpSection
				height      int
			}

			layout := func(cols int, itemsPerLine int, sectionSpacing int) layoutResult {
				colWidths := make([]int, cols)
				usable := width - gap*(cols-1)
				base := max(1, usable/cols)
				extra := max(0, usable%cols)
				for i := 0; i < cols; i++ {
					colWidths[i] = base
					if i < extra {
						colWidths[i]++
					}
				}

				assignments := make([][]helpSection, cols)
				colHeights := make([]int, cols)

				estimateSecHeight := func(sec helpSection) int {
					itemLines := (len(sec.items) + itemsPerLine - 1) / itemsPerLine
					return 1 + itemLines
				}

				for _, sec := range mode.sections {
					best := 0
					for i := 1; i < cols; i++ {
						if colHeights[i] < colHeights[best] {
							best = i
						}
					}
					assignments[best] = append(assignments[best], sec)
					colHeights[best] += estimateSecHeight(sec)
					if sectionSpacing > 0 {
						colHeights[best] += sectionSpacing
					}
				}

				height := 0
				for i := 0; i < cols; i++ {
					h := 0
					for sidx, sec := range assignments[i] {
						itemLines := (len(sec.items) + itemsPerLine - 1) / itemsPerLine
						h += 1 + itemLines
						if sectionSpacing > 0 && sidx != len(assignments[i])-1 {
							h += sectionSpacing
						}
					}
					height = max(height, h)
				}

				return layoutResult{
					cfg: layoutConfig{
						cols:           cols,
						colWidths:      colWidths,
						itemsPerLine:   itemsPerLine,
						sectionSpacing: sectionSpacing,
					},
					assignments: assignments,
					height:      height,
				}
			}

			candidateCols := []int{possibleCols}
			if possibleCols == 3 {
				candidateCols = []int{3, 2, 1}
			} else if possibleCols == 2 {
				candidateCols = []int{2, 1}
			}

			candidateItemsPerLine := []int{mode.itemsPerLine}
			if mode.itemsPerLine == 1 {
				candidateItemsPerLine = []int{1, 2}
			}

			candidateSectionSpacing := []int{mode.sectionSpacing}
			if mode.sectionSpacing > 0 {
				candidateSectionSpacing = []int{mode.sectionSpacing, 0}
			}

			var bestFit *layoutResult
			var bestAny *layoutResult

			betterReadable := func(a, b layoutResult) bool {
				// Prefer more spacing, fewer items per line, more columns.
				if a.cfg.sectionSpacing != b.cfg.sectionSpacing {
					return a.cfg.sectionSpacing > b.cfg.sectionSpacing
				}
				if a.cfg.itemsPerLine != b.cfg.itemsPerLine {
					return a.cfg.itemsPerLine < b.cfg.itemsPerLine
				}
				return a.cfg.cols > b.cfg.cols
			}

			for _, cols := range candidateCols {
				for _, itemsPerLine := range candidateItemsPerLine {
					if itemsPerLine == 2 && width < 60 {
						continue
					}
					for _, sectionSpacing := range candidateSectionSpacing {
						res := layout(cols, itemsPerLine, sectionSpacing)

						if bestAny == nil || res.height < bestAny.height || (res.height == bestAny.height && betterReadable(res, *bestAny)) {
							copyRes := res
							bestAny = &copyRes
						}

						if res.height <= bodyHeight {
							if bestFit == nil || betterReadable(res, *bestFit) {
								copyRes := res
								bestFit = &copyRes
							}
						}
					}
				}
			}

			chosen := bestFit
			fits := true
			if chosen == nil {
				chosen = bestAny
				fits = false
			}

			minWidth := chosen.cfg.colWidths[0]
			for _, w := range chosen.cfg.colWidths[1:] {
				minWidth = min(minWidth, w)
			}
			keyWidth := min(maxKeyWidth, max(6, minWidth-2))

			renderColumn := func(colWidth int, secs []helpSection) []string {
				pad := lipgloss.NewStyle().Width(colWidth)

				renderItem := func(keys, desc string, keyW int, descW int) string {
					k := helpDialogKeyStyle.Width(keyW).Render(keys)
					d := helpDialogDescStyle.Width(descW).Render(desc)
					return k + " " + d
				}

				var lines []string
				for si, sec := range secs {
					lines = append(lines, pad.Render(helpDialogHeaderStyle.Render(sec.title)))

					if chosen.cfg.itemsPerLine == 1 {
						descWidth := max(1, colWidth-keyWidth-1)
						for _, item := range sec.items {
							lines = append(lines, pad.Render(renderItem(item.keys, item.desc, keyWidth, descWidth)))
						}
					} else {
						const innerGap = 2
						leftW := (colWidth - innerGap) / 2
						rightW := colWidth - innerGap - leftW

						blockKeyW := min(keyWidth, max(4, leftW-2))
						leftDescW := max(1, leftW-blockKeyW-1)
						rightKeyW := min(keyWidth, max(4, rightW-2))
						rightDescW := max(1, rightW-rightKeyW-1)

						renderBlock := func(item helpItem, blockW int, kW int, dW int) string {
							return lipgloss.NewStyle().Width(blockW).Render(renderItem(item.keys, item.desc, kW, dW))
						}

						for i := 0; i < len(sec.items); i += 2 {
							left := renderBlock(sec.items[i], leftW, blockKeyW, leftDescW)
							right := lipgloss.NewStyle().Width(rightW).Render("")
							if i+1 < len(sec.items) {
								right = renderBlock(sec.items[i+1], rightW, rightKeyW, rightDescW)
							}
							lines = append(lines, pad.Render(left+strings.Repeat(" ", innerGap)+right))
						}
					}

					if chosen.cfg.sectionSpacing > 0 && si != len(secs)-1 {
						for i := 0; i < chosen.cfg.sectionSpacing; i++ {
							lines = append(lines, pad.Render(""))
						}
					}
				}
				return lines
			}

			renderedCols := make([][]string, chosen.cfg.cols)
			maxLines := 0
			for i := 0; i < chosen.cfg.cols; i++ {
				renderedCols[i] = renderColumn(chosen.cfg.colWidths[i], chosen.assignments[i])
				maxLines = max(maxLines, len(renderedCols[i]))
			}

			for i := 0; i < chosen.cfg.cols; i++ {
				pad := lipgloss.NewStyle().Width(chosen.cfg.colWidths[i])
				for len(renderedCols[i]) < maxLines {
					renderedCols[i] = append(renderedCols[i], pad.Render(""))
				}
			}

			parts := make([]string, 0, chosen.cfg.cols*2-1)
			gapStr := strings.Repeat(" ", gap)
			for i := 0; i < chosen.cfg.cols; i++ {
				if i > 0 {
					parts = append(parts, gapStr)
				}
				parts = append(parts, strings.Join(renderedCols[i], "\n"))
			}

			body := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
			return lipgloss.NewStyle().Width(width).Height(bodyHeight).Render(body), fits
		}

		renderHelpOverlay := func(width, height int, withFooter bool) string {
			width = max(1, width)
			height = max(1, height)

			centered := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)

			modes := []helpRenderMode{
				{
					sections:       sectionsFull,
					showByline:     true,
					showFooter:     withFooter,
					headerGapLines: 2,
					maxCols:        3,
					sectionSpacing: 1,
					itemsPerLine:   1,
				},
				{
					sections:       sectionsCompact,
					showByline:     false,
					showFooter:     withFooter,
					headerGapLines: 1,
					maxCols:        3,
					sectionSpacing: 0,
					itemsPerLine:   1,
				},
				{
					sections:       sectionsTiny,
					showByline:     false,
					showFooter:     withFooter,
					headerGapLines: 1,
					maxCols:        3,
					sectionSpacing: 0,
					itemsPerLine:   2,
				},
				{
					sections:       sectionsTiny,
					showByline:     false,
					showFooter:     false,
					headerGapLines: 0,
					maxCols:        2,
					sectionSpacing: 0,
					itemsPerLine:   2,
				},
			}

			for _, mode := range modes {
				headerParts := []string{aboutStyle.Render(centered.Render(versionLine))}
				if mode.showByline {
					headerParts = append(headerParts, dimTextStyle.Render(centered.Render("by elcuervo")))
				}
				if !mode.showFooter && withFooter {
					headerParts = append(headerParts, dimTextStyle.Render(centered.Render("esc/q/? to close")))
				}
				header := strings.Join(headerParts, "\n")

				footer := ""
				footerLines := 0
				if mode.showFooter && withFooter {
					footer = dimTextStyle.Render(centered.Render("esc/q/? to close"))
					footerLines = 1
				}

				reservedLines := lipgloss.Height(header) + mode.headerGapLines + footerLines
				if reservedLines >= height {
					continue
				}

				bodyHeight := max(1, height-reservedLines)
				body, fits := renderHelpBody(width, bodyHeight, mode)
				if !fits && mode.showFooter {
					continue
				}

				gap := strings.Repeat("\n", mode.headerGapLines)
				if footer != "" {
					return header + gap + body + "\n" + footer
				}
				return header + gap + body
			}

			// Fallback: absolute minimum.
			minHeader := aboutStyle.Render(centered.Render(versionLine))
			minFooter := dimTextStyle.Render(centered.Render("esc/q/? to close"))
			bodyHeight := max(1, height-lipgloss.Height(minHeader)-1)
			body, _ := renderHelpBody(width, bodyHeight, helpRenderMode{
				sections:       sectionsTiny,
				showByline:     false,
				showFooter:     false,
				headerGapLines: 0,
				maxCols:        1,
				sectionSpacing: 0,
				itemsPerLine:   2,
			})
			return minHeader + "\n" + body + "\n" + minFooter
		}

		// Prefer a full-screen boxed dialog when there's enough room; otherwise fall back
		// to a plain, scrollable viewport (still auto-layouts columns by width).
		const boxFrameW = 6 // border (2) + horizontal padding (4)
		const boxFrameH = 4 // border (2) + vertical padding (2)
		useBox := m.windowWidth >= boxFrameW+40 && m.windowHeight >= boxFrameH+12

		if useBox {
			innerW := max(1, m.windowWidth-boxFrameW)
			innerH := max(1, m.windowHeight-boxFrameH)
			content := renderHelpOverlay(innerW, innerH, true)
			content = lipgloss.NewStyle().Width(innerW).Height(innerH).Render(content)
			box := aboutBoxStyle.Render(content)
			return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, box)
		}

		vp := m.viewport
		vp.Width = max(1, m.windowWidth)
		vp.Height = max(1, m.windowHeight)
		vp.YOffset = 0
		vp.SetContent(renderHelpOverlay(vp.Width, vp.Height, true))
		return vp.View()
	}

	if m.editing && m.editingTask != nil {
		titleLine := aboutStyle.Render("Edit Task")

		checkbox := "[ ] "
		if m.editingTask.Done {
			checkbox = "[x] "
		}

		m.textInput.Width = m.inputWidth() - 6

		inputLine := checkbox + m.textInput.View()

		helpLine := "enter save • esc cancel"

		editContent := titleLine + "\n\n" + inputLine
		editHelp := helpStyle.Render(helpLine)
		box := aboutBoxStyle.Render(editContent + "\n\n" + editHelp)

		return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, box)
	}

	if m.deleting && m.deletingTask != nil {
		titleLine := dangerStyle.Render("⚠ Delete Task")

		taskPreview := renderTask(m.deletingTask.Done, m.deletingTask.Description)
		questionLine := helpStyle.Render("This action cannot be undone.")

		contentWidth := int(float64(m.windowWidth) * 0.8)
		if contentWidth < 40 {
			contentWidth = 40
		}

		centered := lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center)

		yesBtn := buttonDangerStyle.Render("y Delete")
		noBtn := buttonNeutralStyle.Render("n Cancel")

		buttons := yesBtn + "  " + noBtn

		deleteContent := centered.Render(titleLine) + "\n\n" + centered.Render(taskPreview) + "\n\n" + centered.Render(questionLine) + "\n\n" + centered.Render(buttons)

		box := dangerBoxStyle.Render(deleteContent)

		return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, box)
	}

	if m.adding && m.addingRef != nil {
		titleLine := confirmStyle.Render("+ Add Task")

		fileInfo := fileStyle.Render(fmt.Sprintf("Adding to: %s", relPath(m.vaultPath, m.addingRef.FilePath)))

		m.addingInput.Width = m.inputWidth() - 6

		inputLine := "[ ] " + m.addingInput.View()

		helpLine := "enter save • esc cancel"

		addContent := titleLine + "\n" + fileInfo + "\n\n" + inputLine
		addHelp := helpStyle.Render(helpLine)
		box := aboutBoxStyle.Render(addContent + "\n\n" + addHelp)

		return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, box)
	}

	// Build mode label if searching
	modeLabel := ""
	if m.searching {
		if m.searchNavigating {
			modeLabel = resultsModeStyle.Render("results")
		} else {
			modeLabel = searchModeStyle.Render("search")
		}
	}

	// Render header line
	titlePrefix := titleStyle.Render("ot")
	var titleLine string

	if m.tabsEnabled && len(m.tabs) > 1 {
		arrow := barColor.Render(" → ")
		titleLine = titlePrefix + arrow + m.renderTabBar()
	} else {
		arrow := barColor.Render(" → ")
		titleLine = titlePrefix + arrow + titleNameStyle.Render(m.titleName)
	}

	headerLines := []string{titleLine}

	windowHeight := m.windowHeight
	if windowHeight <= 0 {
		windowHeight = defaultWindowHeight
	}

	headerHeight := 1
	footerMinHeight := 1
	if windowHeight < headerHeight+footerMinHeight+1 {
		footerMinHeight = max(1, windowHeight-headerHeight-1)
	}

	targetContent := int(math.Round(float64(windowHeight) * 0.80))
	available := windowHeight - headerHeight - footerMinHeight
	if available < 1 {
		available = 1
	}
	contentHeight := max(targetContent, available)
	if contentHeight > windowHeight-headerHeight-footerMinHeight {
		contentHeight = windowHeight - headerHeight - footerMinHeight
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	footerHeight := windowHeight - headerHeight - contentHeight
	if footerHeight < footerMinHeight {
		footerHeight = footerMinHeight
		contentHeight = windowHeight - headerHeight - footerHeight
		if contentHeight < 1 {
			contentHeight = 1
		}
	}

	headerView := headerBarStyle.Width(m.windowWidth).Render(strings.Join(headerLines, "\n"))

	searchLine := helpBarKeyStyle.Render("/") + helpBarDescStyle.Render(" search")
	if m.searching {
		searchLabel := searchStyle.Render("/")
		searchInput := searchInputStyle.Render(m.searchQuery)
		if m.searchNavigating {
			searchLine = searchLabel + searchInput
		} else {
			cursorChar := searchStyle.Render("_")
			searchLine = searchLabel + searchInput + cursorChar
		}
	}
	if len(m.tasks) == 0 {
		lines := []viewLine{
			{content: "No tasks found.", taskIndex: -1},
		}
		viewportView, _, _, _ := m.buildViewport(lines, 0, contentHeight)
		footerLine := m.renderHelpBar("")
		if m.searching {
			footerLine = m.renderFooterSplit(searchLine, modeLabel)
		}
		footerView := buildFooterView([]string{footerLine}, footerHeight)
		return lipgloss.JoinVertical(lipgloss.Left, headerView, viewportView, footerView)
	}

	if m.searching && m.searchQuery != "" {
		tasks := m.activeTasks()

		if len(tasks) == 0 {
			lines := []viewLine{
				{content: fileStyle.Render("  No matching tasks"), taskIndex: -1},
			}
			viewportView, _, _, _ := m.buildViewport(lines, 0, contentHeight)
			footerLine := m.renderHelpBar("0 matches")
			if m.searching {
				footerLine = m.renderFooterSplit(searchLine, modeLabel)
			}
			footerView := buildFooterView([]string{footerLine}, footerHeight)
			return lipgloss.JoinVertical(lipgloss.Left, headerView, viewportView, footerView)
		}

		{
			var lines []viewLine

			query := strings.ToLower(m.searchQuery)

			for i, task := range tasks {
				cursor := " "
				if m.cursor == i {
					cursor = cursorStyle.Render(cursorCharacter)
				}

				sectionName := m.taskToSection[task]
				groupName := m.taskToGroup[task]
				descLower := strings.ToLower(task.Description)

				var matchInfo string
				if strings.Contains(descLower, query) {
					matchInfo = ""
				} else if strings.Contains(strings.ToLower(sectionName), query) {
					matchInfo = matchStyle.Render(fmt.Sprintf("→%s ", sectionName))
				} else if strings.Contains(strings.ToLower(groupName), query) {
					matchInfo = matchStyle.Render(fmt.Sprintf("→%s ", groupName))
				}

				sectionInfo := ""
				if sectionName != "" && matchInfo == "" {
					sectionInfo = countStyle.Render(fmt.Sprintf("[%s] ", sectionName))
				}
				fileInfo := fileStyle.Render(fmt.Sprintf(" (%s:%d)", relPath(m.vaultPath, task.FilePath), task.LineNumber))

				line := renderTask(task.Done, task.Description)

				if m.cursor == i {
					line = selectedStyle.Render(line)
				}

				lines = append(lines, viewLine{
					content:   fmt.Sprintf("%s%s%s%s%s", cursor, matchInfo, sectionInfo, line, fileInfo),
					taskIndex: i,
				})
			}

			viewportView, _, _, _ := m.buildViewport(lines, m.cursor, contentHeight)
			footerLine := m.renderHelpBar(fmt.Sprintf("%d matches", len(tasks)))
			if m.searching {
				footerLine = m.renderFooterSplit(searchLine, modeLabel)
			}
			footerView := buildFooterView([]string{footerLine}, footerHeight)
			return lipgloss.JoinVertical(lipgloss.Left, headerView, viewportView, footerView)
		}
	}

	{
		var lines []viewLine
		taskIndex := 0

		for _, section := range m.sections {
			if len(section.Tasks) == 0 {
				continue
			}

			if section.Name != "" {
				count := len(section.Tasks)
				countText := countStyle.Render(fmt.Sprintf(" (%d)", count))
				lines = append(lines, viewLine{
					content:   sectionStyle.Render(fmt.Sprintf("# %s", section.Name)) + countText,
					taskIndex: -1,
				})
			}

			firstGroup := true

			for _, group := range section.Groups {
				if len(group.Tasks) == 0 {
					continue
				}

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

					cursor := " "
					if m.cursor == taskIndex {
						cursor = cursorStyle.Render(cursorCharacter)
					}

					fileInfo := ""

					if section.Query.GroupBy != "filename" {
						fileInfo = fileStyle.Render(fmt.Sprintf(" (%s:%d)", relPath(m.vaultPath, task.FilePath), task.LineNumber))
					} else {
						fileInfo = fileStyle.Render(fmt.Sprintf(" (:%d)", task.LineNumber))
					}

					line := renderTask(task.Done, task.Description)

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

		cursorLineIdx := 0

		for i, line := range lines {
			if line.taskIndex == m.cursor {
				cursorLineIdx = i
				break
			}
		}

		viewportView, startLine, endLine, totalRenderedLines := m.buildViewport(lines, cursorLineIdx, contentHeight)
		var scrollInfo string
		if totalRenderedLines > contentHeight {
			scrollInfo = fmt.Sprintf("%d-%d of %d", startLine+1, endLine, len(lines))
		}
		footerLine := m.renderHelpBar(scrollInfo)
		if m.searching {
			footerLine = m.renderFooterSplit(searchLine, modeLabel)
		}
		footerView := buildFooterView([]string{footerLine}, footerHeight)
		return lipgloss.JoinVertical(lipgloss.Left, headerView, viewportView, footerView)
	}
}

// calculateVisibleRange returns start/end indices for visible lines
func calculateVisibleRange(cursorLineIdx int, lineHeights []int, visibleHeight int) (startLine, endLine int) {
	totalLines := len(lineHeights)

	if totalLines == 0 {
		return 0, 0
	}

	cursorPos := 0
	totalHeight := 0

	for i, h := range lineHeights {
		if i < cursorLineIdx {
			cursorPos += h
		}
		totalHeight += h
	}

	if totalHeight <= visibleHeight {
		return 0, totalLines
	}

	startRow := cursorPos - (visibleHeight - 1)
	if startRow < 0 {
		startRow = 0
	}

	pos := 0

	for i, h := range lineHeights {
		if pos+h > startRow {
			startLine = i
			break
		}
		pos += h
	}

	rendered := 0

	for i := startLine; i < totalLines; i++ {
		if rendered+lineHeights[i] > visibleHeight {
			break
		}

		rendered += lineHeights[i]
		endLine = i + 1
	}

	if cursorLineIdx >= endLine {
		endLine = cursorLineIdx + 1
	}

	return startLine, endLine
}
