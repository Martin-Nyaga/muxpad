# frozen_string_literal: true

require "shellwords"

module Muxpad
  class Application
    attr_reader :config, :tmux

    # The launch list ordering. Running items are produced by #palette_items too,
    # but the palette routes them into its sidebar rather than this list.
    SECTION_ORDER = ["Tasks", "Agents", "Discovered scripts"].freeze

    def initialize(config: Config.new, tmux: Tmux.new, discovery: Discovery.new, palette: Palette.new,
                   input: $stdin, output: $stdout)
      @config = config
      @tmux = tmux
      @discovery = discovery
      @palette = palette
      @input = input
      @output = output
    end

    def start(project_id: nil, empty: false, attach: true)
      project = project_id ? config.project(project_id) : config.project_for(Dir.pwd)
      raise Error, "unknown project: #{project_id}" if project_id && !project
      session = session_name(project)

      if tmux.inside?
        return session if tmux.current_session == session
        return session unless confirm_switch(session)
        ensure_session(project, launch_defaults: !empty)
        tmux.switch(session)
      else
        _, created = ensure_session(project, launch_defaults: !empty)
        focus_shell(session) if created
        tmux.attach(session) if attach
      end
      session
    end

    def menu(attach: true)
      session, created = session_for_command
      selection = @palette.select(palette_items(session), section_order: SECTION_ORDER)
      unless selection
        tmux.kill_session(session) if created
        return session
      end
      handle_selection(session, selection)
      tmux.attach(session) if attach && !tmux.inside?
      session
    end

    def task(id, placement: nil, attach: true)
      session, = session_for_command
      project = project_for_session(session)
      raise Error, "session #{session.inspect} has no Muxpad project context" unless project
      launch_task(session, project, id, placement:)
      tmux.attach(session) if attach && !tmux.inside?
    end

    def agent(id, placement: nil, attach: true)
      session, = session_for_command
      launch_agent(session, id, placement:)
      tmux.attach(session) if attach && !tmux.inside?
    end

    private

    def ensure_session(project, launch_defaults: true)
      root = project&.root || Dir.pwd
      name = session_name(project)
      return [name, false] if tmux.session_exists?(name)
      tmux.create_session(name, root, project_id: project&.id)
      project.default_tasks.each { |id| launch_task(name, project, id) } if project && launch_defaults
      [name, true]
    end

    def session_name(project)
      project&.id || adhoc_name(Dir.pwd)
    end

    # Ad-hoc sessions are named after the directory. Reuse an existing muxpad
    # session rooted at this directory, otherwise pick the first free name,
    # disambiguating with a numeric suffix only when the name is already taken.
    def adhoc_name(path)
      root = File.expand_path(path)
      existing = tmux.sessions.find { |session| tmux.project_context(session).empty? && tmux.managed_root(session) == root }
      existing || available_name(adhoc_base(root))
    end

    def adhoc_base(root)
      base = File.basename(root).downcase.gsub(/[^a-z0-9_-]+/, "-").gsub(/\A-+|-+\z/, "")
      base.empty? ? "session" : base
    end

    def available_name(base)
      return base unless tmux.session_exists?(base)
      (2..).each { |n| return "#{base}-#{n}" unless tmux.session_exists?("#{base}-#{n}") }
    end

    def session_for_command
      return [tmux.current_session, false] if tmux.inside?
      ensure_session(config.project_for(Dir.pwd))
    end

    def project_for_session(session)
      id = tmux.project_context(session)
      id.empty? ? nil : config.project(id)
    end

    def launch_task(session, project, id, placement: nil)
      definition = project.tasks[id]
      raise Error, "unknown task #{id.inspect} for #{project.name}" unless definition
      existing = tmux.panes(session).find { |pane| pane.kind == "task" && pane.definition_id == id }
      return tmux.focus(existing) if existing
      tmux.launch(session:, definition:, kind: "task", name: definition.name, root: project.root,
                  placement: placement || definition.placement, target: launch_target(session))
    end

    def launch_agent(session, id, placement: nil)
      definition = config.agents[id]
      raise Error, "unknown agent: #{id}" unless definition
      raise Error, "#{definition.name} is disabled" unless definition.enabled
      raise Error, "#{definition.name} is unavailable: missing executable #{definition.executable}" unless executable?(definition.executable)
      used_names = tmux.panes(session).filter_map do |pane|
        pane.name if pane.kind == "agent" && pane.definition_id == id
      end
      instance = 1
      instance += 1 while used_names.include?(agent_instance_name(definition.name, instance))
      name = agent_instance_name(definition.name, instance)
      root = tmux.session_root(session)
      tmux.launch(session:, definition: agent_launch_definition(definition), kind: "agent", name:, root:, placement: placement || definition.placement,
                  target: launch_target(session))
    end

    def launch_target(session)
      tmux.inside? && tmux.current_session == session ? tmux.current_pane : "#{session}:shell"
    end

    def focus_shell(session)
      shell = tmux.panes(session).find { |pane| pane.name.empty? && pane.window } # initial pane has no metadata
      tmux.focus(shell) if shell
    end

    # The structured palette model, ordered to match SECTION_ORDER: configured
    # tasks first so they are never buried, then agents, live agent/script
    # instances, and finally the noisy auto-discovered scripts.
    def palette_items(session)
      project = project_for_session(session)
      panes = tmux.panes(session)
      scripts = discovered_scripts(session, project)
      root = project&.root || tmux.session_root(session)
      items = []
      project&.tasks&.each_value do |task|
        pane = panes.find { |item| item.kind == "task" && item.definition_id == task.id }
        items << launchable_item("task:#{task.id}", "Tasks", task, pane, root)
        items << running_item(pane, "window #{pane.window_index} · #{pane.current_command}") if pane && !pane.done?
      end
      config.agents.each_value do |agent|
        items << agent_item(agent, root)
      end
      panes.select { |pane| pane.kind == "agent" && !pane.done? }.each do |pane|
        items << running_item(pane, "window #{pane.window_index} · #{pane.current_command}")
      end
      panes.select { |pane| pane.kind == "script" && !pane.done? && !scripts.key?(pane.definition_id) }.each do |pane|
        items << running_item(pane, "removed package script · window #{pane.window_index}")
      end
      scripts.each_value do |script|
        pane = panes.find { |item| item.kind == "script" && item.definition_id == script.id }
        items << launchable_item("script:#{script.id}", "Discovered scripts", script, pane, root)
      end
      items
    end

    def launchable_item(token, section, definition, pane, root)
      state, kind = if pane.nil? then ["not running", :idle]
      elsif pane.done? then ["finished", :finished]
      else ["running", :running]
      end
      Item.new(token:, section:, name: definition.name, description: definition.description,
               command: definition.command, directory: resolve_directory(definition, root),
               state:, state_kind: kind, summary: nil)
    end

    def agent_item(agent, root)
      available = agent.enabled && executable?(agent.executable)
      state, kind = if !agent.enabled then ["disabled", :disabled]
      elsif available then ["available", :available]
      else ["unavailable: missing #{agent.executable}", :unavailable]
      end
      Item.new(token: "agent:#{agent.id}", section: "Agents", name: agent.name,
               description: agent.description, command: agent.command, directory: root,
               state:, state_kind: kind, summary: nil)
    end

    def running_item(pane, description)
      Item.new(token: "running:#{pane.id}", section: "Running", name: pane.name,
               description:, command: pane.current_command, directory: nil,
               state: "running", state_kind: :running, summary: agent_summary(pane))
    end

    def resolve_directory(definition, root)
      definition.directory ? File.expand_path(definition.directory, root) : root
    end

    def handle_selection(session, selection)
      action, token = selection
      kind, id = token.split(":", 2)
      if kind == "running"
        pane = tmux.panes(session).find { |item| item.id == id }
        return tmux.focus(pane) if pane
        raise Error, "that agent instance is no longer running"
      end
      action = action_menu(session, kind, id) if action == "tab"
      return unless action
      placement = action if Config::PLACEMENTS.include?(action)
      if kind == "task"
        project = project_for_session(session)
        definition = project.tasks.fetch(id)
        pane = tmux.panes(session).find { |item| item.kind == "task" && item.definition_id == id }
        return tmux.restart(pane, definition) if %w[ctrl-r restart].include?(action) && pane
        launch_task(session, project, id, placement:)
      elsif kind == "script"
        project = project_for_session(session)
        definition = discovered_scripts(session, project)[id]
        raise Error, "package script #{id.inspect} is no longer available" unless definition
        pane = tmux.panes(session).find { |item| item.kind == "script" && item.definition_id == id }
        return tmux.restart(pane, definition) if %w[ctrl-r restart].include?(action) && pane
        launch_script(session, definition, placement:)
      else
        raise Error, "restart applies only to tasks and package scripts" if %w[ctrl-r restart].include?(action)
        launch_agent(session, id, placement:)
      end
    end

    def action_menu(session, kind, id)
      actions = [["window", "New window"], ["vertical", "Vertical split"], ["horizontal", "Horizontal split"]]
      if %w[task script].include?(kind)
        pane = tmux.panes(session).find { |item| item.kind == kind && item.definition_id == id }
        actions << ["restart", "Restart in existing pane"] if pane&.done?
      end
      @palette.choose(actions, title: "Choose placement or action")
    end

    def discovered_scripts(session, project)
      root = project&.root || tmux.session_root(session)
      excludes = project&.discovery_exclude || []
      scripts = @discovery.scripts(root, exclude: excludes)
      return scripts unless project
      configured = project.tasks.values.to_h do |task|
        directory = task.directory ? File.expand_path(task.directory, project.root) : project.root
        [[directory, task.command], true]
      end
      scripts.reject do |_id, script|
        directory = script.directory ? File.expand_path(script.directory, project.root) : project.root
        configured.key?([directory, script.command])
      end
    end

    def launch_script(session, definition, placement: nil)
      existing = tmux.panes(session).find { |pane| pane.kind == "script" && pane.definition_id == definition.id }
      return tmux.focus(existing) if existing
      root = tmux.session_root(session)
      tmux.launch(session:, definition:, kind: "script", name: definition.name, root:,
                  placement: placement || definition.placement, target: launch_target(session))
    end

    def executable?(name)
      return false if name.to_s.empty?
      ENV.fetch("PATH", "").split(File::PATH_SEPARATOR).any? { |path| File.executable?(File.join(path, name)) }
    end

    def agent_instance_name(name, instance)
      instance == 1 ? name : "#{name} #{instance}"
    end

    def agent_launch_definition(definition)
      return definition unless definition.id == "codex"
      command = definition.command.sub(/\A(\s*(?:[^\s]*\/)?codex)(?=\s|\z)/) do |executable|
        config = Shellwords.escape('tui.terminal_title=["thread"]')
        "#{executable} -c #{config}"
      end
      Definition.new(**definition.to_h.merge(command:))
    end

    def agent_summary(pane)
      return unless %w[claude codex].include?(pane.definition_id)
      title = pane.title.to_s.gsub(/\s+/, " ").strip
      title = title.sub(/\A✳(?=\s|\z)/, "*") if pane.definition_id == "claude"
      generic = [pane.name, pane.definition_id, "Claude Code", "Codex", "New thread"]
      codex_id = pane.definition_id == "codex" && title.match?(/\A(?:thread\s+)?[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}\z/i)
      title unless title.empty? || codex_id || generic.any? { |value| title.casecmp?(value) }
    end

    def confirm_switch(session)
      @output.print("Switch tmux client to #{session}? [y/N] ")
      @output.flush
      @input.gets&.match?(/\Ay(?:es)?\s*\z/i)
    end
  end
end
