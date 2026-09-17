# VHS tape for talk_cut TUI live character snapshot testing
# Run with: docker run --rm -v "$PWD":/vhs ghcr.io/charmbracelet/vhs tools/test_tui.tape
# Or locally: vhs tools/test_tui.tape

Output test_snapshots/vhs_screen.txt
Output test_snapshots/vhs_screen.ascii

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 700

# Start talk_cut in test mode
Type "./talk_cut --no-ai examples/26_09_08"
Enter
Sleep 1s

# Move cursor down to Cue #2
Type "j"
Sleep 300ms

# Toggle cut on Cue #2
Space
Sleep 300ms

# Switch to Metadata & Chapters view
Tab
Sleep 500ms

# Return to Cut Review
Escape
Sleep 300ms

# Quit cleanly
Type "q"
Sleep 200ms
