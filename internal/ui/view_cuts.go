// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
	"talk_cut/internal/vtt"
)

type previewMsg struct {
	err error
	msg string
}

// CutsModel manages the interactive cut review screen.
type CutsModel struct {
	theme        Theme
	keys         KeyMap
	cues         []model.SubtitleCue
	media        cutter.MediaInfo
	videoPath    string
	recordingDir string
	cursor       int
	scrollOffset int
	width        int
	height       int
	statusMsg    string
	helpOpen     bool
}

// NewCutsModel creates an initialized CutsModel.
func NewCutsModel(cues []model.SubtitleCue, media cutter.MediaInfo, videoPath, recordingDir string) CutsModel {
	return CutsModel{
		theme:        DefaultTheme(),
		keys:         DefaultKeyMap(),
		cues:         cues,
		media:        media,
		videoPath:    videoPath,
		recordingDir: recordingDir,
		cursor:       0,
		width:        100,
		height:       30,
		statusMsg:    "j/k: navigate | Space: toggle cut | p: preview ffplay | Tab: metadata",
	}
}

// Cues returns the current slice of cues with their cut actions.
func (m CutsModel) Cues() []model.SubtitleCue {
	return m.cues
}

// SetCues replaces the active cue slice.
func (m *CutsModel) SetCues(cues []model.SubtitleCue) {
	m.cues = cues
	m.cursor = 0
	m.scrollOffset = 0
}

// SetDimensions updates the viewport dimensions.
func (m *CutsModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
	m.adjustScroll()
}

// Update handles terminal messages and key inputs for the cut review screen.
func (m CutsModel) Update(msg tea.Msg) (CutsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case previewMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Preview error: %v", msg.err)
		} else {
			m.statusMsg = msg.msg
		}
		return m, nil
	}
	return m, nil
}

// handleKey dispatches keyboard shortcuts.
func (m CutsModel) handleKey(msg tea.KeyMsg) (CutsModel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "ctrl+u":
		m.moveCursor(-m.pageSize())
	case "pgdown", "ctrl+d":
		m.moveCursor(m.pageSize())
	case "g", "home":
		m.setCursor(0)
	case "G", "end":
		m.setCursor(len(m.cues) - 1)
	case " ", "x":
		m.toggleCurrent()
	case "p":
		return m, m.launchPreview()
	case "n":
		m.jumpCut(1)
	case "N":
		m.jumpCut(-1)
	case "?":
		m.helpOpen = !m.helpOpen
	}
	return m, nil
}

// moveCursor adjusts the cursor by delta while preserving bounds and scroll.
func (m *CutsModel) moveCursor(delta int) {
	if len(m.cues) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.cues) {
		m.cursor = len(m.cues) - 1
	}
	m.adjustScroll()
}

// setCursor sets the cursor to an absolute index.
func (m *CutsModel) setCursor(pos int) {
	if len(m.cues) == 0 {
		return
	}
	if pos < 0 {
		pos = 0
	}
	if pos >= len(m.cues) {
		pos = len(m.cues) - 1
	}
	m.cursor = pos
	m.adjustScroll()
}

// adjustScroll ensures the active cursor is visible within the viewport.
func (m *CutsModel) adjustScroll() {
	visible := m.visibleLines()
	if visible <= 0 {
		return
	}
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+visible {
		m.scrollOffset = m.cursor - visible + 1
	}
}

// pageSize returns the number of lines to jump for page up/down.
func (m CutsModel) pageSize() int {
	v := m.visibleLines()
	if v > 2 {
		return v - 2
	}
	return 1
}

// visibleLines returns the available height for cue rows in the top split panel.
// Fixed overhead: header (2), bottom cue card (6), footer (1), panel header (1) = 10 lines.
func (m CutsModel) visibleLines() int {
	h := m.height - 10
	if h < 5 {
		return 5
	}
	return h
}

// toggleCurrent toggles the cut status of the current cue and automatically persists cuts.
func (m *CutsModel) toggleCurrent() {
	if len(m.cues) == 0 || m.cursor < 0 || m.cursor >= len(m.cues) {
		return
	}
	cue := &m.cues[m.cursor]
	if cue.Action == model.ActionCut {
		cue.Action = model.ActionKeep
		m.statusMsg = fmt.Sprintf("Cue #%d marked KEEP (saved)", m.cursor+1)
	} else {
		cue.Action = model.ActionCut
		m.statusMsg = fmt.Sprintf("Cue #%d marked CUT (saved)", m.cursor+1)
	}

	if m.recordingDir != "" {
		_ = model.SaveCutsFile(m.recordingDir, model.BuildCutIntervals(m.cues))
	}
}

