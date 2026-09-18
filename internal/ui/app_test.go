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

	// Direct jump '2' transitions from Cuts to Meta
	res, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	app = res.(AppModel)
	if app.screen != ScreenMeta {
		t.Fatalf("expected screen ScreenMeta after '2', got %d", app.screen)
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

func TestAppModelFiveTabNavigation(t *testing.T) {
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
		Chapters: []model.ChapterMarker{
			{OriginalTime: 10 * time.Second, Title: "Part 1"},
			{OriginalTime: 30 * time.Second, Title: "Part 2"},
		},
	}

	app := NewAppModel(b, media, cues, meta, "output.mp4")
	app.handleWindowSize(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Test direct numeric tab keys: 1, 2, 3, 4, 5
	tabs := []struct {
		key      string
		expected Screen
	}{
		{"2", ScreenMeta},
		{"3", ScreenChapters},
		{"4", ScreenProg},
		{"5", ScreenYouTube},
		{"1", ScreenCuts},
		{"3", ScreenChapters},
		{"2", ScreenMeta},
	}

	for _, tt := range tabs {
		res, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
		app = res.(AppModel)
		if app.screen != tt.expected {
			t.Fatalf("key %q: expected screen %d, got %d", tt.key, tt.expected, app.screen)
		}
		if v := app.View(); v == "" {
			t.Errorf("screen %d produced empty view", app.screen)
		}
	}

	// Test Alt+Right arrow cyclical navigation: 2 -> 3 -> 4 -> 5 -> 1 -> 2
	expectedRight := []Screen{ScreenChapters, ScreenProg, ScreenYouTube, ScreenCuts, ScreenMeta}
	for _, exp := range expectedRight {
		res, _ := app.Update(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
		app = res.(AppModel)
		if app.screen != exp {
			t.Fatalf("alt+right arrow: expected screen %d, got %d", exp, app.screen)
		}
	}

	// Test Alt+Left arrow cyclical navigation: 2 -> 1 -> 5 -> 4 -> 3 -> 2
	expectedLeft := []Screen{ScreenCuts, ScreenYouTube, ScreenProg, ScreenChapters, ScreenMeta}
	for _, exp := range expectedLeft {
		res, _ := app.Update(tea.KeyMsg{Type: tea.KeyLeft, Alt: true})
		app = res.(AppModel)
		if app.screen != exp {
			t.Fatalf("alt+left arrow: expected screen %d, got %d", exp, app.screen)
		}
	}

	// Verify regular Left/Right arrows do not switch tabs
	res, _ := app.Update(tea.KeyMsg{Type: tea.KeyRight})
	app = res.(AppModel)
	if app.screen != ScreenMeta {
		t.Fatalf("expected regular right arrow not to switch screen from ScreenMeta, got %d", app.screen)
	}
}

func TestAppModelChapterJumpKeys(t *testing.T) {
	b := bundle.RecordingBundle{
		Dir:            t.TempDir(),
		PrimaryVideo:   "test_video.mp4",
		TranscriptPath: "test_transcript.vtt",
	}
	media := cutter.MediaInfo{Duration: 60 * time.Second}
	cues := makeTestCues()
	meta := model.TalkMetadata{
		Title: "Test Talk",
		Chapters: []model.ChapterMarker{
			{OriginalTime: cues[1].Start, Title: "Cue 2 Start"},
		},
	}

	app := NewAppModel(b, media, cues, meta, "output.mp4")
	app.handleWindowSize(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Test Tab and ] jump to next chapter on ScreenCuts
	res, _ := app.Update(tea.KeyMsg{Type: tea.KeyTab})
	app = res.(AppModel)
	if app.screen != ScreenCuts {
		t.Fatalf("expected to remain on ScreenCuts, got %d", app.screen)
	}
	if app.cutsView.cursor != 1 {
		t.Fatalf("expected Tab to jump cursor to cue 1 (chapter marker), got %d", app.cutsView.cursor)
	}

	res, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	app = res.(AppModel)
	if app.screen != ScreenCuts {
		t.Fatalf("expected to remain on ScreenCuts, got %d", app.screen)
	}

	// Test [ jumps to prev chapter
	res, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	app = res.(AppModel)
	if app.screen != ScreenCuts {
		t.Fatalf("expected to remain on ScreenCuts, got %d", app.screen)
	}
}
