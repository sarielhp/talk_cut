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
	statusMsg    string
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
		inputs[i] = t
	}
	inputs[0].Focus()

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
		statusMsg:    "↑/↓: scroll | Tab/Shift-Tab: fields | Space: privacy | Ctrl+R: cut | Esc: cuts",
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

// visibleBodyHeight returns the available lines for scrolling body content.
func (m MetaModel) visibleBodyHeight() int {
	h := m.height - 3
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
	allLines, ranges := m.buildContentLines(m.width)
	totalLines := len(allLines)
	visibleHeight := m.visibleBodyHeight()
	maxScroll := totalLines - visibleHeight
	if maxScroll < 0 {
		maxScroll = 0
	}

	switch msg.String() {
	case "down":
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
	case "up":
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
		if m.focusIndex == fieldCommit {
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

	return m.updateInputs(msg)
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

	appendBlock(fieldTitle, m.renderFieldBox(fieldTitle, contentWidth, m.inputs[0].View()))
	appendBlock(fieldSpeaker, m.renderFieldBox(fieldSpeaker, contentWidth, m.inputs[1].View()))
	appendBlock(fieldAffiliation, m.renderFieldBox(fieldAffiliation, contentWidth, m.inputs[2].View()))
	appendBlock(fieldURL, m.renderURLField(contentWidth))
	appendBlock(fieldTags, m.renderFieldBox(fieldTags, contentWidth, m.inputs[4].View()))
	appendBlock(fieldPrivacy, m.renderPrivacyField(contentWidth))
	appendBlock(fieldOutputPath, m.renderFieldBox(fieldOutputPath, contentWidth, m.inputs[5].View()))
	appendBlock(fieldAbstract, m.renderAbstractBox(contentWidth))
	appendBlock(-1, m.renderChaptersBox(contentWidth))
	appendBlock(fieldCommit, m.renderCommitButton(contentWidth))

	return allLines, ranges
}

// renderHeader renders the top title bar for the metadata screen.
func (m MetaModel) renderHeader() string {
	title := m.theme.TitleStyle.Render(" talk_cut > METADATA & CHAPTERS ")
	bar := lipgloss.NewStyle().Width(m.width).MarginBottom(1).Render(title)
	return bar
}

// renderFieldBox wraps an input view with styling based on focus.
func (m MetaModel) renderFieldBox(field, width int, content string) string {
	style := lipgloss.NewStyle().Width(width-2).Padding(0, 1)
	if m.focusIndex == field {
		style = style.Background(lipgloss.Color("#1E293B")).Bold(true)
	}
	return style.Render(content)
}

// renderURLField renders the talk URL with terminal hyperlink OSC 8 embedding.
func (m MetaModel) renderURLField(width int) string {
	urlVal := m.inputs[3].Value()
	if m.focusIndex == fieldURL {
		boxContent := m.inputs[3].View()
		if urlVal != "" {
			styledURL := m.theme.PrimaryText.Underline(true).Render(urlVal)
			embedded := termenv.Hyperlink(urlVal, styledURL)
			boxContent += "\n  " + m.theme.SubtitleStyle.Render("Embedded: ") + embedded
		}
		return m.renderFieldBox(fieldURL, width, boxContent)
	}

	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#818CF8")).Bold(true).Render("URL: ")
	if urlVal == "" {
		return m.renderFieldBox(fieldURL, width, label+m.theme.HelpDesc.Render("(none)"))
	}
	styledURL := m.theme.PrimaryText.Underline(true).Render(urlVal)
	embedded := termenv.Hyperlink(urlVal, styledURL)
	return m.renderFieldBox(fieldURL, width, label+embedded)
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
		style = style.Background(lipgloss.Color("#1E293B"))
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
		boxStyle = boxStyle.BorderForeground(m.theme.Primary).Background(lipgloss.Color("#1E293B"))
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
	chLines = append(chLines, lipgloss.NewStyle().Foreground(lipgloss.Color("#818CF8")).Bold(true).Render("YOUTUBE CHAPTERS PREVIEW"))

	if len(adjChapters) == 0 {
		chLines = append(chLines, "  00:00 Introduction (full talk)")
	} else {
		for _, ch := range adjChapters {
			chLines = append(chLines, "  "+ch.FormatYouTubeLine())
		}
	}

	metaPreview := fmt.Sprintf(
		"UPLOAD PREVIEW\nTitle:   %s\nSpeaker: %s\nPrivacy: %s\nOutput:  %s",
		m.inputs[0].Value(),
		m.inputs[1].Value(),
		strings.ToUpper(m.privacyOpts[m.privacyIdx]),
		m.OutputPath(),
	)
	chLines = append(chLines, metaPreview)

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
		style = style.Background(lipgloss.Color("#059669")).Underline(true)
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
	if msg == "" {
		msg = "↑/↓: scroll | Tab/Shift-Tab: fields | Space: privacy | Ctrl+R: cut | Esc: cuts"
	}
	bar := m.theme.HelpDesc.Render(" " + msg + scrollInfo)
	return lipgloss.NewStyle().Width(m.width).Render(bar)
}
