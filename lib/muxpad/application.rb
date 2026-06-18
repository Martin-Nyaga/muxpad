# frozen_string_literal: true

require "shellwords"

module Muxpad
  class Application
    attr_reader :config, :tmux

    def initialize(config: Config.new, tmux: Tmux.new, discovery: Discovery.new, input: $stdin, output: $stdout)
      @config = config
      @tmux = tmux
      @discovery = discovery
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
      selection = palette(session)
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
      tmux.launch(session:, definition:, kind: "agent", name:, root:, placement: placement || definition.placement,
                  target: launch_target(session))
    end

    def launch_target(session)
      tmux.inside? && tmux.current_session == session ? tmux.current_pane : "#{session}:shell"
    end

    def focus_shell(session)
      shell = tmux.panes(session).find { |pane| pane.name.empty? && pane.window } # initial pane has no metadata
      tmux.focus(shell) if shell
    end

    def palette(session)
      rows = palette_rows(session)
      command = ["fzf", "--delimiter=\t", "--with-nth=2", "--expect=enter,tab,ctrl-r",
                 "--layout=reverse", "--border=rounded", "--padding=1", "--info=inline", "--scrollbar=│",
                 "--prompt=Muxpad> ", "--pointer=>", "--header=Enter default | Tab actions | Ctrl-R restart"]
      stdout, status = Open3.capture2(*command, stdin_data: rows.join("\n") + "\n")
      raise Error, "fzf is required for the Muxpad palette" unless status.success? || status.exitstatus == 130
      lines = stdout.lines(chomp: true)
      return if lines.empty?
      action = lines.length > 1 ? lines[0] : "enter"
      row = lines[-1]
      [action, row.split("\t", 2).first]
    rescue Errno::ENOENT
      raise Error, "fzf is required for the Muxpad palette"
    end

    def palette_rows(session)
      project = project_for_session(session)
      panes = tmux.panes(session)
      scripts = discovered_scripts(session, project)
      rows = []
      project&.tasks&.each_value do |task|
        pane = panes.find { |item| item.kind == "task" && item.definition_id == task.id }
        state = pane ? (pane.dead ? "finished" : "running") : "not running"
        rows << palette_row("task:#{task.id}", "TASK", task.name, task.description, state)
      end
      scripts.each_value do |script|
        pane = panes.find { |item| item.kind == "script" && item.definition_id == script.id }
        state = pane ? (pane.dead ? "finished" : "running") : "not running"
        rows << palette_row("script:#{script.id}", "SCRIPT", script.name, script.description, state)
      end
      config.agents.each_value do |agent|
        available = agent.enabled && executable?(agent.executable)
        status = if !agent.enabled then "disabled" elsif available then "available" else "unavailable: missing #{agent.executable}" end
        rows << palette_row("agent:#{agent.id}", "AGENT", agent.name, agent.description, status)
      end
      panes.select { |pane| pane.kind == "agent" && !pane.dead }.each do |pane|
        rows << palette_row("running:#{pane.id}", "RUNNING", pane.name, "window #{pane.window_index}; process #{pane.current_command}", "running")
      end
      panes.select { |pane| pane.kind == "script" && !pane.dead && !scripts.key?(pane.definition_id) }.each do |pane|
        rows << palette_row("running:#{pane.id}", "RUNNING", pane.name, "removed package script; window #{pane.window_index}", "running")
      end
      rows
    end

    def palette_row(token, kind, name, description, state)
      visible = format("%-9s  %-22s  %-48s  [%s]", kind, truncate(name, 22), truncate(description, 48), state)
      "#{token}\t#{visible}"
    end

    def truncate(value, width)
      value = value.to_s
      value.length > width ? "#{value[0, width - 3]}..." : value
    end

    def handle_selection(session, selection)
      action, token = selection
      kind, id = token.split(":", 2)
      if kind == "running"
        pane = tmux.panes(session).find { |item| item.id == id }
        return tmux.focus(pane) if pane
        raise Error, "that agent instance is no longer running"
      end
      action = action_palette(session, kind, id) if action == "tab"
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

    def action_palette(session, kind, id)
      actions = [["window", "New window"], ["vertical", "Vertical split"], ["horizontal", "Horizontal split"]]
      if %w[task script].include?(kind)
        pane = tmux.panes(session).find { |item| item.kind == kind && item.definition_id == id }
        actions << ["restart", "Restart in existing pane"] if pane&.dead
      end
      input = actions.map { |token, label| "#{token}\t#{label}" }.join("\n") + "\n"
      command = ["fzf", "--delimiter=\t", "--with-nth=2", "--layout=reverse", "--border=rounded",
                 "--padding=1", "--prompt=Action> ", "--header=Choose placement or action"]
      stdout, status = Open3.capture2(*command, stdin_data: input)
      return if status.exitstatus == 130 || stdout.empty?
      raise Error, "fzf action picker failed" unless status.success?
      stdout.split("\t", 2).first
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

    def confirm_switch(session)
      @output.print("Switch tmux client to #{session}? [y/N] ")
      @output.flush
      @input.gets&.match?(/\Ay(?:es)?\s*\z/i)
    end
  end
end
