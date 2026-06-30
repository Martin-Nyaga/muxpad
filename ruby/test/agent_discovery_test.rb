# frozen_string_literal: true

require_relative "test_helper"

class AgentDiscoveryTest < MuxpadTest
  Pane = Data.define(:id, :pid)
  Status = Data.define(:ok) do
    def success? = ok
  end

  def test_detects_interactive_claude_and_node_wrapped_codex_descendants
    discovery = discovery_for(<<~PS)
      100 1 zsh zsh
      101 100 claude /home/me/.local/bin/claude
      200 1 zsh zsh
      201 200 MainThread node /opt/node_modules/@openai/codex/bin/codex.js -c tui.theme=dark
      202 201 codex /opt/codex -c tui.theme=dark
    PS

    assert_equal({ "%1" => "claude", "%2" => "codex" },
                 discovery.detect([Pane.new(id: "%1", pid: "100"), Pane.new(id: "%2", pid: "200")]))
  end

  def test_detects_resumed_codex_after_global_options
    discovery = discovery_for("300 1 codex /opt/codex -c model=gpt-5 resume abc123\n")

    assert_equal({ "%3" => "codex" }, discovery.detect([Pane.new(id: "%3", pid: "300")]))
  end

  def test_ignores_noninteractive_and_unrelated_processes
    discovery = discovery_for(<<~PS)
      100 1 claude /opt/claude --print hello
      200 1 codex /opt/codex app-server
      300 1 node node server.js
      400 1 codex /opt/codex exec run-tests
      500 1 node node server.js codex
    PS

    panes = [100, 200, 300, 400, 500].map { |pid| Pane.new(id: "%#{pid}", pid: pid.to_s) }
    assert_empty discovery.detect(panes)
  end

  def test_returns_nothing_when_ps_fails
    discovery = Muxpad::AgentDiscovery.new(capture: -> { ["", "denied", Status.new(ok: false)] })

    assert_empty discovery.detect([Pane.new(id: "%1", pid: "100")])
  end

  private

  def discovery_for(output)
    Muxpad::AgentDiscovery.new(capture: -> { [output, "", Status.new(ok: true)] })
  end
end
