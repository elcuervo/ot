package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

//go:embed VERSION
var version string
var buildSHA string

// containsGlob checks if a path contains glob pattern characters
func containsGlob(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

func main() {
	queryInput := flag.String("query", "", "Query file path or inline query string")
	queryInputShort := flag.String("q", "", "Query file path or inline query string (short)")
	listOnly := flag.Bool("list", false, "List tasks without TUI (non-interactive)")
	profileName := flag.String("profile", "", "Profile name from config (optional)")
	configFile := flag.String("config", "", "Path to config file (optional)")
	configFileShort := flag.String("c", "", "Path to config file (short)")
	showVersion := flag.Bool("version", false, "Show version and exit")
	initTasks := flag.Bool("init", false, "Create a tasks.md file with an empty task")

	flag.Parse()

	if *initTasks {
		if err := createTasksFile(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Created tasks.md")
		os.Exit(0)
	}

	if *showVersion {
		sha := strings.TrimSpace(buildSHA)

		if sha == "" {
			sha = "unknown"
		}

		fmt.Printf("ot version v%s (%s)\n", strings.TrimSpace(version), sha)
		os.Exit(0)
	}

	args := flag.Args()

	// Get config path from -c or --config flags
	cfgFile := *configFile
	if cfgFile == "" {
		cfgFile = *configFileShort
	}

	cfg, cfgPath, err := loadConfigFrom(cfgFile)

	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize renderer with theme from config
	if cfg.Theme != "" {
		initRenderer(cfg.Theme)
	}

	// Check for tabs mode: enabled in config, no args, no specific profile flag, not list mode
	if cfg.Tabs && len(args) == 0 && *profileName == "" && !*listOnly && len(cfg.Profiles) > 1 {
		tabs, err := loadAllProfileTabs(cfg)
		if err != nil {
			fmt.Printf("Error loading profiles: %v\n", err)
			os.Exit(1)
		}

		if len(tabs) > 0 {
			m := newModelWithTabs(tabs)
			p := tea.NewProgram(m, tea.WithAltScreen())

			// Set program for all debouncers
			for _, tab := range tabs {
				if tab.Debouncer != nil {
					tab.Debouncer.SetProgram(p)
				}
			}

			// Cleanup watchers on exit
			defer func() {
				for _, tab := range tabs {
					if tab.Watcher != nil {
						tab.Watcher.Close()
					}
				}
			}()

			if _, err := p.Run(); err != nil {
				fmt.Printf("Error running TUI: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	var resolvedVault, queryFile, titleName, editorMode string
	var queries []*Query
	var globFiles []string // Files matched by glob pattern

	// Get query from -q or --query flags
	queryStr := *queryInput
	if queryStr == "" {
		queryStr = *queryInputShort
	}

	// Check if positional arg is a glob pattern
	if len(args) > 0 && containsGlob(args[0]) {
		expanded, err := expandPath(args[0])
		if err != nil {
			fmt.Printf("Error expanding glob pattern: %v\n", err)
			os.Exit(1)
		}

		matches, err := filepath.Glob(expanded)
		if err != nil {
			fmt.Printf("Error parsing glob pattern: %v\n", err)
			os.Exit(1)
		}

		if len(matches) == 0 {
			fmt.Printf("No files match pattern: %s\n", args[0])
			os.Exit(1)
		}

		// Filter to only .md files
		for _, match := range matches {
			if strings.HasSuffix(match, ".md") {
				if info, err := os.Stat(match); err == nil && !info.IsDir() {
					globFiles = append(globFiles, match)
				}
			}
		}

		if len(globFiles) == 0 {
			fmt.Printf("No markdown files match pattern: %s\n", args[0])
			os.Exit(1)
		}

		// Use current directory as base for relative paths
		resolvedVault, _ = os.Getwd()
		titleName = args[0] // Show the pattern in title
	} else if len(args) > 0 {
		// First positional arg is vault path (not a glob)
		expanded, err := expandPath(args[0])

		if err != nil {
			fmt.Printf("Error expanding vault path: %v\n", err)
			os.Exit(1)
		}

		resolvedVault = filepath.Clean(expanded)

		if resolved, err := filepath.EvalSymlinks(resolvedVault); err == nil {
			resolvedVault = resolved
		}

		// Use absolute path for better title when using "."
		titleName = filepath.Base(resolvedVault)
		if titleName == "." {
			if abs, err := filepath.Abs(resolvedVault); err == nil {
				titleName = filepath.Base(abs)
			}
		}
	}

	// If no vault from args, try profile
	if resolvedVault == "" {
		name, profile, err := selectProfile(*profileName, cfg)

		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if profile != nil {
			resolved, err := resolveProfilePaths(name, *profile)

			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}

			resolvedVault = resolved.VaultPath
			titleName = name
			editorMode = resolved.EditorMode

			if resolved.QueryIsFile {
				queryFile = resolved.Query
			} else if resolved.Query != "" {
				queryStr = resolved.Query
			}
			// If both are empty, all tasks will be shown (no filter)
		}
	}

	// Still no vault? Show help
	if resolvedVault == "" {
		fmt.Println("Usage:")
		fmt.Println("  ot <vault-path>                Show 'not done' tasks from vault")
		fmt.Println("  ot <glob-pattern>              Show tasks from files matching pattern")
		fmt.Println("  ot <vault-path> -q <query>     Query file or inline query string")
		fmt.Println("  ot                             Use default profile from config")
		fmt.Println("  ot --profile <name>            Use named profile from config")
		fmt.Println("\nOptions:")
		fmt.Println("  -q, --query <query>   Query file path or inline query string")
		fmt.Println("  --profile <name>      Use profile from config")
		fmt.Println("  -c, --config <path>   Path to config file")
		fmt.Println("  --list                List tasks without TUI")
		fmt.Println("  --init                Create tasks.md with an empty task")
		fmt.Println("  --version             Show version")
		fmt.Println("\nSupported query filters:")
		fmt.Println("  not done              Show only incomplete tasks")
		fmt.Println("  due today             Tasks due today")
		fmt.Println("  due today or tomorrow Tasks due today or tomorrow")
		fmt.Println("  due before <date>     Tasks due before date")
		fmt.Println("  due after <date>      Tasks due after date")
		fmt.Println("  group by folder       Group tasks by folder")
		fmt.Println("  group by filename     Group tasks by filename")
		fmt.Println("  sort by priority      Sort tasks by priority")
		fmt.Println("  sort by due           Sort tasks by due date")
		fmt.Println("\nDate values: today, tomorrow, yesterday, or YYYY-MM-DD")
		fmt.Println("\nPriority emojis (highest to lowest):")
		fmt.Println("  🔺 Highest  ⏫ High  🔼 Medium  (none) Normal  🔽 Low  ⏬ Lowest")
		fmt.Println("\nTUI keybindings for priority:")
		fmt.Println("  +   Increase priority (cycle up)")
		fmt.Println("  -   Decrease priority (cycle down)")
		fmt.Println("  !   Set to highest priority")
		fmt.Println("  0   Reset to normal priority")
		fmt.Println("\nExamples:")
		fmt.Println("  ot ~/vault")
		fmt.Println("  ot ~/vault -q 'due today'")
		fmt.Println("  ot ~/vault -q queries/work.md")
		fmt.Println("  ot 'projects/*/todo.md'")

		if cfgPath != "" {
			fmt.Println("\nConfig:")
			fmt.Printf("  %s\n", cfgPath)
			fmt.Println("  Define profiles with vault/query and set default_profile to skip flags.")
		}

		os.Exit(1)
	}

	// Resolve query: from flag, from profile, or default
	if queryStr != "" {
		queries, err = resolveQuery(queryStr, resolvedVault)
		if err != nil {
			fmt.Printf("Error resolving query: %v\n", err)
			os.Exit(1)
		}
	} else if queryFile != "" {
		queries, err = parseAllQueryBlocks(queryFile)
	} else {
		// Default: show "not done" tasks sorted by priority
		queries = []*Query{{NotDone: true, SortBy: "priority"}}
	}

	if err != nil {
		fmt.Printf("Error parsing query file: %v\n", err)
		os.Exit(1)
	}

	// Get files to parse: from glob matches or vault scan
	var files []string
	var allTasks []*Task
	var cache *TaskCache

	if len(globFiles) > 0 {
		// Glob mode: parse files directly (typically small set)
		files = globFiles
		if !*listOnly {
			cache = NewTaskCache()
		}
		for _, file := range files {
			tasks, err := parseFile(file)
			if err != nil {
				fmt.Printf("Warning: could not parse %s: %v\n", file, err)
				continue
			}
			if cache != nil {
				cache.Set(file, tasks)
			}
			allTasks = append(allTasks, tasks...)
		}
	} else {
		// Vault mode: scan recursively
		useCache := !*listOnly
		var scanErr error

		if *listOnly {
			// Non-interactive mode: scan without loader TUI
			files, scanErr = scanVault(resolvedVault)
			if scanErr != nil {
				fmt.Printf("Error scanning vault: %v\n", scanErr)
				os.Exit(1)
			}
			for _, file := range files {
				tasks, err := parseFile(file)
				if err != nil {
					continue
				}
				allTasks = append(allTasks, tasks...)
			}
		} else {
			// Interactive mode: use loader for potentially large vaults
			files, allTasks, cache, scanErr = RunWithLoaderProgress(resolvedVault, useCache)
			if scanErr != nil {
				fmt.Printf("Error scanning vault: %v\n", scanErr)
				os.Exit(1)
			}
		}
	}

	var sections []QuerySection

	totalTasks := 0

	for _, query := range queries {
		filtered := filterTasks(allTasks, query)
		groups := groupTasks(filtered, query.GroupBy, query.SortBy, resolvedVault)

		sections = append(sections, QuerySection{
			Name:   query.Name,
			Query:  query,
			Groups: groups,
			Tasks:  filtered,
		})

		totalTasks += len(filtered)
	}

	if totalTasks == 0 {
		fmt.Println("No tasks found matching any query.")
		os.Exit(0)
	}

	if *listOnly {
		fmt.Printf("Found %d task(s):\n\n", totalTasks)
		for _, section := range sections {
			if section.Name != "" {
				fmt.Printf("## %s (%d)\n", section.Name, len(section.Tasks))
			}

			if len(section.Tasks) == 0 {
				fmt.Println("(no matching tasks)")
				fmt.Println()
				continue
			}

			for _, group := range section.Groups {
				if section.Query.GroupBy != "" && group.Name != "" {
					fmt.Printf("### %s\n", group.Name)
				}

				for _, task := range group.Tasks {
					checkbox := "[ ]"

					if task.Done {
						checkbox = "[x]"
					}

					fmt.Printf("%s %s (%s:%d)\n", checkbox, task.Description, relPath(resolvedVault, task.FilePath), task.LineNumber)
				}
			}
			fmt.Println()
		}

		os.Exit(0)
	}

	// Create watcher for TUI mode (not glob mode)
	var watcher *Watcher
	var debouncer *Debouncer
	if len(globFiles) == 0 {
		watcher, _ = NewWatcher(resolvedVault)
		if watcher != nil {
			debouncer = NewDebouncer(150 * time.Millisecond)
		}
	}

	m := newModel(sections, resolvedVault, titleName, queryFile, queries, editorMode, cache, watcher, debouncer)
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Set program for debouncer to send messages
	if debouncer != nil {
		debouncer.SetProgram(p)
	}

	// Cleanup watcher on exit
	defer func() {
		if watcher != nil {
			watcher.Close()
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

// loadAllProfileTabs loads all profiles as tabs for tabbed mode
func loadAllProfileTabs(cfg Config) ([]ProfileTab, error) {
	if len(cfg.Profiles) == 0 {
		return nil, nil
	}

	// Sort profile names for consistent tab order
	var names []string
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	// Put default profile first if it exists
	if cfg.DefaultProfile != "" {
		for i, name := range names {
			if name == cfg.DefaultProfile {
				names = append([]string{name}, append(names[:i], names[i+1:]...)...)
				break
			}
		}
	}

	var tabs []ProfileTab
	for _, name := range names {
		profile := cfg.Profiles[name]
		resolved, err := resolveProfilePaths(name, profile)
		if err != nil {
			fmt.Printf("Warning: skipping profile %q: %v\n", name, err)
			continue
		}

		// Scan vault
		_, allTasks, cache, scanErr := RunWithLoaderProgress(resolved.VaultPath, true)
		if scanErr != nil {
			fmt.Printf("Warning: skipping profile %q: %v\n", name, scanErr)
			continue
		}

		// Resolve queries
		var queries []*Query
		if resolved.QueryIsFile {
			queries, err = parseAllQueryBlocks(resolved.Query)
			if err != nil {
				queries = []*Query{{NotDone: true, SortBy: "priority"}}
			}
		} else if resolved.Query != "" {
			queries, err = parseInlineQuery(resolved.Query)
			if err != nil {
				queries = []*Query{{NotDone: true, SortBy: "priority"}}
			}
		} else {
			queries = []*Query{{NotDone: true, SortBy: "priority"}}
		}

		// Build sections
		var sections []QuerySection
		var tasks []*Task
		for _, query := range queries {
			filtered := filterTasks(allTasks, query)
			groups := groupTasks(filtered, query.GroupBy, query.SortBy, resolved.VaultPath)
			sections = append(sections, QuerySection{
				Name:   query.Name,
				Query:  query,
				Groups: groups,
				Tasks:  filtered,
			})
			tasks = append(tasks, filtered...)
		}

		// Create watcher
		watcher, _ := NewWatcher(resolved.VaultPath)
		var debouncer *Debouncer
		if watcher != nil {
			debouncer = NewDebouncer(150 * time.Millisecond)
		}

		tabs = append(tabs, ProfileTab{
			Profile:   resolved,
			Sections:  sections,
			Tasks:     tasks,
			Cursor:    0,
			Cache:     cache,
			Watcher:   watcher,
			Debouncer: debouncer,
			Queries:   queries,
		})
	}

	return tabs, nil
}
