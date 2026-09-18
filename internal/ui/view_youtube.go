// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"talk_cut/internal/config"
	"talk_cut/internal/model"
	"talk_cut/internal/youtube"
)

// YouTubeModel manages the interactive YouTube upload, progress, and verification screen (Tab 5).
type YouTubeModel struct {
	theme           Theme
	progBar         progress.Model
	cfg             config.Config
	channels        []string
	channelIdx      int
	hasToken        bool
	tokenPath       string
	videoPath       string
	videoSize       int64
	videoExists     bool
	captionsPath    string
	captionsExists  bool
	talkMeta        model.TalkMetadata
	isUploading     bool
	stage           string
	percent         float64
	bytesSent       int64
	totalBytes      int64
	done            bool
	verification    *youtube.VideoVerification
	captionUploaded bool
	err             error
	width           int
	height          int
	statusMsg       string
	copiedFeedback  bool
}

// NewYouTubeModel creates an initialized YouTubeModel.
func NewYouTubeModel(videoPath, captionsPath string, meta model.TalkMetadata, cfg config.Config) YouTubeModel {
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithoutPercentage(),
	)
	p.Width = 60

	channels := cfg.AvailableChannels()

	m := YouTubeModel{
		theme:        DefaultTheme(),
		progBar:      p,
		cfg:          cfg,
		channels:     channels,
		channelIdx:   0,
		videoPath:    videoPath,
		captionsPath: captionsPath,
		talkMeta:     meta,
		stage:        "idle",
		width:        100,
		height:       30,
		statusMsg:    "Enter / u: upload to YouTube | c: cycle channel | 1..4: switch tabs",
	}

	m.refreshFileStats()
	m.refreshTokenStatus()
	return m
}

// SetDimensions updates the viewport dimensions.
func (m *YouTubeModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
	if w > 20 {
		m.progBar.Width = w - 20
		if m.progBar.Width > 80 {
			m.progBar.Width = 80
		}
	}
}

// SetPreflight synchronizes video artifact paths and talk metadata.
func (m *YouTubeModel) SetPreflight(videoPath, captionsPath string, meta model.TalkMetadata) {
	m.videoPath = videoPath
	m.captionsPath = captionsPath
	m.talkMeta = meta
	m.refreshFileStats()
	m.refreshTokenStatus()
}

// refreshFileStats inspects the filesystem for the target cut video and subtitles.
func (m *YouTubeModel) refreshFileStats() {
	if m.videoPath != "" {
		if stat, err := os.Stat(m.videoPath); err == nil && stat.Size() > 0 {
			m.videoExists = true
			m.videoSize = stat.Size()
		} else {
			m.videoExists = false
			m.videoSize = 0
		}
	}

	if m.captionsPath != "" {
		if stat, err := os.Stat(m.captionsPath); err == nil && stat.Size() > 0 {
			m.captionsExists = true
		} else {
			m.captionsExists = false
		}
	}
}

// refreshTokenStatus verifies OAuth token availability for the currently selected channel.
func (m *YouTubeModel) refreshTokenStatus() {
	channel := m.CurrentChannel()
	m.tokenPath = m.cfg.ResolveChannelTokenFile(channel)
	m.hasToken = m.cfg.HasValidChannelToken(channel)
}

// CurrentChannel returns the currently selected channel profile name.
func (m YouTubeModel) CurrentChannel() string {
	if len(m.channels) == 0 {
		return "default"
	}
	if m.channelIdx < 0 || m.channelIdx >= len(m.channels) {
		return m.channels[0]
	}
	return m.channels[m.channelIdx]
}

// NextChannel advances to the next configured YouTube channel profile.
func (m *YouTubeModel) NextChannel() {
	if len(m.channels) > 1 {
		m.channelIdx = (m.channelIdx + 1) % len(m.channels)
		m.refreshTokenStatus()
	}
}

// PrevChannel moves to the previous configured YouTube channel profile.
func (m *YouTubeModel) PrevChannel() {
	if len(m.channels) > 1 {
		m.channelIdx = (m.channelIdx - 1 + len(m.channels)) % len(m.channels)
		m.refreshTokenStatus()
	}
}

// SetUploading marks whether an upload is actively in progress.
func (m *YouTubeModel) SetUploading(uploading bool) {
	m.isUploading = uploading
	if uploading {
		m.stage = "uploading_video"
		m.done = false
		m.err = nil
		m.verification = nil
		m.captionUploaded = false
		m.percent = 0.0
		m.statusMsg = "Streaming video payload to YouTube..."
	}
}

// SetProgress updates the upload percentage and stage.
func (m *YouTubeModel) SetProgress(bytesSent, totalBytes int64, percent float64, stage string) tea.Cmd {
	m.bytesSent = bytesSent
	m.totalBytes = totalBytes
	m.percent = percent / 100.0
	if m.percent > 1.0 {
		m.percent = 1.0
	}
	m.stage = stage

	switch stage {
	case "uploading_video":
		m.statusMsg = fmt.Sprintf("Uploading video: %.1f%% (%s / %s)", percent, formatBytes(bytesSent), formatBytes(totalBytes))
	case "uploading_captions":
		m.statusMsg = "Uploading closed captions track to YouTube..."
	case "verifying":
		m.statusMsg = "Verifying publication and registration on YouTube..."
	}

	return m.progBar.SetPercent(m.percent)
}

