# frozen_string_literal: true

require "pathname"
require "shellwords"
require "yaml"

module Muxpad
  Definition = Data.define(:id, :name, :description, :command, :directory, :placement, :exit_mode, :enabled, :executable)
  Project = Data.define(:id, :name, :root, :tasks, :default_tasks, :discovery_exclude)

  class Config
    DEFAULT_PATH = File.expand_path("~/.config/muxpad/config.yml")
    PLACEMENTS = %w[window vertical horizontal].freeze
    EXIT_MODES = %w[close keep keep-on-error].freeze
    BUILTIN_AGENTS = {
      "claude" => { "name" => "Claude Code", "description" => "Anthropic coding agent", "command" => "claude", "executable" => "claude" },
      "codex" => { "name" => "Codex", "description" => "OpenAI coding agent", "command" => "codex", "executable" => "codex" },
      "opencode" => { "name" => "OpenCode", "description" => "Open-source coding agent", "command" => "opencode", "executable" => "opencode" }
    }.freeze

    attr_reader :projects, :agents

    def initialize(path = ENV.fetch("MUXPAD_CONFIG", DEFAULT_PATH))
      raw = File.exist?(path) ? YAML.safe_load_file(path, permitted_classes: [], aliases: false) : {}
      raw = {} if raw.nil?
      raise Error, "config must contain a YAML mapping: #{path}" unless raw.is_a?(Hash)

      @projects = parse_projects(raw.fetch("projects", {}), path)
      @agents = parse_agents(raw.fetch("agents", {}), path)
    rescue Psych::Exception => e
      raise Error, "invalid config #{path}: #{e.message}"
    end

    def project(id)
      projects[id]
    end

    def project_for(path)
      expanded = Pathname.new(File.expand_path(path))
      projects.values.select { |project| within?(expanded, Pathname.new(project.root)) }
              .max_by { |project| project.root.length }
    end

    private

    def parse_projects(value, path)
      entries = normalize_entries(value, "projects", path)
      entries.to_h do |id, attrs|
        raise Error, "invalid project identifier: #{id.inspect}" unless id.match?(/\A[a-zA-Z0-9_-]+\z/)
        root = attrs["root"]
        raise Error, "project #{id.inspect} requires root" unless root.is_a?(String) && !root.empty?
        root = File.expand_path(root)
        tasks = normalize_entries(attrs.fetch("tasks", {}), "tasks for #{id}", path).to_h do |task_id, task|
          raise Error, "invalid task identifier: #{task_id.inspect}" unless task_id.match?(/\A[a-zA-Z0-9_-]+\z/)
          command = task["command"]
          raise Error, "task #{id}/#{task_id} requires command" unless command.is_a?(String) && !command.empty?
          raise Error, "task #{id}/#{task_id} requires a display name" unless task["name"].is_a?(String) && !task["name"].empty?
          raise Error, "task #{id}/#{task_id} requires a description" unless task["description"].is_a?(String) && !task["description"].empty?
          directory = task["directory"]
          raise Error, "task #{id}/#{task_id} directory must be relative" if directory && Pathname.new(directory).absolute?
          [task_id, definition(task_id, task, command:, default_exit: "keep-on-error")]
        end
        defaults = Array(attrs.fetch("default_tasks", []))
        missing = defaults - tasks.keys
        raise Error, "project #{id} has unknown default tasks: #{missing.join(', ')}" unless missing.empty?
        discovery = attrs.fetch("discovery", {})
        raise Error, "project #{id} discovery must be a mapping" unless discovery.is_a?(Hash)
        excludes = Array(discovery.fetch("exclude", discovery.fetch(:exclude, []))).map(&:to_s)
        [id, Project.new(id:, name: attrs.fetch("name", id), root:, tasks:, default_tasks: defaults, discovery_exclude: excludes)]
      end
    end

    def parse_agents(value, path)
      overrides = normalize_entries(value, "agents", path)
      unknown = overrides.keys - BUILTIN_AGENTS.keys
      raise Error, "unknown agent overrides: #{unknown.join(', ')}" unless unknown.empty?

      BUILTIN_AGENTS.to_h do |id, defaults|
        override = overrides.fetch(id, {})
        attrs = defaults.merge(override)
        attrs["executable"] = Shellwords.split(attrs["command"]).first if override.key?("command") && !override.key?("executable")
        attrs["enabled"] = !attrs["disabled"] if attrs.key?("disabled") && !attrs.key?("enabled")
        [id, definition(id, attrs, command: attrs["command"], default_exit: "close")]
      end
    end

    def definition(id, attrs, command:, default_exit:)
      raise Error, "#{id} command must be a non-empty string" unless command.is_a?(String) && !command.empty?
      placement = attrs.fetch("placement", "window")
      exit_mode = attrs.fetch("exit_mode", default_exit)
      raise Error, "invalid placement #{placement.inspect} for #{id}" unless PLACEMENTS.include?(placement)
      raise Error, "invalid exit mode #{exit_mode.inspect} for #{id}" unless EXIT_MODES.include?(exit_mode)

      Definition.new(
        id:, name: attrs.fetch("name", id), description: attrs.fetch("description", ""), command:,
        directory: attrs["directory"], placement:, exit_mode:, enabled: attrs.fetch("enabled", true),
        executable: attrs.fetch("executable", Shellwords.split(command).first)
      )
    end

    def normalize_entries(value, label, path)
      case value
      when Hash
        value.to_h do |id, attrs|
          raise Error, "#{label} entry #{id.inspect} must be a mapping in #{path}" unless attrs.is_a?(Hash)
          [id.to_s, attrs.transform_keys(&:to_s)]
        end
      when Array
        value.to_h do |attrs|
          raise Error, "#{label} entries must be mappings in #{path}" unless attrs.is_a?(Hash)
          attrs = attrs.transform_keys(&:to_s)
          id = attrs.delete("id")
          raise Error, "#{label} entry requires id in #{path}" if blank?(id)
          [id.to_s, attrs]
        end
      else
        raise Error, "#{label} must be a mapping or list in #{path}"
      end
    end

    def within?(path, root)
      path == root || path.to_s.start_with?(root.to_s + File::SEPARATOR)
    end

    def blank?(value)
      value.nil? || value.to_s.empty?
    end
  end
end
