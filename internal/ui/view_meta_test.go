// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/model"
)

var stripANSIRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][0-9];[^\x1b]*\x1b\\|\x1b\]8;;[^\x1b]*\x1b\\`)

func stripANSI(s string) string {
	return stripANSIRegex.ReplaceAllString(s, "")
}

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

func TestMetaModelScrollingWithArrowKeys(t *testing.T) {
	longAbstract := "A circle graph is the intersection graph of a set of chords in a circle. " +
		"A dominating set of a graph G=(V,E) is a subset D such that every vertex is adjacent to at least one vertex of D. " +
		"Computing a minimum dominating set is known to be NP-hard on circle graphs. In this work, we study the problem. " +
		"We present a polynomial-time 2-approximation algorithm and develop a PTAS based on local search. " +
		"Talk announcement: https://math.nyu.edu/dynamic/calendars/seminars/geometry-seminar/4488/"

	meta := model.TalkMetadata{
		Title:       "Dominating Sets in Circle Graphs",
		Speaker:     "Karim Abu-Affash",
		Affiliation: "Shamoon College of Engineering",
		URL:         "https://math.nyu.edu/dynamic/calendars/seminars/geometry-seminar/4488/",
		Abstract:    longAbstract,
		Privacy:     "unlisted",
		Tags:        []string{"Geometry", "Algorithms"},
		Chapters: []model.ChapterMarker{
			{OriginalTime: 0, AdjustedTime: 0, Title: "Introduction"},
			{OriginalTime: 60 * time.Second, AdjustedTime: 50 * time.Second, Title: "Main Proof"},
		},
	}

	m := NewMetaModel(meta, nil, "out.mp4")
	m.SetDimensions(100, 20)

	// Initial view at scrollOffset 0 shows Title
	v0 := m.View()
	if !strings.Contains(v0, "Dominating Sets in Circle Graphs") {
		t.Errorf("expected Title in initial view")
	}
	if m.scrollOffset != 0 {
		t.Errorf("expected initial scrollOffset 0, got %d", m.scrollOffset)
	}

	// Press down arrow 15 times to scroll into the abstract
	abstractSeen := false
	for i := 0; i < 15; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		v := m.View()
		if strings.Contains(v, "intersection graph of a set of chords") {
			abstractSeen = true
		}
	}

	if m.scrollOffset <= 0 {
		t.Errorf("expected scrollOffset > 0 after pressing down arrow, got %d", m.scrollOffset)
	}
	if !abstractSeen {
		t.Errorf("expected abstract text to be visible while scrolling down")
	}

	// Continue scrolling down to reach Chapters and Commit button
	commitSeen := false
	for i := 0; i < 25; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		v := m.View()
		if strings.Contains(stripANSI(v), "Commit & Cut Video") {
			commitSeen = true
		}
	}
	if !commitSeen {
		t.Errorf("expected commit button to be visible after scrolling to bottom")
	}

	// Now scroll back up with Up arrow key
	titleSeenAgain := false
	for i := 0; i < 40; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
		v := m.View()
		if strings.Contains(v, "Dominating Sets in Circle Graphs") {
			titleSeenAgain = true
		}
	}
	if m.scrollOffset != 0 {
		t.Errorf("expected scrollOffset 0 after scrolling back up, got %d", m.scrollOffset)
	}
	if !titleSeenAgain {
		t.Errorf("expected Title to be visible again after scrolling back up")
	}
}

func TestMetaModelURLEmbedding(t *testing.T) {
	url := "https://math.nyu.edu/seminar/4488/"
	embedded := embedURLs("Visit "+url+" for details.", DefaultTheme())

	// OSC 8 hyperlink sequence is \x1b]8;;url\x1b\\
	if !strings.Contains(embedded, "\x1b]8;;"+url+"\x1b\\") {
		t.Errorf("expected OSC 8 hyperlink sequence in embedded text, got %q", embedded)
	}

	meta := model.TalkMetadata{
		Title:    "Geometry Seminar",
		URL:      url,
		Abstract: "More info at " + url + ".",
	}
	m := NewMetaModel(meta, nil, "out.mp4")
	m.SetDimensions(120, 30)

	view := m.View()
	// Check that view contains OSC 8 hyperlink escape codes
	if !strings.Contains(view, "\x1b]8;;") {
		t.Errorf("expected view to contain OSC 8 hyperlink sequences, got view:\n%s", view)
	}
}

func TestMetaModelWholeScreenWidth(t *testing.T) {
	meta := model.TalkMetadata{
		Title:    "Wide Screen Seminar",
		Abstract: "Short abstract.",
	}
	m := NewMetaModel(meta, nil, "out.mp4")
	m.SetDimensions(120, 25)

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 25 {
		t.Errorf("expected exactly 25 lines for height 25, got %d", len(lines))
	}
}

func TestMetaModelPageAndHomeEndKeys(t *testing.T) {
	longAbstract := strings.Repeat("A long abstract line for testing page scrolling behavior. ", 15)
	meta := model.TalkMetadata{
		Title:    "Long Abstract Talk",
		Abstract: longAbstract,
	}
	m := NewMetaModel(meta, nil, "out.mp4")
	m.SetDimensions(80, 20)

	// Test PageDown
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.scrollOffset <= 0 {
		t.Errorf("expected scrollOffset > 0 after PgDown, got %d", m.scrollOffset)
	}

	// Test End key
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	endOffset := m.scrollOffset

	// Test PageUp
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if m.scrollOffset >= endOffset {
		t.Errorf("expected scrollOffset < endOffset after PgUp, got %d", m.scrollOffset)
	}

	// Test Home key
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	if m.scrollOffset != 0 {
		t.Errorf("expected scrollOffset 0 after Home, got %d", m.scrollOffset)
	}
}

func TestMetaModelApplyMetadata(t *testing.T) {
	initial := model.TalkMetadata{
		Title:     "Old Title",
		Speaker:   "Old Speaker",
		YouTubeID: "existing_vid_123",
	}
	m := NewMetaModel(initial, nil, "out.mp4")

	incoming := model.TalkMetadata{
		Title:       "New Title from Email",
		Speaker:     "Dr. Alice",
		Affiliation: "UIUC",
		Abstract:    "A detailed talk about algorithms.",
		Tags:        []string{"Algorithms", "Geometry"},
		Privacy:     "public",
	}

	m.ApplyMetadata(incoming)
	res := m.Metadata()

	if res.Title != "New Title from Email" {
		t.Errorf("expected updated title, got %q", res.Title)
	}
	if res.Speaker != "Dr. Alice" {
		t.Errorf("expected updated speaker, got %q", res.Speaker)
	}
	if res.Affiliation != "UIUC" {
		t.Errorf("expected updated affiliation, got %q", res.Affiliation)
	}
	if res.Abstract != "A detailed talk about algorithms." {
		t.Errorf("expected updated abstract, got %q", res.Abstract)
	}
	if res.Privacy != "public" {
		t.Errorf("expected updated privacy, got %q", res.Privacy)
	}
	if res.YouTubeID != "existing_vid_123" {
		t.Errorf("expected preserved YouTubeID, got %q", res.YouTubeID)
	}
}
