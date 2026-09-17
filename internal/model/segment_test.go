package model

import (
	"testing"
	"time"
)

func TestBuildCutIntervals(t *testing.T) {
	cues := []SubtitleCue{
		{ID: 1, Start: 0, End: 5 * time.Second, Action: ActionCut, CutReason: "Intro banter"},
		{ID: 2, Start: 5 * time.Second, End: 10 * time.Second, Action: ActionCut, CutReason: "Intro banter"},
		{ID: 3, Start: 10 * time.Second, End: 20 * time.Second, Action: ActionKeep},
		{ID: 4, Start: 20 * time.Second, End: 30 * time.Second, Action: ActionKeep},
		{ID: 5, Start: 30 * time.Second, End: 40 * time.Second, Action: ActionCut, CutReason: "Q&A"},
	}

	intervals := BuildCutIntervals(cues)
	if len(intervals) != 3 {
		t.Fatalf("expected 3 intervals, got %d", len(intervals))
	}

	if intervals[0].Action != ActionCut || intervals[0].End != 10*time.Second {
		t.Errorf("interval 0 mismatch: %+v", intervals[0])
	}
	if intervals[1].Action != ActionKeep || intervals[1].End != 30*time.Second {
		t.Errorf("interval 1 mismatch: %+v", intervals[1])
	}
	if intervals[2].Action != ActionCut || intervals[2].End != 40*time.Second {
		t.Errorf("interval 2 mismatch: %+v", intervals[2])
	}

	stats := ComputeStats(cues, 40*time.Second)
	if stats.CutCount != 2 {
		t.Errorf("expected 2 cut intervals, got %d", stats.CutCount)
	}
	if stats.TotalCut != 20*time.Second {
		t.Errorf("expected 20s cut, got %v", stats.TotalCut)
	}
	if stats.TotalKept != 20*time.Second {
		t.Errorf("expected 20s kept, got %v", stats.TotalKept)
	}
}

func TestChapterFormat(t *testing.T) {
	tests := []struct {
		name     string
		marker   ChapterMarker
		expected string
	}{
		{
			name:     "Under an hour",
			marker:   ChapterMarker{AdjustedTime: 3*time.Minute + 15*time.Second, Title: "Introduction"},
			expected: "03:15 Introduction",
		},
		{
			name:     "Over an hour",
			marker:   ChapterMarker{AdjustedTime: 1*time.Hour + 4*time.Minute + 5*time.Second, Title: "Q&A"},
			expected: "01:04:05 Q&A",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.marker.FormatYouTubeLine()
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestApplyCutsToCues(t *testing.T) {
	cues := []SubtitleCue{
		{ID: 1, Start: 0, End: 5 * time.Second, Action: ActionKeep},
		{ID: 2, Start: 5 * time.Second, End: 10 * time.Second, Action: ActionKeep},
		{ID: 3, Start: 10 * time.Second, End: 20 * time.Second, Action: ActionKeep},
	}
	cuts := []CutInterval{
		{Start: 0, End: 6 * time.Second, Action: ActionCut, Reason: "Intro"},
	}

	ApplyCutsToCues(cues, cuts)
	if cues[0].Action != ActionCut || cues[0].CutReason != "Intro" {
		t.Errorf("expected cue 0 cut, got %s", cues[0].Action)
	}
	if cues[1].Action != ActionKeep {
		t.Errorf("expected cue 1 kept, got %s", cues[1].Action)
	}
}
