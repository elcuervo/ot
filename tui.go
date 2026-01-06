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
)

type prioritySaveMsg struct {
	key  string
	at   time.Time
	task *Task
}

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

	// Recently toggled tasks (kept visible for undo)
	// Key is "filepath:lineNumber" to survive re-parsing
	recentlyToggled map[string]time.Time

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
		recentlyToggled:     make(map[string]time.Time),
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
			recentlyToggled:     make(map[string]time.Time),
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
		recentlyToggled:     make(map[string]time.Time),
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
	if rightInfo == "" {
		return helpBarStyle.Width(m.windowWidth).Render("")
	}

	rightPart := helpBarInfoStyle.Render(rightInfo)
	spacing := m.windowWidth - lipgloss.Width(rightPart)
	if spacing < 0 {
		spacing = 0
	}

	return helpBarStyle.Width(m.windowWidth).Render(strings.Repeat(" ", spacing) + rightPart)
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

// isRecentlyToggled checks if a task was toggled within the session
func (m *model) isRecentlyToggled(task *Task) bool {
	_, ok := m.recentlyToggled[taskKey(task)]
	return ok
}

// undoLastToggle finds the most recently toggled task and toggles it back
func (m *model) undoLastToggle() {
	if len(m.recentlyToggled) == 0 {
		return
	}

	// Find the most recent toggle
	var latestKey string
	var latestTime time.Time
	for key, t := range m.recentlyToggled {
		if latestKey == "" || t.After(latestTime) {
			latestKey = key
			latestTime = t
		}
	}

	// Find the task in current tasks list
	for _, task := range m.tasks {
		if taskKey(task) == latestKey {
			task.Toggle()
			if err := saveTask(task); err != nil {
				m.err = err
			} else {
				m.selfModifiedFiles[task.FilePath] = time.Now()
				delete(m.recentlyToggled, latestKey)
			}
			return
		}
	}

	// Task not in current view, remove from recently toggled anyway
	delete(m.recentlyToggled, latestKey)
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
	task.Toggle()
	if err := saveTask(task); err != nil {
		m.err = err
		return
	}
	m.selfModifiedFiles[task.FilePath] = time.Now()
	m.recentlyToggled[taskKey(task)] = time.Now()
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
	task.SetPriority(priority)
	return m.schedulePrioritySave(task)
}

func (m *model) cyclePriorityUpDebounced(task *Task) tea.Cmd {
	task.CyclePriorityUp()
	return m.schedulePrioritySave(task)
}

