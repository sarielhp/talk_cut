#!/usr/bin/env ruby
# frozen_string_literal: true

require_relative 'tooling'

HELP = <<~TEXT
  Usage: tools/gate [--full]

  Check the tree without changing it: gofmt, go mod tidy -diff, tools/lint,
  FAST_CHECKS, every test, and the build.

  Options:
    --full      The release gate: adds every test under -race, govulncheck,
                gocritic, shadow, revive and dupl, and FULL_CHECKS.
    -h, --help  Show this help; nothing is run.
TEXT

Tooling.entry(HELP) do
  abort 'Usage: tools/gate [--full]' unless ARGV.empty? || ARGV == ['--full']
  Tooling.gate(ARGV == ['--full'])
end