// SetDone marks upload completion with verification details or an error.
func (m *YouTubeModel) SetDone(ver *youtube.VideoVerification, captionUploaded bool, err error) {
	m.isUploading = false
	m.done = true
	m.err = err
	m.verification = ver
	m.captionUploaded = captionUploaded

	if err != nil {
		m.stage = "error"
		m.statusMsg = fmt.Sprintf("Upload failed: %v", err)
		return
	}

	m.stage = "done"
	m.percent = 1.0
	m.statusMsg = "Published! [o] Open short URL | [c] Copy URL | [p] Preview video | [q] Exit"
}

// IsUploading returns whether an upload is currently streaming.
func (m YouTubeModel) IsUploading() bool {
	return m.isUploading
}

// IsDone returns whether the upload pipeline has completed.
func (m YouTubeModel) IsDone() bool {
	return m.done
}

// Verification returns the verified YouTube publication info if available.
func (m YouTubeModel) Verification() *youtube.VideoVerification {
	return m.verification
}

// SetCopiedFeedback updates the clipboard confirmation badge.
func (m *YouTubeModel) SetCopiedFeedback(copied bool) {
	m.copiedFeedback = copied
}

// Update handles progress bar frame ticks and key events.
func (m YouTubeModel) Update(msg tea.Msg) (YouTubeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case progress.FrameMsg:
		newModel, cmd := m.progBar.Update(msg)
		if pm, ok := newModel.(progress.Model); ok {
			m.progBar = pm
		}
		return m, cmd
	}
	return m, nil
}

// View renders the full YouTube tab screen.
func (m YouTubeModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return "YouTube Upload..."
	}

	header := m.renderHeader()
	body := m.renderBody()
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// renderHeader renders the top tab bar.
func (m YouTubeModel) renderHeader() string {
	return RenderTabBar(4, m.width, m.theme)
}

// renderBody dispatches to the appropriate content card based on current state.
func (m YouTubeModel) renderBody() string {
	if m.err != nil {
		return m.renderErrorBox()
	}
	if m.done && m.verification != nil {
		return m.renderSuccessBox()
	}
	if m.isUploading {
		return m.renderUploadingBox()
	}
	return m.renderPreflightBox()
}

// renderPreflightBox formats channel credentials, target video status, and metadata.
func (m YouTubeModel) renderPreflightBox() string {
	boxWidth := m.boxWidth()
	title := m.theme.TitleStyle.Render(" YOUTUBE PUBLISH & UPLOAD PRE-FLIGHT ")

	channelLine := fmt.Sprintf("Channel:       %s", m.theme.PrimaryText.Bold(true).Render(m.CurrentChannel()))
	if len(m.channels) > 1 {
		channelLine += fmt.Sprintf("  (press 'c' to cycle: %s)", strings.Join(m.channels, ", "))
	}

	var authBadge string
	if m.hasToken {
		authBadge = m.theme.SuccessText.Render("✔ OAuth Token Ready") + fmt.Sprintf(" (%s)", m.tokenPath)
	} else {
		authBadge = m.theme.DangerText.Render("✗ OAuth Token Missing") + fmt.Sprintf(" (run 'talk_cut auth --channel %s')", m.CurrentChannel())
	}

	var videoStatus string
	if m.videoExists {
		videoStatus = m.theme.SuccessText.Render(fmt.Sprintf("✔ Rendered Cut Video Ready (%s)", formatBytes(m.videoSize)))
	} else {
		videoStatus = m.theme.WarningText.Render("⚠ Cut video not rendered yet (press [4] to render, or [r] to render now)")
	}

	captionsStatus := "None"
	if m.captionsExists {
		captionsStatus = m.theme.SuccessText.Render("✔ Retimed WebVTT Closed Captions Found")
	}

	privacy := strings.ToUpper(m.talkMeta.Privacy)
	if privacy == "" {
		privacy = "PUBLIC"
	}

	lines := []string{
		title,
		"",
		channelLine,
		fmt.Sprintf("Authorization: %s", authBadge),
		"",
		fmt.Sprintf("Video Target:  %s", m.videoPath),
		fmt.Sprintf("Video Status:  %s", videoStatus),
		fmt.Sprintf("Captions:      %s", captionsStatus),
		"",
		fmt.Sprintf("Title:         %s", m.talkMeta.Title),
		fmt.Sprintf("Speaker:       %s", m.talkMeta.Speaker),
		fmt.Sprintf("Privacy:       %s", privacy),
		fmt.Sprintf("Chapters:      %d chapter markers defined", len(m.talkMeta.Chapters)),
	}

	if m.videoExists && m.hasToken {
		lines = append(lines,
			"",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#10B981")).Padding(0, 2).Render(" Press [u] or [Enter] to Start YouTube Upload "),
		)
	} else if !m.videoExists {
		lines = append(lines,
			"",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#F59E0B")).Padding(0, 2).Render(" Press [4] or [r] to Render Video First "),
		)
	}

	content := strings.Join(lines, "\n")
	box := m.theme.SidebarBox.Width(boxWidth).Render(content)
	return lipgloss.NewStyle().MarginLeft(2).Render(box)
}

