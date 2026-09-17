// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

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

type clearStatusMsg struct{}

func clearStatusCmd() tea.Cmd {
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
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
	savedAt      time.Time
	saveFeedback string
	saveIsError  bool
	activePlayer *exec.Cmd
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
			m.saveFeedback = fmt.Sprintf("Preview error: %v", msg.err)
			m.saveIsError = true
		} else {
			m.saveFeedback = msg.msg
			m.saveIsError = false
		}
		m.savedAt = time.Now()
		return m, clearStatusCmd()
	case clearStatusMsg:
		m.savedAt = time.Time{}
		m.saveFeedback = ""
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
		return m.toggleCurrent()
	case "s", "ctrl+s":
		return m.saveCuts()
	case "p":
		return m.launchPreview()
	case "n":
		m.jumpCut(1)
	case "N":
		m.jumpCut(-1)
	case "?", "f1":
		m.helpOpen = !m.helpOpen
	case "esc":
		if m.helpOpen {
			m.helpOpen = false
		}
	}
	return m, nil
}

// SaveCuts explicitly persists current cut decisions to disk.
func (m *CutsModel) SaveCuts() {
	_, _ = m.saveCuts()
}

// saveCuts writes cut decisions to talk_cuts.json with prominent visual feedback.
func (m *CutsModel) saveCuts() (CutsModel, tea.Cmd) {
	if m.recordingDir == "" {
		m.saveFeedback = "No recording directory to save cuts"
		m.saveIsError = true
		m.savedAt = time.Now()
		return *m, clearStatusCmd()
	}
	intervals := model.BuildCutIntervals(m.cues)
	if err := model.SaveCutsFile(m.recordingDir, intervals); err != nil {
		m.saveFeedback = fmt.Sprintf("Error saving cuts: %v", err)
		m.saveIsError = true
	} else {
		cuts := m.activeCutIntervals()
		m.saveFeedback = fmt.Sprintf("Saved cuts to %s (%d cut region(s) - %s)",
			model.CutsFileName,
			len(cuts),
			time.Now().Format("15:04:05"),
		)
		m.saveIsError = false
	}
	m.savedAt = time.Now()
	return *m, clearStatusCmd()
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
	m.adjustScrollTo(m.visibleLines())
}

