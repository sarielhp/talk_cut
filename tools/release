#!/usr/bin/env ruby
# frozen_string_literal: true

require_relative 'tooling'

HELP = <<~TEXT
  Usage: tools/release [--install] <message>

  Publish a release: commit, bump the version, push, and optionally install.

    1. Commit the pending work as <message>, after the fast gate (tools/commit).
    2. Bump the patch version, run VERSION_UPDATE, run the full gate, commit
       "chore: bump version to X.Y.Z" and push it to the upstream branch,
       verified (tools/bump).
    3. With --install, install the built artifacts to INSTALL_TARGETS
       (tools/install).

  Steps 1 and 2 change the remote branch and are not undone by rerunning. If a
  step fails, the message says what was published; an interrupted bump is
  finished with tools/bump --resume.

  Options:
    --install   Also install to the targets declared in tools/settings.rb.
    -h, --help  Show this help; nothing is run.
TEXT

Tooling.entry(HELP) do
  install_requested = ARGV.first == '--install'
  ARGV.shift if install_requested
  abort 'Usage: tools/release [--install] <message>' unless ARGV.length == 1
  Tooling.release(Tooling.commit_message(ARGV.first), install_requested)
end
