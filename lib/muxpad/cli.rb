# frozen_string_literal: true

require "optparse"

module Muxpad
  class CLI
    def initialize(argv, output: $stdout, error: $stderr)
      @argv = argv.dup
      @output = output
      @error = error
    end

    def run
      raise Error, "tmux is required" unless application.tmux.available?
      command = @argv.shift
      case command
      when "start" then run_start
      when "menu" then run_menu
      when "task" then run_launch(:task)
      when "agent" then run_launch(:agent)
      when "help", "--help", "-h", nil then @output.puts(help)
      else raise Error, "unknown command: #{command}\n\n#{help}"
      end
      0
    rescue Error, OptionParser::ParseError => e
      @error.puts("muxpad: #{e.message}")
      1
    rescue Interrupt
      130
    end

    private

    def application
      @application ||= Application.new(output: @output)
    end

    def run_start
      empty = false
      parser = OptionParser.new { |opts| opts.on("--empty") { empty = true } }
      parser.parse!(@argv)
      raise Error, "start accepts at most one project" if @argv.length > 1
      application.start(project_id: @argv.shift, empty:)
    end

    def run_launch(kind)
      placement = nil
      parser = OptionParser.new do |opts|
        opts.on("--placement PLACE", Config::PLACEMENTS) { |value| placement = value }
        opts.on("--window") { placement = "window" }
        opts.on("--vertical") { placement = "vertical" }
        opts.on("--horizontal") { placement = "horizontal" }
      end
      parser.parse!(@argv)
      raise Error, "#{kind} requires exactly one name" unless @argv.length == 1
      application.public_send(kind, @argv.first, placement:)
    end

    def run_menu
      reject_arguments!
      if application.tmux.inside? && ENV["MUXPAD_POPUP"] != "1"
        application.tmux.popup_menu(File.expand_path($PROGRAM_NAME))
      else
        application.menu
      end
    end

    def reject_arguments!
      raise Error, "menu accepts no arguments" unless @argv.empty?
      true
    end

    def help
      <<~TEXT
        Usage:
          muxpad start [project] [--empty]
          muxpad menu
          muxpad task <name> [--window|--vertical|--horizontal]
          muxpad agent <name> [--window|--vertical|--horizontal]
      TEXT
    end
  end
end