// adjustScrollTo aligns scroll offset with a specific visible line count.
func (m *CutsModel) adjustScrollTo(visible int) {
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
func (m CutsModel) visibleLines() int {
	h := m.height - 10
	if h < 5 {
		return 5
	}
	return h
}

// toggleCurrent toggles the cut status of the current cue and automatically persists cuts.
func (m *CutsModel) toggleCurrent() (CutsModel, tea.Cmd) {
	if len(m.cues) == 0 || m.cursor < 0 || m.cursor >= len(m.cues) {
		return *m, nil
	}
	cue := &m.cues[m.cursor]
	var actionStr string
	if cue.Action == model.ActionCut {
		cue.Action = model.ActionKeep
		actionStr = "KEEP"
	} else {
		cue.Action = model.ActionCut
		actionStr = "CUT"
	}

	if m.recordingDir != "" {
		_ = model.SaveCutsFile(m.recordingDir, model.BuildCutIntervals(m.cues))
		cuts := m.activeCutIntervals()
		m.saveFeedback = fmt.Sprintf("Cue #%d marked %s (%d cut regions)", m.cursor+1, actionStr, len(cuts))
		m.saveIsError = false
		m.savedAt = time.Now()
		return *m, clearStatusCmd()
	}

	m.statusMsg = fmt.Sprintf("Cue #%d marked %s", m.cursor+1, actionStr)
	return *m, nil
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

// launchPreview fires an external video player subprocess at the active cue timestamp.
func (m *CutsModel) launchPreview() (CutsModel, tea.Cmd) {
	if len(m.cues) == 0 || m.cursor < 0 || m.cursor >= len(m.cues) {
		return *m, nil
	}
	cue := m.cues[m.cursor]
	videoPath := m.videoPath

	cutter.KillPreview(m.activePlayer)
	m.activePlayer = nil

	player, err := cutter.DetectPlayer()
	if err != nil {
		m.saveFeedback = err.Error()
		m.saveIsError = true
		m.savedAt = time.Now()
		return *m, clearStatusCmd()
	}

	cmd, launchErr := cutter.LaunchPreview(player, cue.Start, videoPath)
	if launchErr != nil {
		m.saveFeedback = fmt.Sprintf("Preview error: %v", launchErr)
		m.saveIsError = true
		m.savedAt = time.Now()
		return *m, clearStatusCmd()
	}

	m.activePlayer = cmd
	m.saveFeedback = fmt.Sprintf("Playing at %s via %s", vtt.FormatTimestampShort(cue.Start), player.Name)
	m.saveIsError = false
	m.savedAt = time.Now()
	return *m, clearStatusCmd()
}

// Close terminates any active external player process.
func (m *CutsModel) Close() {
	cutter.KillPreview(m.activePlayer)
	m.activePlayer = nil
}

// View renders the complete cut review screen guaranteeing exact m.height lines.
func (m CutsModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return "Initializing talk_cut..."
	}

	header := m.renderHeader()
	footer := m.renderFooter()

	var activeCue model.SubtitleCue
	if len(m.cues) > 0 && m.cursor >= 0 && m.cursor < len(m.cues) {
		activeCue = m.cues[m.cursor]
	}

	cardHeight, innerLines := m.computeCardHeight(m.width, activeCue)
	bodyHeight := m.height - 2 - cardHeight
	if bodyHeight < 6 {
		bodyHeight = 6
		cardHeight = m.height - 2 - bodyHeight
		if cardHeight < 4 {
			cardHeight = 4
		}
		innerLines = cardHeight - 2
	}

	var bottomCard string
	if m.helpOpen {
		cardHeight = 8
		if cardHeight > m.height-8 {
			cardHeight = m.height - 8
		}
		innerLines = cardHeight - 2
		bodyHeight = m.height - 2 - cardHeight
		bottomCard = m.renderHelpModal(m.width, innerLines)
	} else {
		bottomCard = m.renderBottomCueCard(m.width, innerLines)
	}

	sidebarWidth := 38
	if m.width > 120 {
		sidebarWidth = 42
	}
	leftWidth := m.width - sidebarWidth - 2
	if leftWidth < 30 {
		leftWidth = 30
	}

	leftView := m.renderTranscript(leftWidth, bodyHeight)
	stats := model.ComputeStats(m.cues, m.media.Duration)
	intervals := m.activeCutIntervals()
	rightView := m.renderSidebar(sidebarWidth, bodyHeight, stats, intervals)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftView, rightView)
	fullView := lipgloss.JoinVertical(lipgloss.Left, header, body, bottomCard, footer)

	// Ensure exact height constraint so screen never jumps
	renderedLines := strings.Split(fullView, "\n")
	if len(renderedLines) > m.height {
		res := make([]string, 0, m.height)
		res = append(res, renderedLines[:m.height-1]...)
		res = append(res, renderedLines[len(renderedLines)-1])
		fullView = strings.Join(res, "\n")
	} else if len(renderedLines) < m.height {
		pad := m.height - len(renderedLines)
		res := make([]string, 0, m.height)
		res = append(res, renderedLines[:len(renderedLines)-1]...)
		for i := 0; i < pad; i++ {
			res = append(res, strings.Repeat(" ", m.width))
		}
		res = append(res, renderedLines[len(renderedLines)-1])
		fullView = strings.Join(res, "\n")
	}

	return fullView
}

// activeCutIntervals returns only intervals where Action == ActionCut.
func (m CutsModel) activeCutIntervals() []model.CutInterval {
	all := model.BuildCutIntervals(m.cues)
	var cuts []model.CutInterval
	for _, inv := range all {
		if inv.Action == model.ActionCut {
			cuts = append(cuts, inv)
		}
	}
	return cuts
}

// computeCardHeight dynamically measures wrapped cue text to scale the bottom card.
func (m CutsModel) computeCardHeight(width int, cue model.SubtitleCue) (cardHeight, innerLines int) {
	innerWidth := width - 4
	if innerWidth < 20 {
		innerWidth = 20
	}

	speakerPrefix := ""
	if cue.Speaker != "" {
		speakerPrefix = cue.Speaker + ": "
	}
	fullText := speakerPrefix + "\"" + cue.Text + "\""

	wrapped := lipgloss.NewStyle().Width(innerWidth).Render(fullText)
	textLines := strings.Split(wrapped, "\n")

	maxCard := m.height - 8
	if maxCard < 4 {
		maxCard = 4
	}

	desired := 1 + len(textLines) + 2
	if desired > maxCard {
		desired = maxCard
	}
	if desired < 5 {
		desired = 5
	}

	return desired, desired - 2
}

// renderHeader renders the top title bar (exactly 1 line).
func (m CutsModel) renderHeader() string {
	title := m.theme.TitleStyle.Render(" talk_cut ")
	sub := m.theme.SubtitleStyle.Render(fmt.Sprintf(" %s (%s) ", m.videoPath, vtt.FormatTimestampShort(m.media.Duration)))
	bar := lipgloss.JoinHorizontal(lipgloss.Center, title, sub)
	return lipgloss.NewStyle().Width(m.width).Render(bar)
}

