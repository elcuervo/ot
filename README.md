# ot

CLI tool to interact with [Obsidian Tasks](https://publish.obsidian.md/tasks/Introduction)

![VHS](https://vhs.charm.sh/vhs-1JW2nYb2gJGLEBhKfLI05b.gif)

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

## Config

Create `~/.config/ot/config.toml` (or `$XDG_CONFIG_HOME/ot/config.toml`) with profiles and a default:

```toml
default_profile = "work"

[profiles.work]
vault = "Obsidian"
query = "queries/tasks.md"
```

Usage with profiles:

```sh
ot --profile work
```

If `default_profile` is set, you can run `ot` without arguments.
