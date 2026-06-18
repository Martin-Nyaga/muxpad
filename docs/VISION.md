# Muxpad Vision

## A tmux workspace for agent-heavy development

Muxpad is a project-aware command palette and workspace layer for tmux. It
helps developers assemble the workspace they need now: application processes,
development tools, and coding agents, launched into durable tmux windows and
panes without prescribing a complete layout up front.

Modern development sessions are no longer made up only of an editor, a server,
and a test runner. They increasingly include several long-running coding agents
working alongside conventional project tasks. Muxpad gives both kinds of work a
shared, tmux-native home.

## The problem

Existing session launchers are good at recreating a fixed arrangement of known
commands. That works when every day begins with the same processes and layout,
but it becomes restrictive when the useful workspace depends on the work being
done.

Agent-heavy workflows make that mismatch more obvious. A developer might want
an API server and Claude Code today, two Codex sessions and a test watcher
tomorrow, or OpenCode beside a database console for a short investigation. The
individual commands are easy to run; repeatedly creating, arranging, finding,
and returning to them is the friction.

Muxpad should remove that friction while preserving tmux's flexibility.

## Product promise

From any Muxpad workspace, a developer can open a searchable palette and launch
the next thing they need in the right project context and tmux location.

Muxpad understands three first-class kinds of thing it can launch:

- **Project tasks** are commands specific to a project, such as development
  servers, test watchers, database tools, consoles, and build processes. They
  are intentionally user-defined.
- **Discovered scripts** come from root and workspace package manifests. They
  reduce routine setup while remaining visibly distinct from configured tasks
  and never run automatically.
- **Agents** are coding agents available across projects. Muxpad ships with
  extensible defaults for Claude Code, Codex, and OpenCode, with room for users
  and integrations to add more.

All three are available through the same interface, but they remain distinct in
the configuration and user experience. Tasks describe explicitly configured
project commands, discovered scripts expose package conventions, and agents are
tools that can work on the project.

## The experience

A developer starts or attaches to a workspace associated with a project. The
workspace may begin empty, use a lightweight profile, or already contain work
from an earlier session.

Inside tmux, a key binding opens the Muxpad palette. The developer can search
across project tasks, discovered scripts, and agents, then launch a selection
in a new window or split. Direct CLI commands provide the same operations for
configured tasks and agents without the palette.

For example:

```text
muxpad start sample-app
muxpad task api
muxpad agent claude
muxpad agent codex
```

Users may optionally install `mp` as a shorthand alias or symlink.

Muxpad retains project identity at the tmux session level. Changing directories
inside a pane does not silently change the workspace's project or configuration.
Every task, discovered script, and agent has a clear name and description.
Muxpad carries that identity into tmux, naming the windows and panes it creates
rather than allowing the underlying process to produce an unhelpful title such
as `node`. Windows and panes launched by Muxpad retain enough identity for the
palette to find, focus, restart, or otherwise manage them later.

## Extensible defaults

Agent support should work immediately without forcing every project to repeat
the same configuration. Muxpad's built-in catalogue initially includes:

- Claude Code
- Codex
- OpenCode

These definitions are defaults, not hard-coded special cases. A user can
override their commands and launch behavior, disable them, or define additional
agents. Project configuration may refine a global definition where a particular
repository needs different behavior.

Over time, an agent definition may express more than an executable name. Useful
behavior could include a description, preferred placement, environment, startup
arguments, resume behavior, display naming, or whether repeated launches focus
an existing pane or create another instance. These capabilities should grow
from real usage rather than from an attempt to model every agent in advance.

## Configuration model

Muxpad separates concerns that fixed session definitions often combine:

- Project identity and root directory
- Project-specific tasks
- Reusable agents
- Startup profiles
- Placement and lifecycle behavior

Initial project configurations live outside application repositories, with an
explicit path or registry connecting a project to its configuration. This keeps
Muxpad usable without requiring changes to every repository. Project-local
configuration can be considered later without making it a prerequisite for the
first useful version.

The configuration format should remain small and readable. Common behavior
should come from defaults; configuration should describe what is distinctive
about a project or workflow.

## Principles

### Compose the workspace incrementally

An empty session is a valid starting point. Muxpad should make it quick to add
work as needs emerge instead of requiring the entire session to be declared in
advance.

### Treat agents as first-class participants

Agents are not miscellaneous shell commands hidden among project tasks. They
have distinct configuration, discoverability, and lifecycle needs, while still
sharing tmux placement and project context with conventional processes.

### Build on tmux

Muxpad should use tmux's durable sessions, windows, panes, and popups directly.
It should feel like an extension of tmux, not a replacement terminal or a
parallel workspace model. Where tmux exposes low-level process-derived details,
Muxpad should add the project-aware names and descriptions needed to make the
workspace legible.

### Prefer conventions that remain overridable

Launching common agents should work out of the box. Project tasks and personal
preferences remain configurable. Defaults should reduce setup without limiting
advanced workflows.

### Preserve control

The developer chooses what runs and where. Profiles, default placement, and
singleton behavior may accelerate common choices, but they should not turn the
workspace into an opaque automated layout.

### Grow from a focused core

The first useful product is a reliable palette that loads one project, exposes
its tasks and standard agents, and launches them in tmux. Coordination features
should be added only when they solve demonstrated workflow problems.

## What Muxpad is not

Muxpad is not a coding agent, an agent orchestration runtime, or an editor. It
does not interpret prompts or mediate the internals of Claude Code, Codex,
OpenCode, or future tools.

Muxpad is also not primarily a declarative layout engine. It may support startup
profiles and convenient placement, but its central abstraction is a catalogue
of things that can participate in a project workspace, available on demand.

## Direction

The initial milestone should prove the interaction:

1. Associate a tmux session with one externally stored project configuration.
2. Open a searchable palette in a tmux popup.
3. Present project tasks, discovered scripts, and built-in agents with clear
   names and descriptions.
4. Launch a selection in a clearly named tmux window, with at least one split
   alternative.
5. Preserve task or agent identity to support future focus and lifecycle
   behavior.

Success is not measured by how many layouts Muxpad can encode. It is measured by
how little ceremony stands between deciding what the workspace needs and having
it running, visible, and easy to return to.
