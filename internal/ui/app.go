// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/bundle"
	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
	"talk_cut/internal/vtt"
)

// Screen represents the currently active view.
type Screen int

const (
	// ScreenCuts displays the interactive cut review and transcript.
	ScreenCuts Screen = iota
	// ScreenMeta displays metadata review, tags, and chapter timing.
	ScreenMeta
	// ScreenProg displays the FFmpeg cutting progress and final export.
	ScreenProg
)

type progressMsg cutter.CutProgress

type cutDoneMsg struct {
	err          error
	videoPath    string
	chaptersPath string
	vttPath      string
}

// AppModel is the root Bubble Tea application model.
type AppModel struct {
	screen    Screen
	cutsView  CutsModel
	metaView  MetaModel
	progView  ProgModel
	bundle    bundle.RecordingBundle
	media     cutter.MediaInfo
	progChan  chan cutter.CutProgress
	doneChan  chan cutDoneMsg
	width     int
	height    int
	isCutting bool
}

// NewAppModel creates an initialized AppModel.
func NewAppModel(
	b bundle.RecordingBundle,
	media cutter.MediaInfo,
	cues []model.SubtitleCue,
	meta model.TalkMetadata,
	defaultOutput string,
) AppModel {
	cuts := model.BuildCutIntervals(cues)
	return AppModel{
		screen:    ScreenCuts,
		cutsView:  NewCutsModel(cues, media, b.PrimaryVideo),
		metaView:  NewMetaModel(meta, cuts, defaultOutput),
		progView:  NewProgModel(defaultOutput),
		bundle:    b,
		media:     media,
		progChan:  make(chan cutter.CutProgress, 32),
		doneChan:  make(chan cutDoneMsg, 1),
		width:     100,
		height:    30,
		isCutting: false,
	}
}

// Init initializes the Bubble Tea application.
func (a AppModel) Init() tea.Cmd {
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
	a.progView.SetDimensions(msg.Width, msg.Height)
}

// handleKey routes key inputs based on current screen.
func (a AppModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return a, tea.Quit
	}

	switch a.screen {
	case ScreenCuts:
		return a.handleCutsKey(msg)
	case ScreenMeta:
		return a.handleMetaKey(msg)
	case ScreenProg:
		return a.handleProgKey(msg)
	}

	return a, nil
}

// handleCutsKey processes keys on the cut review screen.
func (a AppModel) handleCutsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return a, tea.Quit
	case "tab", "enter":
		intervals := model.BuildCutIntervals(a.cutsView.Cues())
		a.metaView.UpdateCuts(intervals)
		a.screen = ScreenMeta
		return a, nil
	}

	var cmd tea.Cmd
	a.cutsView, cmd = a.cutsView.Update(msg)
	return a, cmd
}

// handleMetaKey processes keys on the metadata editor screen.
func (a AppModel) handleMetaKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.screen = ScreenCuts
		return a, nil
	case "ctrl+r":
		return a.startRender()
	case "enter":
		if a.metaView.focusIndex == fieldCommit {
			return a.startRender()
		}
	}

	var cmd tea.Cmd
	a.metaView, cmd = a.metaView.Update(msg)
	return a, cmd
}

// handleProgKey processes keys on the progress screen.
func (a AppModel) handleProgKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.progView.IsDone() && (msg.String() == "q" || msg.String() == "esc") {
		return a, tea.Quit
	}

	var cmd tea.Cmd
	a.progView, cmd = a.progView.Update(msg)
	return a, cmd
}

// startRender switches to progress view and fires the background cut pipeline.
func (a AppModel) startRender() (tea.Model, tea.Cmd) {
	if a.isCutting {
		return a, nil
	}
	a.isCutting = true
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
	case ScreenProg:
		a.progView, cmd = a.progView.Update(msg)
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
	case ScreenProg:
		return a.progView.View()
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

	writeChaptersFile(chaptersPath, meta.Chapters, cuts)
	writeAdjustedVTT(vttPath, cues, cuts)

	doneChan <- cutDoneMsg{
		videoPath:    outVideo,
		chaptersPath: chaptersPath,
		vttPath:      vttPath,
	}
}

// writeChaptersFile writes adjusted YouTube chapters to disk.
func writeChaptersFile(path string, rawChapters []model.ChapterMarker, cuts []model.CutInterval) {
	adj := cutter.AdjustChapters(rawChapters, cuts, "Introduction")
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	for _, ch := range adj {
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
