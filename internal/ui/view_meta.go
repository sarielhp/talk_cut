// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
)

const (
	fieldTitle = iota
	fieldSpeaker
	fieldAffiliation
	fieldTags
	fieldPrivacy
	fieldOutputPath
	fieldAbstract
	fieldCommit
	numFields
)

// MetaModel manages the metadata review and export screen.
type MetaModel struct {
	theme       Theme
	metadata    model.TalkMetadata
	cuts        []model.CutInterval
	inputs      []textinput.Model
	focusIndex  int
	privacyOpts []string
	privacyIdx  int
	outputPath  string
	width       int
	height      int
	statusMsg   string
}

// NewMetaModel creates an initialized MetaModel with inputs pre-populated.
func NewMetaModel(meta model.TalkMetadata, cuts []model.CutInterval, defaultOutput string) MetaModel {
	inputs := make([]textinput.Model, 6)
	labels := []string{"Title", "Speaker", "Affiliation", "Tags (comma-separated)", "Output Path", "Abstract"}
	values := []string{
		meta.Title,
		meta.Speaker,
		meta.Affiliation,
		strings.Join(meta.Tags, ", "),
		defaultOutput,
		meta.Abstract,
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
		theme:       DefaultTheme(),
		metadata:    meta,
		cuts:        cuts,
		inputs:      inputs,
		focusIndex:  0,
		privacyOpts: privacyOpts,
		privacyIdx:  pIdx,
		outputPath:  defaultOutput,
		width:       100,
		height:      30,
		statusMsg:   "Tab / Shift+Tab to switch fields, Ctrl+R to start FFmpeg cut, Esc to return to cuts",
	}
}

// Metadata returns the updated metadata from form fields.
func (m MetaModel) Metadata() model.TalkMetadata {
	meta := m.metadata
	meta.Title = m.inputs[0].Value()
	meta.Speaker = m.inputs[1].Value()
	meta.Affiliation = m.inputs[2].Value()

	tagsRaw := m.inputs[3].Value()
	var tags []string
	for _, tag := range strings.Split(tagsRaw, ",") {
		trimmed := strings.TrimSpace(tag)
		if trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	meta.Tags = tags
	meta.Privacy = m.privacyOpts[m.privacyIdx]
	meta.Abstract = m.inputs[5].Value()
	return meta
}

// OutputPath returns the user-configured output video path.
func (m MetaModel) OutputPath() string {
	return m.inputs[4].Value()
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

// Update processes terminal messages and user interactions.
func (m MetaModel) Update(msg tea.Msg) (MetaModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			m.cycleFocus(1)
			return m, nil
		case "shift+tab", "up":
			m.cycleFocus(-1)
			return m, nil
		case " ":
			if m.focusIndex == fieldPrivacy {
				m.privacyIdx = (m.privacyIdx + 1) % len(m.privacyOpts)
				return m, nil
			}
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
	case fieldTags:
		return 3
	case fieldOutputPath:
		return 4
	case fieldAbstract:
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

// View renders the metadata editor and chapters preview screen.
func (m MetaModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return "Initializing metadata..."
	}

	header := m.renderHeader()
	footer := m.renderFooter()

	rightWidth := 46
	if m.width > 120 {
		rightWidth = 52
	}
	leftWidth := m.width - rightWidth - 3
	if leftWidth < 35 {
		leftWidth = 35
	}

	leftView := m.renderForm(leftWidth)
	rightView := m.renderPreview(rightWidth)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftView, rightView)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// renderHeader renders the top title bar for the metadata screen.
func (m MetaModel) renderHeader() string {
	title := m.theme.TitleStyle.Render(" talk_cut > METADATA & CHAPTERS ")
	bar := lipgloss.NewStyle().Width(m.width).MarginBottom(1).Render(title)
	return bar
}

// renderForm renders all editable fields and action buttons on the left.
func (m MetaModel) renderForm(width int) string {
	var elements []string
	elements = append(elements, m.theme.SidebarBox.Width(width-2).Render("TALK & EXPORT CONFIGURATION"))

	elements = append(elements, m.renderFieldBox(fieldTitle, width, m.inputs[0].View()))
	elements = append(elements, m.renderFieldBox(fieldSpeaker, width, m.inputs[1].View()))
	elements = append(elements, m.renderFieldBox(fieldAffiliation, width, m.inputs[2].View()))
	elements = append(elements, m.renderFieldBox(fieldTags, width, m.inputs[3].View()))
	elements = append(elements, m.renderPrivacyField(width))
	elements = append(elements, m.renderFieldBox(fieldOutputPath, width, m.inputs[4].View()))
	elements = append(elements, m.renderFieldBox(fieldAbstract, width, m.inputs[5].View()))
	elements = append(elements, m.renderCommitButton(width))

	return lipgloss.JoinVertical(lipgloss.Left, elements...)
}

// renderFieldBox wraps an input view with styling based on focus.
func (m MetaModel) renderFieldBox(field, width int, content string) string {
	style := lipgloss.NewStyle().Width(width-2).Padding(0, 1)
	if m.focusIndex == field {
		style = style.Background(lipgloss.Color("#1E293B")).Bold(true)
	}
	return style.Render(content)
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

// renderPreview renders the right-hand preview panel for YouTube chapters.
func (m MetaModel) renderPreview(width int) string {
	adjChapters := cutter.AdjustChapters(m.metadata.Chapters, m.cuts, "Introduction")

	var chLines []string
	chLines = append(chLines, "YOUTUBE CHAPTERS PREVIEW")
	if len(adjChapters) == 0 {
		chLines = append(chLines, "  00:00 Introduction (full talk)")
	} else {
		for _, ch := range adjChapters {
			chLines = append(chLines, "  "+ch.FormatYouTubeLine())
		}
	}

	chaptersBox := m.theme.SidebarBox.Width(width - 2).Render(strings.Join(chLines, "\n"))

	metaPreview := fmt.Sprintf(
		"UPLOAD PREVIEW\nTitle:   %s\nSpeaker: %s\nPrivacy: %s\nOutput:  %s",
		m.inputs[0].Value(),
		m.inputs[1].Value(),
		strings.ToUpper(m.privacyOpts[m.privacyIdx]),
		m.inputs[4].Value(),
	)
	summaryBox := m.theme.SidebarBox.Width(width - 2).Render(metaPreview)

	content := lipgloss.JoinVertical(lipgloss.Left, chaptersBox, summaryBox)
	return lipgloss.NewStyle().Width(width).MarginLeft(1).Render(content)
}

// renderFooter renders the bottom help status bar.
func (m MetaModel) renderFooter() string {
	msg := m.statusMsg
	bar := m.theme.HelpDesc.Render(" " + msg)
	return lipgloss.NewStyle().Width(m.width).Render(bar)
}
