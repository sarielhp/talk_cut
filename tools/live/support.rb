# frozen_string_literal: true

require 'fileutils'
require 'open3'

module Live
  # Fatal is raised when a safety precondition fails; the run stops at once.
  class Fatal < StandardError; end

  # Stub programs that stand in for everything talk_cut may launch besides ffmpeg
  # and ffprobe: the browser opener, the video players and the clipboard tools.
  # Each appends its name and arguments to a log, so a test can assert it ran.
  module Stubs
    PROGRAMS = %w[xdg-open mpv vlc cvlc ffplay totem wl-copy xclip xsel].freeze
    SCRIPT = <<~SH
      #!/bin/sh
      name=$(basename "$0")
      case "$name" in xclip|xsel) cat >/dev/null ;; esac
      printf '%s' "$name" >>"$TALK_CUT_LIVE_STUB_LOG"
      for a in "$@"; do printf ' %s' "$a" >>"$TALK_CUT_LIVE_STUB_LOG"; done
      echo >>"$TALK_CUT_LIVE_STUB_LOG"
      exit 0
    SH

    module_function

    # Writes the stubs into dir/bin and returns that directory.
    def install(dir)
      bin = File.join(dir, 'bin')
      FileUtils.mkdir_p(bin)
      PROGRAMS.each do |name|
        path = File.join(bin, name)
        File.write(path, SCRIPT)
        File.chmod(0o755, path)
      end
      bin
    end
  end

  # Collects results: a failed check is reported and the run continues; a Fatal stops it.
  class Report
    attr_reader :failures

    def initialize(verbose: false)
      @verbose = verbose
      @failures = []
      @passes = 0
    end

    def section(title)
      puts "\n\e[1;34m== #{title} ==\e[0m"
    end

    def check(ok, what, detail = nil)
      if ok
        @passes += 1
        puts "  \e[32m✔\e[0m #{what}"
      else
        @failures << what
        puts "  \e[31m✗ #{what}\e[0m"
        puts detail.to_s.lines.map { |l| "      #{l}" }.join if detail
      end
      ok
    end

    def note(msg)
      puts "  \e[33m•\e[0m #{msg}"
    end

    def summary
      color = @failures.empty? ? 32 : 31
      puts "\n\e[1;#{color}m#{@passes} passed, #{@failures.size} failed\e[0m"
      @failures.each { |f| puts "  ✗ #{f}" }
      @failures.empty?
    end
  end

  # Runs talk_cut (or any command) to completion under the read-only environment.
  module Cmd
    module_function

    def run(env, *argv, chdir:)
      out, err, status = Open3.capture3(env, *argv, chdir: chdir, unsetenv_others: false)
      [out, err, status.exitstatus]
    end
  end
end
