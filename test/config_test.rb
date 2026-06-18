# frozen_string_literal: true

require_relative "test_helper"

class ConfigTest < MuxpadTest
  def test_loads_multiple_projects_and_resolves_the_deepest_root
    parent = File.join(@tmp, "parent")
    child = File.join(parent, "child")
    FileUtils.mkdir_p(child)
    config = Muxpad::Config.new(write_config(<<~YAML))
      projects:
        parent:
          root: #{parent}
          tasks: {}
        child:
          root: #{child}
          tasks: {}
    YAML

    assert_equal %w[parent child], config.projects.keys
    assert_equal "child", config.project_for(File.join(child, "nested")).id
    assert_nil config.project_for(@tmp)
  end

  def test_validates_default_tasks_and_definition_values
    error = assert_raises(Muxpad::Error) do
      Muxpad::Config.new(write_config(<<~YAML))
        projects:
          broken:
            root: #{@tmp}
            default_tasks: [missing]
            tasks: {}
      YAML
    end
    assert_match(/unknown default tasks/, error.message)

    error = assert_raises(Muxpad::Error) do
      Muxpad::Config.new(write_config(<<~YAML))
        projects:
          broken:
            root: #{@tmp}
            tasks:
              unnamed:
                command: 'true'
      YAML
    end
    assert_match(/display name/, error.message)
  end

  def test_built_in_agents_can_be_overridden_and_disabled
    config = Muxpad::Config.new(write_config(<<~YAML))
      agents:
        codex:
          command: my-codex --fast
        claude:
          disabled: true
    YAML

    assert_equal "my-codex --fast", config.agents.fetch("codex").command
    assert_equal "my-codex", config.agents.fetch("codex").executable
    refute config.agents.fetch("claude").enabled
    assert_equal %w[claude codex opencode], config.agents.keys
  end

  def test_loads_project_discovery_exclusions
    config = Muxpad::Config.new(write_config(<<~YAML))
      projects:
        work:
          root: #{@tmp}
          tasks: {}
          discovery:
            exclude:
              - "*:postinstall"
              - "mobile:translations:*"
    YAML

    assert_equal ["*:postinstall", "mobile:translations:*"], config.project("work").discovery_exclude
  end
end
