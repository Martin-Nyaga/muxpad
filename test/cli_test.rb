# frozen_string_literal: true

require "stringio"
require_relative "test_helper"

class CliTest < MuxpadTest
  class FakeApplication
    attr_reader :calls, :tmux

    def initialize(inside: false)
      @calls = []
      @tmux = FakeTmux.new(inside:)
    end

    def method_missing(name, *args, **kwargs)
      calls << [name, args, kwargs]
    end

    def respond_to_missing?(_name, _include_private = false) = true
  end

  class FakeTmux
    attr_reader :calls

    def initialize(inside:)
      @inside = inside
      @calls = []
    end

    def available? = true
    def inside? = @inside

    def popup_menu(program)
      calls << [:popup_menu, program]
    end
  end

  def test_dispatches_direct_commands_and_placement_flags
    app = FakeApplication.new
    cli = Muxpad::CLI.new(%w[agent codex --vertical], output: StringIO.new, error: StringIO.new)
    cli.instance_variable_set(:@application, app)

    assert_equal 0, cli.run
    assert_equal [:agent, ["codex"], { placement: "vertical" }], app.calls.fetch(0)
  end

  def test_dispatches_start_empty
    app = FakeApplication.new
    cli = Muxpad::CLI.new(%w[start sample-app --empty], output: StringIO.new, error: StringIO.new)
    cli.instance_variable_set(:@application, app)

    assert_equal 0, cli.run
    assert_equal [:start, [], { project_id: "sample-app", empty: true }], app.calls.fetch(0)
  end

  def test_reports_invalid_commands
    error = StringIO.new
    cli = Muxpad::CLI.new(["unknown"], output: StringIO.new, error:)
    cli.instance_variable_set(:@application, FakeApplication.new)

    assert_equal 1, cli.run
    assert_match(/unknown command/, error.string)
  end

  def test_menu_inside_tmux_opens_a_popup_instead_of_rendering_the_palette_in_the_pane
    app = FakeApplication.new(inside: true)
    cli = Muxpad::CLI.new(["menu"], output: StringIO.new, error: StringIO.new)
    cli.instance_variable_set(:@application, app)

    assert_equal 0, cli.run
    assert_empty app.calls
    assert_equal :popup_menu, app.tmux.calls.fetch(0).first
    assert_match(/muxpad|ruby/, app.tmux.calls.fetch(0).last)
  end
end
