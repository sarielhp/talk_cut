# frozen_string_literal: true

module Live
  # Command-line checks, run under the read-only lock.
  module CLISuite
    HELP = [
      [%w[--help], 'Interactive talk trimmer'],
      [%w[youtube], 'playlist'],
      [%w[youtube playlist], 'tidy'],
      [%w[youtube playlist tidy --help], '--dry-run'],
      [%w[playlists --help], 'without a recording']
    ].freeze

    module_function

    def run(ctx)
      r = ctx.report
      r.section('Command line')
      HELP.each do |argv, text|
        out, _, code = ctx.talk_cut(*argv)
        r.check(code.zero? && out.include?(text), "talk_cut #{argv.join(' ')} shows #{text.inspect}")
      end
      out, err, code = ctx.talk_cut('youtube', 'playlist', 'tidy')
      r.check(code == 1 && err.include?('--all'), 'tidy with no playlist asks for one', out + err)
      check_dry_run(ctx)
      youtube_checks(ctx) if ctx.youtube
    end

    def check_dry_run(ctx)
      dir = ctx.fixture('cli')
      out, err, code = ctx.talk_cut('--no-ai', '--dry-run', dir)
      ctx.report.check(code.zero? && out.include?('540 cues'), 'pipeline --dry-run reads the synthetic recording', out + err)
    end

    def youtube_checks(ctx)
      r = ctx.report
      out, err, code = ctx.talk_cut('youtube', 'status')
      r.check(code.zero?, 'youtube status', out + err)
      first = ctx.playlists.first
      return unless first

      _, err, code = ctx.talk_cut('youtube', 'playlist', 'set-default', first['id'])
      r.check(code == 1 && err.include?('read-only'), 'set-default is refused (config write)', err)
      return r.note('every playlist is tidy; the refused-tidy check is skipped') if ctx.untidy.empty?

      target = ctx.untidy.first
      _, err, code = ctx.talk_cut('youtube', 'playlist', 'tidy', target['id'])
      r.check(code == 1 && err.include?('read-only'), "tidy of #{target['title'].inspect} is refused", err)
    end
  end
end
