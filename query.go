package main

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

var (
	blockRe         = regexp.MustCompile("(?s)```tasks\\s*\\n(.+?)```")
	headerRe        = regexp.MustCompile(`(?m)^##\s+(.+)$`)
	groupByFuncRe   = regexp.MustCompile(`group by function task\.file\.(\w+)`)
	groupBySimpleRe = regexp.MustCompile(`group by (\w+)`)
	dateFilterRe    = regexp.MustCompile(`(due|scheduled|done)\s+((?:today|tomorrow|yesterday)(?:\s+or\s+(?:today|tomorrow|yesterday))*|before\s+\S+|after\s+\S+|on\s+\S+(?:\s+or\s+\S+)*)`)
	sortByRe        = regexp.MustCompile(`sort by (\w+)`)
)

// DateFilter represents a date-based filter
type DateFilter struct {
	Field    string
	Operator string
	Date     string
	Dates    []string
}

// Query represents parsed query options
type Query struct {
	Name        string
	NotDone     bool
	GroupBy     string
	DateFilters []DateFilter
	SortBy      string
}

// TaskGroup represents a group of tasks
type TaskGroup struct {
	Name  string
	Tasks []*Task
}

// QuerySection represents a section with its query and results
type QuerySection struct {
	Name   string
	Query  *Query
	Groups []TaskGroup
	Tasks  []*Task
}

// OrderedMap maintains insertion order for keys
type OrderedMap[K cmp.Ordered, V any] struct {
	data  map[K]V
	order []K
}

func NewOrderedMap[K cmp.Ordered, V any]() *OrderedMap[K, V] {
	return &OrderedMap[K, V]{
		data: make(map[K]V),
	}
}

func (m *OrderedMap[K, V]) Set(key K, value V) {
	if _, exists := m.data[key]; !exists {
		m.order = append(m.order, key)
	}
	m.data[key] = value
}

func (m *OrderedMap[K, V]) Get(key K) (V, bool) {
	v, ok := m.data[key]
	return v, ok
}

func (m *OrderedMap[K, V]) Keys() []K {
	return m.order
}

// parseQueryFile checks if the query file contains "not done" filter
func parseQueryFile(filePath string) (bool, error) {
	queries, err := parseAllQueryBlocks(filePath)

	if err != nil {
		return false, err
	}

	if len(queries) == 0 {
		return false, nil
	}

	return queries[0].NotDone, nil
}

// parseQueryFileExtended parses the first query block
func parseQueryFileExtended(filePath string) (*Query, error) {
	queries, err := parseAllQueryBlocks(filePath)

	if err != nil {
		return nil, err
	}

	if len(queries) == 0 {
		return nil, fmt.Errorf("no ```tasks block found in %s", filePath)
	}

	return queries[0], nil
}

// parseAllQueryBlocks parses all ```tasks blocks from a file
func parseAllQueryBlocks(filePath string) ([]*Query, error) {
	content, err := os.ReadFile(filePath)

	if err != nil {
		return nil, err
	}

	matches := blockRe.FindAllStringSubmatchIndex(string(content), -1)

	if matches == nil {
		return nil, fmt.Errorf("no ```tasks block found in %s", filePath)
	}

	headers := headerRe.FindAllStringSubmatchIndex(string(content), -1)

	var queries []*Query

	for _, match := range matches {
		blockStart := match[0]
		queryContent := string(content[match[2]:match[3]])

		sectionName := ""

		for _, header := range headers {
			headerEnd := header[1]
			if headerEnd < blockStart {
				sectionName = string(content[header[2]:header[3]])
			} else {
				break
			}
		}

		query := parseQueryContent(queryContent)
		query.Name = sectionName
		queries = append(queries, query)
	}

	return queries, nil
}

// parseQueryContent parses the content of a single ```tasks block
func parseQueryContent(queryContent string) *Query {
	query := &Query{}

	if strings.Contains(queryContent, "not done") {
		query.NotDone = true
	}

	dateMatches := dateFilterRe.FindAllStringSubmatch(queryContent, -1)

	for _, dm := range dateMatches {
		field := dm[1]
		operand := dm[2]

		var op, date string
		var dates []string
		switch {
		case strings.HasPrefix(operand, "before "):
			op = "before"
			date = strings.TrimSpace(strings.TrimPrefix(operand, "before "))
		case strings.HasPrefix(operand, "after "):
			op = "after"
			date = strings.TrimSpace(strings.TrimPrefix(operand, "after "))
		case strings.HasPrefix(operand, "on "):
			op = "on"
			dates = splitOrDates(strings.TrimSpace(strings.TrimPrefix(operand, "on ")))
		default:
			op = "on"
			dates = splitOrDates(operand)
		}

		if len(dates) == 1 {
			date = dates[0]
			dates = nil
		}

		query.DateFilters = append(query.DateFilters, DateFilter{
			Field:    field,
			Operator: op,
			Date:     date,
			Dates:    dates,
		})
	}

	if funcMatch := groupByFuncRe.FindStringSubmatch(queryContent); funcMatch != nil {
		query.GroupBy = funcMatch[1]
	} else if simpleMatch := groupBySimpleRe.FindStringSubmatch(queryContent); simpleMatch != nil {
		if simpleMatch[1] != "function" {
			query.GroupBy = simpleMatch[1]
		}
	}

	if sortMatch := sortByRe.FindStringSubmatch(queryContent); sortMatch != nil {
		query.SortBy = sortMatch[1]
	}

	return query
}

