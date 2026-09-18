// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
	"talk_cut/internal/vtt"
)

// ChaptersModel manages the interactive chapter manager screen (Tab 3).
type ChaptersModel struct {
	theme        Theme
	chapters     []model.ChapterMarker
	cuts         []model.CutInterval
	cues         []model.SubtitleCue
	videoPath    string
	cursor       int
	scrollOffset int
	width        int
	height       int
	isEditing    bool
	editInput    textinput.Model
	activePlayer *exec.Cmd
	statusMsg    string
}

// NewChaptersModel creates an initialized ChaptersModel.
func NewChaptersModel(chapters []model.ChapterMarker, cuts []model.CutInterval, cues []model.SubtitleCue, videoPath string) ChaptersModel {
	ti := textinput.New()
	ti.Prompt = "Edit Title: "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#818CF8")).Bold(true)

	return ChaptersModel{
		theme:        DefaultTheme(),
		chapters:     chapters,
		cuts:         cuts,
		cues:         cues,
		videoPath:    videoPath,
		cursor:       0,
		scrollOffset: 0,
		width:        100,
		height:       30,
		isEditing:    false,
		editInput:    ti,
		statusMsg:    "↑/↓: select | Enter: edit | d: delete | a: add | r: AI redo | p: preview | ←/→: tabs",
	}
}

// IsEditing returns whether the chapter title editor is currently active.
func (m ChaptersModel) IsEditing() bool {
	return m.isEditing
}

// SetDimensions updates viewport dimensions.
func (m *ChaptersModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
}

