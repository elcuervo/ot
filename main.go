package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

//go:embed VERSION
var version string

func main() {
	vaultPath := flag.String("vault", "", "Path to Obsidian vault")
	listOnly := flag.Bool("list", false, "List tasks without TUI (non-interactive)")
	profileName := flag.String("profile", "", "Profile name from config (optional)")
	showVersion := flag.Bool("version", false, "Show version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("ot version %s\n", strings.TrimSpace(version))
		os.Exit(0)
	}

	args := flag.Args()
	cfg, cfgPath, err := loadConfig()

	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	var resolvedVault, queryFile, titleName, editorMode string

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
		queryFile = resolved.QueryPath
		titleName = name
		editorMode = resolved.EditorMode
	}

	if *vaultPath != "" {
		expanded, err := expandPath(*vaultPath)

		if err != nil {
			fmt.Printf("Error expanding vault path: %v\n", err)
			os.Exit(1)
		}

		resolvedVault = filepath.Clean(expanded)

		if resolved, err := filepath.EvalSymlinks(resolvedVault); err == nil {
			resolvedVault = resolved
		}

		titleName = filepath.Base(resolvedVault)
	}

	if len(args) > 0 {
		expanded, err := expandPath(args[0])

		if err != nil {
			fmt.Printf("Error expanding query path: %v\n", err)
			os.Exit(1)
		}

		queryFile = filepath.Clean(expanded)
	}

	if titleName == "" && resolvedVault != "" {
		titleName = filepath.Base(resolvedVault)
	}

	if queryFile == "" || resolvedVault == "" {
		fmt.Println("Usage: ot <query-file.md> --vault <path>")
		fmt.Println("\nOptions:")
		fmt.Println("  --vault <path>  Path to Obsidian vault (required)")
		fmt.Println("  --list          List tasks without TUI")
		fmt.Println("  --profile <name>  Use profile from config")
		fmt.Println("\nSupported query filters:")
		fmt.Println("  not done              Show only incomplete tasks")
		fmt.Println("  due today             Tasks due today")
		fmt.Println("  due today or tomorrow Tasks due today or tomorrow")
		fmt.Println("  due before <date>     Tasks due before date")
		fmt.Println("  due after <date>      Tasks due after date")
		fmt.Println("  group by folder       Group tasks by folder")
		fmt.Println("  group by filename     Group tasks by filename")
		fmt.Println("\nDate values: today, tomorrow, yesterday, or YYYY-MM-DD")
		fmt.Println("\nExample:")
		fmt.Println("  ot --vault ~/obsidian-vault query.md")

		if cfgPath != "" {
			fmt.Println("\nConfig:")
			fmt.Printf("  %s\n", cfgPath)
			fmt.Println("  Define profiles with vault/query and set default_profile to skip flags.")
		}

		os.Exit(1)
	}

	queries, err := parseAllQueryBlocks(queryFile)

	if err != nil {
		fmt.Printf("Error parsing query file: %v\n", err)
		os.Exit(1)
	}

	files, err := scanVault(resolvedVault)

	if err != nil {
		fmt.Printf("Error scanning vault: %v\n", err)
		os.Exit(1)
	}

	var allTasks []*Task
	for _, file := range files {
		tasks, err := parseFile(file)
		if err != nil {
			fmt.Printf("Warning: could not parse %s: %v\n", file, err)
			continue
		}
		allTasks = append(allTasks, tasks...)
	}

	var sections []QuerySection

	totalTasks := 0

	for _, query := range queries {
		filtered := filterTasks(allTasks, query)
		groups := groupTasks(filtered, query.GroupBy, resolvedVault)

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

	p := tea.NewProgram(newModel(sections, resolvedVault, titleName, queryFile, queries, editorMode), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
