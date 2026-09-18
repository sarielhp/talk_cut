// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"fmt"
	"os/exec"
	"sort"
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
	chapters     []model.ChapterMarker
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
		statusMsg:    "j/k: nav | Space: cut/keep | Tab: chapter | c: commit | p: preview | F1: help",
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

// SetChapters updates the chapter marker list for inline transcript annotations.
func (m *CutsModel) SetChapters(chapters []model.ChapterMarker) {
	m.chapters = cutter.SnapChaptersToCues(chapters, m.cues)
}

// Chapters returns the active chapter markers.
func (m CutsModel) Chapters() []model.ChapterMarker {
	return m.chapters
}

// isChapterStart checks if a cue is the start of a chapter marker.
func (m CutsModel) isChapterStart(cue model.SubtitleCue) (bool, model.ChapterMarker) {
	for _, ch := range m.chapters {
		if cue.Start == ch.OriginalTime {
			return true, ch
		}
	}
	return false, model.ChapterMarker{}
}

// JumpChapter moves cursor to the next or previous cue that starts a chapter.
func (m *CutsModel) JumpChapter(direction int) bool {
	if len(m.cues) == 0 || len(m.chapters) == 0 {
		return false
	}
	idx := m.cursor + direction
	for idx >= 0 && idx < len(m.cues) {
		if ok, _ := m.isChapterStart(m.cues[idx]); ok {
			m.setCursor(idx)
			return true
		}
		idx += direction
	}
	// Wrap around
	if direction > 0 {
		for i := 0; i < m.cursor; i++ {
			if ok, _ := m.isChapterStart(m.cues[i]); ok {
				m.setCursor(i)
				return true
			}
		}
	} else {
		for i := len(m.cues) - 1; i > m.cursor; i-- {
			if ok, _ := m.isChapterStart(m.cues[i]); ok {
				m.setCursor(i)
				return true
			}
		}
	}
	return false
}

// toggleChapterOnCurrent adds or removes a chapter marker at the focused cue.
func (m *CutsModel) toggleChapterOnCurrent() {
	if len(m.cues) == 0 || m.cursor < 0 || m.cursor >= len(m.cues) {
		return
	}
	cue := m.cues[m.cursor]
	for i, ch := range m.chapters {
		if ok, _ := m.isChapterStart(cue); ok && ch.OriginalTime == cue.Start {
			m.chapters = append(m.chapters[:i], m.chapters[i+1:]...)
			m.SetFeedback(fmt.Sprintf("Removed chapter at %s", vtt.FormatTimestampShort(cue.Start)), false)
			return
		}
	}

	newCh := model.ChapterMarker{
		OriginalTime: cue.Start,
		AdjustedTime: cue.Start,
		Title:        fmt.Sprintf("Chapter %d", len(m.chapters)+1),
	}
	m.chapters = append(m.chapters, newCh)
	sort.Slice(m.chapters, func(i, j int) bool {
		return m.chapters[i].OriginalTime < m.chapters[j].OriginalTime
	})
	m.SetFeedback(fmt.Sprintf("Added chapter at %s", vtt.FormatTimestampShort(cue.Start)), false)
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
	case "tab", "]":
		m.JumpChapter(1)
	case "shift+tab", "[":
		m.JumpChapter(-1)
	case "m":
		m.toggleChapterOnCurrent()
	case "?", "f1":
		m.helpOpen = !m.helpOpen
	case "esc":
		if m.helpOpen {
			m.helpOpen = false
		}
	}
	return m, nil
}

