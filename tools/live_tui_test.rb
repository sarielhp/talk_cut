#!/usr/bin/env ruby
# frozen_string_literal: true

require 'open3'
require 'tmpdir'
require 'fileutils'
require 'optparse'

# live_tui_test.rb
# Live interactive character-level snapshot testing for talk_cut TUI.
# Uses headless tmux to spawn the TUI, send keystrokes, capture the exact 2D
# character matrix, and assert screen transitions and data persistence.

class TUISnapshotTester
  attr_reader :session_name, :tmp_dir, :cols, :rows, :snapshots

  def initialize(cols: 110, rows: 32, verbose: true, save_dir: nil)
    @cols = cols
    @rows = rows
    @verbose = verbose
    @save_dir = save_dir
    @session_name = "tui_test_#{Process.pid}_#{Time.now.to_i}"
    @snapshots = []
    @step_num = 0
  end

  def run!
    check_dependencies!
    setup_sandbox!

    puts "\e[1;34m=== Starting Live Character-Level TUI Test (#{cols}x#{rows}) ===\e[0m\n"

    # Step 1: Launch talk_cut in headless tmux
    log_step("Launch talk_cut in headless session")
    launch_app!
    snap = capture_snapshot("01_initial_cuts_view")
    assert_contains(snap, "TRANSCRIPT", "Should display transcript pane header")
    assert_contains(snap, "STATISTICS", "Should display statistics pane header")
    assert_contains(snap, "CUT REGIONS", "Should display cut regions pane header")
    assert_contains(snap, "Cue #1 of 540", "Should focus first cue in bottom card")
    assert_contains(snap, "Boris Aronov", "Should display speaker of first cue")
    assert_contains(snap, "Go ahead.", "Should display cue text in bottom card")

    # Step 2: Navigate down (j)
    log_step("Navigate down with 'j'")
    send_key("j")
    sleep 0.2
    snap = capture_snapshot("02_cursor_down_cue2")
    assert_contains(snap, "Cue #2 of 540", "Bottom card should update to Cue #2")
    assert_contains(snap, "Adam Sheffer", "Bottom card should show Cue #2 speaker")
    assert_contains(snap, "Great!", "Bottom card should show Cue #2 text")

    # Step 3: Toggle cut on Cue #2 (Space)
    log_step("Toggle cut on Cue #2 with [Space]")
    send_key("Space")
    sleep 0.25
    snap = capture_snapshot("03_toggle_cut_cue2")
    assert_contains(snap, "[CUT]  Great!", "Cue #2 should now show [CUT] badge in transcript")
    assert_contains(snap, "marked CUT", "Status line should confirm cut toggled and saved")
    assert_contains(snap, "CUT REGIONS (3)", "Cut regions counter should update")

    # Step 4: Switch to Metadata & Chapters view (Tab)
    log_step("Switch to Metadata view with [Tab]")
    send_key("Tab")
    sleep 0.25
    snap = capture_snapshot("04_metadata_view")
    assert_contains(snap, "METADATA & CHAPTERS", "Header should show Metadata view")
    assert_contains(snap, "TALK & EXPORT CONFIGURATION", "Should display form header")
    assert_contains(snap, "YOUTUBE CHAPTERS PREVIEW", "Should display chapters preview pane")
    assert_contains(snap, "UPLOAD PREVIEW", "Should display upload preview pane")

    # Step 5: Return to Cut Review view (Escape)
    log_step("Return to Cuts view with [Esc]")
    send_key("Escape")
    sleep 0.25
    snap = capture_snapshot("05_return_cuts_view")
    assert_contains(snap, "TRANSCRIPT", "Should return to transcript pane")
    assert_contains(snap, "[CUT]  Great!", "Cue #2 should still be marked [CUT]")

    # Step 6: Quit application cleanly (q)
    log_step("Quit application with 'q'")
    send_key("q")
    sleep 0.3
    assert_session_closed

    # Step 7: Verify persistence by re-launching talk_cut
    log_step("Re-launch talk_cut to verify auto-saved talk_cuts.json persistence")
    launch_app!
    snap = capture_snapshot("06_reloaded_from_disk")
    assert_contains(snap, "[CUT]  Great!", "Persisted talk_cuts.json should restore Cue #2 as [CUT]")
    assert_contains(snap, "CUT REGIONS (3)", "Persisted cut intervals should be restored")

    # Final quit
    send_key("q")
    sleep 0.3
    assert_session_closed

    puts "\n\e[1;32m✔ All live TUI snapshot assertions passed successfully!\e[0m"
  ensure
    teardown!
  end

  private

  def check_dependencies!
    `which tmux 2>/dev/null`.strip.tap do |p|
      if p.empty?
        abort "\e[31mError: tmux is required for headless character-level TUI testing.\e[0m"
      end
    end
    binary = File.expand_path('../talk_cut', __dir__)
    unless File.exist?(binary)
      puts "Building talk_cut binary first..."
      system('go build -o talk_cut .', exception: true)
    end
  end

  def setup_sandbox!
    @tmp_dir = Dir.mktmpdir('talk_cut_tui_test_')
    example_dir = File.expand_path('../examples/26_09_08', __dir__)
    Dir.glob("#{example_dir}/*").each do |file|
      FileUtils.ln_sf(file, File.join(@tmp_dir, File.basename(file)))
    end
  end

  def launch_app!
    binary = File.expand_path('../talk_cut', __dir__)
    cmd = "tmux new-session -d -s #{@session_name} -x #{@cols} -y #{@rows} '#{binary} --no-ai #{@tmp_dir}'"
    stdout, stderr, status = Open3.capture3(cmd)
    unless status.success?
      abort "\e[31mFailed to launch tmux session:\e[0m #{stderr}"
    end
    # Allow initial render
    sleep 0.4
  end

  def send_key(key)
    cmd = "tmux send-keys -t #{@session_name} #{key}"
    _, stderr, status = Open3.capture3(cmd)
    unless status.success?
      abort "\e[31mFailed to send key '#{key}':\e[0m #{stderr}"
    end
  end

  def capture_snapshot(name)
    cmd = "tmux capture-pane -t #{@session_name} -p"
    stdout, stderr, status = Open3.capture3(cmd)
    unless status.success?
      abort "\e[31mFailed to capture tmux pane:\e[0m #{stderr}"
    end

    @snapshots << { name: name, content: stdout }

    if @save_dir
      FileUtils.mkdir_p(@save_dir)
      File.write(File.join(@save_dir, "#{name}.txt"), stdout)
    end

    if @verbose
      print_snapshot_frame(name, stdout)
    end

    stdout
  end

  def print_snapshot_frame(name, content)
    separator = "─" * (@cols)
    puts "\e[1;36m┌#{separator}┐\e[0m"
    puts "\e[1;36m│ SNAPSHOT: %-#{@cols - 12}s │\e[0m" % name
    puts "\e[1;36m├#{separator}┤\e[0m"
    content.lines.each do |line|
      puts "  " + line.chomp
    end
    puts "\e[1;36m└#{separator}┘\e[0m\n"
  end

  def assert_contains(screen, pattern, msg)
    if screen.include?(pattern)
      puts "  \e[32m✔\e[0m #{msg} (found: #{pattern.inspect})"
    else
      puts "\n\e[31m✗ Assertion Failed: #{msg}\e[0m"
      puts "  Expected to find: #{pattern.inspect}"
      puts "\e[33m--- Current Screen Snapshot ---\e[0m"
      puts screen
      puts "\e[33m--------------------------------\e[0m"
      exit 1
    end
  end

  def assert_session_closed
    _, _, status = Open3.capture3("tmux has-session -t #{@session_name} 2>&1")
    if status.success?
      puts "  \e[31m✗ Process did not exit cleanly; session still alive\e[0m"
      exit 1
    else
      puts "  \e[32m✔\e[0m Process and terminal session exited cleanly"
    end
  end

  def log_step(desc)
    @step_num += 1
    puts "\e[1;33m[Step #{@step_num}] #{desc}\e[0m"
  end

  def teardown!
    Open3.capture3("tmux kill-session -t #{@session_name} 2>&1") if @session_name
    FileUtils.remove_entry(@tmp_dir) if @tmp_dir && File.exist?(@tmp_dir)
  end
end

if __FILE__ == $PROGRAM_NAME
  options = { cols: 110, rows: 32, verbose: true, save_dir: nil }
  OptionParser.new do |opts|
    opts.banner = 'Usage: ruby tools/live_tui_test.rb [options]'
    opts.on('--cols N', Integer, 'Terminal columns (default: 110)') { |v| options[:cols] = v }
    opts.on('--rows N', Integer, 'Terminal rows (default: 32)') { |v| options[:rows] = v }
    opts.on('-q', '--quiet', 'Suppress snapshot character grids in stdout') { options[:verbose] = false }
    opts.on('--save-dir DIR', String, 'Directory to save snapshot text files') { |v| options[:save_dir] = v }
    opts.on('-h', '--help', 'Show help') do
      puts opts
      exit 0
    end
  end.parse!

  tester = TUISnapshotTester.new(**options)
  tester.run!
end
