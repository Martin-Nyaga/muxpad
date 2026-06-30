# frozen_string_literal: true

require "json"
require "pathname"
require "shellwords"
require "yaml"

module Muxpad
  class Discovery
    LIFECYCLE_SCRIPTS = %w[
      dependencies install postinstall postpack postpublish postversion
      preinstall prepack prepare prepublish prepublishOnly preversion version
    ].freeze

    def scripts(root, exclude: [])
      root = File.expand_path(root)
      root_package = read_package(File.join(root, "package.json"))
      return {} unless root_package

      manager = package_manager(root, root_package)
      packages(root, root_package).to_h do |directory, package|
        package_name = package.fetch("name", File.basename(directory)).to_s
        relative = Pathname.new(directory).relative_path_from(Pathname.new(root)).to_s
        prefix = relative == "." ? nil : package_name
        definitions = filtered_scripts(package.fetch("scripts", {})).filter_map do |name, body|
          id = prefix ? "#{prefix}:#{name}" : name
          next if exclude.any? { |pattern| File.fnmatch?(pattern, id, File::FNM_EXTGLOB) }
          command = script_command(manager, name)
          definition = Definition.new(
            id:, name: id, description: body.to_s, command:, directory: relative,
            placement: "window", exit_mode: "keep-on-error", enabled: true, executable: manager
          )
          [id, definition]
        end
        [directory, definitions]
      end.values.flatten(1).to_h
    rescue Errno::ENOENT, Errno::EACCES, JSON::ParserError, Psych::Exception
      {}
    end

    private

    def packages(root, root_package)
      entries = [[root, root_package]]
      workspace_patterns(root, root_package).each do |pattern|
        next if pattern.start_with?("!")
        Dir.glob(File.join(root, pattern, "package.json")).sort.each do |path|
          next unless Pathname.new(path).expand_path.to_s.start_with?(root + File::SEPARATOR)
          package = read_package(path)
          entries << [File.dirname(path), package] if package
        end
      end
      entries.uniq { |directory, _| directory }
    end

    def workspace_patterns(root, root_package)
      workspaces = root_package["workspaces"]
      patterns = case workspaces
      when Array then workspaces
      when Hash then Array(workspaces["packages"])
      else []
      end
      pnpm_path = File.join(root, "pnpm-workspace.yaml")
      if File.exist?(pnpm_path)
        begin
          pnpm = YAML.safe_load_file(pnpm_path, permitted_classes: [], aliases: false) || {}
          patterns += Array(pnpm["packages"])
        rescue Psych::Exception
          # A broken workspace file should not hide valid root-package scripts.
        end
      end
      patterns.map(&:to_s).uniq
    end

    def filtered_scripts(value)
      return {} unless value.is_a?(Hash)
      names = value.keys.map(&:to_s)
      value.to_h.reject do |name, _|
        name = name.to_s
        LIFECYCLE_SCRIPTS.include?(name) || hook_for_existing_script?(name, names)
      end
    end

    def hook_for_existing_script?(name, names)
      %w[pre post].any? { |prefix| name.start_with?(prefix) && names.include?(name.delete_prefix(prefix)) }
    end

    def package_manager(root, package)
      declared = package["packageManager"].to_s
      declared_manager = declared.split("@", 2).first
      return declared_manager if %w[pnpm yarn bun npm].include?(declared_manager)
      return "pnpm" if File.exist?(File.join(root, "pnpm-lock.yaml"))
      return "yarn" if File.exist?(File.join(root, "yarn.lock"))
      return "bun" if %w[bun.lock bun.lockb].any? { |file| File.exist?(File.join(root, file)) }
      "npm"
    end

    def script_command(manager, name)
      name = Shellwords.escape(name.to_s)
      manager == "npm" ? "npm run #{name}" : "#{manager} #{name}"
    end

    def read_package(path)
      value = JSON.parse(File.read(path))
      value if value.is_a?(Hash)
    rescue Errno::ENOENT, Errno::EACCES, JSON::ParserError
      nil
    end
  end
end