// SetFeedback sets a temporary notification banner in the footer.
func (m *CutsModel) SetFeedback(msg string, isError bool) {
	m.saveFeedback = msg
	m.saveIsError = isError
	m.savedAt = time.Now()
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
	items := m.buildTranscriptItems()
	if len(items) == 0 {
		m.scrollOffset = 0
		return
	}
	cIdx := m.cursorItemIndex(items)
	minVisibleIdx := cIdx
	if cIdx > 0 && items[cIdx-1].isChapter {
		minVisibleIdx = cIdx - 1
	}

	if minVisibleIdx < m.scrollOffset {
		m.scrollOffset = minVisibleIdx
	}
	if cIdx >= m.scrollOffset+visible {
		m.scrollOffset = cIdx - visible + 1
	}
	maxOffset := len(items) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
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
		m.chapters = cutter.AlignChaptersToKeptCues(m.chapters, m.cues)
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

	bottomCard := m.renderBottomCueCard(m.width, innerLines)

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

	if m.helpOpen {
		overlayLines := strings.Split(fullView, "\n")
		overlayLines = m.overlayFloatingHelp(overlayLines)
		fullView = strings.Join(overlayLines, "\n")
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
	return RenderTabBar(0, m.width, m.theme)
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
	items := m.buildTranscriptItems()

	for i := 0; i < visible; i++ {
		idx := m.scrollOffset + i
		if idx >= len(items) {
			lines = append(lines, strings.Repeat(" ", width))
			continue
		}
		item := items[idx]
		if item.isChapter {
			lines = append(lines, m.renderChapterBannerRow(item.chMarker, width))
		} else {
			lines = append(lines, m.renderCueRow(item.cueIdx, width))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderCueRow formats a single cue line with unicode symbols and edge-to-edge highlight.
func (m CutsModel) renderCueRow(idx, width int) string {
	cue := m.cues[idx]
	isCur := idx == m.cursor

	cursorTag := "  "
	if isCur {
		cursorTag = "▶ "
	}

	ts := vtt.FormatTimestampShort(cue.Start)
	actionIcon := "✔"
	badgeFg := m.theme.Success
	if cue.Action == model.ActionCut {
		actionIcon = "✂"
		badgeFg = m.theme.Danger
	} else if cue.Action == model.ActionReview {
		actionIcon = "?"
		badgeFg = m.theme.Warning
	}

	text := cue.Text
	prefixLen := lipgloss.Width(cursorTag) + 1 + lipgloss.Width(ts) + 2 + 1 + 2
	availText := width - prefixLen
	if availText > 0 && lipgloss.Width(text) > availText {
		if availText > 3 {
			text = text[:availText-3] + "..."
		} else {
			text = text[:availText]
		}
	}

	if isCur {
		bg := m.theme.Highlight
		fg := lipgloss.Color("#FFFFFF")
		if cue.Action == model.ActionCut {
			fg = m.theme.Danger
		}
		prefixStyle := lipgloss.NewStyle().Bold(true).Foreground(fg).Background(bg)
		badgeStyle := lipgloss.NewStyle().Bold(true).Foreground(badgeFg).Background(bg)
		textStyle := lipgloss.NewStyle().Bold(true).Foreground(fg).Background(bg)

		prefixStr := prefixStyle.Render(fmt.Sprintf("%s[%s] ", cursorTag, ts))
		badgeStr := badgeStyle.Render(actionIcon)

		usedLen := lipgloss.Width(prefixStr) + lipgloss.Width(badgeStr) + 2 + lipgloss.Width(text)
		padLen := width - usedLen
		restText := fmt.Sprintf("  %s", text)
		if padLen > 0 {
			restText += strings.Repeat(" ", padLen)
		}
		restStr := textStyle.Render(restText)
		return prefixStr + badgeStr + restStr
	}

	baseStyle := m.cueStyle(false, cue.Action)
	var badgeStr string
	if cue.Action == model.ActionCut {
		badgeStr = m.theme.IconCut.Render(actionIcon)
	} else if cue.Action == model.ActionReview {
		badgeStr = m.theme.IconReview.Render(actionIcon)
	} else {
		badgeStr = m.theme.IconKept.Render(actionIcon)
	}
	raw := fmt.Sprintf("%s[%s] %s  %s", cursorTag, ts, badgeStr, text)
	return baseStyle.Width(width).Render(raw)
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
		if availWidth >= 75 {
			msg = "j/k: nav | Space: cut/keep | Tab: chapter | c: commit cut | s: save | p: preview | F1: help"
		} else if availWidth >= 55 {
			msg = "j/k: nav | Space: cut/keep | Tab: chapter | c: commit cut | F1: help"
		} else if availWidth >= 30 {
			msg = "j/k: nav | Space: cut | c: cut video"
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
