<p align="center">
  <img src="assets/logo.svg" alt="muxpad" width="420">
</p>

<p align="center">
    A project-aware command palette and launcher for tmux.
</p>

# About

Muxpad puts your configured project tasks, discovered package scripts, and
coding agents in a fuzzy-searchable menu within tmux, so you can quickly launch,
find, and switch between them.

<p align="center">
  <img src="assets/screenshot.png" alt="The Muxpad palette" width="900">
</p>

## How it works

Muxpad is deliberately a thin layer over tmux. It creates and locates ordinary
tmux sessions, windows, and panes, so existing bindings, navigation, and session
management continue to work unchanged.

A key binding opens the palette for the current project, listing configured
tasks, coding agents, and package scripts discovered from the root `package.json`
and workspace packages. From there, you can launch an entry, focus one that is
already running, or choose where it should open. A live sidebar lists running
tasks and agents so you can find and return to them.

I built Muxpad for my own workflow: managing dev servers, workers, databases,
test watchers, and coding agents across multiple repositories. tmux handles
these processes well, but in more complex projects, repeatedly creating, naming,
and finding the corresponding windows and panes becomes tedious. Muxpad
automates that bookkeeping without replacing tmux or the shell tools I already
use.

## Requirements

- Ruby 3.2 or newer
- tmux 3.3 or newer

## Setup

1. Copy the example configuration and edit it (see [Configuration](#configuration)):

   ```sh
   mkdir -p ~/.config/muxpad
   cp config.example.yml ~/.config/muxpad/config.yml
   ```

2. Put this repository's `bin` directory on `PATH`, or link `muxpad` into a
   directory already on `PATH`:

   ```sh
   ln -s "$PWD/bin/muxpad" ~/.local/bin/muxpad
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
- Run `muxpad help` for the available commands.

## Configuration

Since muxpad can autodiscover your package scripts, configuration is not
required. However, you can add common tasks and customize Muxpad's behavior with
a config file.

Muxpad reads a single YAML file at `~/.config/muxpad/config.yml`. You can also set
`MUXPAD_CONFIG` to point at a different file, which is useful for testing a config
without changing your real one.

The config file has two top-level sections: `projects` and `agents`. A complete
example lives in [`config.example.yml`](config.example.yml).

### Projects

Each entry under `projects` registers a directory Muxpad knows about. The key is
a stable identifier (letters, numbers, `-`, `_`); `muxpad start <id>` starts it,
and running `muxpad start` from anywhere inside `root` resolves to it.

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

Each **task** is a command you run often, such as a dev server, test watcher,
database tool, or editor.

**Exit modes** decide what a window or pane does when its command finishes:
- `close`: closes the pane whenever the command exits.
- `keep`: opens an interactive shell whenever the command exits, keeping the
  output in scrollback. This is useful for long-running servers because an
  unexpected exit stays visible.
- `keep-on-error`: closes on success and opens a shell on failure. This is the
  default for tasks and discovered scripts.

A finished task that dropped to a shell is still tracked: it shows as `finished`
in the palette, selecting it focuses the pane without erasing its output, and
`Ctrl-R` restarts the command in place.

### Agents

Coding agents are built in so you don't need to repeat them in every project.
They are automatically available in any directory.

The defaults are **Claude Code**, **Codex**, and **OpenCode**, but you can add
new ones.

Selecting an agent always launches a fresh instance; numbered names (`codex`,
`codex 2`, …) keep multiple instances apart. Use the `agents` section to
override or disable one:

When Claude Code or Codex publishes a thread title to the terminal, Muxpad shows
it beneath the running instance in the sidebar. Missing titles are simply
omitted. Interactive Claude Code and Codex processes launched directly in a
tmux pane are detected when the palette opens, even when Muxpad did not launch
them.

```yaml
agents:
  claude:
    enabled: true
  codex:
    command: codex --model gpt-5   # custom command; executable is inferred
  opencode:
    disabled: true                 # hide it from the palette
```

### Package-script autodiscovery

For any project (and ad hoc directories), Muxpad scans the root `package.json`
and every workspace package, detects the package manager, and offers useful
scripts automatically. Discovered scripts appear labelled `SCRIPT`, separately
from your configured `TASK` entries, and will **never launch automatically**.

## Palette actions

- **Enter** launches the highlighted entry in a new window, or focuses it if
  it's already running.
- **Tab** opens the placement chooser (new window, vertical or horizontal split,
  restart where applicable).
- **Ctrl-R** restarts a retained task or script in place.

## Tests

```sh
ruby -w -Ilib:test -e 'Dir["test/**/*_test.rb"].sort.each { |file| require_relative file }'
```
