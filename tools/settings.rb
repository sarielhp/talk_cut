# frozen_string_literal: true

# Project choices for the copied Go tooling (see ~/prog/standards/go/README.md).
module ToolSettings
  TOOLS = {}.freeze
  BUILD_PACKAGES = ['./...'].freeze
  ARTIFACTS = { 'talk_cut' => '.' }.freeze
  INSTALL_TARGETS = { 'talk_cut' => File.expand_path('~/bin/talk_cut') }.freeze
  VERSION_FILE = 'VERSION'
  # VERSION is embedded via //go:embed in main.go.
  VERSION_KIND = :embedded
  VERSION_UPDATE = [].freeze
  UPDATE_TOOLCHAIN = true
  FAST_CHECKS = [].freeze
  # Build first so the live headless-tmux TUI test (with the 540-cue scroll
  # test) runs the current binary rather than a stale ./talk_cut.
  FULL_CHECKS = [%w[tools/build], %w[ruby tools/live_tui_test.rb -q --scroll]].freeze
  TEST_TIMEOUT = '180s'
  # Text inputs of the live TUI test (the multi-GB media files are not digested).
  VERIFICATION_INPUTS = ['examples/26_09_08/*.{vtt,json,txt}'].freeze
end
