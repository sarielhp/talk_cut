// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
	"talk_cut/internal/vtt"
)

// ProgModel manages the FFmpeg video cutting progress and completion screen.
type ProgModel struct {
	theme        Theme
	progBar      progress.Model
	stage        string
	percent      float64
	detail       string
	done         bool
	err          error
	outputPath   string
	chaptersPath string
	vttPath      string
	talkMeta     model.TalkMetadata
	cuts         []model.CutInterval
	stats        model.CutStats
	inputVideo   string
	isCutting    bool
	width        int
	height       int
	statusMsg    string
}

// NewProgModel creates an initialized ProgModel.
func NewProgModel(outputPath string) ProgModel {
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithoutPercentage(),
	)
	p.Width = 60

	return ProgModel{
		theme:      DefaultTheme(),
		progBar:    p,
		stage:      "starting",
		percent:    0.0,
		detail:     "Preparing FFmpeg cut pipeline...",
		done:       false,
		outputPath: outputPath,
		width:      100,
		height:     30,
		statusMsg:  "Processing video slices...",
	}
}

// SetDimensions updates the viewport dimensions.
func (m *ProgModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
	if w > 20 {
		m.progBar.Width = w - 20
		if m.progBar.Width > 80 {
			m.progBar.Width = 80
		}
	}
}

// SetOutputPaths configures the exported artifact paths upon completion.
func (m *ProgModel) SetOutputPaths(video, chapters, vtt string) {
	m.outputPath = video
	m.chaptersPath = chapters
	m.vttPath = vtt
}

// SetPreflight updates the pre-flight export review information.
func (m *ProgModel) SetPreflight(meta model.TalkMetadata, cuts []model.CutInterval, stats model.CutStats, inputVideo string) {
	m.talkMeta = meta
	m.cuts = cuts
	m.stats = stats
	m.inputVideo = inputVideo
}

// SetCutting marks whether rendering is actively underway.
func (m *ProgModel) SetCutting(cutting bool) {
	m.isCutting = cutting
	if cutting {
		m.stage = "starting"
		m.detail = "Preparing FFmpeg cut pipeline..."
		m.statusMsg = "Rendering video segments with FFmpeg..."
	}
}

// SetProgress updates the progress state from cutter callbacks.
func (m *ProgModel) SetProgress(cp cutter.CutProgress) tea.Cmd {
	m.stage = cp.Stage
	m.percent = cp.Percent / 100.0
	if m.percent > 1.0 {
		m.percent = 1.0
	}

	switch cp.Stage {
	case "slicing":
		m.detail = fmt.Sprintf("Extracting segment %d of %d...", cp.PartIndex, cp.TotalParts)
	case "joining":
		m.detail = "Joining segments via lossless concat demuxer..."
	case "done":
		m.detail = "Finalizing video container and metadata..."
	default:
		m.detail = cp.Stage
	}

	return m.progBar.SetPercent(m.percent)
}

// SetDone marks processing as complete.
func (m *ProgModel) SetDone(err error) {
	m.done = true
	m.err = err
	if err != nil {
		m.stage = "error"
		m.statusMsg = fmt.Sprintf("Processing failed: %v", err)
	} else {
		m.stage = "done"
		m.percent = 1.0
		m.statusMsg = "Done! Press [p] to preview cut video, [q] to exit"
	}
}

// IsDone returns whether the render job has completed.
func (m ProgModel) IsDone() bool {
	return m.done
}

// Update handles terminal messages and key inputs during/after progress.
func (m ProgModel) Update(msg tea.Msg) (ProgModel, tea.Cmd) {
	switch msg := msg.(type) {
	case progress.FrameMsg:
		newModel, cmd := m.progBar.Update(msg)
		if pm, ok := newModel.(progress.Model); ok {
			m.progBar = pm
		}
		return m, cmd
	case tea.KeyMsg:
		if m.done {
			switch msg.String() {
			case "p":
				return m, m.launchPreview()
			}
		}
	}
	return m, nil
}

// launchPreview opens the rendered cut video in ffplay.
func (m ProgModel) launchPreview() tea.Cmd {
	videoPath := m.outputPath
	return func() tea.Msg {
		cmd := exec.Command("ffplay", "-autoexit", videoPath)
		cmd.Stdin = nil
		cmd.Stdout = nil
		cmd.Stderr = nil
		_ = cmd.Start()
		return previewMsg{msg: fmt.Sprintf("Playing %s (ffplay)...", videoPath)}
	}
}

