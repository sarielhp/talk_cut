package cutter

import (
	"testing"
	"time"

	"talk_cut/internal/model"
)

func TestAdjustTime(t *testing.T) {
	cuts := []model.CutInterval{
		// Cut from 0s to 10s (e.g. intro)
		{Start: 0, End: 10 * time.Second, Action: model.ActionCut},
		// Cut from 30s to 40s (e.g. mid-talk pause)
		{Start: 30 * time.Second, End: 40 * time.Second, Action: model.ActionCut},
	}

	tests := []struct {
		name     string
		input    time.Duration
		expected time.Duration
	}{
		{"Timestamp during first cut", 5 * time.Second, 0},
		{"Timestamp right at end of first cut", 10 * time.Second, 0},
		{"Timestamp in first kept block", 20 * time.Second, 10 * time.Second},
		{"Timestamp right at start of second cut", 30 * time.Second, 20 * time.Second},
		{"Timestamp during second cut", 35 * time.Second, 20 * time.Second},
		{"Timestamp after second cut", 50 * time.Second, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AdjustTime(tt.input, cuts)
			if got != tt.expected {
				t.Errorf("AdjustTime(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestAdjustChapters(t *testing.T) {
	original := []model.ChapterMarker{
		{OriginalTime: 0, Title: "Pre-talk banter"},
		{OriginalTime: 10 * time.Second, Title: "Introduction"},
		{OriginalTime: 25 * time.Second, Title: "Core Theorems"},
		{OriginalTime: 50 * time.Second, Title: "Conclusion"},
	}

	cuts := []model.CutInterval{
		// Cut 0 to 10s
		{Start: 0, End: 10 * time.Second, Action: model.ActionCut},
	}

	adjusted := AdjustChapters(original, cuts, "Talk Start")
	if len(adjusted) == 0 {
		t.Fatalf("expected non-empty adjusted chapters")
	}

	// First chapter must be at 00:00
	if adjusted[0].AdjustedTime != 0 {
		t.Errorf("first chapter must be at 0, got %v", adjusted[0].AdjustedTime)
	}

	// Core Theorems was at 25s, minus 10s cut -> 15s
	if adjusted[1].AdjustedTime != 15*time.Second {
		t.Errorf("Core Theorems adjusted time = %v, want 15s", adjusted[1].AdjustedTime)
	}

	// Conclusion was at 50s, minus 10s cut -> 40s
	if adjusted[2].AdjustedTime != 40*time.Second {
		t.Errorf("Conclusion adjusted time = %v, want 40s", adjusted[2].AdjustedTime)
	}
}
