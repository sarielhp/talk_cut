// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/ai"
	"talk_cut/internal/bundle"
	"talk_cut/internal/config"
	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
	"talk_cut/internal/vtt"
	"talk_cut/internal/youtube"
)

// Screen represents the currently active view.
type Screen int

const (
	// ScreenCuts displays the interactive cut review and transcript.
	ScreenCuts Screen = iota
	// ScreenMeta displays metadata review, tags, and talk settings.
	ScreenMeta
	// ScreenChapters displays YouTube chapter markers manager.
	ScreenChapters
	// ScreenProg displays pre-flight export summary, FFmpeg cutting progress, and final review.
	ScreenProg
	// ScreenYouTube displays interactive YouTube upload, streaming progress, and verification.
	ScreenYouTube
)

type progressMsg cutter.CutProgress

type cutDoneMsg struct {
	err          error
	videoPath    string
	chaptersPath string
	vttPath      string
}

type aiChaptersMsg struct {
	chapters []model.ChapterMarker
	err      error
}

type ytProgressMsg struct {
	bytesSent  int64
	totalBytes int64
	percent    float64
	stage      string
}

type ytDoneMsg struct {
	verification    *youtube.VideoVerification
	captionUploaded bool
	playlists       []model.PlaylistRef
	err             error
}

type ytCopiedMsg struct{}

// AppModel is the root Bubble Tea application model.
type AppModel struct {
	screen       Screen
	cutsView     CutsModel
	metaView     MetaModel
	chaptersView ChaptersModel
	progView     ProgModel
	youtubeView  YouTubeModel
	bundle       bundle.RecordingBundle
	media        cutter.MediaInfo
	progChan     chan cutter.CutProgress
	doneChan     chan cutDoneMsg
	ytProgChan   chan ytProgressMsg
	ytDoneChan   chan ytDoneMsg
	width        int
	height       int
	isCutting    bool
	isUploading  bool
	cfg          config.Config
}

// NewAppModel creates an initialized AppModel.
func NewAppModel(
	b bundle.RecordingBundle,
	media cutter.MediaInfo,
	cues []model.SubtitleCue,
	meta model.TalkMetadata,
	defaultOutput string,
) AppModel {
	meta.Chapters = cutter.SnapChaptersToCues(meta.Chapters, cues)
	cuts := model.BuildCutIntervals(cues)
	cutsView := NewCutsModel(cues, media, b.PrimaryVideo, b.Dir)
	cutsView.SetChapters(meta.Chapters)

	metaView := NewMetaModel(meta, cuts, defaultOutput)
	chaptersView := NewChaptersModel(meta.Chapters, cuts, cues, b.PrimaryVideo)
	progView := NewProgModel(defaultOutput)
	stats := model.ComputeStats(cues, media.Duration)
	progView.SetPreflight(meta, cuts, stats, b.PrimaryVideo)

	cfg, _ := config.LoadConfig()
	outBase := strings.TrimSuffix(defaultOutput, filepath.Ext(defaultOutput))
	vttPath := outBase + ".vtt"
	youtubeView := NewYouTubeModel(defaultOutput, vttPath, meta, cfg)

	return AppModel{
		screen:       ScreenCuts,
		cutsView:     cutsView,
		metaView:     metaView,
		chaptersView: chaptersView,
		progView:     progView,
		youtubeView:  youtubeView,
		bundle:       b,
		media:        media,
		progChan:     make(chan cutter.CutProgress, 32),
		doneChan:     make(chan cutDoneMsg, 1),
		ytProgChan:   make(chan ytProgressMsg, 32),
		ytDoneChan:   make(chan ytDoneMsg, 1),
		width:        100,
		height:       30,
		isCutting:    false,
		isUploading:  false,
		cfg:          cfg,
	}
}

// Init initializes the Bubble Tea application.
func (a AppModel) Init() tea.Cmd {
	if a.youtubeView.VideoID() == "" {
		return a.checkYouTubeExistingUpload()
	}
	return nil
}

