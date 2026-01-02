package main

import (
	"os"
	"sync"
	"time"
)

// CachedFile stores parsed tasks with modification time for cache validation
type CachedFile struct {
	ModTime time.Time
	Tasks   []*Task
}

// TaskCache provides thread-safe caching of parsed tasks per file
type TaskCache struct {
	mu    sync.RWMutex
	files map[string]*CachedFile
}

// NewTaskCache creates a new empty task cache
func NewTaskCache() *TaskCache {
	return &TaskCache{files: make(map[string]*CachedFile)}
}

// Get returns cached tasks if the file hasn't been modified since caching
func (c *TaskCache) Get(path string) ([]*Task, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.files[path]
	if !exists {
		return nil, false
	}

	info, err := os.Stat(path)
	if err != nil || info.ModTime().After(cached.ModTime) {
		return nil, false
	}

	return cached.Tasks, true
}

// Set stores tasks in the cache with the file's current modification time
func (c *TaskCache) Set(path string, tasks []*Task) {
	c.mu.Lock()
	defer c.mu.Unlock()

	info, err := os.Stat(path)
	if err != nil {
		return
	}

	c.files[path] = &CachedFile{ModTime: info.ModTime(), Tasks: tasks}
}

// Invalidate removes a file from the cache
func (c *TaskCache) Invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.files, path)
}
