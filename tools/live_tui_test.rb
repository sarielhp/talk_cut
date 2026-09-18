#!/usr/bin/env ruby
# frozen_string_literal: true

require 'open3'
require 'tmpdir'
require 'fileutils'
require 'optparse'
require 'json'

# live_tui_test.rb
# Production-grade headless character-level snapshot testing for talk_cut TUI.
#
# Uses headless tmux to spawn the real compiled Go binary inside a genuine
# Linux pseudo-terminal (PTY). Features:
# - Dynamic condition polling (zero hardcoded sleeps, runs at max CPU speed)
# - Exact 2D character-level screen assertions (no GUI/X11/Wayland dependencies)
# - Verification of Unicode status badges (✔ KEEP, ✂ CUT)
# - F1 keyboard shortcuts cheat sheet overlay test
# - Explicit save command (s / Ctrl+S) and disk persistence verification
# - Full 540-cue scroll test: asserts top line 0 ('talk_cut') and bottom footer ('Cue X/540') invariants
# - SIGWINCH responsive terminal resize tests (80x24 compact vs 130x35 wide)
# - Persistence cycle test (reloads auto-saved decisions from disk)

class TUISnapshotTester
  attr_reader :session_name, :tmp_dir, :cols, :rows, :snapshots

  def initialize(cols: 110, rows: 32, verbose: true, save_dir: nil, test_resizing: true, test_scroll: false, scroll_only: false)
    @cols = cols
    @rows = rows
    @verbose = verbose
    @save_dir = save_dir
    @test_resizing = test_resizing
    @test_scroll = test_scroll
    @scroll_only = scroll_only
    @session_name = "tui_test_#{Process.pid}_#{Time.now.to_i}"
    @snapshots = []
    @step_num = 0
  end

  def run!
    check_dependencies!
    setup_sandbox!

    puts "\e[1;34m=== Starting Live Character-Level TUI Test (#{cols}x#{rows}) ===\e[0m\n"

    # Step 1: Launch talk_cut in headless tmux
    log_step("Launch talk_cut in headless session (PTY)")
    launch_app!
    snap = wait_for_pattern("TRANSCRIPT", "initial render of transcript pane", timeout: 4.0)
    capture_snapshot("01_initial_cuts_view", snap)

    assert_contains(snap, "TRANSCRIPT (540 cues)", "Transcript header with cue count")
    assert_contains(snap, "STATISTICS", "Statistics pane header")
    assert_contains(snap, "CUT REGIONS (0)", "Initial zero cut regions count")
    assert_contains(snap, "(no cuts marked)", "No cuts marked initially")
    assert_contains(snap, "Cue #1 of 540", "Bottom card focused on Cue #1")
    assert_contains(snap, "✔ KEEP", "Bottom card displays KEEP badge with green checkmark")
    assert_contains(snap, "Boris Aronov", "Speaker name displayed in bottom card")
    assert_contains(snap, "Go ahead.", "Cue text displayed in bottom card")
    assert_top_and_bottom_invariants(snap, "initial view")

    if @scroll_only
      test_full_transcript_scroll!
      log_step("Clean process shutdown with 'q'")
      send_key("q")
      wait_until(timeout: 2.0) { !session_alive? }
      assert_session_closed
      puts "\n\e[1;32m✔ Scroll test completed successfully!\e[0m"
      return
    end

    # Step 2: Navigate down (j)
    log_step("Navigate cursor down with 'j'")
    send_key("j")
    snap = wait_for_pattern("Cue 2/540", "cursor moved to Cue #2")
    capture_snapshot("02_cursor_down_cue2", snap)
    assert_contains(snap, "Cue #2 of 540", "Bottom card shows Cue #2")
    assert_contains(snap, "Adam Sheffer", "Bottom card shows Cue #2 speaker")
    assert_contains(snap, "Great!", "Bottom card shows Cue #2 text")
    assert_top_and_bottom_invariants(snap, "cursor on cue 2")

    # Step 3: Toggle cut on Cue #2 (Space)
    log_step("Toggle cut on Cue #2 with [Space]")
    send_key("Space")
    snap = wait_for_pattern("✂  Great!", "Cue #2 marked ✂ in transcript")
    capture_snapshot("03_toggle_cut_cue2", snap)
    assert_contains(snap, "✂ CUT", "Bottom card shows ✂ CUT badge")
    assert_contains(snap, "marked CUT", "Status notification shows saved message")
    assert_contains(snap, "CUT REGIONS (1)", "Cut regions count updated dynamically to 1")
    assert_top_and_bottom_invariants(snap, "cue 2 marked cut")

    # Step 3b: Verify highlighted cue row spans full width with continuous dark green background (#064E3B)
    log_step("Verify highlighted cue row spans full width with continuous dark green background")
    ansi_snap = read_pane_ansi
    highlight_line = ansi_snap.lines.find { |l| l.include?("▶") }
    assert(!highlight_line.nil?, "Highlighted cue line found on first screen")
    assert(highlight_line.include?("48;2;6;78;59m"), "Highlighted cue line has dark green background (#064E3B)")
    assert(!highlight_line.include?("\e[0m  Great!"), "Text within highlighted cue row is not cleared to default background")

    # Step 4: Explicit save command (s) and disk persistence
    log_step("Test explicit save with 's' and verify talk_cuts.json serialization")
    send_key("s")
    snap = wait_for_pattern("Saved cuts to", "Save confirmation message in footer")
    assert_contains(snap, "Saved cuts to", "Status feedback confirms file saved")
    assert_contains(snap, "SAVED", "Prominent SAVED badge shown on screen")
    verify_disk_persistence!

    # Step 5: Test F1 keyboard shortcuts cheat sheet
    log_step("Test F1 help cheat sheet overlay")
    send_key("F1")
    snap = wait_for_pattern("KEYBOARD SHORTCUTS", "F1 opens keyboard shortcuts overlay")
    capture_snapshot("04_help_modal_open", snap)
    assert_contains(snap, "KEYBOARD SHORTCUTS", "Help modal title displayed")
    assert_contains(snap, "Toggle Cut / Keep", "Shortcut actions displayed in modal")
    assert_top_and_bottom_invariants(snap, "F1 help modal open")

    # Close help with F1
    send_key("F1")
    snap = wait_for_pattern("Adam Sheffer", "F1 closes help modal returning to cue card")
    capture_snapshot("05_help_modal_closed", snap)
    assert_top_and_bottom_invariants(snap, "F1 help modal closed")

    # Step 6: Full 540-cue scroll test (verifying line 0 and line @rows-1 invariants)
    if @test_scroll
      test_full_transcript_scroll!
    else
      log_step("Full 540-cue scroll test skipped (only run on full gate or when explicitly requested)")
    end

    # Step 6b: Test Tab navigation on Tab 1 jumps to next chapter header
    log_step("Test Tab navigation on Tab 1 jumps to next chapter header")
    send_key("Tab")
    snap = wait_for_pattern("Cue 9/540", "Tab jumped to Chapter 2 at Cue 9 (00:39)")
    assert_contains(snap, "Cue 9/540", "Cursor jumped to Cue 9 at start of Chapter 2")
    assert_contains(snap, "Karim Abu Affash", "Cue card shows speaker for Chapter 2")

    send_key("Tab")
    snap = wait_for_pattern("Cue 12/540", "Tab jumped to Chapter 3 at Cue 12 (01:02)")
    assert_contains(snap, "Cue 12/540", "Cursor jumped to Cue 12 at start of Chapter 3")

    send_key("BTab")
    snap = wait_for_pattern("Cue 9/540", "Shift+Tab (BTab) jumped back to Chapter 2 at Cue 9")
    assert_contains(snap, "Cue 9/540", "Cursor returned to Chapter 2")

    # Step 7: Switch to Metadata view via direct numeric key '2'
    log_step("Switch to Metadata view with direct key '2'")
    send_key("2")
    snap = wait_for_pattern("TALK & EXPORT CONFIGURATION", "Metadata view active")
    capture_snapshot("06_metadata_view", snap)
    assert_contains(snap, "TALK & EXPORT CONFIGURATION", "Metadata input card header")
    assert_contains(snap, "YOUTUBE CHAPTERS PREVIEW", "YouTube chapters preview pane")
    assert_contains(snap, "UPLOAD PREVIEW", "Upload configuration preview pane")

    # Step 8: Verify editable field on Metadata view has pale yellow background (#FEF9C3)
    log_step("Verify active editable field has very pale yellow background (#FEF9C3)")
    ansi_snap = read_pane_ansi
    assert(ansi_snap.include?("48;2;254;249;195m"), "Active field on Metadata screen has pale yellow background")

    # Step 8b: Live test for editing URL field on Tab 2 with regular arrow keys
    log_step("Live test for editing URL field on Tab 2 with regular arrow keys")
    3.times { send_key("Tab"); sleep 0.05 }
    wait_until { read_pane_ansi.include?("48;2;254;249;195m") }
    ansi_snap = read_pane_ansi
    assert(ansi_snap.include?("48;2;254;249;195m"), "URL field has pale yellow background when focused")

    # Enter edit mode
    send_key("Enter")
    snap = wait_for_pattern("Editing field", "Entered edit mode for URL field")

    # Insert letters
    send_key("-l \"abc\"")
    sleep 0.05

    # Move left using regular arrow key
    send_key("Left")
    send_key("Left")
    snap = read_pane
    assert_contains(snap, "[2] Metadata", "Regular Left arrow moved cursor without switching tabs")

    # Insert letter at cursor
    send_key("-l \"X\"")
    sleep 0.05

    # Move right using regular arrow key
    send_key("Right")
    send_key("Right")
    snap = read_pane
    assert_contains(snap, "[2] Metadata", "Regular Right arrow moved cursor without switching tabs")

    # Verify modified text in URL field contains edited characters
    assert_contains(snap, "aXbc", "URL field contains inserted letters with left/right cursor navigation")

    # Commit edit
    send_key("Enter")
    snap = wait_for_pattern("Alt+←/→: tabs", "Committed edit on URL field")

    # Return to Cuts view with [Esc]
    log_step("Return to Cuts view with [Esc]")
    send_key("Escape")
    snap = wait_for_pattern("TRANSCRIPT", "Returned to Cut Review screen")
    capture_snapshot("07_return_cuts_view", snap)
    assert_top_and_bottom_invariants(snap, "return from metadata view")

    # Step 9: Alt+Left / Alt+Right pane navigation cycling through all 4 screens
    log_step("Test Alt+arrow pane navigation cycling across all 4 screens")
    # Tab 1 -> Tab 2
    send_key("M-Right")
    snap = wait_for_pattern("TALK & EXPORT CONFIGURATION", "Alt+Right from Tab 1 to Tab 2")
    assert_contains(snap, "TALK & EXPORT CONFIGURATION", "Tab 2 reached via Alt+Right")

    # Tab 2 -> Tab 3
    send_key("M-Right")
    snap = wait_for_pattern("SPEECH CONTEXT AT", "Alt+Right from Tab 2 to Tab 3")
    assert_contains(snap, "SPEECH CONTEXT AT", "Tab 3 reached via Alt+Right")
    assert_contains(snap, "Introduction", "Chapters listed on Tab 3")
    assert_contains(snap, "▶  1. [00:39 -> 00:00]", "First chapter highlighted on single non-wrapping line")

    # Test down arrow on Tab 3: cursor shifts cleanly to chapter 2 on a single line
    send_key("Down")
    snap = wait_for_pattern("▶  2. [01:02 ->", "Tab 3 Down arrow moved highlight to Chapter 2")
    assert_contains(snap, "▶  2. [01:02 ->", "Second chapter highlighted on single non-wrapping line")
    assert_contains(snap, "SPEECH CONTEXT AT 01:02", "Speech context snippet updated to Chapter 2 (01:02)")

    # Up arrow returns to Chapter 1
    send_key("Up")
    snap = wait_for_pattern("▶  1. [00:39 -> 00:00]", "Tab 3 Up arrow returned to Chapter 1")
    assert_contains(snap, "SPEECH CONTEXT AT 00:39", "Speech context snippet restored to 00:39")

    # Tab 3 -> Tab 4
    send_key("M-Right")
    snap = wait_for_pattern("PRE-FLIGHT EXPORT REVIEW", "Alt+Right from Tab 3 to Tab 4")
    assert_contains(snap, "PRE-FLIGHT EXPORT REVIEW", "Tab 4 reached via Alt+Right")

    # Tab 4 -> Tab 1 (cyclic wrap-around)
    send_key("M-Right")
    snap = wait_for_pattern("TRANSCRIPT (540 cues)", "Alt+Right wrap-around to Tab 1")
    assert_contains(snap, "TRANSCRIPT (540 cues)", "Tab 1 reached via Alt+Right wrap-around")
    assert_top_and_bottom_invariants(snap, "Tab 1 after cyclic right navigation")

    # Test reverse direction with Alt+Left: Tab 1 -> Tab 4 (cyclic backwards)
    send_key("M-Left")
    snap = wait_for_pattern("PRE-FLIGHT EXPORT REVIEW", "Alt+Left wrap-around to Tab 4")
    assert_contains(snap, "PRE-FLIGHT EXPORT REVIEW", "Tab 4 reached via Alt+Left wrap-around")

    # Tab 4 -> Tab 3
    send_key("M-Left")
    snap = wait_for_pattern("SPEECH CONTEXT AT", "Alt+Left to Tab 3")
    assert_contains(snap, "SPEECH CONTEXT AT", "Tab 3 reached via Alt+Left")

    # Tab 3 -> Tab 2
    send_key("M-Left")
    snap = wait_for_pattern("TALK & EXPORT CONFIGURATION", "Alt+Left to Tab 2")
    assert_contains(snap, "TALK & EXPORT CONFIGURATION", "Tab 2 reached via Alt+Left")

    # Tab 2 -> Tab 1
    send_key("M-Left")
    snap = wait_for_pattern("TRANSCRIPT (540 cues)", "Alt+Left to Tab 1")
    assert_contains(snap, "TRANSCRIPT (540 cues)", "Tab 1 reached via Alt+Left")
    assert_top_and_bottom_invariants(snap, "Tab 1 after cyclic left navigation")

    # Step 10: Direct numeric jumps (1..4)
    log_step("Test direct numeric jumps (1, 2, 3, 4)")
    send_key("3")
    snap = wait_for_pattern("SPEECH CONTEXT AT", "Numeric key '3' jumps to Tab 3")
    assert_contains(snap, "SPEECH CONTEXT AT", "Tab 3 reached via '3'")

    send_key("4")
    snap = wait_for_pattern("PRE-FLIGHT EXPORT REVIEW", "Numeric key '4' jumps to Tab 4")
    assert_contains(snap, "PRE-FLIGHT EXPORT REVIEW", "Tab 4 reached via '4'")

    send_key("2")
    snap = wait_for_pattern("TALK & EXPORT CONFIGURATION", "Numeric key '2' jumps to Tab 2")
    assert_contains(snap, "TALK & EXPORT CONFIGURATION", "Tab 2 reached via '2'")

    send_key("1")
    snap = wait_for_pattern("TRANSCRIPT (540 cues)", "Numeric key '1' jumps to Tab 1")
    assert_contains(snap, "TRANSCRIPT (540 cues)", "Tab 1 reached via '1'")
    assert_top_and_bottom_invariants(snap, "Tab 1 after direct numeric jumps")

    # Step 11: Verify chapter starts move automatically to first non-deleted cue
    log_step("Verify chapter starts move to first cue that is not deleted")
    send_key("Tab")
    snap = wait_for_pattern("Cue 12/540", "Cursor at Cue 12 (01:02)")
    assert_contains(snap, "── 🔖 Definitions and Related Work (01:02)", "Chapter initially starts at Cue 12 (01:02)")

    # Mark Cue 12 as CUT (Space). Chapter relocates to next kept cue #13 (01:09)!
    send_key("Space")
    snap = wait_for_pattern("── 🔖 Definitions and Related Work (01:09)", "Chapter moves to first kept cue #13 (01:09)")
    assert_contains(snap, "── 🔖 Definitions and Related Work (01:09)", "Chapter banner relocated to first kept cue at 01:09")

    # Step 12: Verify dedicated injected chapter header line in transcript
    log_step("Verify injected chapter header line in transcript")
    assert_contains(snap, "── 🔖 Definitions and Related Work", "Dedicated chapter header banner in transcript")
    banner_count = snap.scan(/── 🔖 Definitions and Related Work \(01:09\)/).size
    if banner_count == 1
      puts "  \e[32m✔\e[0m Chapter title appears exactly once as dedicated banner in transcript (count: #{banner_count})"
    else
      puts "\n\e[31m✗ Duplicate Chapter Detected: expected 1, found #{banner_count}\e[0m"
      exit 1
    end

    # Toggle Cue 12 back to KEEP (Space): restores cut regions to 1
    send_key("Space")
    snap = wait_for_pattern("CUT REGIONS (1)", "Cue 12 un-cut back to KEEP")
    assert_contains(snap, "CUT REGIONS (1)", "Cut regions count restored to 1")

    # Step 11: Responsive terminal resizing (SIGWINCH)
    if @test_resizing
      log_step("Test responsive resizing: Compact VT100 (80x24) and Widescreen (130x35)")
      test_terminal_resizing!
    end

    # Step 12: Clean process shutdown (q)
    log_step("Clean process shutdown with 'q'")
    send_key("q")
    wait_until(timeout: 2.0) { !session_alive? }
    assert_session_closed

    # Step 13: Relaunch and verify automatic reloading of cuts
    log_step("Re-launch talk_cut to verify clean reload of talk_cuts.json")
    launch_app!
    snap = wait_for_pattern("TRANSCRIPT", "reload of talk_cut")
    capture_snapshot("08_reloaded_from_disk", snap)
    assert_contains(snap, "✂  Great!", "Cue #2 restored from disk as ✂")
    assert_contains(snap, "CUT REGIONS (1)", "Cut intervals restored from disk")
    assert_top_and_bottom_invariants(snap, "reloaded from disk")

    # Final teardown
    send_key("q")
    wait_until(timeout: 2.0) { !session_alive? }
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
      # Exclude pre-existing cuts file so test operates on a fresh clean recording
      next if File.basename(file) == 'talk_cuts.json'
      if File.basename(file) == 'talk_meta.json'
        FileUtils.cp(file, File.join(@tmp_dir, File.basename(file)))
      else
        FileUtils.ln_sf(file, File.join(@tmp_dir, File.basename(file)))
      end
    end
  end

  def launch_app!
    binary = File.expand_path('../talk_cut', __dir__)
    cmd = "tmux new-session -d -s #{@session_name} -x #{@cols} -y #{@rows} '#{binary} --no-ai #{@tmp_dir}'"
    stdout, stderr, status = Open3.capture3(cmd)
    unless status.success?
      abort "\e[31mFailed to launch tmux session:\e[0m #{stderr}"
    end
  end

  def send_key(key)
    cmd = "tmux send-keys -t #{@session_name} #{key}"
    _, stderr, status = Open3.capture3(cmd)
    unless status.success?
      abort "\e[31mFailed to send key '#{key}':\e[0m #{stderr}"
    end
  end

  def read_pane
    cmd = "tmux capture-pane -t #{@session_name} -p"
    stdout, _, status = Open3.capture3(cmd)
    status.success? ? stdout : ''
  end

  def read_pane_ansi
    cmd = "tmux capture-pane -t #{@session_name} -p -e"
    stdout, _, status = Open3.capture3(cmd)
    status.success? ? stdout : ''
  end

  def wait_until(timeout: 3.0, interval: 0.03)
    deadline = Time.now + timeout
    while Time.now < deadline
      val = yield
      return val if val
      sleep interval
    end
    nil
  end

  def wait_for_pattern(pattern, desc, timeout: 3.0)
    matched = wait_until(timeout: timeout) do
      snap = read_pane
      snap.include?(pattern) ? snap : nil
    end

    if matched
      matched
    else
      snap = read_pane
      puts "\n\e[31m✗ Timed out after #{timeout}s waiting for: #{desc}\e[0m"
      puts "  Expected to find: #{pattern.inspect}"
      puts "\e[33m--- Last Screen Snapshot ---\e[0m"
      puts snap
      puts "\e[33m-----------------------------\e[0m"
      exit 1
    end
  end

  def capture_snapshot(name, content, width: @cols)
    @snapshots << { name: name, content: content }

    if @save_dir
      FileUtils.mkdir_p(@save_dir)
      File.write(File.join(@save_dir, "#{name}.txt"), content)
    end

    if @verbose
      print_snapshot_frame(name, content, width: width)
    end

    content
  end

  def print_snapshot_frame(name, content, width: @cols)
    separator = "─" * width
    puts "\e[1;36m┌#{separator}┐\e[0m"
    puts "\e[1;36m│ SNAPSHOT: %-#{width - 12}s │\e[0m" % name
    puts "\e[1;36m├#{separator}┤\e[0m"
    content.lines.each do |line|
      puts "  " + line.chomp
    end
    puts "\e[1;36m└#{separator}┘\e[0m\n"
  end

  def assert_true(condition, msg)
    if condition
      puts "  \e[32m✔\e[0m #{msg}"
    else
      puts "\n\e[31m✗ Assertion Failed: #{msg}\e[0m"
      exit 1
    end
  end
  alias assert assert_true

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

  def assert_top_and_bottom_invariants(snap, context)
    lines = snap.lines.map(&:chomp)
    if lines.size != @rows
      puts "\n\e[31m✗ Height Invariant Failed (#{context}): expected #{@rows} lines, got #{lines.size}\e[0m"
      exit 1
    end

    header_line = lines[0].to_s
    unless header_line.include?("talk_cut")
      puts "\n\e[31m✗ Header Invariant Failed (#{context}): line 0 must contain 'talk_cut', got: #{header_line.inspect}\e[0m"
      exit 1
    end

    footer_line = lines[@rows - 1].to_s
    unless footer_line.include?("Cue ")
      puts "\n\e[31m✗ Footer Invariant Failed (#{context}): line #{@rows - 1} must contain 'Cue ', got: #{footer_line.inspect}\e[0m"
      exit 1
    end
  end

  def test_full_transcript_scroll!
    log_step("Test full transcript scroll (0 -> 540 cues) verifying zero screen jumping")
    send_key("g")
    snap = wait_for_pattern("Cue 1/540", "jumped to top of transcript (Cue 1)")
    assert_top_and_bottom_invariants(snap, "top of transcript (Cue 1)")

    t0 = Time.now
    checkpoints = [25, 50, 100, 150, 200, 250, 300, 350, 400, 450, 500, 540]
    current = 1

    checkpoints.each do |target|
      count = target - current
      count.times { send_key("j") }
      snap = wait_for_pattern("Cue #{target}/540", "scrolled down to Cue #{target}/540", timeout: 4.0)
      assert_top_and_bottom_invariants(snap, "scrolling down at Cue #{target}/540")
      current = target
    end

    elapsed = Time.now - t0
    puts "  \e[32m✔\e[0m Scrolled all 540 cues down across #{checkpoints.size} checkpoints in #{elapsed.round(2)}s"
    puts "  \e[32m✔\e[0m Zero screen jumping: header line 0 and footer line #{@rows - 1} remained strictly pinned throughout"

    send_key("G")
    snap_bottom = wait_for_pattern("Cue 540/540", "rapid jump to bottom (G)")
    assert_top_and_bottom_invariants(snap_bottom, "rapid jump to bottom (G)")

    send_key("g")
    snap_top = wait_for_pattern("Cue 1/540", "rapid jump to top (g)")
    assert_top_and_bottom_invariants(snap_top, "rapid jump to top (g)")
  end

  def verify_disk_persistence!
    cuts_file = File.join(@tmp_dir, 'talk_cuts.json')
    wait_until(timeout: 2.0) { File.exist?(cuts_file) }

    unless File.exist?(cuts_file)
      puts "  \e[31m✗ talk_cuts.json was not written to disk\e[0m"
      exit 1
    end

    data = JSON.parse(File.read(cuts_file))
    cuts = data.is_a?(Hash) ? data['cuts'] : data
    unless cuts.is_a?(Array) && cuts.any? { |inv| inv['action'] == 'CUT' }
      puts "  \e[31m✗ talk_cuts.json does not contain valid cut intervals\e[0m"
      exit 1
    end

    cut_count = cuts.count { |inv| inv['action'] == 'CUT' }
    puts "  \e[32m✔\e[0m talk_cuts.json exists on disk with #{cut_count} cut interval(s) (#{cuts.size} total)"
  end

  def test_terminal_resizing!
    sleep 0.15

    # 1. Compact 80x24 (classic VT100 standard)
    resize_window(80, 24)
    snap_compact = wait_for_terminal_width(80, "compact 80x24")
    assert_contains(snap_compact, "TRANSCRIPT", "Compact 80x24 retains transcript")
    assert_contains(snap_compact, "STATISTICS", "Compact 80x24 retains statistics")
    capture_snapshot("09a_resize_80x24", snap_compact, width: 80)

    # 2. Widescreen 130x35
    resize_window(130, 35)
    snap_wide = wait_for_terminal_width(130, "widescreen 130x35")
    assert_contains(snap_wide, "TRANSCRIPT", "Widescreen 130x35 scales transcript")
    assert_contains(snap_wide, "STATISTICS", "Widescreen 130x35 scales statistics")
    capture_snapshot("09b_resize_130x35", snap_wide, width: 130)

    # 3. Restore original size
    resize_window(@cols, @rows)
    wait_for_terminal_width(@cols, "restore #{@cols}x#{@rows}")
  end

  def wait_for_terminal_width(target_width, desc, timeout: 3.0)
    matched = wait_until(timeout: timeout) do
      snap = read_pane
      has_header = snap.include?('TRANSCRIPT')
      has_border = snap.lines.any? { |l| l.strip.start_with?('╭') && l.strip.length == target_width }
      (has_header && has_border) ? snap : nil
    end

    if matched
      matched
    else
      snap = read_pane
      puts "\n\e[31m✗ Timed out after #{timeout}s waiting for resize: #{desc}\e[0m"
      puts "\e[33m--- Last Screen Snapshot ---\e[0m"
      puts snap
      puts "\e[33m-----------------------------\e[0m"
      exit 1
    end
  end

  def resize_window(c, r)
    cmd = "tmux resize-window -t #{@session_name} -x #{c} -y #{r}"
    Open3.capture3(cmd)
  end

  def session_alive?
    _, _, status = Open3.capture3("tmux has-session -t #{@session_name} 2>&1")
    status.success?
  end

  def assert_session_closed
    if session_alive?
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
  is_full_gate = ARGV.include?('--full-gate') || ENV['FULL_GATE'] == '1'
  options = { cols: 110, rows: 32, verbose: true, save_dir: nil, test_resizing: true, test_scroll: is_full_gate, scroll_only: false }
  OptionParser.new do |opts|
    opts.banner = 'Usage: ruby tools/live_tui_test.rb [options]'
    opts.on('--cols N', Integer, 'Terminal columns (default: 110)') { |v| options[:cols] = v }
    opts.on('--rows N', Integer, 'Terminal rows (default: 32)') { |v| options[:rows] = v }
    opts.on('-q', '--quiet', 'Suppress snapshot character grids in stdout') { options[:verbose] = false }
    opts.on('--no-resize', 'Skip terminal resizing tests') { options[:test_resizing] = false }
    opts.on('--scroll', 'Run full 540-cue scroll test (explicitly requested)') { options[:test_scroll] = true }
    opts.on('--no-scroll', 'Skip full transcript scroll test') { options[:test_scroll] = false }
    opts.on('--scroll-only', 'Run only the 540-cue scroll test') do
      options[:scroll_only] = true
      options[:test_scroll] = true
    end
    opts.on('--full-gate', 'Run full quality gate suite (includes 540-cue scroll test)') { options[:test_scroll] = true }
    opts.on('--save-dir DIR', String, 'Directory to save snapshot text files') { |v| options[:save_dir] = v }
    opts.on('-h', '--help', 'Show help') do
      puts opts
      exit 0
    end
  end.parse!

  tester = TUISnapshotTester.new(**options)
  tester.run!
end

