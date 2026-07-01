# Muxpad on Herdr

Muxpad runs as a plugin for [Herdr](https://herdr.dev), exposing a task palette
and a project launcher as Herdr overlay panes. It manages Herdr workspaces and
panes, so your existing bindings and navigation keep working unchanged.

## Install

1. Build the plugin binary:

   ```sh
   go build -o dist/muxpad-herdr ./cmd/muxpad-herdr
   ```

2. Link the plugin into Herdr, pointing it at this repo. Herdr reads
   [`herdr-plugin.toml`](../herdr-plugin.toml) and registers the actions and
   panes:

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
Add a config file to register projects and common tasks.

Muxpad reads a single TOML file from the plugin config directory. Find it with
`herdr plugin config-dir muxpad` (usually
`~/.config/herdr/plugins/config/muxpad/config.toml`).

### Projects

Each entry under `projects` registers a directory Muxpad knows about. The key is
a stable identifier (letters, numbers, `-`, `_`). The project launcher lists
these entries.

```toml
[projects.northstar]
name = "northstar"                 # display name (defaults to the identifier)
root = "~/code/northstar"          # project root directory (required)
default_tasks = ["api", "web"]     # tasks launched on first start

[projects.northstar.discovery]
exclude = ["@northstar/web:e2e"]   # glob patterns to hide from autodiscovery

[projects.northstar.tasks.api]
name = "api"                       # label shown in the palette
description = "API dev server"     # one-line description (required)
command = "pnpm --filter api dev"  # the command to run (required)
directory = "apps/api"             # optional, relative to root
placement = "window"               # optional: window | vertical | horizontal
exit_mode = "keep"                 # optional: close | keep | keep-on-error
```

Each **task** is a command you run often, such as a dev server, test watcher,
database tool, or editor. `placement` of `window` opens a new Herdr tab; the
split values open a pane beside or below the current one.

**Exit modes** decide what a pane does when its command finishes:
- `close`: closes the pane whenever the command exits.
- `keep`: returns to the pane's shell whenever the command exits, keeping the
  output in scrollback.
- `keep-on-error`: the default for tasks and discovered scripts.

Because Herdr panes are interactive shells, Muxpad launches a task as a plain
command in that shell. It returns to a prompt when it exits, so `keep` and
`keep-on-error` behave the same, and the command lands in shell history so you
can rerun it with the up arrow. A finished task is still tracked: it shows as
`finished` in the palette, and selecting it focuses the pane without erasing its
output.

### Package-script autodiscovery

For any project, Muxpad scans the root `package.json` and every workspace
package, detects the package manager, and offers useful scripts automatically.
Discovered scripts appear labelled `SCRIPT`, separately from your configured
`TASK` entries, and will **never launch automatically**.

## Palette actions

- **Enter** launches the highlighted entry, or focuses it if it's already
  running.
- **Tab** opens the placement chooser (new tab, vertical or horizontal split).

## Agents

Muxpad's agent integration is deliberately thin on the Herdr path. Herdr has its
own, more capable workflow for launching and tracking coding agents, so Muxpad
leaves that to Herdr rather than duplicating it. The built-in agents that appear
in the tmux palette are not listed here. Use Herdr's own agent features instead.
