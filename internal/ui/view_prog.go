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
	title := m.theme.TitleStyle.Render(" talk_cut > RENDERING VIDEO ")
	bar := lipgloss.NewStyle().Width(m.width).MarginBottom(2).Render(title)
	return bar
}

// renderBody renders the progress bar or completion summary.
func (m ProgModel) renderBody() string {
	var lines []string

	if m.err != nil {
		lines = append(lines, m.theme.DangerText.Bold(true).Render("✗ RENDERING FAILED"))
		lines = append(lines, fmt.Sprintf("\nError: %v\n", m.err))
		lines = append(lines, "Press [q] or [esc] to return.")
		box := m.theme.SidebarBox.Width(m.width - 4).Render(strings.Join(lines, "\n"))
		return box
	}

	pctStr := fmt.Sprintf("%.0f%%", m.percent*100.0)
	barLine := fmt.Sprintf("%s  %s", m.progBar.View(), m.theme.StatsValue.Render(pctStr))
	lines = append(lines, barLine)
	lines = append(lines, m.theme.SubtitleStyle.Render(m.detail))

	if m.done {
		lines = append(lines, "")
		lines = append(lines, m.theme.SuccessText.Bold(true).Render("✔ SPLICING & EXPORT COMPLETE"))
		lines = append(lines, fmt.Sprintf("Video:    %s", m.outputPath))
		if m.chaptersPath != "" {
			lines = append(lines, fmt.Sprintf("Chapters: %s", m.chaptersPath))
		}
		if m.vttPath != "" {
			lines = append(lines, fmt.Sprintf("Subtitles: %s", m.vttPath))
		}
		lines = append(lines, "")
		lines = append(lines, "[p] Preview Cut Video in ffplay    [q] Exit talk_cut")
	}

	content := strings.Join(lines, "\n")
	box := m.theme.SidebarBox.Width(m.width - 4).Render(content)
	return lipgloss.NewStyle().MarginLeft(2).Render(box)
}

// renderFooter renders the bottom status bar.
func (m ProgModel) renderFooter() string {
	msg := m.statusMsg
	bar := m.theme.HelpDesc.Render(" " + msg)
	return lipgloss.NewStyle().Width(m.width).MarginTop(2).Render(bar)
}
