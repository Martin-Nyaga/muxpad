# Muxpad MVP

## Purpose

The Muxpad MVP proves that a small convenience layer on top of tmux can make
project tasks and coding agents easy to launch, identify, revisit, and arrange
without owning tmux's session model.

The prototype is deliberately disposable. It validates the product interaction
before Muxpad is rebuilt as a single cross-platform binary.

## Shared language

**Project**
: A registered directory with a stable identifier, display name, tasks, and a
  list of tasks to launch when its tmux session is first created.

**Session**
: An ordinary tmux session. Tmux is the source of truth for its live windows,
  panes, and processes. A registered project's session uses the project
  identifier as its stable identity.

**Task**
: A project-specific command such as an application server, test runner,
  database tool, or editor. Tasks are manually configured in the MVP. By
  default, a task has at most one Muxpad-managed instance in a session.

**Discovered script**
: A script found in a root or workspace `package.json`. It is labelled `SCRIPT`
  and has at most one Muxpad-managed instance in a session. Discovery never
  launches a command automatically.

**Agent**
: An interactive coding-agent CLI available across projects and unregistered
  directories. The MVP includes Claude Code, Codex, and OpenCode. Launching an
  agent creates a new instance by default.

**Agent instance**
: One running copy of an agent in a tmux window or pane. Multiple instances use
  numbered names such as `Codex`, `Codex 2`, and `Codex 3`.

**Palette**
: The unified, searchable list of tasks, agents, and running agent instances.
  Labels make each kind visually unambiguous.

**Default tasks**
: The single configurable list of tasks launched when a project's session is
  first created. `--empty` suppresses these tasks for a particular start.

**Placement**
: Where a task, discovered script, or agent is launched in tmux: a new window,
  a vertical split, or a horizontal split. The default is a new window.

**Exit mode**
: What happens to a Muxpad-created window or pane when its command exits:
  `close`, `keep`, or `keep-on-error`.

## Audience And Environment

The MVP is for one user in the current WSL environment. It may depend on the
installed tmux command. Ruby and standard-library YAML are acceptable
because the prototype will be discarded.

The eventual product targets Unix-like systems, initially Linux, macOS, and
environments such as WSL where tmux runs. Native Windows terminal support is not
a goal.

## Configuration

The prototype reads one file at:

```text
~/.config/muxpad/config.yml
```

The file contains multiple registered projects and personal overrides for
built-in agents. Each project can define:

- Identifier and display name
- Root directory
- Tasks
- Default tasks

Each task has:

- A command-line identifier, such as `api`
- A display name, such as `api`
- A manually written description
- A command
- An optional working directory relative to the project root
- Optional placement and exit-mode overrides

The task name and description must be explicit enough to make the palette and
tmux session legible. Package scripts are discovered separately and may be
filtered with project-level `discovery.exclude` glob patterns.

Agents are baked into Muxpad rather than repeated in every project. The initial
definitions are:

- Claude Code
- Codex
- OpenCode

A user can override or disable these definitions. An unavailable agent remains
visible in the palette, is clearly marked unavailable, and explains which
executable is missing instead of attempting a launch.

### Package Script Discovery

Muxpad scans the root `package.json` and workspace packages for registered
projects and ad hoc sessions. It detects the package manager from the root
`packageManager` field, then common lockfiles, falling back to npm.

Discovered entries:

- Are labelled `SCRIPT`, separately from configured `TASK` entries
- Use the script name at the root and `<package-name>:<script>` in workspaces
- Run from the package directory through the detected package manager
- Have at most one Muxpad-managed instance in a session
- Are rescanned whenever the palette opens and are never cached persistently
- Never launch automatically or become default tasks

Muxpad omits package lifecycle hooks and `pre`/`post` hooks associated with
another script. A project may add `discovery.exclude` glob patterns matched
against displayed script identifiers. If a configured task and discovered
script have the same resolved working directory and identical command string,
only the configured task is shown. Muxpad does not otherwise guess that two
commands are equivalent.