// Update coordinates messages and screen transitions.
func (a AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.handleWindowSize(msg)
		return a, nil
	case progressMsg:
		cmd := a.progView.SetProgress(cutter.CutProgress(msg))
		return a, tea.Batch(cmd, a.listenNextEvent())
	case cutDoneMsg:
		a.isCutting = false
		a.progView.SetDone(msg.err)
		if msg.err == nil {
			a.progView.SetOutputPaths(msg.videoPath, msg.chaptersPath, msg.vttPath)
		}
		return a, nil
	case ytProgressMsg:
		cmd := a.youtubeView.SetProgress(msg.bytesSent, msg.totalBytes, msg.percent, msg.stage)
		return a, tea.Batch(cmd, a.listenNextYtEvent())
	case ytDoneMsg:
		a.isUploading = false
		a.youtubeView.SetDone(msg.verification, msg.captionUploaded, msg.err)
		if msg.err == nil && msg.verification != nil && msg.verification.VideoID != "" {
			meta := a.metaView.Metadata()
			meta.YouTubeID = msg.verification.VideoID
			meta.YouTubeURL = msg.verification.ShortURL
			if meta.YouTubeURL == "" {
				meta.YouTubeURL = fmt.Sprintf("https://youtu.be/%s", msg.verification.VideoID)
			}
			for _, pl := range msg.playlists {
				meta.AddPlaylist(pl.ID, pl.Title)
				a.youtubeView.SetPlaylistAdded(pl.ID, pl.Title)
			}
			a.metaView.ApplyMetadata(meta)
			_ = model.SaveMetaFile(a.bundle.Dir, meta)
		}
		return a, nil
	case ytDiscoveredMsg:
		return a.handleYTDiscoveredMsg(msg)
	case ytPlaylistsLoadedMsg:
		return a.handleYTPlaylistsLoadedMsg(msg)
	case ytVideoAddedToPlaylistMsg:
		return a.handleYTVideoAddedToPlaylistMsg(msg)
	case ytVideoRemovedFromPlaylistMsg:
		return a.handleYTVideoRemovedFromPlaylistMsg(msg)
	case ytCopiedMsg:
		a.youtubeView.SetCopiedFeedback(true)
		return a, nil
	case aiChaptersMsg:
		return a.handleAIChaptersMsg(msg)
	case emlMetadataMsg:
		return a.handleEMLMetadataMsg(msg)
	case urlMetadataMsg:
		return a.handleURLMetadataMsg(msg)
	case tea.KeyMsg:
		return a.handleKey(msg)
	}

	return a.forwardToActiveView(msg)
}

// handleWindowSize updates all views with the new terminal size.
func (a *AppModel) handleWindowSize(msg tea.WindowSizeMsg) {
	a.width = msg.Width
	a.height = msg.Height
	a.cutsView.SetDimensions(msg.Width, msg.Height)
	a.metaView.SetDimensions(msg.Width, msg.Height)
	a.chaptersView.SetDimensions(msg.Width, msg.Height)
	a.progView.SetDimensions(msg.Width, msg.Height)
	a.youtubeView.SetDimensions(msg.Width, msg.Height)
}

// isEditingText returns whether an active text input currently has focus.
func (a AppModel) isEditingText() bool {
	if a.screen == ScreenMeta && a.metaView.IsEditing() {
		return true
	}
	if a.screen == ScreenChapters && a.chaptersView.IsEditing() {
		return true
	}
	return false
}

