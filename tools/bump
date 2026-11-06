#!/usr/bin/env ruby
# frozen_string_literal: true

require_relative 'tooling'

HELP = <<~TEXT
  Usage: tools/bump [--resume|--abandon]

  Release the committed tree. The working tree must be clean.

    1. With UPDATE_TOOLCHAIN, move go.mod to the latest patch of its toolchain.
    2. Bump the patch version and run VERSION_UPDATE.
    3. Run the full gate (only the build, when the tree was already fully
       verified), commit "chore: bump version to X.Y.Z", and push it to the
       upstream branch, verified.

  This changes the remote branch. Nothing is tagged or installed.

  Options:
    --resume    Finish a bump that stopped after its local commit, without
                bumping the version again. Refused if HEAD has moved.
    --abandon   Drop a stopped bump that published nothing: remove its
                unpushed commit, or restore the version file.
    -h, --help  Show this help; nothing is run.
TEXT

Tooling.entry(HELP) do
  abort 'Usage: tools/bump [--resume|--abandon]' unless ARGV.empty? || [['--resume'], ['--abandon']].include?(ARGV)
  case ARGV.first
  when '--resume' then Tooling.resume
  when '--abandon' then Tooling.abandon
  else Tooling.bump
  end
end
