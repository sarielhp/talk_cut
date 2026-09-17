// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
