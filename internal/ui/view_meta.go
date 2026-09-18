// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
)

const (
	fieldTitle = iota
	fieldSpeaker
	fieldAffiliation
	fieldURL
	fieldTags
	fieldPrivacy
	fieldOutputPath
	fieldAbstract
	fieldCommit
	numFields
)

var urlRegex = regexp.MustCompile(`https?://[^\s"'<>\)]+`)

// embedURLs finds all HTTP/HTTPS URLs in text and wraps them in OSC 8 hyperlinks.
func embedURLs(text string, theme Theme) string {
	return urlRegex.ReplaceAllStringFunc(text, func(raw string) string {
		trailing := ""
		trimmed := raw
		for len(trimmed) > 0 && (strings.HasSuffix(trimmed, ".") || strings.HasSuffix(trimmed, ",") || strings.HasSuffix(trimmed, ";")) {
			trailing = string(trimmed[len(trimmed)-1]) + trailing
			trimmed = trimmed[:len(trimmed)-1]
		}
		if trimmed == "" {
			return raw
		}
		styled := theme.PrimaryText.Underline(true).Render(trimmed)
		return termenv.Hyperlink(trimmed, styled) + trailing
	})
}

type fieldRange struct {
	start int
	end   int
}

// MetaModel manages the metadata review and export screen.
type MetaModel struct {
	theme        Theme
	metadata     model.TalkMetadata
	cuts         []model.CutInterval
	inputs       []textinput.Model
	abstract     string
	focusIndex   int
	privacyOpts  []string
	privacyIdx   int
	outputPath   string
	width        int
	height       int
	scrollOffset int
	isEditing    bool
	statusMsg    string
}

// IsEditing returns whether the user is actively editing a text field.
func (m MetaModel) IsEditing() bool {
	return m.isEditing
}

// NewMetaModel creates an initialized MetaModel with inputs pre-populated.
func NewMetaModel(meta model.TalkMetadata, cuts []model.CutInterval, defaultOutput string) MetaModel {
	inputs := make([]textinput.Model, 6)
	labels := []string{
		"Title",
		"Speaker",
		"Affiliation",
		"URL",
		"Tags (comma-separated)",
		"Output Path",
	}
	values := []string{
		meta.Title,
		meta.Speaker,
		meta.Affiliation,
		meta.URL,
		strings.Join(meta.Tags, ", "),
		defaultOutput,
	}

	for i := range inputs {
		t := textinput.New()
		t.Prompt = labels[i] + ": "
		t.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#818CF8")).Bold(true)
		t.SetValue(values[i])
		t.Blur()
		inputs[i] = t
	}

	privacyOpts := []string{"unlisted", "public", "private"}
	pIdx := 0
	for i, opt := range privacyOpts {
		if strings.EqualFold(opt, meta.Privacy) {
			pIdx = i
			break
		}
	}

	return MetaModel{
		theme:        DefaultTheme(),
		metadata:     meta,
		cuts:         cuts,
		inputs:       inputs,
		abstract:     meta.Abstract,
		focusIndex:   0,
		privacyOpts:  privacyOpts,
		privacyIdx:   pIdx,
		outputPath:   defaultOutput,
		width:        100,
		height:       30,
		scrollOffset: 0,
		isEditing:    false,
		statusMsg:    "↑/↓: fields | Enter: edit | e: fill from .eml | u: fill from URL | Space: privacy | 1-5: tabs",
	}
}

