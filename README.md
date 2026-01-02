# ot

CLI tool to interact with [Obsidian Tasks](https://publish.obsidian.md/tasks/Introduction)

![VHS](https://vhs.charm.sh/vhs-EKUmLBaqqlEFUJtgA9ZeI.gif)

## Install

With Go:

```sh
go install .
```

Or with `just`:

```sh
just install
```

With Nix:

```sh
nix profile install .#default
```

Or install directly from GitHub:

```sh
nix profile install github:elcuervo/ot
```

## Usage

```bash
ot ~/obsidian-vault              # Show 'not done' tasks from vault
ot ~/vault -q 'due today'        # Inline query
ot ~/vault -q queries/tasks.md   # Query file
ot 'projects/*/todo.md'          # Glob pattern
ot                               # Use default profile
ot --profile work                # Use named profile
```

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `k` or arrows | Navigate up/down |
| `g` / `G` | Jump to top/bottom |
| `space` / `enter` / `x` | Toggle task |
| `u` | Undo last toggle |
| `a` / `n` | Add task after current |
| `e` | Edit task |
| `d` | Delete task |
| `/` | Search tasks |
| `r` | Refresh |
| `?` | Help |
| `q` | Quit |

## Features

### Inline Editor

Press `e` to edit a task's description in a popup.

- `Enter` to save
- `Esc` to cancel

### External Editor

Set the `editor` option in your config to `"external"` (or leave it unset with `$EDITOR` defined)

```toml
[profiles.work]
vault = "Obsidian"
query = "queries/tasks.md"
editor = "external"  # or "inline" (default if $EDITOR is not set)
```

### Search

Press `/` to search. Matches against:
- Task description
- Section name (e.g., "Due Today", "Upcoming")
- Group name (folder or filename when grouped)

## Config

Create `~/.config/ot/config.toml` (or `$XDG_CONFIG_HOME/ot/config.toml`) with profiles and a default:

```toml
default_profile = "work"

[profiles.work]
vault = "Obsidian"
query = "queries/tasks.md"
editor = "inline"  # "inline" or "external"
```

If `default_profile` is set, run `ot` without arguments to use it.

## Query File

The query file uses Obsidian Tasks syntax:

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
```
</pre>

Supported filters:
- `not done` - incomplete tasks only
- `due today`, `due tomorrow`, `due yesterday`
- `due before <date>`, `due after <date>`
- `group by folder`, `group by filename`
