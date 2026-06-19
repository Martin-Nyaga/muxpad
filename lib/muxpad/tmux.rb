# frozen_string_literal: true

require "open3"
require "shellwords"

module Muxpad
  Pane = Data.define(:id, :session, :window, :window_index, :kind, :definition_id, :name, :dead, :finished, :current_command, :title) do
    # A managed command has run to completion when its pane is a dropped-to
    # shell (finished) or a retained corpse (dead).
    def done? = dead || finished
  end

  class Tmux
    FORMAT = ['#{pane_id}', '#{session_name}', '#{window_id}', '#{window_index}', '#{@muxpad_kind}', '#{@muxpad_id}', '#{@muxpad_name}', '#{pane_dead}', '#{@muxpad_finished}', '#{pane_current_command}', '#{pane_title}'].join("\t")

    def initialize
      @prefix = [ENV.fetch("MUXPAD_TMUX", "tmux")]
      @prefix += ["-L", ENV["MUXPAD_TMUX_SOCKET"]] if ENV["MUXPAD_TMUX_SOCKET"]
    end

    def available?
      system(*@prefix, "-V", out: File::NULL, err: File::NULL)
    end

    def inside?
      !ENV["TMUX"].to_s.empty?
    end

    def current_session
      capture("display-message", "-p", '#{session_name}').strip
    end

    def current_pane
      capture("display-message", "-p", '#{pane_id}').strip
    end

    def session_exists?(name)
      run(*@prefix, "has-session", "-t", "=#{name}", allow_failure: true).success?
    end

    def sessions
      capture("list-sessions", "-F", '#{session_name}', allow_failure: true).lines(chomp: true)
    end

    def create_session(name, root, project_id: nil)
      pane = capture("new-session", "-d", "-P", "-F", '#{pane_id}', "-s", name, "-c", root, "-n", "shell").strip
      run! "set-option", "-t", name, "@muxpad_root", root
      run! "set-option", "-t", name, "@muxpad_project", project_id.to_s
      run! "set-option", "-w", "-t", "#{name}:shell", "automatic-rename", "off"
      run! "select-pane", "-t", pane, "-T", "shell"
      pane
    end

    def project_context(session)
      capture("show-options", "-qv", "-t", session, "@muxpad_project", allow_failure: true).strip
    end

    def session_root(session)
      root = managed_root(session)
      return root unless root.empty?
      capture("display-message", "-p", "-t", session, '#{pane_current_path}').strip
    end

    # The directory a muxpad-managed session was created for. Empty for sessions
    # muxpad did not create, so it can be used to recognise our own sessions.
    def managed_root(session)
      capture("show-options", "-qv", "-t", session, "@muxpad_root", allow_failure: true).strip
    end

    def panes(session)
      output = capture("list-panes", "-s", "-t", session, "-F", FORMAT)
      output.lines(chomp: true).filter_map do |line|
        fields = line.split("\t", -1)
        next if fields.length < 11
        Pane.new(id: fields[0], session: fields[1], window: fields[2], window_index: fields[3], kind: fields[4], definition_id: fields[5], name: fields[6], dead: fields[7] == "1", finished: fields[8] == "1", current_command: fields[9], title: fields[10])
      end
    end

    def launch(session:, definition:, kind:, name:, root:, placement:, target: nil)
      sync_path(session)
      directory = definition.directory ? File.expand_path(definition.directory, root) : root
      command = wrapped_command(definition.command, definition.exit_mode)
      placeholder = "sh -c 'while :; do sleep 60; done'"
      args = if placement == "window"
        target = "#{session}:#{next_window_index(session)}"
        ["new-window", "-d", "-P", "-F", '#{pane_id}', "-t", target, "-n", name, "-c", directory, placeholder]
      else
        flag = placement == "horizontal" ? "-h" : "-v"
        ["split-window", "-d", "-P", "-F", '#{pane_id}', flag, "-t", target || "#{session}:shell", "-c", directory, placeholder]
      end
      pane = capture(*args).strip
      run! "set-option", "-w", "-t", pane, "automatic-rename", "off" if placement == "window"
      run! "set-option", "-p", "-t", pane, "remain-on-exit", "off"
      { "@muxpad_kind" => kind, "@muxpad_id" => definition.id, "@muxpad_name" => name,
        "@muxpad_command" => definition.command, "@muxpad_directory" => directory,
        "@muxpad_exit_mode" => definition.exit_mode, "@muxpad_finished" => "0" }.each do |key, value|
        run! "set-option", "-p", "-t", pane, key, value
      end
      run! "select-pane", "-t", pane, "-T", name
      launched = panes(session).find { |item| item.id == pane }
      focus(launched) if launched
      run! "respawn-pane", "-k", "-t", pane, "-c", directory, command
      pane
    end

    def focus(pane)
      run! "select-window", "-t", pane.window
      run! "select-pane", "-t", pane.id
    end

    def restart(pane, definition)
      raise Error, "#{pane.name} is still running" unless pane.done?
      directory = capture("show-options", "-pqv", "-t", pane.id, "@muxpad_directory").strip
      run! "set-option", "-p", "-t", pane.id, "remain-on-exit", "off"
      run! "set-option", "-p", "-t", pane.id, "@muxpad_command", definition.command
      run! "set-option", "-p", "-t", pane.id, "@muxpad_exit_mode", definition.exit_mode
      run! "set-option", "-p", "-t", pane.id, "@muxpad_finished", "0"
      run! "respawn-pane", "-k", "-t", pane.id, "-c", directory, wrapped_command(definition.command, definition.exit_mode)
      run! "select-pane", "-t", pane.id, "-T", pane.name
      focus(pane)
    end

    def attach(session)
      exec(*@prefix, "attach-session", "-t", session)
    end

    def switch(session)
      run! "switch-client", "-t", session
    end

    def popup_menu(program)
      command = "MUXPAD_POPUP=1 #{Shellwords.escape(program)} menu"
      run! "display-popup", "-E", "-w", "96", "-h", "22", "-T", " Muxpad ", command
    end

    def kill_session(session)
      run! "kill-session", "-t", session
    end

    private

    def wrapped_command(command, exit_mode)
      return "sh -c #{Shellwords.escape("exec #{command}")}" if exit_mode == "close"
      tmux = @prefix.map { |part| Shellwords.escape(part) }.join(" ")
      # muxpad_drop marks the pane finished and hands it back to an interactive
      # shell so the output stays scrollable and the user can keep working in
      # place. Trapping INT/TERM means an interrupted command still reverts to a
      # shell instead of killing the pane (and the session, if it is the last).
      drop = <<~SH.chomp
        muxpad_drop() {
          if [ "${status:-1}" -eq 0 ]; then label=exited; else label=failed; fi
          printf '\\n[Muxpad] Command %s with status %s. Dropped to a shell; scroll output with prefix + [.\\n' "$label" "$status" >&2
          #{tmux} set-option -p -t "$TMUX_PANE" @muxpad_finished 1
          exec "${SHELL:-/bin/sh}"
        }
        trap 'status=$?; muxpad_drop' INT TERM
      SH
      tail = case exit_mode
      when "keep-on-error"
        "if [ $status -eq 0 ]; then #{tmux} kill-pane -t \"$TMUX_PANE\"; else muxpad_drop; fi"
      when "keep"
        "muxpad_drop"
      end
      inner = "#{drop}\n( #{command}\n); status=$?\n#{tail}"
      "sh -c #{Shellwords.escape(inner)}"
    end

    def sync_path(session)
      run! "set-environment", "-t", session, "PATH", ENV.fetch("PATH", "")
    end

    def next_window_index(session)
      indexes = capture("list-windows", "-t", session, "-F", '#{window_index}').lines.map(&:to_i)
      indexes.max + 1
    end

    def run!(*args)
      result = run(*@prefix, *args)
      raise Error, "tmux #{args.first} failed: #{result.stderr.strip}" unless result.success?
      result
    end

    def capture(*args, allow_failure: false)
      result = run(*@prefix, *args, allow_failure:)
      raise Error, "tmux #{args.first} failed: #{result.stderr.strip}" unless allow_failure || result.success?
      result.stdout
    end

    Result = Data.define(:stdout, :stderr, :status) do
      def success? = status.success?
    end

    def run(*args, allow_failure: false)
      stdout, stderr, status = Open3.capture3(*args)
      Result.new(stdout:, stderr:, status:)
    end
  end
end