// renderTranscript renders the scrolling transcript panel with exact body height.
func (m *CutsModel) renderTranscript(width, bodyHeight int) string {
	var lines []string
	headerText := fmt.Sprintf(" TRANSCRIPT (%d cues) ", len(m.cues))
	bar := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#334155")).
		Width(width).
		Render(headerText)
	lines = append(lines, bar)

	visible := bodyHeight - 1
	if visible < 1 {
		visible = 1
	}
	m.adjustScrollTo(visible)

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

// renderCueRow formats a single cue line with unicode symbols.
func (m CutsModel) renderCueRow(idx, width int) string {
	cue := m.cues[idx]
	isCur := idx == m.cursor

	cursorTag := "  "
	if isCur {
		cursorTag = "▶ "
	}

	ts := vtt.FormatTimestampShort(cue.Start)
	var actionBadge string
	if cue.Action == model.ActionCut {
		actionBadge = m.theme.IconCut.Render("✂")
	} else if cue.Action == model.ActionReview {
		actionBadge = m.theme.IconReview.Render("?")
	} else {
		actionBadge = m.theme.IconKept.Render("✔")
	}

	text := cue.Text
	prefixLen := len(cursorTag) + len(ts) + 3 + 2 + 1
	availText := width - prefixLen - 3
	if availText > 0 && len(text) > availText {
		text = text[:availText-3] + "..."
	}

	raw := fmt.Sprintf("%s[%s] %s  %s", cursorTag, ts, actionBadge, text)
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

// renderFooter renders the bottom status bar (strictly 1 single line) with live cue counter.
func (m CutsModel) renderFooter() string {
	var cueBadge string
	if len(m.cues) > 0 && m.cursor >= 0 && m.cursor < len(m.cues) {
		cue := m.cues[m.cursor]
		durSec := fmt.Sprintf("%.2fs", cue.Duration().Seconds())
		cueBadge = fmt.Sprintf(" Cue %d/%d [%s] (%s) ",
			m.cursor+1, len(m.cues),
			vtt.FormatTimestampShort(cue.Start),
			durSec,
		)
	}

	left := m.theme.TitleStyle.Render(cueBadge)
	leftWidth := lipgloss.Width(left)

	availRight := m.width - leftWidth - 1
	if availRight < 0 {
		availRight = 0
	}

	var right string
	hasFeedback := !m.savedAt.IsZero() && time.Since(m.savedAt) < 4*time.Second && m.saveFeedback != ""
	if hasFeedback {
		right = m.renderFeedbackBanner(availRight)
	} else {
		right = m.renderDefaultHints(availRight)
	}

	bar := lipgloss.JoinHorizontal(lipgloss.Center, left, right)
	lines := strings.Split(bar, "\n")
	if len(lines) > 1 {
		bar = lines[0]
	}
	return lipgloss.NewStyle().Width(m.width).MaxHeight(1).Render(bar)
}

// renderFeedbackBanner formats the prominent status/feedback banner in the footer.
func (m CutsModel) renderFeedbackBanner(availWidth int) string {
	var badge lipgloss.Style
	var textStyle lipgloss.Style
	var badgeText string

	if m.saveIsError {
		badge = m.theme.BadgeCut
		textStyle = m.theme.DangerText.Bold(true)
		badgeText = " ✗ ERROR "
	} else if strings.Contains(m.saveFeedback, "Playing") {
		badge = m.theme.TitleStyle
		textStyle = m.theme.PrimaryText.Bold(true)
		badgeText = " ▶ PREVIEW "
	} else {
		badge = m.theme.BadgeKept
		textStyle = m.theme.SuccessText.Bold(true)
		badgeText = " ✔ SAVED "
	}

	bStr := badge.Render(badgeText)
	bWidth := lipgloss.Width(bStr)
	availText := availWidth - bWidth - 1
	text := m.saveFeedback
	if availText > 3 && len(text) > availText {
		text = text[:availText-3] + "..."
	} else if availText <= 3 {
		text = ""
	}
	tStr := textStyle.Render(" " + text)
	return lipgloss.JoinHorizontal(lipgloss.Center, bStr, tStr)
}

// renderDefaultHints formats keyboard shortcut hints tailored to terminal width.
func (m CutsModel) renderDefaultHints(availWidth int) string {
	msg := m.statusMsg
	if msg == "" {
		if availWidth >= 68 {
			msg = "j/k: nav | Space: cut/keep | s: save | p: preview | Tab: meta | F1: help | q: quit"
		} else if availWidth >= 45 {
			msg = "j/k: nav | Space: cut/keep | s: save | Tab: meta | F1: help"
		} else if availWidth >= 25 {
			msg = "j/k: nav | Space: cut | s: save"
		} else {
			msg = ""
		}
	}

	if len(msg) > availWidth && availWidth > 3 {
		msg = msg[:availWidth-3] + "..."
	} else if len(msg) > availWidth {
		msg = ""
	}

	return m.theme.HelpDesc.Render(" " + msg)
}
