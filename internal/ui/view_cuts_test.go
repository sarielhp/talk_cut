// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
)

func makeTestCues() []model.SubtitleCue {
	return []model.SubtitleCue{
		{
			ID:      1,
			Start:   0,
			End:     3 * time.Second,
			Speaker: "Alice",
			Text:    "Hello, can you hear me?",
			Action:  model.ActionCut,
		},
		{
			ID:      2,
			Start:   3 * time.Second,
			End:     10 * time.Second,
			Speaker: "Bob",
			Text:    "Yes, we can hear you clearly.",
			Action:  model.ActionKeep,
		},
		{
			ID:      3,
			Start:   10 * time.Second,
			End:     20 * time.Second,
			Speaker: "Alice",
			Text:    "Great! Let us begin the presentation.",
			Action:  model.ActionKeep,
		},
		{
			ID:      4,
			Start:   20 * time.Second,
			End:     25 * time.Second,
			Speaker: "Alice",
			Text:    "Thank you everyone for joining.",
			Action:  model.ActionCut,
		},
	}
}

func TestCutsModelNavigation(t *testing.T) {
	cues := makeTestCues()
	media := cutter.MediaInfo{Duration: 30 * time.Second}
	m := NewCutsModel(cues, media, "test.mp4", "")
	m.SetDimensions(80, 24)

	if m.cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", m.cursor)
	}

	// Move down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("expected cursor at 1 after down, got %d", m.cursor)
	}

	// Move up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("expected cursor at 0 after up, got %d", m.cursor)
	}

	// Move up past bound
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("expected cursor clamped at 0, got %d", m.cursor)
	}

	// End
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if m.cursor != len(cues)-1 {
		t.Errorf("expected cursor at %d after End, got %d", len(cues)-1, m.cursor)
	}

	// Home
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.cursor != 0 {
		t.Errorf("expected cursor at 0 after Home, got %d", m.cursor)
	}
}

func TestCutsModelToggle(t *testing.T) {
	cues := makeTestCues()
	media := cutter.MediaInfo{Duration: 30 * time.Second}
	m := NewCutsModel(cues, media, "test.mp4", "")

	// Initial cue #0 is ActionCut
	if m.cues[0].Action != model.ActionCut {
		t.Fatalf("expected cue 0 to be ActionCut")
	}

	// Toggle to keep
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.cues[0].Action != model.ActionKeep {
		t.Errorf("expected cue 0 to become ActionKeep after toggle, got %s", m.cues[0].Action)
	}

	// Toggle back to cut
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.cues[0].Action != model.ActionCut {
		t.Errorf("expected cue 0 to become ActionCut after second toggle, got %s", m.cues[0].Action)
	}
}

func TestCutsModelJumpCut(t *testing.T) {
	cues := makeTestCues() // cut at 0, keep at 1, keep at 2, cut at 3
	media := cutter.MediaInfo{Duration: 30 * time.Second}
	m := NewCutsModel(cues, media, "test.mp4", "")

	// Cursor at 0 (cut) -> jump to next cut
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.cursor != 3 {
		t.Errorf("expected jump to cue 3, got %d", m.cursor)
	}

	// Cursor at 3 -> jump to next cut (wraps around to 0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.cursor != 0 {
		t.Errorf("expected wrap-around jump to cue 0, got %d", m.cursor)
	}

	// Jump prev cut (wraps around to 3)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	if m.cursor != 3 {
		t.Errorf("expected prev jump to cue 3, got %d", m.cursor)
	}
}

func TestCutsModelView(t *testing.T) {
	cues := makeTestCues()
	media := cutter.MediaInfo{Duration: 30 * time.Second}
	m := NewCutsModel(cues, media, "talk_video.mp4", "")
	m.SetDimensions(100, 30)

	view := m.View()
	if view == "" {
		t.Fatalf("expected non-empty View output")
	}

	if len(m.Cues()) != len(cues) {
		t.Errorf("expected %d cues, got %d", len(cues), len(m.Cues()))
	}
}

func TestCutsModelSaveAndHelp(t *testing.T) {
	cues := makeTestCues()
	media := cutter.MediaInfo{Duration: 30 * time.Second}
	tmpDir := t.TempDir()
	m := NewCutsModel(cues, media, "talk_video.mp4", tmpDir)
	m.SetDimensions(100, 30)

	// Test F1 toggles help
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	if !m.helpOpen {
		t.Errorf("expected helpOpen to be true after F1")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.helpOpen {
		t.Errorf("expected helpOpen to be false after Esc")
	}

	// Test s saves cuts to disk
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if !model.HasSavedCuts(tmpDir) {
		t.Errorf("expected talk_cuts.json to exist after 's'")
	}
}

func TestCutsModelExactHeight(t *testing.T) {
	cues := makeTestCues()
	// Add a very long cue to test text wrapping and dynamic trimming
	cues = append(cues, model.SubtitleCue{
		ID:      5,
		Start:   25 * time.Second,
		End:     50 * time.Second,
		Speaker: "Alice",
		Text: "This is an extremely long subtitle cue text designed to test multi-line text wrapping " +
			"in the bottom cue card. It spans across multiple sentences and paragraphs to verify that " +
			"when the card expands to accommodate long text, the transcript above trims its visible rows " +
			"and the entire layout string never exceeds the target terminal height.",
		CutReason: "Preamble and speaker pleasantries with long description",
		Action:    model.ActionCut,
	})

	media := cutter.MediaInfo{Duration: 60 * time.Second}
	for _, height := range []int{24, 30, 35} {
		m := NewCutsModel(cues, media, "talk_video.mp4", "")
		m.SetDimensions(100, height)

		// Check initial view height
		v := m.View()
		lineCount := len(strings.Split(v, "\n"))
		if lineCount != height {
			t.Errorf("height %d: expected %d lines, got %d", height, height, lineCount)
		}

		// Navigate to the long cue and verify height remains identical
		m.setCursor(len(cues) - 1)
		v2 := m.View()
		lineCount2 := len(strings.Split(v2, "\n"))
		if lineCount2 != height {
			t.Errorf("height %d with long cue: expected %d lines, got %d", height, height, lineCount2)
		}
	}
}

func TestRenderCueRow(t *testing.T) {
	cues := makeTestCues()
	media := cutter.MediaInfo{Duration: 30 * time.Second}
	m := NewCutsModel(cues, media, "talk_video.mp4", "")
	m.SetDimensions(100, 30)
	lipgloss.SetColorProfile(termenv.TrueColor)
	cues[0].Action = model.ActionCut
	row := m.renderCueRow(0, 100)
	if !strings.Contains(row, "✂") {
		t.Errorf("expected row to contain ✂, got: %s", row)
	}
	cleanRow0 := strings.ReplaceAll(row, "\x1b[", "")
	if strings.Contains(cleanRow0, "[1;38;") {
		t.Errorf("row contains leaked ANSI escape text: %s", cleanRow0)
	}

	cues[1].Action = model.ActionKeep
	row1 := m.renderCueRow(1, 100)
	if !strings.Contains(row1, "✔") {
		t.Errorf("expected row1 to contain ✔, got: %s", row1)
	}
	cleanRow1 := strings.ReplaceAll(row1, "\x1b[", "")
	if strings.Contains(cleanRow1, "[1;38;") {
		t.Errorf("row1 contains leaked ANSI escape text: %s", cleanRow1)
	}
}
