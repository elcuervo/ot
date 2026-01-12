# ot

_organized tasks_

![VHS](https://vhs.charm.sh/vhs-5LcSkVmSo0Y0UCt1Td8GM.gif)

## Install

```bash
go install .                          # Go
just install                          # Just
nix profile install .#default         # Nix (local)
nix profile install github:elcuervo/ot # Nix (remote)
```

## Usage

```bash
ot ~/vault                       # Show 'not done' tasks from vault
ot ~/vault -q 'due today'        # Inline query
ot ~/vault -q queries/tasks.md   # Query file
ot 'projects/*/todo.md'          # Glob pattern
ot                               # Use default profile
ot --profile work                # Use named profile
ot --tabs                        # Multi-profile tabbed mode
ot --list                        # Plain text output (no TUI)
ot --init                        # Create tasks.md in current dir
```

## Keybindings

| Key | Action |
|-----|--------|
| `j`/`k` or arrows | Navigate up/down |
| `g`/`G` | Jump to top/bottom |
| `space`/`enter`/`x` | Toggle task |
| `u` | Undo last toggle |
| `a`/`n` | Add task after current |
| `e` | Edit task |
| `d` | Delete task |
| `/` | Search tasks |
| `r` | Refresh |
| `+`/`-` | Increase/decrease priority |
| `!` | Set highest priority |
| `0` | Reset to normal priority |
| `Tab`/`Shift+Tab` | Switch tabs (tabbed mode) |
| `?` | Help |
| `q` | Quit |

## Features

- **Inline/External Editor**: Press `e` to edit. Use `editor = "external"` in config for `$EDITOR`
- **Search**: `/` to search across task description, section, and group names
- **File Watching**: Auto-refresh on file changes with debouncing
- **Tabbed Mode**: Multiple profiles as tabs with `--tabs` or `tabs = true` in config
- **Theming**: Configurable via `theme` option (uses Glamour themes)

### Priority

Stored as emojis: `🔺` Highest, `⏫` High, `🔼` Medium, (none) Normal, `🔽` Low, `⏬` Lowest

Use `+`/`-` to cycle, `!` for highest, `0` to reset.

### Task Metadata

- **Due date**: `📅 YYYY-MM-DD`
- **Scheduled date**: `⏳ YYYY-MM-DD`
- **Completion**: Auto-appends `✅ YYYY-MM-DD` when toggled done

## Config

Create `~/.config/ot/config.toml`:

```toml
default_profile = "work"
tabs = true                    # Enable tabbed interface
theme = "dracula"              # Glamour theme

[profiles.work]
vault = "Obsidian"
query = "queries/tasks.md"
editor = "inline"              # "inline" or "external"

[profiles.personal]
vault = "~/notes"
query = "not done"
```

## Query Syntax

Uses [Obsidian Tasks](https://publish.obsidian.md/tasks/Introduction) syntax in markdown code blocks:

<pre>
## Due Today

```tasks
not done
due today
```

## Upcoming

```tasks
not done
due after today
group by folder
sort by priority
```
</pre>

### Supported Filters

| Filter | Description |
|--------|-------------|
| `not done` | Incomplete tasks only |
| `due today/tomorrow/yesterday` | Relative date filters |
| `due before/after/on <date>` | Date comparisons (YYYY-MM-DD) |
| `scheduled today/tomorrow/yesterday` | Relative date filters |
| `scheduled before/after/on <date>` | Date comparisons (YYYY-MM-DD) |
| `done today/tomorrow/yesterday` | Relative date filters |
| `done before/after/on <date>` | Date comparisons (YYYY-MM-DD) |
| `path includes <text>` | Include tasks whose file path matches text |
| `path does not include <text>` | Exclude tasks whose file path matches text |
| `group by folder/filename` | Group tasks |
| `sort by priority/due` | Sort tasks |
