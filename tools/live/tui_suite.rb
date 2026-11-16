# frozen_string_literal: true

module Live
  # Specific tasks in the recording TUI (tabs 4-6) and the standalone playlist editor.
  module TUISuite
    module_function

    def run_recording(ctx)
      r = ctx.report
      r.section('Recording TUI: tabs 1-4 (tools/live_tui_test.rb, read-only)')
      out, code = ctx.run_live_tui_test
      failed = out.lines.grep(/✗|Expected to find/).first(4).join
      r.check(code.zero?, 'live_tui_test.rb passes under the lock', "#{failed}full output: #{ctx.run_dir}/live_tui_test.out")

      r.section('Recording TUI: render, YouTube and Playlists tabs')
      dir = ctx.fixture('tabs')
      s = ctx.session('rec')
      s.launch([ctx.bin, '--no-ai', dir])
      ctx.require_marker!(s)
      render_task(ctx, s, dir)
      youtube_tab_tasks(ctx, s)
      playlists_tab_tasks(ctx, s) if ctx.youtube
      quit_task(ctx, s, %w[1 q])
    end

    def render_task(ctx, session, dir)
      session.keys('4')
      ctx.report.check(session.wait_for('PRE-FLIGHT EXPORT REVIEW'), 'tab 4 shows the export review')
      session.keys('Enter')
      rendered = ctx.wait_until(120) { Dir.glob(File.join(dir, '*_cut.mp4')).any? { |f| File.size(f).positive? } }
      ctx.report.check(rendered, 'Enter on tab 4 renders the cut video (local, in the run directory)')
      sleep 1
    end

    def youtube_tab_tasks(ctx, session)
      r = ctx.report
      session.keys('5')
      r.check(session.wait_for('YOUTUBE PUBLISH'), 'tab 5 shows the YouTube pre-flight')
      mark = ctx.stub_mark
      session.keys('P')
      r.check(ctx.wait_until(5) { ctx.stub_lines(mark).any? { |l| l.match?(/^(mpv|vlc|ffplay|totem) /) } },
              'P launches the cut preview in the (stub) player')
      return unless ctx.youtube

      session.keys('u')
      r.check(ctx.wait_norm(session, /read-\s*only/, 60), 'u (upload) is refused by the lock', session.screen)
    end

    def playlists_tab_tasks(ctx, session)
      r = ctx.report
      session.keys('6')
      r.check(session.wait_for(/\(\d+ videos, /, timeout: 30), 'tab 6 lists the channel playlists', session.screen)
      session.keys('Enter')
      r.check(session.wait_for('PLAYLIST:'), 'Enter opens a playlist')
      session.keys('Escape')
      r.check(session.wait_for('YOUTUBE PLAYLISTS'), 'Esc returns to the playlist list')
    end

    def quit_task(ctx, session, keys)
      # One send-keys call per key: keys sent together arrive as one multi-rune key.
      keys.each do |k|
        session.keys(k)
        sleep 0.3
      end
      ok = session.wait_exit(timeout: 10) && session.exit_status.zero? && !session.stderr.include?('panic')
      ctx.report.check(ok, 'quits cleanly (exit 0, no panic)', session.stderr + session.screen)
      session.kill
    end

    def run_editor(ctx)
      r = ctx.report
      r.section('Playlist editor (talk_cut playlists)')
      return r.note('skipped: YouTube unavailable') unless ctx.youtube

      s = ctx.session('editor')
      s.launch([ctx.bin, 'playlists'])
      ctx.require_marker!(s)
      r.check(s.wait_for(/\(\d+ videos, /, timeout: 30), 'loads the channel playlists', s.screen)
      navigation_tasks(ctx, s)
      membership_task(ctx, s)
      create_tasks(ctx, s)
      EditorTidy.run(ctx, s)
      s.keys('r')
      r.check(s.wait_for(/\(\d+ videos, /, timeout: 30), 'r reloads the playlists')
      quit_task(ctx, s, %w[q])
    end

    def navigation_tasks(ctx, session)
      r = ctx.report
      before = ctx.selected_line(session)
      session.keys('j')
      r.check(ctx.wait_until(3) { ctx.selected_line(session) != before }, 'j moves the cursor')
      session.keys('k')
      r.check(ctx.wait_until(3) { ctx.selected_line(session) == before }, 'k moves it back')
      mark = ctx.stub_mark
      session.keys('o')
      r.check(ctx.wait_until(5) { ctx.stub_lines(mark).any? { |l| l.start_with?('xdg-open https://www.youtube.com/playlist?list=') } },
              'o opens the playlist URL (stub xdg-open)')
      mark = ctx.stub_mark
      session.keys('c')
      r.check(ctx.wait_until(5) { ctx.stub_lines(mark).any? { |l| l.match?(/^(wl-copy|xclip|xsel)/) } } &&
              session.wait_for('Copied link'), 'c copies the share link (stub clipboard)')
    end

    def membership_task(ctx, session)
      r = ctx.report
      session.keys('Enter')
      r.check(session.wait_for('PLAYLIST:'), 'Enter opens the membership view')
      loaded = ctx.wait_until(30) { !session.screen.include?('Reading playlist contents') }
      r.check(loaded && session.screen.match?(/\[[ ✔]\]/), 'membership marks load', session.screen)
      session.keys('Space')
      r.check(ctx.wait_norm(session, /update failed:.*read-\s*only/i, 30), 'Space (add/remove) is refused by the lock', session.screen)
      session.keys('Escape')
      r.check(session.wait_for('YOUTUBE PLAYLISTS'), 'Esc returns to the list')
    end

    def create_tasks(ctx, session)
      r = ctx.report
      session.keys('n')
      r.check(session.wait_for('NEW YOUTUBE PLAYLIST'), 'n opens the new-playlist form')
      session.text('Live test ✓ ünïcode – 中文')
      session.keys('Tab')
      r.check(session.wait_for('Live test ✓'), 'the title field takes Unicode text')
      session.keys('Escape')
      r.check(session.wait_for('YOUTUBE PLAYLISTS'), 'Esc cancels the form')
      session.keys('n')
      session.text('talk_cut live test')
      session.keys('Enter')
      r.check(ctx.wait_norm(session, /Failed to create playlist:.*read-\s*only/i, 30), 'creating a playlist is refused by the lock',
              session.screen)
      session.keys('Escape') # a refused create leaves the form open
      r.check(session.wait_for('YOUTUBE PLAYLISTS'), 'Esc closes the form after the refusal')
    end
  end

  # The tidy confirmation flow in the editor, on a playlist that needs a tidy when there is one.
  module EditorTidy
    module_function

    def run(ctx, session)
      target = ctx.untidy.first
      return already_tidy(ctx, session) unless target && ctx.select_playlist(session, target['title'])

      r = ctx.report
      session.keys('t')
      r.check(ctx.wait_norm(session, /\[y\]\s*apply/, 30), "t plans a tidy of #{target['title'].inspect} and asks first", session.screen)
      session.keys('n')
      r.check(ctx.wait_norm(session, /Tidy\s+cancelled/, 5), 'any other key cancels it')
      session.keys('t')
      ctx.wait_norm(session, /\[y\]\s*apply/, 30)
      session.keys('y')
      r.check(ctx.wait_norm(session, /failed:.*read-\s*only/i, 30), 'y is refused by the lock', session.screen)
    end

    def already_tidy(ctx, session)
      session.keys('t')
      ctx.report.check(ctx.wait_norm(session, /already\s+tidy/, 30), 't on a tidy playlist says so', session.screen)
    end
  end
end