func splitOrDates(value string) []string {
	parts := strings.Split(value, " or ")

	var dates []string

	for _, part := range parts {
		part = strings.TrimSpace(part)

		if part != "" {
			dates = append(dates, part)
		}
	}
	return dates
}

// startOfDay returns the time truncated to midnight UTC
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// resolveDate converts relative date strings to actual dates
func resolveDate(dateStr string) time.Time {
	today := startOfDay(time.Now())

	switch dateStr {
	case "today":
		return today
	case "tomorrow":
		return today.AddDate(0, 0, 1)
	case "yesterday":
		return today.AddDate(0, 0, -1)
	default:
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			return parsed
		}
		return today
	}
}

// matchDateFilter checks if a task matches a date filter
func matchDateFilter(task *Task, filter DateFilter) bool {
	var taskDate *time.Time

	switch filter.Field {
	case "due":
		taskDate = task.DueDate
	default:
		return true
	}

	if taskDate == nil {
		return false
	}

	targetDate := resolveDate(filter.Date)
	taskDateOnly := startOfDay(*taskDate)

	if len(filter.Dates) > 0 {
		for _, date := range filter.Dates {
			target := resolveDate(date)

			switch filter.Operator {
			case "on":
				if taskDateOnly.Equal(target) {
					return true
				}
			case "before":
				if taskDateOnly.Before(target) {
					return true
				}
			case "after":
				if taskDateOnly.After(target) {
					return true
				}
			}
		}

		return false
	}

	switch filter.Operator {
	case "on":
		return taskDateOnly.Equal(targetDate)
	case "before":
		return taskDateOnly.Before(targetDate)
	case "after":
		return taskDateOnly.After(targetDate)
	default:
		return true
	}
}

// matchAllDateFilters checks if a task matches all date filters
func matchAllDateFilters(task *Task, filters []DateFilter) bool {
	for _, filter := range filters {
		if !matchDateFilter(task, filter) {
			return false
		}
	}

	return true
}

// filterTasks applies a query's filters to a task list
func filterTasks(allTasks []*Task, query *Query) []*Task {
	return Filter(allTasks, func(task *Task) bool {
		if query.NotDone && task.Done {
			return false
		}
		if len(query.DateFilters) > 0 && !matchAllDateFilters(task, query.DateFilters) {
			return false
		}
		return true
	})
}

// sortTasks sorts tasks by the specified field (stable sort preserves original order for equal elements)
func sortTasks(tasks []*Task, sortBy string) []*Task {
	if sortBy == "" {
		return tasks
	}

	// Make a copy to avoid modifying the original slice
	sorted := make([]*Task, len(tasks))
	copy(sorted, tasks)

	switch sortBy {
	case "priority":
		slices.SortStableFunc(sorted, func(a, b *Task) int {
			return cmp.Compare(a.Priority, b.Priority)
		})
	case "due":
		slices.SortStableFunc(sorted, func(a, b *Task) int {
			// Tasks without due dates go to the end
			if a.DueDate == nil && b.DueDate == nil {
				return 0
			}
			if a.DueDate == nil {
				return 1
			}
			if b.DueDate == nil {
				return -1
			}
			return a.DueDate.Compare(*b.DueDate)
		})
	}

	return sorted
}

// groupTasks groups tasks by the specified field and optionally sorts within each group
func groupTasks(tasks []*Task, groupBy string, sortBy string, vaultPath string) []TaskGroup {
	if groupBy == "" {
		return []TaskGroup{{Name: "", Tasks: sortTasks(tasks, sortBy)}}
	}

	groups := NewOrderedMap[string, []*Task]()

	for _, task := range tasks {
		var key string
		rel := relPath(vaultPath, task.FilePath)

		switch groupBy {
		case "folder":
			key = filepath.Dir(rel)
			if key == "." {
				key = "/"
			}
		case "filename":
			key = filepath.Base(task.FilePath)
		default:
			key = ""
		}

		existing, _ := groups.Get(key)
		groups.Set(key, append(existing, task))
	}

	result := make([]TaskGroup, 0, len(groups.Keys()))

	for _, name := range groups.Keys() {
		groupTasks, _ := groups.Get(name)
		// Sort within each group
		result = append(result, TaskGroup{
			Name:  name,
			Tasks: sortTasks(groupTasks, sortBy),
		})
	}

	return result
}

// relPath returns the relative path from basePath
func relPath(basePath, filePath string) string {
	if rel, err := filepath.Rel(basePath, filePath); err == nil {
		return rel
	}
	return filePath
}

// resolveQuery determines if input is a file path or inline query string
// and returns parsed queries accordingly
func resolveQuery(input string, vaultPath string) ([]*Query, error) {
	// Try to resolve as file path first
	expanded, err := expandPath(input)
	if err != nil {
		// If expansion fails, treat as inline query
		return parseInlineQuery(input)
	}

	// Determine file path (absolute or relative to vault)
	var filePath string
	if filepath.IsAbs(expanded) {
		filePath = expanded
	} else if vaultPath != "" {
		filePath = filepath.Join(vaultPath, expanded)
	} else {
		filePath = expanded
	}

	// Check if file exists and is not a directory
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		return parseAllQueryBlocks(filePath)
	}

	// Not a file - treat as inline query
	return parseInlineQuery(input)
}

// parseInlineQuery parses an inline query string like "not done" or "due today"
func parseInlineQuery(queryStr string) ([]*Query, error) {
	query := parseQueryContent(queryStr)
	return []*Query{query}, nil
}

// Filter returns elements from slice that satisfy the predicate
func Filter[T any](slice []T, predicate func(T) bool) []T {
	var result []T
	for _, v := range slice {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}
