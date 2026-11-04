#!/usr/bin/env ruby
# frozen_string_literal: true

require_relative 'tooling'

HELP = <<~TEXT
  Usage: tools/build

  Build the declared artifacts into bin/. Nothing is installed.

  Options:
    -h, --help  Show this help; nothing is run.
TEXT

Tooling.entry(HELP) do
  abort 'Usage: tools/build' unless ARGV.empty?
  Tooling.build
end
