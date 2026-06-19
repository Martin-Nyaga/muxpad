# frozen_string_literal: true

require "stringio"
require_relative "test_helper"

class ApplicationTest < MuxpadTest
  class InsideTmux
    attr_reader :calls

    def initialize(existing: false, panes: [], project_context: "")
      @existing = existing
      @panes = panes
      @project_context = project_context
      @calls = []
    end

    def inside? = true
    def current_session = "original"
    def current_pane = "%9"
    def session_exists?(name) = @existing
    def panes(_session) = @panes
    def project_context(_session) = @project_context
    def session_root(_session) = "/ordinary"

    def create_session(*args, **kwargs)
      calls << [:create_session, args, kwargs]
    end

    def launch(**kwargs)
      calls << [:launch, kwargs]
      "%10"
    end

    def switch(session)
      calls << [:switch, session]
    end
  end

  class StaticAgentDiscovery
    def initialize(detected)
      @detected = detected
    end

    def detect(_panes) = @detected
  end

  def test_declining_inside_tmux_switch_does_not_create_or_change_target
    project = File.join(@tmp, "project")
    FileUtils.mkdir_p(project)
    config = config_for(project)
    tmux = InsideTmux.new
    output = StringIO.new
    app = Muxpad::Application.new(config:, tmux:, input: StringIO.new("\n"), output:)

    assert_equal "work", app.start(project_id: "work", attach: false)
    assert_empty tmux.calls
    assert_match(/Switch tmux client/, output.string)
  end

  def test_accepting_inside_tmux_switch_creates_defaults_then_switches
    project = File.join(@tmp, "project")
    FileUtils.mkdir_p(project)
    tmux = InsideTmux.new
    app = Muxpad::Application.new(config: config_for(project), tmux:, input: StringIO.new("yes\n"), output: StringIO.new)

    app.start(project_id: "work", attach: false)

    assert_equal :create_session, tmux.calls[0][0]
    assert_equal :launch, tmux.calls[1][0]
    assert_equal [:switch, "work"], tmux.calls[2]
  end

  def test_direct_agent_inside_ordinary_session_targets_current_pane
    config = Muxpad::Config.new(write_config(<<~YAML))
      agents:
        codex:
          command: sleep 30
          executable: sleep
    YAML
    tmux = InsideTmux.new(existing: true)
    app = Muxpad::Application.new(config:, tmux:)

    app.agent("codex", placement: "horizontal", attach: false)

    launch = tmux.calls.fetch(0).fetch(1)
    assert_equal "original", launch[:session]
    assert_equal "%9", launch[:target]
    assert_equal "horizontal", launch[:placement]
  end

  def test_codex_launch_requests_the_thread_terminal_title
    config = Muxpad::Config.new(write_config(<<~YAML))
      agents:
        codex:
          command: codex --model test-model
          executable: sleep
    YAML
    tmux = InsideTmux.new(existing: true)
    app = Muxpad::Application.new(config:, tmux:)

    app.agent("codex", attach: false)

    command = tmux.calls.fetch(0).fetch(1).fetch(:definition).command
    assert_match(/\Acodex -c .*terminal_title.*thread.* --model test-model\z/, command)
  end

  def test_agent_summary_uses_only_meaningful_claude_and_codex_titles
    app = Muxpad::Application.new
    pane = Muxpad::Pane.new(id: "%1", session: "work", window: "@1", window_index: "1",
                            kind: "agent", definition_id: "codex", name: "codex", dead: false,
                            finished: false, current_command: "codex", title: "  Fix   flaky tests  ",
                            pid: "100", current_path: "/work")

    assert_equal "Fix flaky tests", app.send(:agent_summary, pane)
    assert_nil app.send(:agent_summary, pane.with(title: "Codex"))
    assert_nil app.send(:agent_summary, pane.with(title: "019edd47-91f2-7102-b113-d047160a33d8"))
    assert_nil app.send(:agent_summary, pane.with(definition_id: "opencode", title: "Useful title"))
    assert_equal "* Refactor authentication", app.send(:agent_summary,
      pane.with(definition_id: "claude", name: "claude", title: "✳ Refactor authentication"))
  end

  def test_running_task_appears_in_sidebar_and_remains_in_launch_list
    project = File.join(@tmp, "project")
    FileUtils.mkdir_p(project)
    pane = Muxpad::Pane.new(id: "%1", session: "work", window: "@1", window_index: "1",
                            kind: "task", definition_id: "server", name: "Server", dead: false,
                            finished: false, current_command: "sleep", title: "Server", pid: "100",
                            current_path: project)
    tmux = InsideTmux.new(panes: [pane], project_context: "work")
    app = Muxpad::Application.new(config: config_for(project), tmux:)

    items = app.send(:palette_items, "work")

    launch = items.find { |item| item.token == "task:server" }
    running = items.find { |item| item.token == "running:%1" }
    assert_equal ["Tasks", "running", :running], [launch.section, launch.state, launch.state_kind]
    assert_equal ["Running", "Server", "window 1 · sleep"],
                 [running.section, running.name, running.description]
  end

  def test_unmanaged_detected_agent_appears_as_a_numbered_running_instance
    managed = Muxpad::Pane.new(id: "%1", session: "work", window: "@1", window_index: "1",
                               kind: "agent", definition_id: "codex", name: "codex", dead: false,
                               finished: false, current_command: "node", title: "Codex", pid: "100",
                               current_path: "/work")
    unmanaged = Muxpad::Pane.new(id: "%2", session: "work", window: "@2", window_index: "2",
                                 kind: "", definition_id: "", name: "", dead: false, finished: false,
                                 current_command: "node", title: "Investigate timeout", pid: "200",
                                 current_path: "/work")
    tmux = InsideTmux.new(panes: [managed, unmanaged])
    discovery = StaticAgentDiscovery.new("%2" => "codex")
    app = Muxpad::Application.new(tmux:, agent_discovery: discovery)

    item = app.send(:palette_items, "work").find { |candidate| candidate.token == "running:%2" }

    assert_equal ["Running", "codex 2", "external agent · window 2", "Investigate timeout"],
                 [item.section, item.name, item.description, item.summary]
  end

  private

  def config_for(project)
    Muxpad::Config.new(write_config(<<~YAML))
      projects:
        work:
          name: Work
          root: #{project}
          default_tasks: [server]
          tasks:
            server:
              name: Server
              description: Run server
              command: sleep 30
    YAML
  end
end
