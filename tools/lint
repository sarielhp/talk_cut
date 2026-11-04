#!/usr/bin/env ruby
# frozen_string_literal: true

require_relative 'tooling'

HELP = <<~TEXT
  Usage: tools/lint

  Static feedback only: go vet, staticcheck and go-audit --strict. No tests
  and no build.

  Options:
    -h, --help  Show this help; nothing is run.
TEXT

Tooling.entry(HELP) do
  abort 'Usage: tools/lint' unless ARGV.empty?
  Tooling.lint
end
