# frozen_string_literal: true

require 'digest'
require 'open3'

module Live
  # Safety checks before any key is sent, and the read-only state snapshot.
  module Preflight
    LOCK_TESTS = ['go', 'test', '-count=1', '-run', 'TestReadOnly|TestGuard|TestCheck|TestParse',
                  './internal/readonly', './internal/youtube'].freeze
    LIST_LINE = /^\s+• \[(?<id>[^\]]+)\] (?<title>.+) \(\d+ videos, \w+\)/

    module_function

    def run(ctx)
      r = ctx.report
      r.section('Preflight: any failure here stops the run')
      require_tools!
      fatal_unless(r, sh(ctx, 'go', 'build', '-o', ctx.bin, '.'), 'build talk_cut into the run directory')
      fatal_unless(r, lock_tests_pass?(ctx), 'Go read-only lock tests ran and passed (every write entry point refuses)')
      check_banner!(ctx)
      check_stubs!(ctx)
      probe_youtube(ctx)
      ctx.before = capture_state(ctx)
    end

    def require_tools!
      %w[tmux ffmpeg ffprobe go].each do |tool|
        _, _, st = Open3.capture3('which', tool)
        raise Fatal, "#{tool} is required" unless st.success?
      end
    end

    def sh(ctx, *argv)
      _, err, st = Open3.capture3(*argv, chdir: ctx.root)
      warn err unless st.success?
      st.success?
    end

    # Exit status 0 is not enough: a renamed or skipped test would pass vacuously.
    def lock_tests_pass?(ctx)
      out, st = Open3.capture2e(*LOCK_TESTS, '-v', chdir: ctx.root)
      warn out unless st.success?
      st.success? && out.include?('--- PASS: TestReadOnlyLock ') && out.include?('--- PASS: TestGuardPassesOnlyReads')
    end

    def fatal_unless(report, ok, what)
      report.check(ok, what)
      raise Fatal, what unless ok
    end

    def check_banner!(ctx)
      _, err, code = ctx.talk_cut('--version')
      fatal_unless(ctx.report, code.zero? && err.include?('READ-ONLY mode'), 'the binary reports READ-ONLY mode')
    end

    def check_stubs!(ctx)
      Stubs::PROGRAMS.each do |prog|
        out, = Open3.capture3(ctx.env, 'sh', '-c', "command -v #{prog}")
        next if out.strip.start_with?(ctx.stub_bin)

        fatal_unless(ctx.report, false, "#{prog} resolves to the stub (got #{out.strip.inspect})")
      end
      ctx.report.check(true, "launched programs are stubs (#{Stubs::PROGRAMS.join(', ')})")
    end

    def probe_youtube(ctx)
      out, err, code = ctx.talk_cut('youtube', 'playlist', 'list')
      ctx.youtube = code.zero?
      unless ctx.youtube
        ctx.report.note("YouTube unavailable; YouTube checks are skipped: #{err.lines.last}")
        return
      end
      ctx.playlists = out.lines.filter_map { |l| l.match(LIST_LINE)&.named_captures }
      ctx.report.check(true, "YouTube reachable: #{ctx.playlists.size} playlists")
      dry, = ctx.talk_cut('youtube', 'playlist', 'tidy', '--dry-run', '--all')
      ctx.untidy = untidy_playlists(dry)
      names = ctx.untidy.map { |p| p['title'] }
      ctx.report.note("playlists a tidy would change: #{names.empty? ? 'none' : names.join(', ')}")
    end

    # The playlists ({"title", "id"}) a dry-run tidy reports changes for.
    def untidy_playlists(dry_run_output)
      found = []
      current = nil
      dry_run_output.each_line do |line|
        current = { 'title' => Regexp.last_match(1), 'id' => Regexp.last_match(2) } if line =~ /^=== (.+) \[([^\]]+)\] ===$/
        found << current if current && line.include?('Would remove')
      end
      found.uniq
    end

    # What must be identical before and after the run: the channel's playlists, what a
    # tidy would change, and the user's config, token and secrets files.
    def capture_state(ctx)
      state = { files: config_digests }
      return state unless ctx.youtube

      state[:list] = ctx.talk_cut('youtube', 'playlist', 'list').first
      state[:dry_run] = ctx.talk_cut('youtube', 'playlist', 'tidy', '--dry-run', '--all').first
      state
    end

    def config_digests
      paths = Dir.glob(File.expand_path('~/.config/talk_cut/*')) + Dir.glob(File.expand_path('~/.config/auth/*.json'))
      paths.select { |p| File.file?(p) }.sort.to_h { |p| [p, Digest::SHA256.file(p).hexdigest] }
    end
  end
end