// ApplyMetadata updates form inputs and state with incoming metadata.
func (m *MetaModel) ApplyMetadata(meta model.TalkMetadata) {
	if meta.Title != "" {
		m.inputs[0].SetValue(meta.Title)
		m.metadata.Title = meta.Title
	}
	if meta.Speaker != "" {
		m.inputs[1].SetValue(meta.Speaker)
		m.metadata.Speaker = meta.Speaker
	}
	if meta.Affiliation != "" {
		m.inputs[2].SetValue(meta.Affiliation)
		m.metadata.Affiliation = meta.Affiliation
	}
	if meta.URL != "" {
		m.inputs[3].SetValue(meta.URL)
		m.metadata.URL = meta.URL
	}
	if len(meta.Tags) > 0 {
		m.inputs[4].SetValue(strings.Join(meta.Tags, ", "))
		m.metadata.Tags = meta.Tags
	}
	if meta.Abstract != "" {
		m.abstract = meta.Abstract
		m.metadata.Abstract = meta.Abstract
	}
	if meta.Privacy != "" {
		for i, opt := range m.privacyOpts {
			if strings.EqualFold(opt, meta.Privacy) {
				m.privacyIdx = i
				m.metadata.Privacy = opt
				break
			}
		}
	}
	if len(meta.Chapters) > 0 {
		m.metadata.Chapters = meta.Chapters
	}
	if meta.YouTubeID != "" {
		m.metadata.YouTubeID = meta.YouTubeID
	}
	if meta.YouTubeURL != "" {
		m.metadata.YouTubeURL = meta.YouTubeURL
	}
	if len(meta.AllPlaylists()) > 0 {
		m.metadata.Playlists = meta.AllPlaylists()
		m.metadata.PlaylistID = meta.PlaylistID
		m.metadata.PlaylistTitle = meta.PlaylistTitle
	} else if meta.PlaylistID == "" && len(meta.Playlists) == 0 {
		m.metadata.Playlists = nil
		m.metadata.PlaylistID = ""
		m.metadata.PlaylistTitle = ""
	}
}

