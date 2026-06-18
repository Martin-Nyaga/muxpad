# frozen_string_literal: true

require_relative "test_helper"

class IntegrationTest < MuxpadTest
  # Stands in for the interactive Palette so menu flows can be driven headlessly.
  class StubPalette
    def initialize(select: nil, choose: nil)
      @select = select
      @choose = choose
    end

    def select(items, section_order:)
      @select.respond_to?(:call) ? @select.call(items) : @select
    end

    def choose(options, title:)
      @choose.respond_to?(:call) ? @choose.call(options) : @choose
    end
  end

  def setup
    super
    ENV.delete("TMUX")
    ENV["MUXPAD_TMUX_SOCKET"] = "muxpad-test-#{Process.pid}-#{object_id}"
    @project = File.join(@tmp, "project")
    @mobile = File.join(@project, "mobile")
    FileUtils.mkdir_p(@mobile)
    @discovery_marker = File.join(@tmp, "discovered-ran")
    File.write(File.join(@project, "package.json"), JSON.generate(
      name: "first", packageManager: "pnpm@9", workspaces: ["mobile"],
      scripts: { duplicate: "sleep 30", rootcheck: "printf ok > #{@discovery_marker}; sleep 30" }
    ))
    File.write(File.join(@mobile, "package.json"), JSON.generate(
      name: "app-mobile", scripts: { dev: "sleep 30", "noise:internal": "sleep 30" }
    ))
    @config_path = write_config(<<~YAML)
      projects:
        first:
          name: First
          root: #{@project}
          default_tasks: [api, mobile]
          tasks:
            api:
              name: API server
              description: API
              command: sleep 30
              exit_mode: keep
            mobile:
              name: Mobile app
              description: Mobile
              command: sleep 30
              directory: mobile
              exit_mode: keep
            failure:
              name: Failure
              description: Fails
              command: 'exit 7'
            success:
              name: Success
              description: Succeeds
              command: 'exit 0'
            kept:
              name: Kept
              description: Kept output
              command: 'exit 0'
              exit_mode: keep
            closed:
              name: Closed
              description: Closed output
              command: 'exit 7'
              exit_mode: close
            envcheck:
              name: Environment check
              description: Uses the invoking PATH
              command: muxpad-env-check
              exit_mode: keep
            duplicate:
              name: duplicate
              description: Configured version wins
              command: pnpm duplicate
          discovery:
            exclude:
              - "app-mobile:noise:*"
      agents:
        codex:
          command: sleep 30
          executable: sleep
        claude:
          disabled: true
        opencode:
          executable: muxpad-test-missing-opencode
    YAML
    ENV["MUXPAD_CONFIG"] = @config_path
    @tmux = Muxpad::Tmux.new
    @app = Muxpad::Application.new(config: Muxpad::Config.new, tmux: @tmux)
  end

  def teardown
    system("tmux", "-L", ENV["MUXPAD_TMUX_SOCKET"], "kill-server", out: File::NULL, err: File::NULL)
    super
  end

  def test_start_defaults_repeat_empty_and_nested_resolution
    Dir.chdir(File.join(@project, "mobile")) do
      assert_equal "first", @app.start(attach: false)
    end
    assert_equal ["API server", "Mobile app", "shell"], windows("first").sort
    assert_equal "shell", active_window("first")
    indexes = window_indexes("first")
    assert_equal((indexes.min..indexes.max).to_a, indexes.sort)
    before = pane_ids("first")
    @app.start(project_id: "first", attach: false)
    assert_equal before, pane_ids("first")

    kill_session("first")
    @app.start(project_id: "first", empty: true, attach: false)
    assert_equal ["shell"], windows("first")
  end

  def test_task_singleton_agent_numbering_and_placements
    @app.start(project_id: "first", empty: true, attach: false)
    Dir.chdir(@project) do
      @app.task("api", attach: false)
      @app.task("api", attach: false)
      assert_equal "API server", active_window("first")
      @app.agent("codex", attach: false)
      @app.agent("codex", placement: "vertical", attach: false)
    end
    assert_equal 1, managed("first", "task", "api").length
    agents = managed("first", "agent", "codex")
    assert_equal ["codex", "codex 2"], agents.map(&:name).sort
    assert_equal 3, windows("first").length
    assert_equal 4, pane_ids("first").length
  end

  def test_agent_instance_names_do_not_collide_after_a_middle_instance_closes
    @app.start(project_id: "first", empty: true, attach: false)
    Dir.chdir(@project) do
      3.times { @app.agent("codex", attach: false) }
    end
    second = managed("first", "agent", "codex").find { |pane| pane.name == "codex 2" }
    system("tmux", "-L", ENV.fetch("MUXPAD_TMUX_SOCKET"), "kill-pane", "-t", second.id)

    Dir.chdir(@project) { @app.agent("codex", attach: false) }

    names = managed("first", "agent", "codex").map(&:name)
    assert_equal ["codex", "codex 2", "codex 3"], names.sort
  end

  def test_exit_modes_and_restart
    @app.start(project_id: "first", empty: true, attach: false)
    Dir.chdir(@project) do
      @app.task("failure", attach: false)
      @app.task("success", attach: false)
      @app.task("kept", attach: false)
      @app.task("closed", attach: false)
    end
    sleep 0.4

    failure = managed("first", "task", "failure").first
    kept = managed("first", "task", "kept").first
    assert failure.finished
    assert kept.finished
    assert_empty managed("first", "task", "success")
    assert_empty managed("first", "task", "closed")

    # Finished panes drop to an interactive shell rather than a frozen corpse.
    refute failure.dead
    refute kept.dead

    definition = @app.config.project("first").tasks.fetch("failure")
    assert_includes captured_pane(failure.id), "[Muxpad] Command failed with status 7"
    assert_includes captured_pane(kept.id), "[Muxpad] Command exited with status 0"
    @tmux.restart(failure, definition)
    sleep 0.2
    assert managed("first", "task", "failure").first.finished
  end

  def test_interrupting_a_keep_command_drops_to_a_shell_instead_of_closing_the_pane
    @app.start(project_id: "first", empty: true, attach: false)
    Dir.chdir(@project) { @app.task("api", attach: false) } # sleep 30, exit_mode: keep
    sleep 0.4
    pane = managed("first", "task", "api").first
    refute pane.finished, "task should still be running before interrupt"

    system("tmux", "-L", ENV.fetch("MUXPAD_TMUX_SOCKET"), "send-keys", "-t", pane.id, "C-c")
    sleep 0.6

    survivor = managed("first", "task", "api").first
    refute_nil survivor, "interrupting a keep command must not close the pane"
    assert survivor.finished
    assert_includes captured_pane(survivor.id), "[Muxpad] Command failed with status 130"
  end

  def test_ad_hoc_session_has_no_project_tasks
    Dir.chdir(@tmp) do
      session = @app.start(attach: false)
      assert_equal File.basename(@tmp), session
      assert_equal "", @tmux.project_context(session)
      assert_raises(Muxpad::Error) { @app.task("api", attach: false) }
    end
  end

  def test_ad_hoc_sessions_use_clean_names_and_disambiguate_conflicts
    one = File.join(@tmp, "a", "web")
    two = File.join(@tmp, "b", "web")
    FileUtils.mkdir_p(one)
    FileUtils.mkdir_p(two)

    first = Dir.chdir(one) { @app.start(attach: false) }
    assert_equal "web", first

    # Re-running in the same directory reuses the existing session.
    reused = Dir.chdir(one) { @app.start(attach: false) }
    assert_equal "web", reused

    # A different directory with the same basename gets a numeric suffix.
    second = Dir.chdir(two) { @app.start(attach: false) }
    assert_equal "web-2", second

    assert_equal one, @tmux.session_root("web")
    assert_equal two, @tmux.session_root("web-2")
  ensure
    @tmux.kill_session("web") if @tmux.session_exists?("web")
    @tmux.kill_session("web-2") if @tmux.session_exists?("web-2")
  end

  def test_direct_agent_creation_launches_project_defaults
    Dir.chdir(@project) { @app.agent("codex", attach: false) }

    assert_equal ["API server", "Mobile app", "codex", "shell"], windows("first").sort
  end

  def test_palette_labels_availability_running_instances_and_alternate_action
    @app.start(project_id: "first", empty: true, attach: false)

    # Selecting an agent with Tab opens the action menu; choosing "vertical"
    # launches it as a split rather than a new window.
    palette = StubPalette.new(select: ["tab", "agent:codex"], choose: "vertical")
    Dir.chdir(@project) { app_with(palette).menu(attach: false) }

    items = @app.send(:palette_items, "first")
    task = items.find { |item| item.token == "task:api" }
    assert_equal ["Tasks", "API server"], [task.section, task.name]

    claude = items.find { |item| item.token == "agent:claude" }
    assert_equal ["Agents", "disabled", :disabled], [claude.section, claude.state, claude.state_kind]

    running = items.find { |item| item.token.start_with?("running:") }
    assert_equal "Running", running.section
    assert_match(/codex/, running.name)
    assert_match(/window \d+/, running.description)
    refute_includes running.description, "window @"
    assert_equal "running", running.state

    assert_equal 2, pane_ids("first").length
    assert_equal 1, windows("first").length
  end

  def test_unavailable_and_disabled_agents_explain_why_they_cannot_launch
    @app.start(project_id: "first", empty: true, attach: false)
    disabled = assert_raises(Muxpad::Error) { Dir.chdir(@project) { @app.agent("claude", attach: false) } }
    unavailable = assert_raises(Muxpad::Error) { Dir.chdir(@project) { @app.agent("opencode", attach: false) } }

    assert_match(/disabled/, disabled.message)
    assert_match(/missing executable muxpad-test-missing-opencode/, unavailable.message)
  end

  def test_ordinary_tmux_session_uses_active_path_without_gaining_project_context
    # Hold the pane on a non-shell command so its working directory stays put;
    # a login shell may cd during startup and race the path captures below.
    system("tmux", "-L", ENV.fetch("MUXPAD_TMUX_SOCKET"), "new-session", "-d", "-s", "ordinary", "-c", @tmp, "sleep 300")

    assert_equal "", @tmux.project_context("ordinary")
    actual_path, = Open3.capture2("tmux", "-L", ENV.fetch("MUXPAD_TMUX_SOCKET"), "display-message", "-p", "-t", "ordinary", '#{pane_current_path}')
    assert_equal actual_path.strip, @tmux.session_root("ordinary")
    items = @app.send(:palette_items, "ordinary")
    refute items.any? { |item| item.section == "Tasks" }
    assert_equal 3, items.count { |item| item.section == "Agents" }
  end

  def test_canceling_menu_outside_tmux_removes_a_new_session_and_does_not_attach
    palette = StubPalette.new(select: nil)
    Dir.chdir(@project) { app_with(palette).menu(attach: true) }

    refute @tmux.session_exists?("first")
  end

  def test_tasks_inherit_the_invoking_path_even_when_tmux_server_is_already_running
    @app.start(project_id: "first", empty: true, attach: false)
    bin = File.join(@tmp, "task-bin")
    marker = File.join(@tmp, "envcheck-ran")
    FileUtils.mkdir_p(bin)
    executable = File.join(bin, "muxpad-env-check")
    File.write(executable, "#!/bin/sh\nprintf ok > #{marker}\nsleep 30\n")
    FileUtils.chmod(0o755, executable)
    ENV["PATH"] = "#{bin}:#{ENV.fetch('PATH')}"

    Dir.chdir(@project) { @app.task("envcheck", attach: false) }
    sleep 0.2

    assert_equal "ok", File.read(marker)
    refute managed("first", "task", "envcheck").first.done?
  end

  def test_discovers_deduplicates_refreshes_and_launches_package_scripts
    @app.start(project_id: "first", empty: true, attach: false)
    items = @app.send(:palette_items, "first")

    rootcheck = items.find { |item| item.token == "script:rootcheck" }
    assert_equal "Discovered scripts", rootcheck.section
    assert items.any? { |item| item.token == "script:app-mobile:dev" }
    refute items.any? { |item| item.token == "script:duplicate" }
    refute items.any? { |item| item.token.include?("noise") }
    assert_empty managed("first", "script", "rootcheck")

    @app.send(:handle_selection, "first", ["enter", "script:rootcheck"])
    @app.send(:handle_selection, "first", ["enter", "script:rootcheck"])
    sleep 0.2
    assert_equal "ok", File.read(@discovery_marker)
    assert_equal 1, managed("first", "script", "rootcheck").length

    package = JSON.parse(File.read(File.join(@project, "package.json")))
    package.fetch("scripts")["added"] = "sleep 30"
    File.write(File.join(@project, "package.json"), JSON.generate(package))
    assert @app.send(:palette_items, "first").any? { |item| item.token == "script:added" }
  end

  def test_discovers_scripts_in_an_ad_hoc_session
    directory = File.join(@tmp, "adhoc")
    FileUtils.mkdir_p(directory)
    File.write(File.join(directory, "package.json"), JSON.generate(scripts: { hello: "echo hello" }))

    Dir.chdir(directory) do
      session = @app.start(attach: false)
      assert @app.send(:palette_items, session).any? { |item| item.token == "script:hello" }
    end
  end

  private

  def app_with(palette)
    Muxpad::Application.new(config: Muxpad::Config.new, tmux: @tmux, palette:)
  end

  def windows(session)
    stdout, = Open3.capture2("tmux", "-L", ENV.fetch("MUXPAD_TMUX_SOCKET"), "list-windows", "-t", session, "-F", '#{window_name}')
    stdout.lines(chomp: true)
  end

  def active_window(session)
    stdout, = Open3.capture2("tmux", "-L", ENV.fetch("MUXPAD_TMUX_SOCKET"), "display-message", "-p", "-t", session, '#{window_name}')
    stdout.strip
  end

  def window_indexes(session)
    stdout, = Open3.capture2("tmux", "-L", ENV.fetch("MUXPAD_TMUX_SOCKET"), "list-windows", "-t", session, "-F", '#{window_index}')
    stdout.lines.map(&:to_i)
  end

  def captured_pane(pane)
    stdout, = Open3.capture2("tmux", "-L", ENV.fetch("MUXPAD_TMUX_SOCKET"), "capture-pane", "-p", "-S", "-", "-t", pane)
    stdout
  end

  def pane_ids(session)
    @tmux.panes(session).map(&:id)
  end

  def managed(session, kind, id)
    @tmux.panes(session).select { |pane| pane.kind == kind && pane.definition_id == id }
  end

  def kill_session(session)
    system("tmux", "-L", ENV.fetch("MUXPAD_TMUX_SOCKET"), "kill-session", "-t", session)
  end
end