// renderUploadingBox displays real-time video payload streaming and caption progress.
func (m YouTubeModel) renderUploadingBox() string {
	boxWidth := m.boxWidth()
	pctStr := fmt.Sprintf("%.1f%%", m.percent*100.0)
	barLine := fmt.Sprintf("%s  %s", m.progBar.View(), m.theme.StatsValue.Render(pctStr))

	var stageDesc string
	switch m.stage {
	case "uploading_video":
		stageDesc = fmt.Sprintf("Streaming video bytes to YouTube: %s / %s", formatBytes(m.bytesSent), formatBytes(m.totalBytes))
	case "uploading_captions":
		stageDesc = "Uploading retimed WebVTT subtitles as closed captions..."
	case "verifying":
		stageDesc = "Contacting YouTube Data API to verify video publication..."
	default:
		stageDesc = "Processing YouTube upload..."
	}

	lines := []string{
		m.theme.TitleStyle.Render(" YOUTUBE UPLOAD IN PROGRESS "),
		"",
		fmt.Sprintf("Channel: %s  |  Privacy: %s", m.CurrentChannel(), strings.ToUpper(m.talkMeta.Privacy)),
		"",
		barLine,
		"",
		m.theme.SubtitleStyle.Render(stageDesc),
	}

	content := strings.Join(lines, "\n")
	box := m.theme.SidebarBox.Width(boxWidth).Render(content)
	return lipgloss.NewStyle().MarginLeft(2).Render(box)
}

// renderSuccessBox presents the verified YouTube video, processing status, and short share link.
func (m YouTubeModel) renderSuccessBox() string {
	boxWidth := m.boxWidth()
	v := m.verification

	shortURLDisplay := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#06B6D4")).
		Render(v.ShortURL)

	hyperlink := fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", v.ShortURL, shortURLDisplay)

	procDesc := v.ProcessingStatus
	if procDesc == "" {
		procDesc = v.UploadStatus
	}

	lines := []string{
		m.theme.SuccessText.Bold(true).Render("✔ VIDEO PUBLISHED & VERIFIED ON YOUTUBE"),
		"",
		fmt.Sprintf("Title:       %s", v.Title),
		fmt.Sprintf("Channel:     %s", m.CurrentChannel()),
		fmt.Sprintf("Privacy:     %s", strings.ToUpper(v.PrivacyStatus)),
		fmt.Sprintf("Status:      %s (YouTube processing: %s)", strings.ToUpper(v.UploadStatus), procDesc),
		fmt.Sprintf("Video ID:    %s", v.VideoID),
		fmt.Sprintf("Chapters:    %d markers synchronized", len(m.talkMeta.Chapters)),
	}

	if m.captionUploaded {
		lines = append(lines, "Captions:    ✔ Closed captions uploaded (English)")
	}

	lines = append(lines,
		"",
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#10B981")).Render("Shareable Short URL:"),
		fmt.Sprintf("  %s", hyperlink),
	)

	if m.copiedFeedback {
		lines = append(lines, "  "+m.theme.SuccessText.Render("✔ Copied short URL to clipboard!"))
	}

	lines = append(lines,
		"",
		"[o] Open URL in Browser    [c] Copy Short URL    [p] Preview Local Cut    [q] Exit",
	)

	content := strings.Join(lines, "\n")
	box := m.theme.SidebarBox.Width(boxWidth).Render(content)
	return lipgloss.NewStyle().MarginLeft(2).Render(box)
}

// renderErrorBox formats an upload or verification failure.
func (m YouTubeModel) renderErrorBox() string {
	boxWidth := m.boxWidth()
	lines := []string{
		m.theme.DangerText.Bold(true).Render("✗ YOUTUBE UPLOAD FAILED"),
		fmt.Sprintf("\nError: %v\n", m.err),
		"Press [u] to retry, or [esc] to return to cuts review.",
	}
	box := m.theme.SidebarBox.Width(boxWidth).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().MarginLeft(2).Render(box)
}

// renderFooter renders the bottom status bar for Tab 5.
func (m YouTubeModel) renderFooter() string {
	msg := m.statusMsg
	if !m.isUploading && !m.done && m.err == nil {
		msg = "u / Enter: upload | c: switch channel | ←/→: switch tabs | Esc: cuts review"
	}
	bar := m.theme.HelpDesc.Render(" " + msg)
	return lipgloss.NewStyle().Width(m.width).MarginTop(1).Render(bar)
}

// boxWidth computes the interior content box width.
func (m YouTubeModel) boxWidth() int {
	w := m.width - 4
	if w < 20 {
		return 20
	}
	return w
}

// formatBytes formats an integer byte count into a human-readable size string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
