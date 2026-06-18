# frozen_string_literal: true

require "stringio"
require_relative "test_helper"

class ApplicationTest < MuxpadTest
  class InsideTmux
    attr_reader :calls

    def initialize(existing: false)
      @existing = existing
      @calls = []
    end

    def inside? = true
    def current_session = "original"
    def current_pane = "%9"
    def session_exists?(name) = @existing
    def panes(_session) = []
    def project_context(_session) = ""
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
