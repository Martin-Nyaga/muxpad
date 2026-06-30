# frozen_string_literal: true

require "fileutils"
require "minitest/autorun"
require "tmpdir"

$LOAD_PATH.unshift(File.expand_path("../lib", __dir__))
require "muxpad"

class MuxpadTest < Minitest::Test
  def setup
    @tmp = File.realpath(Dir.mktmpdir("muxpad-test"))
    @old_env = ENV.to_h
  end

  def teardown
    ENV.replace(@old_env)
    FileUtils.remove_entry(@tmp)
  end

  def write_config(content)
    path = File.join(@tmp, "config.yml")
    File.write(path, content)
    path
  end
end
