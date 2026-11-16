# frozen_string_literal: true

require 'open3'

module Live
  # One talk_cut process in a detached tmux session on a private tmux server.
  #
  # Every tmux call is argument-array form, so keys and random text never pass
  # through a shell. The program's environment is given on its own command line
  # (`env ...`), because a tmux server started earlier would otherwise supply its
  # own. Its exit status and stderr are written to files in the work directory.
  class Session
    WRAPPER = 'status="$1"; err="$2"; shift 2; "$@" 2>"$err"; echo $? >"$status"'

    attr_reader :name, :cols, :rows

    def initialize(socket:, name:, workdir:, env:, cols: 110, rows: 32)
      @socket = socket
      @name = name
      @workdir = workdir
      @env = env
      @cols = cols
      @rows = rows
      @status_file = File.join(workdir, "#{name}.status")
      @err_file = File.join(workdir, "#{name}.stderr")
    end

    def launch(argv)
      kill
      File.delete(@status_file) if File.exist?(@status_file)
      env_args = @env.map { |k, v| "#{k}=#{v}" }
      tmux('new-session', '-d', '-s', @name, '-x', @cols.to_s, '-y', @rows.to_s, '-c', @workdir, '--',
           'env', '-u', 'TALK_CUT_PLAYER', *env_args, 'sh', '-c', WRAPPER, 'sh', @status_file, @err_file, *argv)
      # Keep the last screen after the program ends, so a crash can be inspected.
      Open3.capture3('tmux', '-L', @socket, 'set-option', '-t', @name, 'remain-on-exit', 'on')
    end

    # Sends tmux key names, such as "j", "Enter", "M-Left" or "F1".
    def keys(*names)
      tmux('send-keys', '-t', @name, *names)
    end

    # Types text literally.
    def text(str)
      tmux('send-keys', '-t', @name, '-l', '--', str)
    end

    # Sets the size the next launch uses.
    def preset_size(cols, rows)
      @cols = cols
      @rows = rows
    end

    def resize(cols, rows)
      @cols = cols
      @rows = rows
      tmux('resize-window', '-t', @name, '-x', cols.to_s, '-y', rows.to_s)
    end

    def screen(ansi: false)
      args = ['capture-pane', '-t', @name, '-p']
      args << '-e' if ansi
      out, _, status = Open3.capture3('tmux', '-L', @socket, *args)
      status.success? ? out : ''
    end

    # Waits until the screen contains pattern (String or Regexp); returns the screen or nil.
    def wait_for(pattern, timeout: 10.0)
      deadline = Time.now + timeout
      while Time.now < deadline
        snap = screen
        return snap if pattern.is_a?(Regexp) ? snap.match?(pattern) : snap.include?(pattern)
        return nil if exited?

        sleep 0.05
      end
      nil
    end

    def wait_exit(timeout: 5.0)
      deadline = Time.now + timeout
      sleep 0.05 until exited? || Time.now > deadline
      exited?
    end

    def exited?
      File.exist?(@status_file) && !File.read(@status_file).strip.empty?
    end

    def exit_status
      exited? ? File.read(@status_file).strip.to_i : nil
    end

    def stderr
      File.exist?(@err_file) ? File.read(@err_file) : ''
    end

    def kill
      Open3.capture3('tmux', '-L', @socket, 'kill-session', '-t', @name)
    end

    private

    def tmux(*args)
      _, err, status = Open3.capture3('tmux', '-L', @socket, *args)
      raise "tmux #{args.first} failed: #{err}" unless status.success?
    end
  end
end