// jumpCut moves the cursor to the next or previous cut/review interval.
func (m *CutsModel) jumpCut(direction int) {
	if len(m.cues) == 0 {
		return
	}
	idx := m.cursor + direction
	for idx >= 0 && idx < len(m.cues) {
		if m.cues[idx].Action == model.ActionCut || m.cues[idx].Action == model.ActionReview {
			m.setCursor(idx)
			return
		}
		idx += direction
	}
	// Wrap around
	if direction > 0 {
		idx = 0
		for idx < m.cursor {
			if m.cues[idx].Action == model.ActionCut || m.cues[idx].Action == model.ActionReview {
				m.setCursor(idx)
				return
			}
			idx++
		}
	} else {
		idx = len(m.cues) - 1
		for idx > m.cursor {
			if m.cues[idx].Action == model.ActionCut || m.cues[idx].Action == model.ActionReview {
				m.setCursor(idx)
				return
			}
			idx--
		}
	}
}

// launchPreview fires an ffplay subprocess in the background starting at the cue.
func (m CutsModel) launchPreview() tea.Cmd {
	if len(m.cues) == 0 || m.cursor < 0 || m.cursor >= len(m.cues) {
		return nil
	}
	cue := m.cues[m.cursor]
	videoPath := m.videoPath
	return func() tea.Msg {
		startSec := fmt.Sprintf("%.3f", cue.Start.Seconds())
		cmd := exec.Command("ffplay", "-ss", startSec, "-autoexit", videoPath)
		cmd.Stdin = nil
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			return previewMsg{err: err}
		}
		return previewMsg{msg: fmt.Sprintf("Previewing at %s (ffplay)...", vtt.FormatTimestampShort(cue.Start))}
	}
}

// View renders the complete cut review screen.
func (m CutsModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return "Initializing talk_cut..."
	}

	header := m.renderHeader()
	bottomCard := m.renderBottomCueCard(m.width)
	footer := m.renderFooter()

	sidebarWidth := 38
	if m.width > 120 {
		sidebarWidth = 42
	}
	leftWidth := m.width - sidebarWidth - 2
	if leftWidth < 30 {
		leftWidth = 30
	}

	leftView := m.renderTranscript(leftWidth)
	stats := model.ComputeStats(m.cues, m.media.Duration)
	intervals := model.BuildCutIntervals(m.cues)
	rightView := m.renderSidebar(sidebarWidth, stats, intervals)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftView, rightView)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, bottomCard, footer)
}

// renderHeader renders the top title bar.
func (m CutsModel) renderHeader() string {
	title := m.theme.TitleStyle.Render(" talk_cut ")
	sub := m.theme.SubtitleStyle.Render(fmt.Sprintf(" %s (%s) ", m.videoPath, vtt.FormatTimestampShort(m.media.Duration)))
	bar := lipgloss.JoinHorizontal(lipgloss.Center, title, sub)
	return lipgloss.NewStyle().Width(m.width).MarginBottom(1).Render(bar)
}

