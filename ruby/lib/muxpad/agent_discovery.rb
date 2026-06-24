# frozen_string_literal: true

require "open3"
require "shellwords"

module Muxpad
  class AgentDiscovery
    ProcessInfo = Data.define(:pid, :parent_pid, :command, :arguments)

    CODEX_NONINTERACTIVE = %w[app app-server apply archive cloud completion debug delete doctor exec
                              mcp mcp-server plugin remote-control sandbox unarchive update].freeze
    OPTIONS_WITH_VALUES = %w[-a --ask-for-approval -C --cd -c --config -i --image -m --model
                             -p --profile -s --sandbox --add-dir --disable --enable --remote
                             --remote-auth-token-env].freeze

    def initialize(capture: nil)
      @capture = capture || -> { Open3.capture3("ps", "-A", "-ww", "-o", "pid=,ppid=,comm=,args=") }
    end

    def detect(panes)
      processes = process_table
      return {} if processes.empty?
      children = processes.values.group_by(&:parent_pid)
      panes.filter_map do |pane|
        provider = detect_tree(pane.pid.to_i, processes, children)
        [pane.id, provider] if provider
      end.to_h
    rescue SystemCallError, IOError
      {}
    end

    private

    def process_table
      stdout, _, status = @capture.call
      return {} unless status.success?
      stdout.lines(chomp: true).filter_map do |line|
        match = line.match(/\A\s*(\d+)\s+(\d+)\s+(\S+)\s+(.*)\z/)
        next unless match
        process = ProcessInfo.new(pid: match[1].to_i, parent_pid: match[2].to_i,
                                  command: File.basename(match[3]), arguments: match[4])
        [process.pid, process]
      end.to_h
    end

    def detect_tree(root_pid, processes, children)
      queue = [root_pid]
      until queue.empty?
        pid = queue.shift
        process = processes[pid]
        return "claude" if process && claude?(process)
        return "codex" if process && codex?(process)
        queue.concat(children.fetch(pid, []).map(&:pid))
      end
      nil
    end

    def claude?(process)
      process.command == "claude" && !process.arguments.match?(/(?:\A|\s)(?:-p|--print)(?:\s|=|\z)/)
    end

    def codex?(process)
      arguments = Shellwords.split(process.arguments)
      executable = if process.command == "codex"
        arguments.index { |argument| File.basename(argument) == "codex" } || 0
      elsif %w[node MainThread].include?(process.command)
        arguments.index { |argument| argument.end_with?("/@openai/codex/bin/codex.js") }
      end
      return false unless executable
      !CODEX_NONINTERACTIVE.include?(first_codex_argument(arguments.drop(executable + 1)))
    rescue ArgumentError
      false
    end

    def first_codex_argument(arguments)
      index = 0
      while index < arguments.length
        argument = arguments[index]
        return argument unless argument.start_with?("-")
        index += OPTIONS_WITH_VALUES.include?(argument) && !argument.include?("=") ? 2 : 1
      end
      nil
    end
  end
end
