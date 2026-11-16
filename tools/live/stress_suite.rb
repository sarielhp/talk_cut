# frozen_string_literal: true

module Live
  # Seeded random input against a running TUI: keys, literal text bursts and resizes.
  #
  # Quitting is allowed (Esc or q can end a screen); the program is relaunched and the
  # run goes on. A non-zero exit, a panic, or a blank screen is a failure, reported
  # with the seed, the step and the last actions; the full action log stays in the run
  # directory.
  class Stress
    NAMED = %w[Up Down Left Right Enter Escape Tab BTab Space BSpace Home End PPage NPage F1 M-Left M-Right
               C-s C-a C-e C-k C-u].freeze
    PUNCTUATION = '[]-=,.;?!@#$%&*()+:'.chars << "'"
    PLAIN = ((('a'..'z').to_a - %w[q]) + ('A'..'Z').to_a + ('0'..'9').to_a + PUNCTUATION).freeze
    # No "/" or "~": typed into the output-path field, they could point a render outside the run directory.
    TEXT = (('a'..'z').to_a + ('A'..'Z').to_a + ('0'..'9').to_a +
            [' ', ' ', '-', '_', '.', ',', '!', '?', 'é', 'ü', '✓', '中', '—', '"']).freeze
    HEALTH_EVERY = 10

    def initialize(ctx, name:, argv:, steps:, seed:)
      @ctx = ctx
      @name = name
      @argv = argv
      @steps = steps
      @rng = Random.new(seed)
      @seed = seed
      @recent = []
      @quits = 0
      @log = File.open(File.join(ctx.run_dir, "stress-#{name}.log"), 'w')
    end

    def run
      @ctx.report.section("Stress: #{@name} (#{@steps} random actions, seed #{@seed})")
      @session = @ctx.session("stress-#{@name}")
      launch
      failure = (1..@steps).lazy.filter_map { |step| act_and_check(step) }.first || responsive?
      finish(failure)
    ensure
      @log.close
      @session&.kill
    end

    private

    def launch
      @session.preset_size(110, 32)
      @session.launch(@argv)
      @ctx.require_marker!(@session)
    end

    def act_and_check(step)
      action = act
      @log.puts("#{step}\t#{action}")
      @log.flush
      @recent = (@recent << "#{step}: #{action}").last(25)
      sleep(@rng.rand(0.01..0.08))
      health(step) if (step % HEALTH_EVERY).zero? || step == @steps
    end

    def act
      roll = @rng.rand
      if roll < 0.45 then key(NAMED.sample(random: @rng))
      elsif roll < 0.75 then key(PLAIN.sample(random: @rng))
      elsif roll < 0.95 then burst
      else resize
      end
    end

    def key(name)
      @session.keys(name)
      "key #{name}"
    end

    def burst
      text = Array.new(@rng.rand(1..40)) { TEXT.sample(random: @rng) }.join
      @session.text(text)
      "text #{text.inspect}"
    end

    def resize
      cols = @rng.rand(40..200)
      rows = @rng.rand(10..60)
      @session.resize(cols, rows)
      "resize #{cols}x#{rows}"
    end

    # Returns a failure description, or nil when the program is healthy (relaunching it after a clean quit).
    def health(step)
      return exited(step) if @session.wait_exit(timeout: 0.2)

      snap = @session.wait_for(/\S/, timeout: 5)
      return "step #{step}: the screen stayed blank for 5s" unless snap
      return "step #{step}: panic on screen" if snap.include?('panic:')

      nil
    end

    # A hung program does not answer Ctrl+C: it must exit with status 0 within 5 seconds.
    def responsive?
      return nil if @session.exited? && @session.exit_status.zero?
    
      @session.keys('C-c')
      return "after the last action: no exit within 5s of Ctrl+C (hung?)" unless @session.wait_exit(timeout: 5)
      return "after the last action: Ctrl+C exit status #{@session.exit_status}" unless @session.exit_status.zero?
    
      nil
    end
    
    def exited(step)
      status = @session.exit_status
      err = @session.stderr
      return "step #{step}: exit status #{status}\n#{err.lines.last(20).join}" if status != 0 || err.include?('panic')

      @quits += 1
      @log.puts("#{step}\tclean quit; relaunched")
      launch
      nil
    end

    def finish(failure)
      detail = failure && [failure, 'last actions:', *@recent, 'last screen:', @session.screen].join("\n")
      @ctx.report.check(failure.nil?,
                        "#{@steps} random actions on #{@name}: no crash, panic or hang; exits on Ctrl+C (#{@quits} clean quit(s), relaunched)",
                        detail)
      @ctx.report.note("action log: #{@log.path}; replay with --seed #{@seed}") if failure
    end
  end
end
