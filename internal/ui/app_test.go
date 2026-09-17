// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/bundle"
	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
)

func TestAppModelScreenTransitions(t *testing.T) {
	b := bundle.RecordingBundle{
		Dir:            t.TempDir(),
		PrimaryVideo:   "test_video.mp4",
		TranscriptPath: "test_transcript.vtt",
	}
	media := cutter.MediaInfo{Duration: 60 * time.Second}
	cues := makeTestCues()
	meta := model.TalkMetadata{
		Title:   "Test Talk",
		Speaker: "Speaker Name",
	}

	app := NewAppModel(b, media, cues, meta, "output.mp4")
	app.handleWindowSize(tea.WindowSizeMsg{Width: 100, Height: 30})

	if app.screen != ScreenCuts {
		t.Fatalf("expected initial screen ScreenCuts, got %d", app.screen)
	}

	// Tab transitions from Cuts to Meta
	res, _ := app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = res.(AppModel)
	if app.screen != ScreenMeta {
		t.Fatalf("expected screen ScreenMeta after tab, got %d", app.screen)
	}

	// Esc transitions back from Meta to Cuts
	res, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = res.(AppModel)
	if app.screen != ScreenCuts {
		t.Fatalf("expected screen ScreenCuts after esc, got %d", app.screen)
	}

	// Render view test
	view := app.View()
	if view == "" {
		t.Errorf("expected non-empty app view")
	}
}
