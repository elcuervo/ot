package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

// FileChangeMsg is sent when a watched file changes
type FileChangeMsg struct {
	Path    string
	Deleted bool
}

// DebouncedRefreshMsg signals that enough time has passed to trigger a refresh
type DebouncedRefreshMsg struct{}

// Watcher wraps fsnotify to watch vault directories for changes
type Watcher struct {
	watcher   *fsnotify.Watcher
	vaultPath string
}

func (w *Watcher) addDirRecursive(root string) {
	if root == "" {
		return
	}
	if strings.HasPrefix(filepath.Base(root), ".") {
		return
	}
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") && path != root {
			return filepath.SkipDir
		}
		_ = w.watcher.Add(path)
		return nil
	})
}

// NewWatcher creates a new file watcher for the given vault path
func NewWatcher(vaultPath string) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(vaultPath)
	if err == nil && !info.IsDir() {
		// Watch the file itself and its parent directory to catch updates and renames.
		_ = w.Add(vaultPath)
		parent := filepath.Dir(vaultPath)
		if parent != "" && parent != "." {
			_ = w.Add(parent)
		}
		return &Watcher{watcher: w, vaultPath: vaultPath}, nil
	}

	watcher := &Watcher{watcher: w, vaultPath: vaultPath}
	// Walk vault and add all directories (skip hidden ones)
	watcher.addDirRecursive(vaultPath)

	return watcher, nil
}

// WatchCmd returns a BubbleTea command that listens for file changes
func (w *Watcher) WatchCmd() tea.Cmd {
	return func() tea.Msg {
		for {
			select {
			case event, ok := <-w.watcher.Events:
				if !ok {
					return nil
				}

				if event.Has(fsnotify.Create) {
					if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
						w.addDirRecursive(event.Name)
						continue
					}
				}

				// Only care about .md files
				if !strings.HasSuffix(strings.ToLower(event.Name), ".md") {
					continue
				}

				deleted := event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)
				return FileChangeMsg{Path: event.Name, Deleted: deleted}

			case _, ok := <-w.watcher.Errors:
				if !ok {
					return nil
				}
				continue
			}
		}
	}
}

// Close stops the watcher
func (w *Watcher) Close() error {
	return w.watcher.Close()
}

// Debouncer coalesces rapid file change events into a single refresh
type Debouncer struct {
	mu       sync.Mutex
	timer    *time.Timer
	duration time.Duration
	program  *tea.Program
}

// NewDebouncer creates a new debouncer with the given delay duration
func NewDebouncer(d time.Duration) *Debouncer {
	return &Debouncer{duration: d}
}

// SetProgram sets the BubbleTea program to send messages to
func (d *Debouncer) SetProgram(p *tea.Program) {
	d.program = p
}

// Trigger starts or resets the debounce timer
func (d *Debouncer) Trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil {
		d.timer.Stop()
	}

	d.timer = time.AfterFunc(d.duration, func() {
		if d.program != nil {
			d.program.Send(DebouncedRefreshMsg{})
		}
	})
}
