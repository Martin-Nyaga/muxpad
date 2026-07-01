<p align="center">
  <img src="assets/logo.svg" alt="muxpad" width="420">
</p>

<p align="center">
    A project-aware command palette and launcher for tmux and Herdr.
</p>

# About

Muxpad puts your configured project tasks, discovered package scripts, and
coding agents in a fuzzy-searchable menu, so you can quickly launch, find, and
switch between them. It runs as a standalone tmux tool and as a plugin for
[Herdr](https://herdr.dev).

<p align="center">
  <img src="assets/screenshot.png" alt="The Muxpad palette" width="900">
</p>

I built Muxpad for my own workflow: managing dev servers, workers, databases,
test watchers, and coding agents across several repositories. tmux and Herdr
both handle these processes well, but repeatedly creating, naming, and finding
the corresponding windows and panes gets tedious. Muxpad automates that
bookkeeping without replacing the tools underneath.

## How it works

A key binding opens the palette for the current project. It lists your
configured tasks and the package scripts discovered from the root `package.json`
and workspace packages, and on the tmux path it also lists coding agents. From
there you can launch an entry, focus one that is already running, or choose where
it should open. A live sidebar lists running tasks so you can find and return to
them.

Muxpad is a thin layer over whichever multiplexer you use. On tmux it creates and
locates ordinary sessions, windows, and panes. As a Herdr plugin it opens the
palette and a project launcher as Herdr overlay panes and manages Herdr
workspaces and panes. Either way, your existing bindings and navigation keep
working unchanged.

## Requirements

- Go 1.26 or newer to build from source
- tmux 3.3 or newer, for the tmux path
- Herdr 0.7.1 or newer, for the Herdr plugin

## Install

### tmux

1. Copy the example configuration and edit it (see [Configuration](#configuration)):

   ```sh
   mkdir -p ~/.config/muxpad
   cp config.example.yml ~/.config/muxpad/config.yml
   ```

2. Build the binary and put it on `PATH`:

   ```sh
   go build -o dist/muxpad ./cmd/muxpad
   ln -sf "$PWD/dist/muxpad" ~/.local/bin/muxpad
   ```

3. Add one overridable tmux binding so `prefix + b` opens the palette (change
   `b` to any free key):

   ```tmux
   bind-key b run-shell -b 'muxpad menu'
   ```

Then:
- `muxpad start <project>` creates the project's tmux session and launches its
  default tasks
- `prefix + b` opens the palette from anywhere inside tmux
- `muxpad help` lists the available commands

### Herdr

1. Build the plugin binary:

   ```sh
   go build -o dist/muxpad-herdr ./cmd/muxpad-herdr
   ```

2. Link the plugin into Herdr, pointing it at this repo. Herdr reads
   [`herdr-plugin.toml`](herdr-plugin.toml) and registers the actions and panes:

   ```sh
   herdr plugin link /path/to/muxpad
   ```

   Re-run `herdr plugin link` after editing `herdr-plugin.toml`, and rebuild the
   binary after changing Go code, since Herdr runs `dist/muxpad-herdr` directly.

3. Bind the actions in `~/.config/herdr/config.toml` (change the keys to any free
   ones):

   ```toml
   [[keys.command]]
   key = "prefix+down"
   type = "plugin_action"
   command = "muxpad.open-palette"          # task palette
   description = "muxpad: open task palette"

   [[keys.command]]
   key = "prefix+up"
   type = "plugin_action"
   command = "muxpad.open-project-palette"  # project launcher
   description = "muxpad: open project launcher"
   ```

Then:
- `prefix + up` opens the project launcher, which fuzzy-selects a configured
  project and focuses its Herdr workspace, creating it if it does not exist yet
- `prefix + down` opens the task palette for the current workspace

## Configuration

Configuration is optional, since Muxpad can autodiscover your package scripts.
Add a config file to register projects and common tasks, and to customize
behavior.

Both paths use the same model of projects and tasks. They differ only in file
location and format:

- **tmux** reads YAML from `~/.config/muxpad/config.yml`. Set `MUXPAD_CONFIG` to
  point at a different file, which is useful for testing a config without
  touching your real one. The tmux config also has an `agents` section.
- **Herdr** reads TOML from its plugin config directory. Find it with
  `herdr plugin config-dir muxpad` (usually
  `~/.config/herdr/plugins/config/muxpad/config.toml`).

A complete YAML example lives in [`config.example.yml`](config.example.yml).

### Projects

Each entry under `projects` registers a directory Muxpad knows about. The key is
a stable identifier (letters, numbers, `-`, `_`). On tmux, `muxpad start <id>`
starts it, and running `muxpad start` from anywhere inside `root` resolves to it.
On Herdr, the project launcher lists these entries.

```yaml
projects:
  northstar:                  # project identifier
    name: northstar           # display name (defaults to the identifier)
    root: ~/code/northstar    # project root directory (required)
    default_tasks: [api, web] # tasks launched on first start
    discovery:
      exclude:
        - "@northstar/web:e2e" # glob patterns to hide from autodiscovery
    tasks:
      api:
        name: api                            # label shown in the palette
        description: API dev server          # one-line description (required)
        command: pnpm --filter api dev       # the command to run (required)
        directory: apps/api                  # optional, relative to root
        placement: window                    # optional: window | vertical | horizontal
        exit_mode: keep                      # optional: close | keep | keep-on-error
```

The same project as TOML for the Herdr config:

```toml
[projects.northstar]
name = "northstar"
root = "~/code/northstar"
default_tasks = ["api", "web"]

[projects.northstar.discovery]
exclude = ["@northstar/web:e2e"]

[projects.northstar.tasks.api]
name = "api"
description = "API dev server"
command = "pnpm --filter api dev"
directory = "apps/api"
placement = "window"
exit_mode = "keep"
```

Each **task** is a command you run often, such as a dev server, test watcher,
database tool, or editor.

**Exit modes** decide what a window or pane does when its command finishes:
- `close`: closes the pane whenever the command exits.
- `keep`: opens an interactive shell whenever the command exits, keeping the
  output in scrollback. This is useful for long-running servers because an
  unexpected exit stays visible.
- `keep-on-error`: closes on success and opens a shell on failure. This is the
  default for tasks and discovered scripts.

On the Herdr path, tasks run in the pane's interactive shell. A launched task
returns to a prompt when it exits, so `keep-on-error` behaves like `keep`, and
the command lands in shell history so you can rerun it with the up arrow.

A finished task that dropped to a shell is still tracked: it shows as `finished`
in the palette, and selecting it focuses the pane without erasing its output. On
tmux, `Ctrl-R` restarts the command in place.

### Agents

Coding agents appear in the tmux palette. They are built in so you don't need to
repeat them in every project, and they are available in any directory.

The defaults are **Claude Code**, **Codex**, and **OpenCode**, but you can add
new ones. Selecting an agent always launches a fresh instance; numbered names
(`codex`, `codex 2`, ...) keep multiple instances apart. Use the `agents` section
to override or disable one:

```yaml
agents:
  claude:
    enabled: true
  codex:
    command: codex --model gpt-5   # custom command; executable is inferred
  opencode:
    disabled: true                 # hide it from the palette
```

When Claude Code or Codex publishes a thread title to the terminal, Muxpad shows
it beneath the running instance in the sidebar. Missing titles are simply
omitted. Interactive Claude Code and Codex processes launched directly in a tmux
pane are detected when the palette opens, even when Muxpad did not launch them.

### Package-script autodiscovery

For any project (and ad hoc directories), Muxpad scans the root `package.json`
and every workspace package, detects the package manager, and offers useful
scripts automatically. Discovered scripts appear labelled `SCRIPT`, separately
from your configured `TASK` entries, and will **never launch automatically**.

## Palette actions

- **Enter** launches the highlighted entry, or focuses it if it's already
  running.
- **Tab** opens the placement chooser (new window, vertical or horizontal split).
- **Ctrl-R** restarts a retained task or script in place (tmux only).

## Tests

```sh
go test ./...
```

The original Ruby prototype is kept temporarily under `ruby/` (with its own
parity suite) while the Go port is validated.
