package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/model"
)

func TestChaptersModelNavigation(t *testing.T) {
	chapters := []model.ChapterMarker{
		{OriginalTime: 0, AdjustedTime: 0, Title: "Intro"},
		{OriginalTime: 60 * time.Second, AdjustedTime: 60 * time.Second, Title: "Problem"},
	}

	m := NewChaptersModel(chapters, nil, nil, "video.mp4")
	if m.cursor != 0 {
		t.Fatalf("expected initial cursor 0, got %d", m.cursor)
	}

	// Move down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("expected cursor 1 after down, got %d", m.cursor)
	}

	// Move up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("expected cursor 0 after up, got %d", m.cursor)
	}
}

func TestChaptersModelDelete(t *testing.T) {
	chapters := []model.ChapterMarker{
		{OriginalTime: 0, AdjustedTime: 0, Title: "Intro"},
		{OriginalTime: 60 * time.Second, AdjustedTime: 60 * time.Second, Title: "Problem"},
	}

	m := NewChaptersModel(chapters, nil, nil, "video.mp4")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if len(m.Chapters()) != 1 {
		t.Fatalf("expected 1 chapter after delete, got %d", len(m.Chapters()))
	}
	if m.Chapters()[0].Title != "Problem" {
		t.Errorf("expected 'Problem' remaining, got %q", m.Chapters()[0].Title)
	}
}

func TestChaptersModelAdd(t *testing.T) {
	chapters := []model.ChapterMarker{
		{OriginalTime: 0, AdjustedTime: 0, Title: "Intro"},
	}

	m := NewChaptersModel(chapters, nil, nil, "video.mp4")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	if len(m.Chapters()) != 2 {
		t.Fatalf("expected 2 chapters after add, got %d", len(m.Chapters()))
	}
}
