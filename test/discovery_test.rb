# frozen_string_literal: true

require_relative "test_helper"

class DiscoveryTest < MuxpadTest
  def test_discovers_root_and_workspace_scripts_with_noise_filtering
    root = File.join(@tmp, "project")
    package = File.join(root, "packages", "mobile")
    FileUtils.mkdir_p(package)
    File.write(File.join(root, "package.json"), JSON.generate(
      name: "root", packageManager: "pnpm@9.0.0", workspaces: ["packages/*"],
      scripts: { dev: "vite", predev: "setup", postinstall: "setup", lint: "eslint ." }
    ))
    File.write(File.join(package, "package.json"), JSON.generate(
      name: "app-mobile", scripts: { dev: "expo start", "translations:check": "node check.js" }
    ))

    scripts = Muxpad::Discovery.new.scripts(root, exclude: ["app-mobile:translations:*"])

    assert_equal %w[app-mobile:dev dev lint], scripts.keys.sort
    assert_equal "pnpm dev", scripts.fetch("dev").command
    assert_equal "packages/mobile", scripts.fetch("app-mobile:dev").directory
    assert_equal "expo start", scripts.fetch("app-mobile:dev").description
  end

  def test_uses_lockfiles_then_npm_fallback
    root = File.join(@tmp, "project")
    FileUtils.mkdir_p(root)
    File.write(File.join(root, "package.json"), JSON.generate(scripts: { test: "vitest" }))
    File.write(File.join(root, "yarn.lock"), "")

    assert_equal "yarn test", Muxpad::Discovery.new.scripts(root).fetch("test").command
    FileUtils.rm(File.join(root, "yarn.lock"))
    assert_equal "npm run test", Muxpad::Discovery.new.scripts(root).fetch("test").command
  end

  def test_invalid_or_missing_package_files_produce_no_scripts
    File.write(File.join(@tmp, "package.json"), "not json")
    assert_empty Muxpad::Discovery.new.scripts(@tmp)
    assert_empty Muxpad::Discovery.new.scripts(File.join(@tmp, "missing"))
  end
end
