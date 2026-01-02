package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	// Minimum time before showing the loading screen
	loadingDelay = 200 * time.Millisecond
)

// ScanResult holds the final scan results
type ScanResult struct {
	Files []string
	Tasks []*Task
	Cache *TaskCache
	Error error
}

// ScanProgress represents progress during vault scanning
type ScanProgress struct {
	Phase       string // "scanning" or "parsing"
	CurrentFile string
	FilesFound  int
	FilesParsed int
	TasksFound  int
}

// scanProgressMsg is sent to update loading progress
type scanProgressMsg ScanProgress

// scanCompleteMsg is sent when scanning is complete
type scanCompleteMsg struct{}

// loaderModel handles the loading screen
type loaderModel struct {
	spinner      spinner.Model
	progress     ScanProgress
	windowWidth  int
	windowHeight int
	startTime    time.Time
	showLoader   bool
}

func newLoaderModel() loaderModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return loaderModel{
		spinner:   s,
		startTime: time.Now(),
	}
}

func (m loaderModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		tea.WindowSize(),
	)
}

func (m loaderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if !m.showLoader && time.Since(m.startTime) > loadingDelay {
			m.showLoader = true
		}
		return m, cmd

	case scanProgressMsg:
		m.progress = ScanProgress(msg)
		if !m.showLoader && time.Since(m.startTime) > loadingDelay {
			m.showLoader = true
		}
		return m, nil

	case scanCompleteMsg:
		return m, tea.Quit
	}

	return m, nil
}

func (m loaderModel) View() string {
	if !m.showLoader {
		return ""
	}

	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	countStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("212"))

	b.WriteString(titleStyle.Render("ot") + " ")
	b.WriteString(m.spinner.View() + " ")

	switch m.progress.Phase {
	case "scanning":
		b.WriteString("Scanning vault...")
		if m.progress.FilesFound > 0 {
			b.WriteString(countStyle.Render(fmt.Sprintf(" %d files", m.progress.FilesFound)))
		}
	case "parsing":
		b.WriteString("Parsing files...")
		if m.progress.FilesParsed > 0 && m.progress.FilesFound > 0 {
			pct := float64(m.progress.FilesParsed) / float64(m.progress.FilesFound) * 100
			b.WriteString(countStyle.Render(fmt.Sprintf(" %d/%d", m.progress.FilesParsed, m.progress.FilesFound)))
			b.WriteString(dimStyle.Render(fmt.Sprintf(" (%.0f%%)", pct)))
		}
		if m.progress.TasksFound > 0 {
			b.WriteString(dimStyle.Render(fmt.Sprintf(" • %d tasks", m.progress.TasksFound)))
		}
	default:
		b.WriteString("Loading...")
	}

	if m.progress.CurrentFile != "" {
		file := m.progress.CurrentFile
		maxLen := m.windowWidth - 40
		if maxLen < 20 {
			maxLen = 20
		}
		if len(file) > maxLen {
			file = "..." + file[len(file)-maxLen+3:]
		}
		b.WriteString("\n" + dimStyle.Render(file))
	}

	content := b.String()
	return lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, content)
}

// RunWithLoader runs the scan with a loading screen if it takes too long
func RunWithLoader(vaultPath string, useCache bool) ([]string, []*Task, *TaskCache, error) {
	var result ScanResult
	var mu sync.Mutex
	done := make(chan struct{})

	// Start scanning in background
	go func() {
		defer close(done)

		files, err := scanVault(vaultPath)
		if err != nil {
			mu.Lock()
			result.Error = err
			mu.Unlock()
			return
		}

		mu.Lock()
		result.Files = files
		mu.Unlock()

		var cache *TaskCache
		if useCache {
			cache = NewTaskCache()
		}

		var allTasks []*Task
		for _, file := range files {
			tasks, err := parseFile(file)
			if err != nil {
				continue
			}
			if cache != nil {
				cache.Set(file, tasks)
			}
			allTasks = append(allTasks, tasks...)
		}

		mu.Lock()
		result.Tasks = allTasks
		result.Cache = cache
		mu.Unlock()
	}()

	// Wait a bit to see if scanning finishes quickly
	select {
	case <-done:
		// Fast path: scanning finished before delay
		return result.Files, result.Tasks, result.Cache, result.Error
	case <-time.After(loadingDelay):
		// Slow path: show loader
	}

	// Start the loader TUI
	m := newLoaderModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Monitor for completion and quit the TUI
	go func() {
		<-done
		p.Send(scanCompleteMsg{})
	}()

	// Run the TUI (blocks until quit)
	p.Run()

	return result.Files, result.Tasks, result.Cache, result.Error
}

// RunWithLoaderProgress runs the scan with detailed progress updates
func RunWithLoaderProgress(vaultPath string, useCache bool) ([]string, []*Task, *TaskCache, error) {
	var result ScanResult
	done := make(chan struct{})
	progress := make(chan ScanProgress, 10)

	// Start scanning in background with progress reporting
	go func() {
		defer close(done)
		defer close(progress)

		// Phase 1: Scan for files
		progress <- ScanProgress{Phase: "scanning"}

		files, err := scanVault(vaultPath)
		if err != nil {
			result.Error = err
			return
		}

		result.Files = files
		progress <- ScanProgress{Phase: "scanning", FilesFound: len(files)}

		// Phase 2: Parse files
		var cache *TaskCache
		if useCache {
			cache = NewTaskCache()
		}

		var allTasks []*Task
		for i, file := range files {
			select {
			case progress <- ScanProgress{
				Phase:       "parsing",
				FilesFound:  len(files),
				FilesParsed: i,
				TasksFound:  len(allTasks),
				CurrentFile: file,
			}:
			default:
				// Don't block if channel is full
			}

			tasks, err := parseFile(file)
			if err != nil {
				continue
			}
			if cache != nil {
				cache.Set(file, tasks)
			}
			allTasks = append(allTasks, tasks...)
		}

		result.Tasks = allTasks
		result.Cache = cache
	}()

	// Wait a bit to see if scanning finishes quickly
	select {
	case <-done:
		return result.Files, result.Tasks, result.Cache, result.Error
	case <-time.After(loadingDelay):
		// Continue to show loader
	}

	// Start the loader TUI
	m := newLoaderModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Forward progress to TUI
	go func() {
		for prog := range progress {
			p.Send(scanProgressMsg(prog))
		}
	}()

	// Monitor for completion
	go func() {
		<-done
		p.Send(scanCompleteMsg{})
	}()

	p.Run()

	return result.Files, result.Tasks, result.Cache, result.Error
}
