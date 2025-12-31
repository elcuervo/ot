package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultWindowHeight = 24
	defaultWindowWidth  = 80
	reservedUILines     = 5
	minVisibleHeight    = 3
)

// model is the BubbleTea model
type model struct {
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
}

func newModel(sections []QuerySection, vaultPath string, titleName string, queryFile string, queries []*Query, editorMode string) model {
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
		sections:      sections,
		tasks:         tasks,
		vaultPath:     vaultPath,
		titleName:     titleName,
		queryFile:     queryFile,
		queries:       queries,
		windowHeight:  defaultWindowHeight,
		windowWidth:   defaultWindowWidth,
		taskToSection: taskToSection,
		taskToGroup:   taskToGroup,
		editorMode:    editorMode,
	}
}

func (m model) Init() tea.Cmd {
	return tea.WindowSize()
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

	if m.cursor >= len(filtered) {
		m.cursor = len(filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *model) activeTasks() []*Task {
	if m.searching && m.searchQuery != "" {
		return m.filteredTasks
	}
	return m.tasks
}

func (m *model) refresh() {
	queries, err := parseAllQueryBlocks(m.queryFile)
	if err != nil {
		m.err = err
		return
	}

	m.queries = queries

	files, err := scanVault(m.vaultPath)

	if err != nil {
		m.err = err
		return
	}

	var allTasks []*Task

	for _, file := range files {
		tasks, err := parseFile(file)
		if err != nil {
			continue
		}

		allTasks = append(allTasks, tasks...)
	}

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

	if m.cursor >= len(m.tasks) {
		m.cursor = len(m.tasks) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *model) startEdit(task *Task) tea.Cmd {
	useInline := m.editorMode == "inline"

	if !useInline && m.editorMode != "external" {
		if os.Getenv("EDITOR") == "" {
			useInline = true
		}
	}

	if useInline {
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowHeight = msg.Height
		m.windowWidth = msg.Width

	case editorFinishedMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		m.refresh()
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
			case "y", "Y":
				if m.deletingTask != nil {
					if err := deleteTask(m.deletingTask); err != nil {
						m.err = err
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
						task := tasks[m.cursor]
						task.Toggle()
						if err := saveTask(task); err != nil {
							m.err = err
						}
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

				case "a":
					tasks := m.activeTasks()
					if len(tasks) > 0 && m.cursor < len(tasks) {
						m.adding = true
						m.addingRef = tasks[m.cursor]
						m.addingInput = textinput.New()
						m.addingInput.Placeholder = "New task description..."
						m.addingInput.Focus()
						m.addingInput.CharLimit = 500
					}
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
				task := m.tasks[m.cursor]
				task.Toggle()
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

		case "a":
			if len(m.tasks) > 0 {
				m.adding = true
				m.addingRef = m.tasks[m.cursor]
				m.addingInput = textinput.New()
				m.addingInput.Placeholder = "New task description..."
				m.addingInput.Focus()
				m.addingInput.CharLimit = 500
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

	var b strings.Builder

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

		// Left column: Navigation + Search
		leftCol := headerStyle.Render("Navigation") + "\n"
		leftCol += renderKey("↑ k", "up") + "\n"
		leftCol += renderKey("↓ j", "down") + "\n"
		leftCol += renderKey("g", "first") + "\n"
		leftCol += renderKey("G", "last") + "\n"
		leftCol += "\n"
		leftCol += headerStyle.Render("Search") + "\n"
		leftCol += renderKey("/", "search") + "\n"
		leftCol += renderKey("esc", "exit") + "\n"

		// Right column: Actions + General
		rightCol := headerStyle.Render("Actions") + "\n"
		rightCol += renderKey("space", "toggle") + "\n"
		rightCol += renderKey("a", "add") + "\n"
		rightCol += renderKey("e", "edit") + "\n"
		rightCol += renderKey("d", "delete") + "\n"
		rightCol += renderKey("r", "refresh") + "\n"
		rightCol += "\n"
		rightCol += headerStyle.Render("General") + "\n"
		rightCol += renderKey("?", "help") + "\n"
		rightCol += renderKey("q", "quit") + "\n"

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
		centered := lipgloss.NewStyle().Width(totalWidth).Align(lipgloss.Center)
		versionLine := fmt.Sprintf("ot v%s (%s)", strings.TrimSpace(version), sha)
		header := aboutStyle.Render(centered.Render(versionLine)) + "\n"
		header += dimStyle.Render(centered.Render("by elcuervo")) + "\n\n"

		// Footer
		footer := "\n" + dimStyle.Render(centered.Render("esc to close"))

		box := aboutBoxStyle.Render(header + columns + footer)

		return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, box)
	}

	if m.editing && m.editingTask != nil {
		titleLine := aboutStyle.Render("Edit Task")

		checkbox := "[ ] "
		if m.editingTask.Done {
			checkbox = "[x] "
		}

		maxWidth := m.windowWidth - 10
		if maxWidth > 70 {
			maxWidth = 70
		}
		if maxWidth < 30 {
			maxWidth = 30
		}

		m.textInput.Width = maxWidth - 6

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

		maxWidth := m.windowWidth - 10
		if maxWidth > 70 {
			maxWidth = 70
		}
		if maxWidth < 30 {
			maxWidth = 30
		}

		m.addingInput.Width = maxWidth - 6

		inputLine := "[ ] " + m.addingInput.View()

		helpLine := "enter save • esc cancel"

		addContent := titleLine + "\n" + fileInfo + "\n\n" + inputLine
		addHelp := helpStyle.Render(helpLine)
		box := aboutBoxStyle.Render(addContent + "\n\n" + addHelp)

		return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, box)
	}

	titlePrefix := titleStyle.Render("ot → ")
	titleName := titleNameStyle.Render(m.titleName)
	modeLabel := ""

	if m.searching {
		if m.searchNavigating {
			modeLabel = resultsModeStyle.Render("results")
		} else {
			modeLabel = searchModeStyle.Render("search")
		}
	}

	titleLine := titlePrefix + titleName
	if modeLabel != "" {
		titleLine += " " + modeLabel
	}
	b.WriteString(titleLine + "\n")

	if m.searching {
		searchLabel := searchStyle.Render("/")
		searchInput := searchInputStyle.Render(m.searchQuery)
		if m.searchNavigating {
			b.WriteString(searchLabel + searchInput + "\n")
		} else {
			cursorChar := searchStyle.Render("_")
			b.WriteString(searchLabel + searchInput + cursorChar + "\n")
		}
	} else {
		b.WriteString("\n")
	}

	if len(m.tasks) == 0 {
		b.WriteString("\nNo tasks found.\n")
	} else if m.searching && m.searchQuery != "" {
		tasks := m.activeTasks()

		if len(tasks) == 0 {
			b.WriteString(fileStyle.Render("  No matching tasks\n"))
		} else {
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

			visibleHeight := m.windowHeight - reservedUILines - 1
			if visibleHeight < minVisibleHeight {
				visibleHeight = minVisibleHeight
			}

			lineHeights := make([]int, len(lines))
			totalRenderedLines := 0
			for i, line := range lines {
				height := 1 + strings.Count(line.content, "\n")
				lineHeights[i] = height
				totalRenderedLines += height
			}

			startLine, endLine := calculateVisibleRange(m.cursor, lineHeights, visibleHeight)

			for i := startLine; i < endLine; i++ {
				b.WriteString(lines[i].content + "\n")
			}

			helpText := "? help"
			matchInfo := fmt.Sprintf("[%d matches]", len(tasks))
			padding := m.windowWidth - len(helpText) - len(matchInfo) - 1
			if padding < 2 {
				padding = 2
			}
			helpText = helpText + strings.Repeat(" ", padding) + matchInfo
			help := helpStyle.Render(helpText)
			b.WriteString("\n" + help)
		}
	} else {
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

		visibleHeight := m.windowHeight - reservedUILines

		if visibleHeight < minVisibleHeight {
			visibleHeight = minVisibleHeight
		}

		lineHeights := make([]int, len(lines))
		totalRenderedLines := 0

		for i, line := range lines {
			height := 1 + strings.Count(line.content, "\n")
			lineHeights[i] = height
			totalRenderedLines += height
		}

		cursorLineIdx := 0

		for i, line := range lines {
			if line.taskIndex == m.cursor {
				cursorLineIdx = i
				break
			}
		}

		startLine, endLine := calculateVisibleRange(cursorLineIdx, lineHeights, visibleHeight)

		for i := startLine; i < endLine; i++ {
			b.WriteString(lines[i].content + "\n")
		}

		helpText := "? help"

		if totalRenderedLines > visibleHeight {
			scrollInfo := fmt.Sprintf("[%d-%d of %d]", startLine+1, endLine, len(lines))
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
		help := helpStyle.Render("? help")
		b.WriteString("\n" + help)
	}

	return b.String()
}

// calculateVisibleRange returns start/end indices for visible lines
func calculateVisibleRange(cursorLineIdx int, lineHeights []int, visibleHeight int) (startLine, endLine int) {
	totalLines := len(lineHeights)

	if totalLines == 0 {
		return 0, 0
	}

	totalHeight := 0
	cursorPos := 0

	for i, h := range lineHeights {
		if i < cursorLineIdx {
			cursorPos += h
		}
		totalHeight += h
	}

	if totalHeight <= visibleHeight {
		return 0, totalLines
	}

	targetStart := cursorPos - visibleHeight/2

	if targetStart < 0 {
		targetStart = 0
	}

	pos := 0

	for i, h := range lineHeights {
		if pos >= targetStart {
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

	rendered = 0

	for i := startLine; i < totalLines; i++ {
		rendered += lineHeights[i]
	}

	for startLine > 0 && rendered < visibleHeight {
		startLine--
		rendered += lineHeights[startLine]
	}

	rendered = 0
	endLine = startLine

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