// renderTranscript renders the scrolling transcript panel with exact line count.
func (m CutsModel) renderTranscript(width int) string {
	var lines []string
	headerText := fmt.Sprintf(" TRANSCRIPT (%d cues) ", len(m.cues))
	bar := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#334155")).
		Width(width).
		Render(headerText)
	lines = append(lines, bar)

	visible := m.visibleLines()
	for i := 0; i < visible; i++ {
		idx := m.scrollOffset + i
		if idx >= len(m.cues) {
			lines = append(lines, strings.Repeat(" ", width))
			continue
		}
		lines = append(lines, m.renderCueRow(idx, width))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderCueRow formats a single cue line with highlighting and status tags.
func (m CutsModel) renderCueRow(idx, width int) string {
	cue := m.cues[idx]
	isCur := idx == m.cursor

	cursorTag := "  "
	if isCur {
		cursorTag = "▶ "
	}

	ts := vtt.FormatTimestampShort(cue.Start)
	actionBadge := "[KEEP]"
	if cue.Action == model.ActionCut {
		actionBadge = "[CUT] "
	} else if cue.Action == model.ActionReview {
		actionBadge = "[REV] "
	}

	text := cue.Text
	prefixLen := len(cursorTag) + len(ts) + 3 + len(actionBadge) + 1
	availText := width - prefixLen - 3
	if availText > 0 && len(text) > availText {
		text = text[:availText-3] + "..."
	}

	raw := fmt.Sprintf("%s[%s] %s %s", cursorTag, ts, actionBadge, text)
	style := m.cueStyle(isCur, cue.Action)
	return style.Width(width).Render(raw)
}

// cueStyle determines the lipgloss style for a cue based on cursor and action.
func (m CutsModel) cueStyle(isCur bool, action model.CutAction) lipgloss.Style {
	if isCur && action == model.ActionCut {
		return m.theme.CueCutSelected
	}
	if isCur {
		return m.theme.CueSelected
	}
	if action == model.ActionCut {
		return m.theme.CueCut
	}
	if action == model.ActionReview {
		return m.theme.CueReview
	}
	return m.theme.CueNormal
}

// renderSidebar renders the right-hand panel with fixed-height stats, cuts list, and keys.
func (m CutsModel) renderSidebar(width int, stats model.CutStats, intervals []model.CutInterval) string {
	statsBox := m.renderStatsBox(width, stats)
	cutsBox := m.renderCutsBox(width, intervals)
	helpBox := m.renderHelpBox(width)

	content := lipgloss.JoinVertical(lipgloss.Left, statsBox, cutsBox, helpBox)
	return lipgloss.NewStyle().Width(width).MarginLeft(1).Render(content)
}

// renderStatsBox formats the duration and cut statistics.
func (m CutsModel) renderStatsBox(width int, stats model.CutStats) string {
	orig := vtt.FormatTimestampShort(stats.TotalOriginal)
	cut := vtt.FormatTimestampShort(stats.TotalCut)
	kept := vtt.FormatTimestampShort(stats.TotalKept)
	pct := fmt.Sprintf("%.1f%%", stats.KeptPercent())

	content := fmt.Sprintf(
		"Original: %s  Cut: %s\nKept:     %s (%s kept)",
		m.theme.StatsValue.Render(orig),
		m.theme.DangerText.Render("-"+cut),
		m.theme.SuccessText.Render(kept),
		pct,
	)

	return m.theme.SidebarBox.Width(width - 2).Render("STATISTICS\n" + content)
}

// renderCutsBox lists active cut intervals.
func (m CutsModel) renderCutsBox(width int, intervals []model.CutInterval) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("CUT REGIONS (%d)", len(intervals)))

	if len(intervals) == 0 {
		lines = append(lines, "  (no cuts marked)")
	} else {
		maxShow := 3
		for i, cut := range intervals {
			if i >= maxShow {
				lines = append(lines, fmt.Sprintf("  ...and %d more", len(intervals)-maxShow))
				break
			}
			durSec := fmt.Sprintf("%.1fs", cut.Duration().Seconds())
			lines = append(lines, fmt.Sprintf("  %d. %s-%s (%s)",
				i+1,
				vtt.FormatTimestampShort(cut.Start),
				vtt.FormatTimestampShort(cut.End),
				durSec,
			))
		}
	}

	return m.theme.SidebarBox.Width(width - 2).Render(strings.Join(lines, "\n"))
}

// renderHelpBox renders keyboard shortcut hints.
func (m CutsModel) renderHelpBox(width int) string {
	hints := "[Space] Cut/Keep   [p] Preview\n" +
		"[n/N]   Jump Cut   [Tab] Metadata\n" +
		"[?]     Help       [q] Quit"
	return m.theme.SidebarBox.Width(width - 2).Render(hints)
}

// renderBottomCueCard renders the active cue in a fixed-height card spanning the full width of the screen.
func (m CutsModel) renderBottomCueCard(width int) string {
	if len(m.cues) == 0 || m.cursor < 0 || m.cursor >= len(m.cues) {
		return ""
	}
	cue := m.cues[m.cursor]

	statusBadge := m.theme.BadgeKept.Render(" KEEP ")
	if cue.Action == model.ActionCut {
		statusBadge = m.theme.BadgeCut.Render(" CUT ")
	} else if cue.Action == model.ActionReview {
		statusBadge = m.theme.BadgeReview.Render(" REVIEW ")
	}

	durSec := fmt.Sprintf("%.2fs", cue.Duration().Seconds())
	line1 := fmt.Sprintf(
		"Cue #%d of %d  [%s -> %s] (%s)  %s",
		m.cursor+1,
		len(m.cues),
		vtt.FormatTimestampShort(cue.Start),
		vtt.FormatTimestampShort(cue.End),
		durSec,
		statusBadge,
	)
	if cue.CutReason != "" {
		line1 += "  " + m.theme.HelpDesc.Render("Reason: "+cue.CutReason)
	}

	var contentLines []string
	contentLines = append(contentLines, line1)

	speakerPrefix := ""
	if cue.Speaker != "" {
		speakerPrefix = m.theme.SpeakerStyle.Render(cue.Speaker+": ") + " "
	}
	contentLines = append(contentLines, speakerPrefix+"\""+cue.Text+"\"")

	cardContent := strings.Join(contentLines, "\n")
	box := m.theme.SidebarBox.
		Width(width - 2).
		Height(4).
		Render(cardContent)

	return lipgloss.NewStyle().Width(width).MarginTop(1).Render(box)
}

// renderFooter renders the bottom status bar.
func (m CutsModel) renderFooter() string {
	msg := m.statusMsg
	if msg == "" {
		msg = "Ready"
	}
	bar := m.theme.HelpDesc.Render(" " + msg)
	return lipgloss.NewStyle().Width(m.width).Render(bar)
}