// switchTab navigates to a new tab while synchronizing cuts, chapters, and metadata.
func (a *AppModel) switchTab(target Screen) {
	if a.screen == target {
		return
	}

	cues := a.cutsView.Cues()
	cuts := model.BuildCutIntervals(cues)

	switch a.screen {
	case ScreenCuts:
		a.metaView.UpdateCuts(cuts)
		a.chaptersView.UpdateCuts(cuts)
		a.chaptersView.SetCues(cues)
		if len(a.cutsView.Chapters()) > 0 {
			aligned := cutter.AlignChaptersToKeptCues(a.cutsView.Chapters(), cues)
			a.cutsView.SetChapters(aligned)
			a.chaptersView.SetChapters(aligned)
			a.metaView.SetChapters(aligned)
		}
	case ScreenMeta:
		meta := a.metaView.Metadata()
		_ = model.SaveMetaFile(a.bundle.Dir, meta)
		a.cutsView.SetChapters(meta.Chapters)
		a.chaptersView.SetChapters(meta.Chapters)
	case ScreenChapters:
		chapters := a.chaptersView.Chapters()
		a.cutsView.SetChapters(chapters)
		a.metaView.SetChapters(chapters)
		meta := a.metaView.Metadata()
		meta.Chapters = chapters
		_ = model.SaveMetaFile(a.bundle.Dir, meta)
	}

	if target == ScreenProg {
		stats := model.ComputeStats(cues, a.media.Duration)
		meta := a.metaView.Metadata()
		aligned := cutter.AlignChaptersToKeptCues(meta.Chapters, cues)
		meta.Chapters = cutter.AdjustChapters(aligned, cuts, "Introduction")
		a.progView.SetPreflight(meta, cuts, stats, a.bundle.PrimaryVideo)
	}

	if target == ScreenYouTube {
		meta := a.metaView.Metadata()
		aligned := cutter.AlignChaptersToKeptCues(meta.Chapters, cues)
		meta.Chapters = cutter.AdjustChapters(aligned, cuts, "Introduction")
		outVideo := a.metaView.OutputPath()
		outBase := strings.TrimSuffix(outVideo, filepath.Ext(outVideo))
		vttPath := outBase + ".vtt"
		a.youtubeView.SetPreflight(outVideo, vttPath, meta)
	}

	a.screen = target
}

// handleKey routes key inputs based on current screen and global navigation.
func (a AppModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		a.cutsView.Close()
		return a, tea.Quit
	}

	if a.handleGlobalNav(msg) {
		return a, nil
	}

	switch a.screen {
	case ScreenCuts:
		return a.handleCutsKey(msg)
	case ScreenMeta:
		return a.handleMetaKey(msg)
	case ScreenChapters:
		return a.handleChaptersKey(msg)
	case ScreenProg:
		return a.handleProgKey(msg)
	case ScreenYouTube:
		return a.handleYouTubeKey(msg)
	}

	return a, nil
}

// handleGlobalNav routes Alt+arrows and numeric tab switching across all views.
func (a *AppModel) handleGlobalNav(msg tea.KeyMsg) bool {
	if a.screen == ScreenYouTube && a.youtubeView.IsSelectingPlaylist() {
		return false
	}
	if msg.String() == "alt+left" || (msg.Alt && msg.Type == tea.KeyLeft) {
		prev := (int(a.screen) - 1 + 5) % 5
		a.switchTab(Screen(prev))
		return true
	}
	if msg.String() == "alt+right" || (msg.Alt && msg.Type == tea.KeyRight) {
		next := (int(a.screen) + 1) % 5
		a.switchTab(Screen(next))
		return true
	}

	if !a.isEditingText() {
		switch msg.String() {
		case "1":
			a.switchTab(ScreenCuts)
			return true
		case "2":
			a.switchTab(ScreenMeta)
			return true
		case "3":
			a.switchTab(ScreenChapters)
			return true
		case "4":
			a.switchTab(ScreenProg)
			return true
		case "5":
			a.switchTab(ScreenYouTube)
			return true
		}
	}
	return false
}

// handleCutsKey processes keys on the cut review screen.
func (a AppModel) handleCutsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		a.cutsView.Close()
		return a, tea.Quit
	case "tab", "]":
		a.cutsView.JumpChapter(1)
		return a, nil
	case "shift+tab", "[":
		a.cutsView.JumpChapter(-1)
		return a, nil
	case "ctrl+r", "c":
		a.switchTab(ScreenProg)
		return a.startRender()
	case "r", "ctrl+a":
		return a, a.triggerAIChapters()
	}

	var cmd tea.Cmd
	a.cutsView, cmd = a.cutsView.Update(msg)
	return a, cmd
}

// handleMetaKey processes keys on the metadata editor screen.
func (a AppModel) handleMetaKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !a.metaView.IsEditing() {
		switch msg.String() {
		case "esc":
			a.switchTab(ScreenCuts)
			return a, nil
		case "ctrl+r", "c":
			a.switchTab(ScreenProg)
			return a.startRender()
		case "ctrl+a", "r":
			return a, a.triggerAIChapters()
		case "e", "E":
			return a, a.triggerEMLMetadata()
		case "u", "U":
			return a, a.triggerURLMetadata()
		case "enter":
			if a.metaView.focusIndex == fieldCommit {
				a.switchTab(ScreenProg)
				return a.startRender()
			}
		}
	}

	var cmd tea.Cmd
	a.metaView, cmd = a.metaView.Update(msg)
	meta := a.metaView.Metadata()
	_ = model.SaveMetaFile(a.bundle.Dir, meta)
	return a, cmd
}

