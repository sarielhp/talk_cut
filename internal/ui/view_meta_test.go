// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/model"
)

func TestMetaModelFocusCycling(t *testing.T) {
	meta := model.TalkMetadata{
		Title:    "Algorithms in Geometry",
		Speaker:  "Sariel Har-Peled",
		Abstract: "Approximation schemes for geometric problems.",
		Privacy:  "unlisted",
		Tags:     []string{"geometry", "algorithms"},
	}
	cuts := []model.CutInterval{
		{Start: 0, End: 5 * time.Second, Action: model.ActionCut},
	}

	m := NewMetaModel(meta, cuts, "output.mp4")
	m.SetDimensions(100, 30)

	if m.focusIndex != 0 {
		t.Fatalf("expected initial focus index 0, got %d", m.focusIndex)
	}

	// Tab down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focusIndex != 1 {
		t.Errorf("expected focus index 1 after tab, got %d", m.focusIndex)
	}

	// Shift+Tab up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focusIndex != 0 {
		t.Errorf("expected focus index 0 after shift+tab, got %d", m.focusIndex)
	}

	// Shift+Tab wrap around to last field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focusIndex != numFields-1 {
		t.Errorf("expected wrap around to %d, got %d", numFields-1, m.focusIndex)
	}
}

func TestMetaModelPrivacyToggle(t *testing.T) {
	meta := model.TalkMetadata{
		Title:   "Sample Talk",
		Privacy: "unlisted",
	}
	m := NewMetaModel(meta, nil, "out.mp4")
	m.focusIndex = fieldPrivacy

	// Initial privacy is unlisted
	if m.privacyOpts[m.privacyIdx] != "unlisted" {
		t.Fatalf("expected initial privacy unlisted, got %s", m.privacyOpts[m.privacyIdx])
	}

	// Toggle with Space
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.privacyOpts[m.privacyIdx] != "public" {
		t.Errorf("expected privacy public after toggle, got %s", m.privacyOpts[m.privacyIdx])
	}

	// Toggle again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.privacyOpts[m.privacyIdx] != "private" {
		t.Errorf("expected privacy private after toggle, got %s", m.privacyOpts[m.privacyIdx])
	}
}

func TestMetaModelValues(t *testing.T) {
	meta := model.TalkMetadata{
		Title:    "Original Title",
		Speaker:  "Original Speaker",
		Abstract: "Original Abstract",
		Privacy:  "unlisted",
		Tags:     []string{"tag1", "tag2"},
	}
	m := NewMetaModel(meta, nil, "out.mp4")
	m.inputs[0].SetValue("Updated Title")
	m.inputs[1].SetValue("Updated Speaker")

	updated := m.Metadata()
	if updated.Title != "Updated Title" {
		t.Errorf("expected Title %q, got %q", "Updated Title", updated.Title)
	}
	if updated.Speaker != "Updated Speaker" {
		t.Errorf("expected Speaker %q, got %q", "Updated Speaker", updated.Speaker)
	}
	if m.OutputPath() != "out.mp4" {
		t.Errorf("expected OutputPath %q, got %q", "out.mp4", m.OutputPath())
	}

	view := m.View()
	if view == "" {
		t.Errorf("expected non-empty metadata view")
	}
}