Removed scripts disappear on the next scan. An already-running managed
instance remains focusable until it exits. Missing, invalid, or unsupported
package metadata does not prevent configured tasks and agents from appearing.

## Core Journeys

### Start A Registered Project

The user can start a registered project explicitly:

```text
muxpad start sample-app
```

They can also run `muxpad start` from anywhere inside the registered root.
Muxpad matches the directory to the project.

When no project session exists, Muxpad:

1. Creates a normal tmux session rooted at the project.
2. Creates an interactive window named `shell`.
3. Launches the project's default tasks, initially `api` and `mobile`.
4. Returns focus to `shell`.
5. Attaches the invoking terminal to the session.

`muxpad start sample-app --empty` creates the session and `shell` window without
launching default tasks.

When the session already exists, Muxpad attaches without reconciling,
recreating, or otherwise changing its contents. The live tmux session is the
source of truth.

When invoked inside a different tmux session, Muxpad asks whether to switch to
the project and defaults to doing nothing. It never creates a nested tmux client.

### Start From An Unregistered Directory

Running `muxpad start` in an unregistered directory creates or attaches to an ad
hoc session rooted at that directory. Muxpad derives a safe session name. The
session has a `shell` window and baked-in agents but no project tasks.

### Open The Palette

One explicit line in `.tmux.conf` installs a simple, overridable binding that
opens the unified palette in a tmux popup. Muxpad does not rewrite the user's
tmux configuration or reserve a large key map.

The palette works in any tmux session, including sessions Muxpad did not create.
Built-in agents are always available. Project tasks appear only when the session
has explicit Muxpad project context; the active pane's changing directory does
not silently redefine that context.

`muxpad menu` also works outside tmux. It resolves or creates the session for the
current directory, presents the same palette in the invoking terminal, launches
the selection, and attaches.

The palette displays:

- Tasks, clearly labelled `TASK`
- Discovered package scripts, clearly labelled `SCRIPT`
- Agents, clearly labelled `AGENT`
- Running agent instances, clearly labelled `RUNNING`
- Names, descriptions, availability, and meaningful running state

Selecting an agent launches a new instance. Selecting a running agent instance
focuses its existing window or pane.

Selecting a task or discovered script launches it when absent and focuses it
when a Muxpad-managed instance already exists. A manually launched matching
process is not detected in the MVP.

### Choose Placement

Pressing Enter launches a task, discovered script, or agent using its configured
placement, which defaults to a new window. Tab opens an action chooser for a new
window, vertical split, horizontal split, and restart where applicable.

Muxpad gives every created window and pane a human-readable name derived from
the task, agent, or agent-instance name. It does not accept process-derived
window names such as `node` as the useful identity.

Muxpad sets pane titles and its own tmux metadata but does not globally enable
visible pane borders or otherwise impose presentation settings.

### Launch Directly

The direct CLI vocabulary is:

```text
muxpad start [project]
muxpad menu
muxpad task <name>
muxpad agent <name>
```

Task and agent commands accept placement overrides.

Inside tmux, direct launch commands act on the current session. Outside tmux,
they resolve or create the session for the invoking directory, launch there,
and attach. This makes the commands suitable building blocks for personal shell
aliases and tmux bindings.

### Handle Command Exit

Each definition can select one of three exit modes:

- `close`: close the created window or pane on any exit.
- `keep`: on any exit, print a footer and drop the pane back to an interactive
  shell, leaving the command output in scrollback.
- `keep-on-error`: close after exit status zero; after a nonzero exit, behave
  like `keep` and drop to a shell.

When a command exits in `keep` (or `keep-on-error` after a failure), Muxpad
hands the pane back to a shell rather than leaving a frozen, uninteractive
pane: the output stays scrollable and the pane is immediately usable. The pane
is still marked finished, so it reports as `finished` in the palette and can be
restarted in place.

Tasks and discovered scripts default to `keep-on-error`. Agents default to
`close`. Long-running servers can explicitly use `keep` so an unexpected exit
remains visible.