// SetChapters replaces the current chapter marker list.
func (m *ChaptersModel) SetChapters(chapters []model.ChapterMarker) {
	m.chapters = chapters
	if m.cursor >= len(m.chapters) {
		m.cursor = len(m.chapters) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// Chapters returns the current slice of chapter markers.
func (m ChaptersModel) Chapters() []model.ChapterMarker {
	return m.chapters
}

// UpdateCuts updates the active cut intervals used for adjusted time calculation.
func (m *ChaptersModel) UpdateCuts(cuts []model.CutInterval) {
	m.cuts = cuts
}

// SetCues updates the subtitle cues slice used for speech snippets.
func (m *ChaptersModel) SetCues(cues []model.SubtitleCue) {
	m.cues = cues
}

// Update handles messages and key inputs for the chapter manager.
func (m ChaptersModel) Update(msg tea.Msg) (ChaptersModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.isEditing {
		var cmd tea.Cmd
		m.editInput, cmd = m.editInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

// handleKey dispatches keyboard interactions.
func (m ChaptersModel) handleKey(msg tea.KeyMsg) (ChaptersModel, tea.Cmd) {
	if m.isEditing {
		switch msg.String() {
		case "enter":
			return m.commitTitleEdit()
		case "esc":
			m.isEditing = false
			m.editInput.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.editInput, cmd = m.editInput.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "enter", "e":
		return m.startTitleEdit()
	case "d", "x":
		m.deleteCurrentChapter()
	case "a":
		m.addChapterAtCursor()
	case "p":
		return m.launchPreview()
	}

	return m, nil
}

// moveCursor shifts the selection cursor bounded by chapter count.
func (m *ChaptersModel) moveCursor(delta int) {
	if len(m.chapters) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.chapters) {
		m.cursor = len(m.chapters) - 1
	}
}

// startTitleEdit opens textinput for editing the focused chapter's title.
func (m ChaptersModel) startTitleEdit() (ChaptersModel, tea.Cmd) {
	if len(m.chapters) == 0 || m.cursor < 0 || m.cursor >= len(m.chapters) {
		return m, nil
	}
	m.isEditing = true
	m.editInput.SetValue(m.chapters[m.cursor].Title)
	m.editInput.Focus()
	return m, nil
}

// commitTitleEdit saves the edited title back to the chapter list.
func (m ChaptersModel) commitTitleEdit() (ChaptersModel, tea.Cmd) {
	newTitle := strings.TrimSpace(m.editInput.Value())
	if newTitle != "" && m.cursor >= 0 && m.cursor < len(m.chapters) {
		m.chapters[m.cursor].Title = newTitle
	}
	m.isEditing = false
	m.editInput.Blur()
	return m, nil
}

// deleteCurrentChapter removes the currently selected chapter marker.
func (m *ChaptersModel) deleteCurrentChapter() {
	if len(m.chapters) == 0 || m.cursor < 0 || m.cursor >= len(m.chapters) {
		return
	}
	m.chapters = append(m.chapters[:m.cursor], m.chapters[m.cursor+1:]...)
	if m.cursor >= len(m.chapters) && m.cursor > 0 {
		m.cursor = len(m.chapters) - 1
	}
}

// addChapterAtCursor inserts a new chapter marker at the current cue or time.
func (m *ChaptersModel) addChapterAtCursor() {
	newTime := time.Duration(0)
	if len(m.chapters) > 0 && m.cursor >= 0 && m.cursor < len(m.chapters) {
		newTime = m.chapters[m.cursor].OriginalTime + 2*time.Minute
	}
	newCh := model.ChapterMarker{
		OriginalTime: newTime,
		AdjustedTime: newTime,
		Title:        "New Chapter",
	}
	m.chapters = append(m.chapters, newCh)
	sort.Slice(m.chapters, func(i, j int) bool {
		return m.chapters[i].OriginalTime < m.chapters[j].OriginalTime
	})
}

// launchPreview launches video playback at the selected chapter's original timestamp.
func (m ChaptersModel) launchPreview() (ChaptersModel, tea.Cmd) {
	if len(m.chapters) == 0 || m.cursor < 0 || m.cursor >= len(m.chapters) {
		return m, nil
	}
	ch := m.chapters[m.cursor]
	cutter.KillPreview(m.activePlayer)
	m.activePlayer = nil

	player, err := cutter.DetectPlayer()
	if err != nil {
		m.statusMsg = err.Error()
		return m, nil
	}

	cmd, launchErr := cutter.LaunchPreview(player, ch.OriginalTime, m.videoPath)
	if launchErr != nil {
		m.statusMsg = fmt.Sprintf("Preview error: %v", launchErr)
		return m, nil
	}

	m.activePlayer = cmd
	m.statusMsg = fmt.Sprintf("Playing %s at %s via %s", ch.Title, vtt.FormatTimestampShort(ch.OriginalTime), player.Name)
	return m, nil
}

// View renders the chapter manager screen.
func (m ChaptersModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return "Loading chapters..."
	}

	header := m.renderHeader()
	bodyHeight := m.height - 2
	if bodyHeight < 6 {
		bodyHeight = 6
	}
	body := m.renderBody(bodyHeight)
	footer := m.renderFooter()

	fullView := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	renderedLines := strings.Split(fullView, "\n")
	if len(renderedLines) > m.height {
		res := make([]string, 0, m.height)
		res = append(res, renderedLines[:m.height-1]...)
		res = append(res, renderedLines[len(renderedLines)-1])
		return strings.Join(res, "\n")
	}
	for len(renderedLines) < m.height {
		renderedLines = append(renderedLines, strings.Repeat(" ", m.width))
	}
	return strings.Join(renderedLines, "\n")
}

// renderHeader renders the top tab bar for the chapters manager screen.
func (m ChaptersModel) renderHeader() string {
	return RenderTabBar(2, m.width, m.theme)
}

// renderBody compiles the chapter list and the snippet preview card.
func (m *ChaptersModel) renderBody(bodyHeight int) string {
	m.chapters = cutter.AlignChaptersToKeptCues(m.chapters, m.cues)
	adjChapters := cutter.AdjustChapters(m.chapters, m.cuts, "Introduction")

	barText := fmt.Sprintf(" YOUTUBE CHAPTERS (%d markers) [Original -> Post-Cut] ", len(m.chapters))
	tableHeader := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#334155")).
		Width(m.width).
		Render(barText)

	snippetHeight := 4
	tableHeaderHeight := 1
	availRows := bodyHeight - snippetHeight - tableHeaderHeight
	if availRows < 3 {
		availRows = 3
	}

	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+availRows {
		m.scrollOffset = m.cursor - availRows + 1
	}
	maxOffset := len(m.chapters) - availRows
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	var rows []string
	rows = append(rows, tableHeader)

	for i := 0; i < availRows; i++ {
		idx := m.scrollOffset + i
		if idx >= len(m.chapters) {
			rows = append(rows, strings.Repeat(" ", m.width))
			continue
		}
		ch := m.chapters[idx]
		isCur := idx == m.cursor
		cursorTag := "  "
		if isCur {
			cursorTag = "▶ "
		}

		adjTimeStr := "00:00"
		if idx < len(adjChapters) {
			adjTimeStr = vtt.FormatTimestampShort(adjChapters[idx].AdjustedTime)
		}
		origTimeStr := vtt.FormatTimestampShort(ch.OriginalTime)

		title := ch.Title
		if isCur && m.isEditing {
			title = m.editInput.View()
		}

		prefix := fmt.Sprintf("%s%2d. [%s -> %s]  ", cursorTag, idx+1, origTimeStr, adjTimeStr)
		availTitle := m.width - len(prefix) - 1
		if availTitle > 0 && len(title) > availTitle {
			if availTitle > 3 {
				title = title[:availTitle-3] + "..."
			} else {
				title = title[:availTitle]
			}
		}

		rowStr := prefix + title
		var style lipgloss.Style
		if isCur {
			style = m.theme.RowSelected.Width(m.width)
		} else {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0")).Width(m.width)
		}
		rows = append(rows, style.Render(rowStr))
	}

	snippetBox := m.renderSnippetBox(m.width)
	rows = append(rows, snippetBox)

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// renderSnippetBox shows speech context near the active chapter start.
func (m ChaptersModel) renderSnippetBox(width int) string {
	boxWidth := width - 2
	if boxWidth < 20 {
		boxWidth = 20
	}
	if len(m.chapters) == 0 || m.cursor < 0 || m.cursor >= len(m.chapters) {
		return lipgloss.NewStyle().Width(width).Height(4).Render("")
	}
	ch := m.chapters[m.cursor]
	snippet := m.findSpeechSnippet(ch.OriginalTime)

	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#818CF8")).Bold(true).Render(fmt.Sprintf(" SPEECH CONTEXT AT %s", vtt.FormatTimestampShort(ch.OriginalTime)))
	content := fmt.Sprintf("%s\n  \"%s\"", label, snippet)
	return m.theme.SidebarBox.Width(boxWidth).Height(4).Render(content)
}

// findSpeechSnippet locates the subtitle text matching a chapter timestamp.
func (m ChaptersModel) findSpeechSnippet(t time.Duration) string {
	for _, c := range m.cues {
		if c.Start <= t && t <= c.End {
			return c.FullText()
		}
		if c.Start > t {
			return c.FullText()
		}
	}
	if len(m.cues) > 0 {
		return m.cues[0].FullText()
	}
	return "(no transcript cue at this timestamp)"
}

// renderFooter renders the bottom status hints bar.
func (m ChaptersModel) renderFooter() string {
	msg := m.statusMsg
	if m.isEditing {
		msg = "Enter: save title | Esc: cancel edit"
	}
	bar := m.theme.HelpDesc.Render(" " + msg)
	return lipgloss.NewStyle().Width(m.width).Render(bar)
}