// View renders the progress and completion screen.
func (m ProgModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return "Processing..."
	}

	header := m.renderHeader()
	content := m.renderBody()
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

// renderHeader renders the top title bar.
func (m ProgModel) renderHeader() string {
	return RenderTabBar(3, m.width, m.theme)
}

// renderBody renders the progress bar or completion summary.
func (m ProgModel) renderBody() string {
	if m.err != nil {
		lines := []string{
			m.theme.DangerText.Bold(true).Render("✗ RENDERING FAILED"),
			fmt.Sprintf("\nError: %v\n", m.err),
			"Press [q] or [esc] to return.",
		}
		box := m.theme.SidebarBox.Width(m.width - 4).Render(strings.Join(lines, "\n"))
		return box
	}

	if m.done {
		lines := []string{
			m.theme.SuccessText.Bold(true).Render("✔ SPLICING & EXPORT COMPLETE"),
			fmt.Sprintf("Video:    %s", m.outputPath),
		}
		if m.chaptersPath != "" {
			lines = append(lines, fmt.Sprintf("Chapters: %s", m.chaptersPath))
		}
		if m.vttPath != "" {
			lines = append(lines, fmt.Sprintf("Subtitles: %s", m.vttPath))
		}
		lines = append(lines, "", "[p] Preview Video    [u] Upload to YouTube (Tab 5)    [q] Exit talk_cut")

		content := strings.Join(lines, "\n")
		box := m.theme.SidebarBox.Width(m.width - 4).Render(content)
		return lipgloss.NewStyle().MarginLeft(2).Render(box)
	}

	if m.isCutting {
		pctStr := fmt.Sprintf("%.0f%%", m.percent*100.0)
		barLine := fmt.Sprintf("%s  %s", m.progBar.View(), m.theme.StatsValue.Render(pctStr))
		lines := []string{
			m.theme.TitleStyle.Render(" RENDERING IN PROGRESS "),
			"",
			barLine,
			m.theme.SubtitleStyle.Render(m.detail),
		}
		content := strings.Join(lines, "\n")
		box := m.theme.SidebarBox.Width(m.width - 4).Render(content)
		return lipgloss.NewStyle().MarginLeft(2).Render(box)
	}

	return m.renderPreflightBox()
}

// renderPreflightBox formats the pre-render summary and launch call-to-action.
func (m ProgModel) renderPreflightBox() string {
	boxWidth := m.width - 4
	if boxWidth < 20 {
		boxWidth = 20
	}

	title := m.theme.TitleStyle.Render(" PRE-FLIGHT EXPORT REVIEW ")
	privacy := strings.ToUpper(m.talkMeta.Privacy)
	if privacy == "" {
		privacy = "PUBLIC"
	}

	lines := []string{
		title,
		"",
		fmt.Sprintf("Title:        %s", m.talkMeta.Title),
		fmt.Sprintf("Speaker:      %s", m.talkMeta.Speaker),
		fmt.Sprintf("Privacy:      %s", privacy),
		fmt.Sprintf("Input Video:  %s", m.inputVideo),
		fmt.Sprintf("Output Video: %s", m.outputPath),
		"",
		fmt.Sprintf("Original Duration: %s", vtt.FormatTimestampShort(m.stats.TotalOriginal)),
		fmt.Sprintf("Kept Duration:     %s (%d cut regions removed, %s excised)",
			vtt.FormatTimestampShort(m.stats.TotalKept),
			m.stats.CutCount,
			vtt.FormatTimestampShort(m.stats.TotalCut),
		),
		fmt.Sprintf("YouTube Chapters:  %d chapter markers defined", len(m.talkMeta.Chapters)),
		"",
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#10B981")).Padding(0, 2).Render(" Press [c] or [Enter] to Start Video Rendering "),
	}

	content := strings.Join(lines, "\n")
	box := m.theme.SidebarBox.Width(boxWidth).Render(content)
	return lipgloss.NewStyle().MarginLeft(2).Render(box)
}

// renderFooter renders the bottom status bar.
func (m ProgModel) renderFooter() string {
	msg := m.statusMsg
	if !m.isCutting && !m.done && m.err == nil {
		msg = "Enter / c: start rendering | ←/→: switch tabs | Esc: cuts review"
	} else if m.done && m.err == nil {
		msg = "p: preview video | u: upload to YouTube (Tab 5) | ←/→: switch tabs | q: exit"
	}
	bar := m.theme.HelpDesc.Render(" " + msg)
	return lipgloss.NewStyle().Width(m.width).MarginTop(1).Render(bar)
}
