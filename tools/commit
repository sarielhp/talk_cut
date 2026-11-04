#!/usr/bin/env ruby
# frozen_string_literal: true

require_relative 'tooling'

HELP = <<~TEXT
  Usage: tools/commit [--full] <message>

  Run the gate, then stage every change (git add -A) and commit it as
  <message>. Nothing is pushed and the version is not bumped. A release runs
  the full gate, so a commit that passes only the fast gate can still stop it.

  Options:
    --full      Run the full gate instead of the fast one.
    -h, --help  Show this help; nothing is run.
TEXT

Tooling.entry(HELP) do
  full = ARGV.first == '--full'
  ARGV.shift if full
  abort 'Usage: tools/commit [--full] <message>' unless ARGV.length == 1
  Tooling.commit(Tooling.commit_message(ARGV.first), full)
end
