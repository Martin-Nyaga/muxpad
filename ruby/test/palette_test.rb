# frozen_string_literal: true

require_relative "test_helper"

class PaletteTest < MuxpadTest
  def test_sidebar_renders_an_optional_summary_below_the_running_agent
    palette = Muxpad::Palette.new
    palette.instance_variable_set(:@running, [running_item(summary: "Fix flaky tests")])
    palette.instance_variable_set(:@focus, :launch)
    palette.instance_variable_set(:@run_cursor, 0)

    lines = palette.send(:sidebar_lines, 30).join("\n")

    assert_includes lines, "codex"
    assert_includes lines, "Fix flaky tests"
  end

  def test_sidebar_summary_keeps_space_before_divider
    palette = Muxpad::Palette.new
    palette.instance_variable_set(:@running, [running_item(summary: "A summary long enough to reach the divider")])
    palette.instance_variable_set(:@focus, :launch)
    palette.instance_variable_set(:@run_cursor, 0)

    summary = palette.send(:sidebar_lines, 30).last.delete_prefix(Muxpad::Palette::DIM).delete_suffix(Muxpad::Palette::RESET)

    assert_equal 30, summary.length
    assert summary.end_with?(" ")
  end

  def test_sidebar_keeps_single_line_item_when_summary_is_missing
    palette = Muxpad::Palette.new
    palette.instance_variable_set(:@running, [running_item(summary: nil)])
    palette.instance_variable_set(:@focus, :launch)
    palette.instance_variable_set(:@run_cursor, 0)

    assert_equal 2, palette.send(:sidebar_lines, 30).length
  end

  private

  def running_item(summary:)
    Muxpad::Item.new(token: "running:%1", section: "Running", name: "codex",
                     description: "window 1", command: "codex", directory: nil,
                     state: "running", state_kind: :running, summary:)
  end
end