func (m *model) cyclePriorityDownDebounced(task *Task) tea.Cmd {
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
			case "y", "Y", "enter":
				if m.deletingTask != nil {
					filePath := m.deletingTask.FilePath
					if err := deleteTask(m.deletingTask); err != nil {
						m.err = err
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
					m.undoLastToggle()
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
			// Clear recently toggled tasks so done tasks are hidden
			m.recentlyToggled = make(map[string]time.Time)
			m.refresh()

		case "u":
			m.undoLastToggle()

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

		// Styles
		keyStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))

		descStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

		headerStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("99"))

		dimStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

		// Column layout
		keyWidth := 8
		descWidth := 12
		colWidth := keyWidth + descWidth + 1
		totalWidth := colWidth*2 + 4

		renderKey := func(key, desc string) string {
			k := keyStyle.Width(keyWidth).Render(key)
			d := descStyle.Width(descWidth).Render(desc)
			return k + " " + d
		}

		// Left column: Navigation + Search + Priority
		leftCol := headerStyle.Render("Navigation") + "\n"
		leftCol += renderKey("↑ k", "up") + "\n"
		leftCol += renderKey("↓ j", "down") + "\n"
		leftCol += renderKey("g", "first") + "\n"
		leftCol += renderKey("G", "last") + "\n"
		leftCol += "\n"
		leftCol += headerStyle.Render("Priority") + "\n"
		leftCol += renderKey("+", "increase") + "\n"
		leftCol += renderKey("-", "decrease") + "\n"
		leftCol += renderKey("!", "highest") + "\n"
		leftCol += renderKey("0", "reset") + "\n"

		// Right column: Actions + Search + General
		rightCol := headerStyle.Render("Actions") + "\n"
		rightCol += renderKey("space", "toggle") + "\n"
		rightCol += renderKey("u", "undo") + "\n"
		rightCol += renderKey("a n", "add") + "\n"
		rightCol += renderKey("e", "edit") + "\n"
		rightCol += renderKey("d", "delete") + "\n"
		rightCol += renderKey("r", "refresh") + "\n"
		rightCol += "\n"
		rightCol += headerStyle.Render("Search") + "\n"
		rightCol += renderKey("/", "search") + "\n"
		rightCol += renderKey("esc", "exit") + "\n"
		rightCol += "\n"
		rightCol += headerStyle.Render("General") + "\n"
		rightCol += renderKey("?", "help") + "\n"
		rightCol += renderKey("q", "quit") + "\n"
		if m.tabsEnabled && len(m.tabs) > 1 {
			rightCol += "\n"
			rightCol += headerStyle.Render("Tabs") + "\n"
			rightCol += renderKey("tab", "next") + "\n"
			rightCol += renderKey("S-tab", "prev") + "\n"
		}

		// Join columns side by side
		leftLines := strings.Split(leftCol, "\n")
		rightLines := strings.Split(rightCol, "\n")

		maxLines := len(leftLines)
		if len(rightLines) > maxLines {
			maxLines = len(rightLines)
		}

		columns := ""
		for i := 0; i < maxLines; i++ {
			left := ""
			right := ""
			if i < len(leftLines) {
				left = leftLines[i]
			}
			if i < len(rightLines) {
				right = rightLines[i]
			}
			// Pad left column
			leftVisible := lipgloss.Width(left)
			if leftVisible < colWidth {
				left += strings.Repeat(" ", colWidth-leftVisible)
			}
			columns += left + "    " + right + "\n"
		}

		// Header
		versionLine := fmt.Sprintf("ot v%s (%s)", strings.TrimSpace(version), sha)
		centered := lipgloss.NewStyle().Width(totalWidth).Align(lipgloss.Center)
		header := aboutStyle.Render(centered.Render(versionLine)) + "\n"
		header += dimStyle.Render(centered.Render("by elcuervo")) + "\n\n"

		// Footer
		footer := "\n" + dimStyle.Render(centered.Render("esc to close"))

		useColumns := m.windowWidth >= totalWidth+4 && m.windowHeight >= 12
		if useColumns {
			box := aboutBoxStyle.Render(header + columns + footer)
			return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, box)
		}

		compactKey := func(key, desc string) string {
			return keyStyle.Render(key) + " " + descStyle.Render(desc)
		}

		lines := []string{
			aboutStyle.Render(versionLine),
			dimStyle.Render("by elcuervo"),
			"",
			headerStyle.Render("Navigation"),
			compactKey("↑ k", "up"),
			compactKey("↓ j", "down"),
			compactKey("g", "first"),
			compactKey("G", "last"),
			"",
			headerStyle.Render("Priority"),
			compactKey("+", "increase"),
			compactKey("-", "decrease"),
			compactKey("!", "highest"),
			compactKey("0", "reset"),
			"",
			headerStyle.Render("Actions"),
			compactKey("space", "toggle"),
			compactKey("u", "undo"),
			compactKey("a n", "add"),
			compactKey("e", "edit"),
			compactKey("d", "delete"),
			compactKey("r", "refresh"),
			"",
			headerStyle.Render("Search"),
			compactKey("/", "search"),
			compactKey("esc", "exit"),
			"",
			headerStyle.Render("General"),
			compactKey("?", "help"),
			compactKey("q", "quit"),
		}

		if m.tabsEnabled && len(m.tabs) > 1 {
			lines = append(lines,
				"",
				headerStyle.Render("Tabs"),
				compactKey("tab", "next"),
				compactKey("S-tab", "prev"),
			)
		}

		lines = append(lines, "", dimStyle.Render("esc to close"))

		vp := m.viewport
		vp.Width = max(1, m.windowWidth)
		vp.Height = max(1, m.windowHeight)
		vp.YOffset = 0
		vp.SetContent(strings.Join(lines, "\n"))
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

		yesBtn := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(lipgloss.Color("196")).
			Padding(0, 2).
			Render("y Delete")

		noBtn := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(lipgloss.Color("240")).
			Padding(0, 2).
			Render("n Cancel")

		buttons := yesBtn + "  " + noBtn

		deleteContent := centered.Render(titleLine) + "\n\n" + centered.Render(taskPreview) + "\n\n" + centered.Render(questionLine) + "\n\n" + centered.Render(buttons)

		dangerBoxStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("196")).
			Padding(1, 2)

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

	if modeLabel != "" {
		titleLine += " " + modeLabel
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
	searchLine = barColor.Width(m.windowWidth).Render(searchLine)

	if len(m.tasks) == 0 {
		lines := []viewLine{
			{content: "No tasks found.", taskIndex: -1},
		}
		viewportView, _, _, _ := m.buildViewport(lines, 0, contentHeight)
		footerLine := m.renderHelpBar("")
		if m.searching {
			footerLine = searchLine
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
				footerLine = searchLine
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
					cursor = cursorStyle.Render(">")
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
				footerLine = searchLine
			}
			footerView := buildFooterView([]string{footerLine}, footerHeight)
			return lipgloss.JoinVertical(lipgloss.Left, headerView, viewportView, footerView)
		}
	}

	{
		var lines []viewLine
		taskIndex := 0

		for _, section := range m.sections {
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
						cursor = cursorStyle.Render(">")
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
			footerLine = searchLine
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