// handleChaptersKey processes keys on the chapters manager screen.
func (a AppModel) handleChaptersKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !a.chaptersView.IsEditing() {
		switch msg.String() {
		case "esc":
			a.switchTab(ScreenCuts)
			return a, nil
		case "r", "ctrl+a":
			return a, a.triggerAIChapters()
		case "ctrl+r", "c":
			a.switchTab(ScreenProg)
			return a.startRender()
		}
	}

	var cmd tea.Cmd
	a.chaptersView, cmd = a.chaptersView.Update(msg)
	meta := a.metaView.Metadata()
	meta.Chapters = a.chaptersView.Chapters()
	_ = model.SaveMetaFile(a.bundle.Dir, meta)
	return a, cmd
}

// handleProgKey processes keys on the progress and pre-flight screen.
func (a AppModel) handleProgKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.progView.IsDone() {
		switch msg.String() {
		case "q", "esc":
			a.cutsView.Close()
			return a, tea.Quit
		case "u":
			a.switchTab(ScreenYouTube)
			return a, nil
		}
	}

	if !a.isCutting {
		switch msg.String() {
		case "esc":
			a.switchTab(ScreenCuts)
			return a, nil
		case "c", "enter":
			return a.startRender()
		case "q":
			a.cutsView.Close()
			return a, tea.Quit
		}
	}

	var cmd tea.Cmd
	a.progView, cmd = a.progView.Update(msg)
	return a, cmd
}

// handleAIChaptersMsg processes the asynchronous AI chapter detection result.
func (a *AppModel) handleAIChaptersMsg(msg aiChaptersMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		errStr := fmt.Sprintf("AI chapter error: %v", msg.err)
		a.cutsView.SetFeedback(errStr, true)
		a.metaView.SetFeedback(errStr, true)
		return *a, clearStatusCmd()
	}

	snapped := cutter.SnapChaptersToCues(msg.chapters, a.cutsView.Cues())
	a.metaView.SetChapters(snapped)
	a.cutsView.SetChapters(snapped)
	a.chaptersView.SetChapters(snapped)

	meta := a.metaView.Metadata()
	meta.Chapters = snapped
	_ = model.SaveMetaFile(a.bundle.Dir, meta)

	feedback := fmt.Sprintf("AI generated %d natural chapters", len(msg.chapters))
	a.cutsView.SetFeedback(feedback, false)
	a.metaView.SetFeedback(feedback, false)
	return *a, clearStatusCmd()
}

