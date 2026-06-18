<p align="center">
  <img src="assets/logo.svg" alt="muxpad" width="420">
</p>

# Muxpad

Muxpad is a project-aware command palette and launcher for tmux. It puts your
configured project tasks, discovered package scripts, and coding agents in one
searchable menu. From there, you can launch them into clearly named tmux windows
or splits, focus an existing process, and restart a retained command.

It is intentionally a layer on top of tmux. Tmux continues to own sessions,
windows, panes, processes, and navigation; Muxpad adds project context, useful
names, discovery, and a faster way to launch and revisit work.

Muxpad is currently an early prototype.

## Motivation

Application development already involves more moving parts than it used to:
servers, workers, databases, mobile tooling, test watchers, and supporting
services. The applications themselves keep getting more complex. Coding agents
add another dimension because it is now useful to run *many* of them at once,
often across several projects and alongside all of those existing processes.

Tmux handles that scale well, but repeatedly creating, naming, and finding
everything becomes tedious. I wanted some automation for this denser workflow
without moving to a specialized agent terminal or replacing the shell tools I
already use.

## Requirements

- Ruby 3.2 or newer
- tmux 3.3 or newer

## Setup

1. Copy the example configuration and edit its project roots and tasks:

   ```sh
   mkdir -p ~/.config/muxpad
   cp config.example.yml ~/.config/muxpad/config.yml
   ```

   The essential shape is:

   ```yaml
   projects:
     sample-app:
       name: sample-app
       root: ~/code/sample-app
       default_tasks: [api]
       tasks:
         api:
           name: api
           description: Start the API development server
           command: npm run dev:api
   ```

2. Put this repository's `bin` directory on `PATH`, or link it into a directory already on `PATH`:

   ```sh
   ln -s "$PWD/bin/muxpad" ~/.local/bin/muxpad
   ```

3. Optionally add one overridable tmux binding:

   ```tmux
   bind-key b run-shell -b 'muxpad menu'
   ```

   This binds the menu to `prefix + b`. Change `b` to any free key you prefer.

Run `muxpad help` for the direct command vocabulary. `MUXPAD_CONFIG` can point
at another configuration file, which is useful for testing.

## Package scripts

Muxpad discovers scripts from the root `package.json` and workspace packages.
They appear as `SCRIPT` entries and never launch automatically. Configured tasks
remain `TASK` entries and win when their command and working directory exactly
match a discovered script.

Use `discovery.exclude` under a project to hide noisy entries:

```yaml
discovery:
  exclude:
    - "mobile:translations:*"
```

The palette groups entries into sections — configured tasks first, then agents,
running instances, and finally the auto-discovered package scripts — so the
things you launch most are never buried. Type to fuzzy-search across every
section at once (name, command, and description); matching items stay grouped
and empty sections disappear. The highlighted entry's full command shows in a
detail strip at the bottom. Enter uses the default action, Tab opens the
placement/action chooser, and Ctrl-R restarts a retained task or script.

## Tests

```sh
ruby -w -Ilib:test -e 'Dir["test/**/*_test.rb"].sort.each { |file| require_relative file }'
```