If a task or discovered script has finished and dropped to a shell, selecting it
focuses that pane without erasing its output. Restart is a separate palette
action that respawns the command in the same pane.

## Acceptance Example

A sample project configuration contains these tasks:

| Identifier | Display name | Command |
| --- | --- | --- |
| `web` | web | `npm run dev:web` |
| `api` | api | `npm run dev:api` |
| `mobile` | mobile | `npm run dev` from `packages/mobile` |
| `studio` | studio | `npm run db:studio` from `packages/api` |
| `editor` | editor | `$EDITOR .` |

Its default tasks are `api` and `mobile`. `editor` is an ordinary task, not a
special Muxpad concept.

The prototype is accepted when all of the following are true:

- It can register and start more than one project from the single config file.
- `muxpad start sample-app` creates the expected shell, api, and mobile windows on
  first use.
- Repeating `muxpad start sample-app` attaches without modifying the session.
- `--empty` creates the project without API or mobile tasks.
- `muxpad start` resolves the project from a nested directory under its root.
- An unregistered directory can create a useful session with baked-in agents.
- The unified palette works inside Muxpad-created and ordinary tmux sessions.
- Tasks, discovered scripts, and agents are clearly labelled, named, and
  described.
- Root and workspace package scripts are discovered, filtered, and labelled
  `SCRIPT` without being launched automatically.
- Exact task/script matches prefer the configured task; project exclusion globs
  hide selected discovered scripts.
- Claude Code, Codex, and OpenCode can be launched when installed and are
  clearly unavailable otherwise.
- Re-selecting a running task focuses it; repeatedly selecting an agent creates
  numbered instances.
- Running agent instances can be found and focused from the palette.
- Default and alternate placements work without losing Muxpad identity.
- Exit modes produce their documented behavior.
- Direct task and agent commands work both inside and outside tmux.

## Product Boundaries

Muxpad does not replace tmux, maintain a parallel model of live session state,
or reconcile an existing session against configuration. It layers commands,
identity, discovery, and navigation on tmux's actual state.

The MVP does not:

- Interpret prompts or assign work to agents
- Detect agents or tasks launched outside Muxpad
- Automatically restart commands
- Support multiple named groups of default tasks
- Attach project context to an arbitrary existing session
- Generate configured-task names or descriptions from commands
- Manage elaborate declarative layouts
- Target production portability or packaging

## Dependencies And Risks

- Tmux popup support is required for the intended palette experience. The
  palette itself is a self-contained terminal UI with no external dependencies.
- Tmux metadata must remain reliable when users move, rename, or kill windows
  and panes manually. Muxpad must query tmux rather than trust a separate cache.
- Shell commands and working directories require careful quoting, but the MVP
  need only support the explicit WSL environment.
- Retaining failed panes in splits must not accidentally destroy unrelated
  panes when later restarted or dismissed.
- Ad hoc session-name collisions need deterministic, understandable handling.

## Implementer Judgement

The implementer may choose:

- The exact YAML shape and validation messages
- The exact popup dimensions and visual styling
- The default tmux binding and its one-line setup syntax
- Palette action keys, provided Enter uses the configured default and alternate
  window and split actions remain discoverable
- How safe ad hoc session names are derived and disambiguated
- The tmux user-option names used to preserve project, task, and agent identity
- Internal Ruby structure and test strategy appropriate to disposable code

These choices must preserve the journeys and visible behavior above.

## Deferred Decisions

- The production implementation language; Go is a candidate, not a commitment
- Production packaging and installation
- Repository-local project configuration
- Splitting the single config into multiple files
- Multiple named task groups
- Per-definition policies such as always-singleton or always-new
- Prompt templates and richer agent automation
- Automatic process recognition and generated descriptions
- Attaching project context to arbitrary sessions
- Native Windows support
- Explicit include rules that override built-in script filtering
- Script metadata beyond package name, script name, and command
- Persistent discovery indexes or background package-file watching

## Rollout

The prototype is installed with a personal configuration and one tmux
integration line, then validated through normal development sessions before a
production rewrite or packaging work begins.