// triggerAIChapters dispatches a background task to analyze kept cues for chapters.
func (a *AppModel) triggerAIChapters() tea.Cmd {
	var kept []model.SubtitleCue
	for _, c := range a.cutsView.Cues() {
		if c.Action != model.ActionCut {
			kept = append(kept, c)
		}
	}
	if len(kept) == 0 {
		a.cutsView.SetFeedback("Cannot generate chapters: all cues cut", true)
		a.metaView.SetFeedback("Cannot generate chapters: all cues cut", true)
		return clearStatusCmd()
	}

	a.cutsView.SetFeedback("Analyzing kept speech for natural chapters...", false)
	a.metaView.SetFeedback("Analyzing kept speech for natural chapters...", false)

	meta := a.metaView.Metadata()
	return func() tea.Msg {
		cfg, err := config.LoadConfig()
		if err != nil {
			return aiChaptersMsg{err: err}
		}
		client, err := ai.NewClient(cfg)
		if err != nil {
			return aiChaptersMsg{err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		chapters, err := ai.DetectChapters(ctx, client, kept, meta.Title, meta.Abstract)
		return aiChaptersMsg{chapters: chapters, err: err}
	}
}

// startRender switches to progress view and fires the background cut pipeline.
func (a AppModel) startRender() (tea.Model, tea.Cmd) {
	if a.isCutting {
		return a, nil
	}
	a.cutsView.Close()
	a.isCutting = true
	a.progView.SetCutting(true)
	a.screen = ScreenProg
	return a, a.startCuttingPipeline()
}

// forwardToActiveView forwards non-key messages to the active view.
func (a AppModel) forwardToActiveView(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch a.screen {
	case ScreenCuts:
		a.cutsView, cmd = a.cutsView.Update(msg)
	case ScreenMeta:
		a.metaView, cmd = a.metaView.Update(msg)
	case ScreenChapters:
		a.chaptersView, cmd = a.chaptersView.Update(msg)
	case ScreenProg:
		a.progView, cmd = a.progView.Update(msg)
	case ScreenYouTube:
		a.youtubeView, cmd = a.youtubeView.Update(msg)
	}
	return a, cmd
}

// View renders the current active screen.
func (a AppModel) View() string {
	switch a.screen {
	case ScreenCuts:
		return a.cutsView.View()
	case ScreenMeta:
		return a.metaView.View()
	case ScreenChapters:
		return a.chaptersView.View()
	case ScreenProg:
		return a.progView.View()
	case ScreenYouTube:
		return a.youtubeView.View()
	}
	return "talk_cut"
}

// listenNextEvent returns a tea.Cmd waiting on progress updates or completion.
func (a AppModel) listenNextEvent() tea.Cmd {
	progChan := a.progChan
	doneChan := a.doneChan
	return func() tea.Msg {
		select {
		case p, ok := <-progChan:
			if ok {
				return progressMsg(p)
			}
		case d := <-doneChan:
			return d
		}
		return nil
	}
}

// startCuttingPipeline initiates the background FFmpeg slice-and-join worker.
func (a AppModel) startCuttingPipeline() tea.Cmd {
	cues := a.cutsView.Cues()
	cuts := model.BuildCutIntervals(cues)
	meta := a.metaView.Metadata()
	outVideo := a.metaView.OutputPath()
	inVideo := a.bundle.PrimaryVideo
	progChan := a.progChan
	doneChan := a.doneChan

	return func() tea.Msg {
		go runCutWorker(inVideo, outVideo, cuts, cues, meta, progChan, doneChan)
		return a.listenNextEvent()()
	}
}

// runCutWorker executes the video cutting and metadata exports in the background.
func runCutWorker(
	inVideo, outVideo string,
	cuts []model.CutInterval,
	cues []model.SubtitleCue,
	meta model.TalkMetadata,
	progChan chan<- cutter.CutProgress,
	doneChan chan<- cutDoneMsg,
) {
	opts := cutter.CutOptions{
		InputPath:  inVideo,
		OutputPath: outVideo,
		Cuts:       cuts,
		Mode:       cutter.ModeLossless,
		OnProgress: func(p cutter.CutProgress) {
			progChan <- p
		},
	}

	if err := cutter.CutVideo(context.Background(), opts); err != nil {
		doneChan <- cutDoneMsg{err: err}
		return
	}

	outBase := strings.TrimSuffix(outVideo, filepath.Ext(outVideo))
	chaptersPath := outBase + "_chapters.txt"
	vttPath := outBase + ".vtt"

	aligned := cutter.AlignChaptersToKeptCues(meta.Chapters, cues)
	normalized := cutter.AdjustChapters(aligned, cuts, "Introduction")
	meta.Chapters = normalized
	_ = model.SaveMetaFile(filepath.Dir(outVideo), meta)

	writeChaptersFile(chaptersPath, normalized)
	writeAdjustedVTT(vttPath, cues, cuts)

	doneChan <- cutDoneMsg{
		videoPath:    outVideo,
		chaptersPath: chaptersPath,
		vttPath:      vttPath,
	}
}

// writeChaptersFile writes adjusted YouTube chapters to disk.
func writeChaptersFile(path string, chapters []model.ChapterMarker) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	for _, ch := range chapters {
		fmt.Fprintln(f, ch.FormatYouTubeLine())
	}
}

// writeAdjustedVTT writes excised & retimed WebVTT subtitles to disk.
func writeAdjustedVTT(path string, cues []model.SubtitleCue, cuts []model.CutInterval) {
	var kept []model.SubtitleCue
	for _, c := range cues {
		if c.Action == model.ActionCut {
			continue
		}
		adjCue := c
		adjCue.Start = cutter.AdjustTime(c.Start, cuts)
		adjCue.End = cutter.AdjustTime(c.End, cuts)
		kept = append(kept, adjCue)
	}

	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	_ = vtt.Write(f, kept)
}
