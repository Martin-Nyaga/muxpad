# Muxpad Glossary

## Project

A registered directory with a stable identifier, display name, configured
tasks, and a list of tasks to launch when its tmux session is first created.

## Session

An ordinary tmux session. Tmux is the source of truth for its live windows,
panes, and processes. A registered project's session uses the project
identifier as its stable identity.

## Task

A project-specific command manually defined in the Muxpad configuration, such
as an application server, test runner, database tool, or editor. By default, a
task has at most one Muxpad-managed instance in a session.

## Discovered script

A script found in a project's root `package.json` or one of its workspace
packages. Discovered scripts appear separately from configured tasks. By
default, a discovered script has at most one Muxpad-managed instance in a
session.

## Agent

An interactive coding-agent CLI available across projects and unregistered
directories. The built-in agents are Claude Code, Codex, and OpenCode.
Launching an agent creates a new instance by default.

## Agent instance

One running copy of an agent in a tmux window or pane. Multiple instances use
numbered names such as `Codex`, `Codex 2`, and `Codex 3`.

## Palette

The unified, searchable list of configured tasks, discovered scripts, agents,
and running agent instances. Labels distinguish each kind.

## Default tasks

The configured list of tasks launched when a project's tmux session is first
created. `--empty` suppresses them for a particular start.

## Placement

Where a task, discovered script, or agent is launched in tmux: a new window, a
vertical split, or a horizontal split. The default is a new window.

## Exit mode

What happens to a Muxpad-created window or pane when its command exits:
`close`, `keep`, or `keep-on-error`.

## Muxpad-managed instance

A tmux pane created by Muxpad and marked with Muxpad metadata so it can be
found, focused, and, where applicable, restarted later.
