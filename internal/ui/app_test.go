// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/bundle"
	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
)

var errCleanTranscriptTest = errors.New("boom")

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

func TestAppModelTranscriptKeyOnRender(t *testing.T) {
	b := bundle.RecordingBundle{Dir: t.TempDir(), PrimaryVideo: "test_video.mp4"}
	media := cutter.MediaInfo{Duration: 60 * time.Second}
	app := NewAppModel(b, media, makeTestCues(), model.TalkMetadata{Title: "Test Talk"}, "output.mp4")
	app.handleWindowSize(tea.WindowSizeMsg{Width: 100, Height: 30})
	app.screen = ScreenProg

	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if cmd == nil {
		t.Fatalf("expected a command to be returned for the transcript key on the render screen")
	}
}

func TestAppModelTranscriptMsgFeedback(t *testing.T) {
	b := bundle.RecordingBundle{Dir: t.TempDir(), PrimaryVideo: "test_video.mp4"}
	media := cutter.MediaInfo{Duration: 60 * time.Second}
	app := NewAppModel(b, media, makeTestCues(), model.TalkMetadata{Title: "Test Talk"}, "output.mp4")
	app.handleWindowSize(tea.WindowSizeMsg{Width: 100, Height: 30})

	res, _ := app.Update(transcriptMsg{path: "/tmp/out_transcript.md", polished: true})
	app = res.(AppModel)
	if !strings.Contains(app.progView.feedback, "local + AI polish") {
		t.Errorf("expected success feedback mentioning AI polish, got %q", app.progView.feedback)
	}
	if app.progView.feedbackErr {
		t.Errorf("expected non-error feedback")
	}

	res, _ = app.Update(transcriptMsg{err: errCleanTranscriptTest})
	app = res.(AppModel)
	if !app.progView.feedbackErr {
		t.Errorf("expected error feedback on failure")
	}
}

func TestBuildTalkDocIncludesMetadataAndTranscript(t *testing.T) {
	meta := model.TalkMetadata{
		Title:       "Diversity in Metric Spaces",
		Speaker:     "Marc van Kreveld",
		Affiliation: "Utrecht University",
		URL:         "https://example.org/talk",
		Privacy:     "public",
		Tags:        []string{"geometry", "diversity"},
		Abstract:    "A talk about diversity measures.",
		Chapters: []model.ChapterMarker{
			{AdjustedTime: 0, Title: "Introduction"},
			{AdjustedTime: 90 * time.Second, Title: "Shannon Index"},
		},
	}

	doc := buildTalkDoc(meta, "Marc van Kreveld: Hello and welcome.")
	for _, want := range []string{
		"# Diversity in Metric Spaces",
		"**Speaker:** Marc van Kreveld (Utrecht University)",
		"**Talk URL:** https://example.org/talk",
		"**Privacy:** public",
		"**Tags:** geometry, diversity",
		"## Abstract",
		"A talk about diversity measures.",
		"## Chapters",
		"00:00 Introduction",
		"01:30 Shannon Index",
		"## Transcript",
		"Marc van Kreveld: Hello and welcome.",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("talk doc missing %q\n---\n%s", want, doc)
		}
	}
}

func TestGenerateTranscriptKeepsOnlySurvivingText(t *testing.T) {
	dir := t.TempDir()
	outBase := filepath.Join(dir, "out")
	cues := []model.SubtitleCue{
		{ID: 1, Speaker: "Alice", Text: "Cut this entirely.", Action: model.ActionCut},
		{ID: 2, Speaker: "Alice", Text: "Keep this sentence.", Action: model.ActionKeep},
	}
	meta := model.TalkMetadata{Title: "Test Talk"}

	path, polished, err := generateTranscript(outBase, cues, meta, false)
	if err != nil {
		t.Fatalf("generateTranscript: %v", err)
	}
	if polished {
		t.Errorf("polished should be false when polish=false")
	}
	if want := outBase + transcriptSuffix; path != want {
		t.Errorf("path = %q, want %q", path, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	doc := string(data)
	if strings.Contains(doc, "Cut this entirely") {
		t.Errorf("cut cue leaked into transcript:\n%s", doc)
	}
	if !strings.Contains(doc, "Keep this sentence") {
		t.Errorf("kept cue missing from transcript:\n%s", doc)
	}
}

func TestGenerateTranscriptEmbedsChapterHeadings(t *testing.T) {
	dir := t.TempDir()
	outBase := filepath.Join(dir, "out")
	cues := []model.SubtitleCue{
		{ID: 1, Start: 0, End: time.Second, Speaker: "Alice", Text: "Welcome.", Action: model.ActionKeep},
		{ID: 2, Start: time.Second, End: 2 * time.Second, Speaker: "Alice", Text: "Let us begin.", Action: model.ActionKeep},
	}
	meta := model.TalkMetadata{
		Title: "T",
		Chapters: []model.ChapterMarker{
			{OriginalTime: 0, AdjustedTime: 0, Title: "Introduction"},
		},
	}

	path, _, err := generateTranscript(outBase, cues, meta, false)
	if err != nil {
		t.Fatalf("generateTranscript: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	doc := string(data)
	if !strings.Contains(doc, "### Introduction") {
		t.Errorf("chapter heading missing from transcript body:\n%s", doc)
	}
	if !strings.Contains(doc, "## Chapters") || !strings.Contains(doc, "00:00 Introduction") {
		t.Errorf("chapters section missing:\n%s", doc)
	}
}
