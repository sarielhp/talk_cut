# frozen_string_literal: true

require 'open3'
require 'fileutils'
require_relative 'support'
require_relative 'session'
require_relative '../tui_fixture'

module Live
  # Shared state of one live-test run: paths, the read-only environment, and helpers.
  class Context
    MARKER = '[READ-ONLY]'

    attr_reader :root, :run_dir, :bin, :env, :stub_bin, :report, :socket
    attr_accessor :youtube, :playlists, :untidy, :before

    def initialize(root:, run_dir:, report:)
      @root = root
      @run_dir = run_dir
      @report = report
      @bin = File.join(run_dir, 'talk_cut')
      @stub_bin = Stubs.install(run_dir)
      @stub_log = File.join(run_dir, 'launched.log')
      File.write(@stub_log, '')
      @socket = "talk_cut_live_#{Process.pid}"
      @env = read_only_env
      @youtube = false
      @playlists = []
      @untidy = []
    end

    def read_only_env
      tmp = File.join(@run_dir, 'tmp')
      FileUtils.mkdir_p(tmp)
      { 'TALK_CUT_READ_ONLY' => '1', 'PATH' => "#{@stub_bin}:#{ENV.fetch('PATH')}",
        'TALK_CUT_LIVE_STUB_LOG' => @stub_log, 'TMPDIR' => tmp }
    end

    # Runs the built binary to completion; returns [stdout, stderr, exit code].
    def talk_cut(*args)
      out, err, st = Open3.capture3(@env.merge('TALK_CUT_PLAYER' => nil), @bin, *args, chdir: @run_dir)
      [out, err, st.exitstatus]
    end

    def fixture(name)
      dir = File.join(@run_dir, "rec-#{name}")
      FileUtils.mkdir_p(dir)
      TUIFixture.write(dir)
    end

    def session(name)
      Session.new(socket: @socket, name: name, workdir: @run_dir, env: @env)
    end

    # Stops the run unless the TUI shows the read-only marker before any key is sent.
    def require_marker!(session)
      return if session.wait_for(MARKER, timeout: 20)

      raise Fatal, "#{session.name}: no #{MARKER} marker on screen; refusing to send keys\n#{session.screen}#{session.stderr}"
    end

    def run_live_tui_test
      env = @env.merge('TALK_CUT_BINARY' => @bin, 'TALK_CUT_TMUX_SOCKET' => @socket)
      out, st = Open3.capture2e(env, 'ruby', File.join(@root, 'tools/live_tui_test.rb'), '-q', '--scroll', chdir: @root)
      File.write(File.join(@run_dir, 'live_tui_test.out'), out)
      [out, st.exitstatus]
    end

    def stub_mark
      File.size(@stub_log)
    end

    # Lines the stubs logged since mark.
    def stub_lines(mark)
      File.open(@stub_log) do |f|
        f.seek(mark)
        f.read.lines.map(&:chomp)
      end
    end

    def wait_until(timeout)
      deadline = Time.now + timeout
      until Time.now > deadline
        val = yield
        return val if val

        sleep 0.1
      end
      nil
    end

    # Waits for pattern in the screen text with box borders and line breaks removed,
    # so a message wrapped across lines still matches.
    def wait_norm(session, pattern, timeout)
      wait_until(timeout) { normalize(session.screen).match?(pattern) }
    end

    def normalize(screen)
      screen.gsub(/[│╭╮╰╯─\n]/, ' ').squeeze(' ')
    end

    def selected_line(session)
      session.screen.lines.find { |l| l.include?(' > ') }.to_s.strip
    end

    # Moves the playlist cursor to the playlist whose title starts like title; reports success.
    def select_playlist(session, title, limit: 60)
      prefix = title[0, 24]
      limit.times do
        return true if selected_line(session).include?(prefix)

        session.keys('j')
        sleep 0.1
      end
      false
    end
  end
end