// Metadata returns the updated metadata from form fields.
func (m MetaModel) Metadata() model.TalkMetadata {
	meta := m.metadata
	meta.Title = m.inputs[0].Value()
	meta.Speaker = m.inputs[1].Value()
	meta.Affiliation = m.inputs[2].Value()
	meta.URL = m.inputs[3].Value()

	tagsRaw := m.inputs[4].Value()
	var tags []string
	for _, tag := range strings.Split(tagsRaw, ",") {
		trimmed := strings.TrimSpace(tag)
		if trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	meta.Tags = tags
	meta.Privacy = m.privacyOpts[m.privacyIdx]
	meta.Abstract = m.abstract
	if meta.YouTubeID != "" && meta.YouTubeURL == "" {
		meta.YouTubeURL = fmt.Sprintf("https://youtu.be/%s", meta.YouTubeID)
	} else if meta.YouTubeURL != "" && meta.YouTubeID == "" {
		meta.YouTubeID = meta.EffectiveYouTubeID()
	}
	return meta
}

// OutputPath returns the user-configured output video path.
func (m MetaModel) OutputPath() string {
	return m.inputs[5].Value()
}

// SetAbstract updates the talk abstract.
func (m *MetaModel) SetAbstract(abstract string) {
	m.abstract = abstract
}

// Abstract returns the current abstract text.
func (m MetaModel) Abstract() string {
	return m.abstract
}

// SetDimensions updates the viewport dimensions.
func (m *MetaModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
}

// UpdateCuts updates the active cut intervals used for live chapter math.
func (m *MetaModel) UpdateCuts(cuts []model.CutInterval) {
	m.cuts = cuts
}

// SetChapters updates the talk's chapter markers.
func (m *MetaModel) SetChapters(chapters []model.ChapterMarker) {
	m.metadata.Chapters = chapters
}

// SetFeedback updates the status feedback banner in the footer.
func (m *MetaModel) SetFeedback(msg string, _ bool) {
	m.statusMsg = msg
}

// visibleBodyHeight returns the available lines for scrolling body content.
func (m MetaModel) visibleBodyHeight() int {
	h := m.height - 2
	if h < 5 {
		return 5
	}
	return h
}

// Update processes terminal messages and user interactions.
func (m MetaModel) Update(msg tea.Msg) (MetaModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m.updateInputs(msg)
}

// handleKey processes keyboard shortcuts on the metadata screen.
func (m MetaModel) handleKey(msg tea.KeyMsg) (MetaModel, tea.Cmd) {
	if m.isEditing {
		switch msg.String() {
		case "enter", "esc":
			m.isEditing = false
			m.blurCurrent()
			return m, nil
		}
		return m.updateInputs(msg)
	}

	allLines, ranges := m.buildContentLines(m.width)
	totalLines := len(allLines)
	visibleHeight := m.visibleBodyHeight()
	maxScroll := totalLines - visibleHeight
	if maxScroll < 0 {
		maxScroll = 0
	}

	switch msg.String() {
	case "down", "j":
		if m.scrollOffset < maxScroll {
			m.scrollOffset++
			m.updateFocusFromScroll(ranges)
			return m, nil
		}
		if m.focusIndex < fieldCommit {
			m.cycleFocus(1)
			m.scrollToField(m.focusIndex, ranges, visibleHeight, maxScroll)
			return m, nil
		}
	case "up", "k":
		if m.scrollOffset > 0 {
			m.scrollOffset--
			m.updateFocusFromScroll(ranges)
			return m, nil
		}
		if m.focusIndex > 0 {
			m.cycleFocus(-1)
			m.scrollToField(m.focusIndex, ranges, visibleHeight, maxScroll)
			return m, nil
		}
	case "tab":
		m.cycleFocus(1)
		m.scrollToField(m.focusIndex, ranges, visibleHeight, maxScroll)
		return m, nil
	case "shift+tab":
		m.cycleFocus(-1)
		m.scrollToField(m.focusIndex, ranges, visibleHeight, maxScroll)
		return m, nil
	case "pgdown", "ctrl+d":
		step := visibleHeight / 2
		if step < 1 {
			step = 1
		}
		m.scrollOffset += step
		if m.scrollOffset > maxScroll {
			m.scrollOffset = maxScroll
		}
		m.updateFocusFromScroll(ranges)
		return m, nil
	case "pgup", "ctrl+u":
		step := visibleHeight / 2
		if step < 1 {
			step = 1
		}
		m.scrollOffset -= step
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
		m.updateFocusFromScroll(ranges)
		return m, nil
	case "home":
		m.scrollOffset = 0
		m.updateFocusFromScroll(ranges)
		return m, nil
	case "end":
		m.scrollOffset = maxScroll
		m.updateFocusFromScroll(ranges)
		return m, nil
	case "enter":
		if m.focusIndex == fieldPrivacy {
			m.privacyIdx = (m.privacyIdx + 1) % len(m.privacyOpts)
			return m, nil
		}
		if m.focusIndex == fieldCommit {
			return m, nil
		}
		if m.inputIndexForField(m.focusIndex) >= 0 {
			m.isEditing = true
			m.focusCurrent()
			return m, nil
		}
		m.cycleFocus(1)
		m.scrollToField(m.focusIndex, ranges, visibleHeight, maxScroll)
		return m, nil
	case " ":
		if m.focusIndex == fieldPrivacy {
			m.privacyIdx = (m.privacyIdx + 1) % len(m.privacyOpts)
			return m, nil
		}
	}

	return m, nil
}

// cycleFocus advances or retreats the focused field index.
func (m *MetaModel) cycleFocus(delta int) {
	m.blurCurrent()
	m.focusIndex = (m.focusIndex + delta + numFields) % numFields
	m.focusCurrent()
}

// setFocus changes focus to a specific field.
func (m *MetaModel) setFocus(field int) {
	if m.focusIndex == field {
		return
	}
	m.blurCurrent()
	m.focusIndex = field
	m.focusCurrent()
}

// updateFocusFromScroll synchronizes focus with the top visible line in the viewport.
func (m *MetaModel) updateFocusFromScroll(ranges []fieldRange) {
	for f := 0; f < numFields; f++ {
		if ranges[f].start <= m.scrollOffset && m.scrollOffset <= ranges[f].end {
			m.setFocus(f)
			return
		}
		if m.scrollOffset < ranges[f].start {
			m.setFocus(f)
			return
		}
	}
	m.setFocus(fieldCommit)
}

// scrollToField adjusts scrollOffset to ensure the field is visible in the viewport.
func (m *MetaModel) scrollToField(field int, ranges []fieldRange, visibleHeight, maxScroll int) {
	if field < 0 || field >= len(ranges) {
		return
	}
	r := ranges[field]
	if r.start < m.scrollOffset {
		m.scrollOffset = r.start
	} else if r.end >= m.scrollOffset+visibleHeight {
		m.scrollOffset = r.end - visibleHeight + 1
	}
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// blurCurrent blurs the active textinput if applicable.
func (m *MetaModel) blurCurrent() {
	idx := m.inputIndexForField(m.focusIndex)
	if idx >= 0 && idx < len(m.inputs) {
		m.inputs[idx].Blur()
	}
}

// focusCurrent focuses the active textinput if applicable.
func (m *MetaModel) focusCurrent() {
	idx := m.inputIndexForField(m.focusIndex)
	if idx >= 0 && idx < len(m.inputs) {
		m.inputs[idx].Focus()
	}
}

// inputIndexForField maps field enum to inputs slice index (-1 for non-text fields).
func (m MetaModel) inputIndexForField(field int) int {
	switch field {
	case fieldTitle:
		return 0
	case fieldSpeaker:
		return 1
	case fieldAffiliation:
		return 2
	case fieldURL:
		return 3
	case fieldTags:
		return 4
	case fieldOutputPath:
		return 5
	default:
		return -1
	}
}

// updateInputs forwards the key event to the focused textinput.
func (m MetaModel) updateInputs(msg tea.Msg) (MetaModel, tea.Cmd) {
	idx := m.inputIndexForField(m.focusIndex)
	if idx >= 0 && idx < len(m.inputs) {
		var cmd tea.Cmd
		m.inputs[idx], cmd = m.inputs[idx].Update(msg)
		return m, cmd
	}
	return m, nil
}

// View renders the metadata editor and preview screen.
func (m MetaModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return "Initializing metadata..."
	}

	header := m.renderHeader()
	visibleHeight := m.visibleBodyHeight()

	allLines, _ := m.buildContentLines(m.width)
	totalLines := len(allLines)
	maxScroll := totalLines - visibleHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := m.scrollOffset
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}

	endIdx := scroll + visibleHeight
	if endIdx > totalLines {
		endIdx = totalLines
	}

	visibleLines := make([]string, 0, visibleHeight)
	visibleLines = append(visibleLines, allLines[scroll:endIdx]...)
	for len(visibleLines) < visibleHeight {
		visibleLines = append(visibleLines, strings.Repeat(" ", m.width))
	}

	body := strings.Join(visibleLines, "\n")
	footer := m.renderFooter(totalLines, visibleHeight)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// buildContentLines compiles all metadata sections across the full screen width.
func (m MetaModel) buildContentLines(width int) ([]string, []fieldRange) {
	contentWidth := width
	if contentWidth < 30 {
		contentWidth = 30
	}

	ranges := make([]fieldRange, numFields)
	var allLines []string

	appendBlock := func(field int, rendered string) {
		lines := strings.Split(rendered, "\n")
		if field >= 0 && field < numFields {
			ranges[field] = fieldRange{
				start: len(allLines),
				end:   len(allLines) + len(lines) - 1,
			}
		}
		allLines = append(allLines, lines...)
	}

	headerBox := m.theme.SidebarBox.Width(contentWidth - 2).Render("TALK & EXPORT CONFIGURATION")
	appendBlock(-1, headerBox)

	appendBlock(fieldTitle, m.renderInputField(fieldTitle, 0, contentWidth))
	appendBlock(fieldSpeaker, m.renderInputField(fieldSpeaker, 1, contentWidth))
	appendBlock(fieldAffiliation, m.renderInputField(fieldAffiliation, 2, contentWidth))
	appendBlock(fieldURL, m.renderURLField(contentWidth))
	appendBlock(fieldTags, m.renderInputField(fieldTags, 4, contentWidth))
	appendBlock(fieldPrivacy, m.renderPrivacyField(contentWidth))
	appendBlock(fieldOutputPath, m.renderInputField(fieldOutputPath, 5, contentWidth))
	appendBlock(fieldAbstract, m.renderAbstractBox(contentWidth))
	appendBlock(-1, m.renderChaptersBox(contentWidth))
	appendBlock(fieldCommit, m.renderCommitButton(contentWidth))

	return allLines, ranges
}

// renderHeader renders the top persistent tab bar for the metadata screen.
func (m MetaModel) renderHeader() string {
	return RenderTabBar(1, m.width, m.theme)
}

// renderInputField renders an editable textinput with pale yellow highlight when active.
func (m MetaModel) renderInputField(field, inputIdx, width int) string {
	paleYellow := lipgloss.Color("#FEF9C3")
	darkText := lipgloss.Color("#0F172A")
	darkPrompt := lipgloss.Color("#1E3A8A")

	inp := m.inputs[inputIdx]
	isFocused := m.focusIndex == field

	if isFocused {
		inp.PromptStyle = lipgloss.NewStyle().Foreground(darkPrompt).Bold(true).Background(paleYellow)
		inp.TextStyle = lipgloss.NewStyle().Foreground(darkText).Background(paleYellow)
		inp.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#DC2626")).Background(paleYellow)
		rendered := inp.View()
		boxStyle := lipgloss.NewStyle().Width(width-2).Padding(0, 1).Background(paleYellow).Foreground(darkText)
		return boxStyle.Render(rendered)
	}

	inp.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#818CF8")).Bold(true)
	inp.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0"))
	inp.Cursor.Style = lipgloss.NewStyle()
	rendered := inp.View()
	boxStyle := lipgloss.NewStyle().Width(width-2).Padding(0, 1)
	return boxStyle.Render(rendered)
}

// renderURLField renders the talk URL with terminal hyperlink OSC 8 embedding and pale yellow highlight when active.
func (m MetaModel) renderURLField(width int) string {
	paleYellow := lipgloss.Color("#FEF9C3")
	darkText := lipgloss.Color("#0F172A")
	darkPrompt := lipgloss.Color("#1E3A8A")
	isFocused := m.focusIndex == fieldURL

	inp := m.inputs[3]
	if isFocused {
		inp.PromptStyle = lipgloss.NewStyle().Foreground(darkPrompt).Bold(true).Background(paleYellow)
		inp.TextStyle = lipgloss.NewStyle().Foreground(darkText).Background(paleYellow)
		inp.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#DC2626")).Background(paleYellow)
		boxContent := inp.View()
		urlVal := inp.Value()
		if urlVal != "" {
			styledURL := lipgloss.NewStyle().Foreground(lipgloss.Color("#1D4ED8")).Underline(true).Background(paleYellow).Render(urlVal)
			embedded := termenv.Hyperlink(urlVal, styledURL)
			embedLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#475569")).Background(paleYellow).Render("Embedded: ")
			boxContent += "\n  " + embedLabel + embedded
		}
		boxStyle := lipgloss.NewStyle().Width(width-2).Padding(0, 1).Background(paleYellow).Foreground(darkText)
		return boxStyle.Render(boxContent)
	}

	urlVal := inp.Value()
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#818CF8")).Bold(true).Render("URL: ")
	if urlVal == "" {
		boxContent := label + m.theme.HelpDesc.Render("(none)")
		return lipgloss.NewStyle().Width(width-2).Padding(0, 1).Render(boxContent)
	}
	styledURL := m.theme.PrimaryText.Underline(true).Render(urlVal)
	embedded := termenv.Hyperlink(urlVal, styledURL)
	return lipgloss.NewStyle().Width(width-2).Padding(0, 1).Render(label + embedded)
}

// renderPrivacyField renders the privacy selector.
func (m MetaModel) renderPrivacyField(width int) string {
	var opts []string
	for i, opt := range m.privacyOpts {
		if i == m.privacyIdx {
			opts = append(opts, m.theme.BadgeKept.Render(strings.ToUpper(opt)))
		} else {
			opts = append(opts, m.theme.HelpDesc.Render(opt))
		}
	}
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#818CF8")).Bold(true).Render("Privacy (Space to toggle): ")
	line := label + strings.Join(opts, "  ")

	style := lipgloss.NewStyle().Width(width-2).Padding(0, 1)
	if m.focusIndex == fieldPrivacy {
		style = style.Background(m.theme.Highlight).Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	}
	return style.Render(line)
}

// renderAbstractBox renders the multi-line abstract box with OSC 8 hyperlinks.
func (m MetaModel) renderAbstractBox(width int) string {
	boxWidth := width - 2
	if boxWidth < 20 {
		boxWidth = 20
	}
	innerWidth := boxWidth - 4
	if innerWidth < 16 {
		innerWidth = 16
	}

	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#818CF8")).Bold(true).Render("ABSTRACT")
	var body string
	if strings.TrimSpace(m.abstract) == "" {
		body = m.theme.HelpDesc.Render("  (No abstract provided)")
	} else {
		embedded := embedURLs(m.abstract, m.theme)
		body = lipgloss.NewStyle().Width(innerWidth).Render(embedded)
	}

	content := label + "\n" + body
	boxStyle := m.theme.SidebarBox.Width(boxWidth)
	if m.focusIndex == fieldAbstract {
		boxStyle = boxStyle.BorderForeground(lipgloss.Color("#34D399")).Background(m.theme.Highlight)
	}
	return boxStyle.Render(content)
}

// renderChaptersBox renders the adjusted YouTube chapters preview and summary.
func (m MetaModel) renderChaptersBox(width int) string {
	boxWidth := width - 2
	if boxWidth < 20 {
		boxWidth = 20
	}

	adjChapters := cutter.AdjustChapters(m.metadata.Chapters, m.cuts, "Introduction")
	var chLines []string

	header := lipgloss.NewStyle().Foreground(lipgloss.Color("#818CF8")).Bold(true).Render("YOUTUBE CHAPTERS PREVIEW & UPLOAD PREVIEW")
	metaPreview := fmt.Sprintf(
		"Title: %s  |  Speaker: %s  |  Privacy: %s",
		m.inputs[0].Value(),
		m.inputs[1].Value(),
		strings.ToUpper(m.privacyOpts[m.privacyIdx]),
	)
	chLines = append(chLines, header, metaPreview)

	if len(adjChapters) == 0 {
		chLines = append(chLines, "  00:00 Introduction (full talk)")
		chLines = append(chLines, "  (Press Ctrl+A to auto-generate chapters from kept speech)")
	} else {
		for _, ch := range adjChapters {
			chLines = append(chLines, "  "+ch.FormatYouTubeLine())
		}
	}

	return m.theme.SidebarBox.Width(boxWidth).Render(strings.Join(chLines, "\n"))
}

// renderCommitButton renders the action button to start the FFmpeg render.
func (m MetaModel) renderCommitButton(width int) string {
	btnText := " [ Commit & Cut Video (Ctrl+R / Enter) ] "
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#10B981")).
		MarginTop(1)

	if m.focusIndex == fieldCommit {
		style = style.Background(m.theme.Highlight).Underline(true)
	}

	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(style.Render(btnText))
}

// renderFooter renders the bottom status bar with scroll percentage.
func (m MetaModel) renderFooter(totalLines, visibleHeight int) string {
	pct := 100
	maxScroll := totalLines - visibleHeight
	if maxScroll > 0 {
		pct = int(float64(m.scrollOffset) / float64(maxScroll) * 100)
		if pct > 100 {
			pct = 100
		}
	}
	scrollInfo := ""
	if maxScroll > 0 {
		scrollInfo = fmt.Sprintf(" [%d%%] ", pct)
	}

	msg := m.statusMsg
	if m.isEditing {
		msg = "Editing field... Enter/Esc: done | ←/→: move cursor"
	} else if msg == "" {
		msg = "↑/↓: fields | Enter: edit | e: fill from .eml | u: fill from URL | Space: privacy | 1-5: tabs"
	}
	bar := m.theme.HelpDesc.Render(" " + msg + scrollInfo)
	return lipgloss.NewStyle().Width(m.width).Render(bar)
}
