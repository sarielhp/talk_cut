#!/usr/bin/env ruby
# frozen_string_literal: true

require_relative 'tooling'

HELP = <<~TEXT
  Usage: tools/install

  Build, then copy each artifact to its INSTALL_TARGETS destination from
  tools/settings.rb and verify the copy's contents and permissions.

  Options:
    -h, --help  Show this help; nothing is run.
TEXT

Tooling.entry(HELP) do
  abort 'Usage: tools/install' unless ARGV.empty?
  Tooling.install
end
