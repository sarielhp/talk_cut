package cutter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"talk_cut/internal/model"
)

func TestGetKeptIntervals(t *testing.T) {
	total := 100 * time.Second

	tests := []struct {
		name     string
		cuts     []model.CutInterval
		expected []model.CutInterval
	}{
		{
			name: "No cuts",
			cuts: nil,
			expected: []model.CutInterval{
				{Start: 0, End: 100 * time.Second, Action: model.ActionKeep},
			},
		},
		{
			name: "Intro cut",
			cuts: []model.CutInterval{
				{Start: 0, End: 15 * time.Second, Action: model.ActionCut},
			},
			expected: []model.CutInterval{
				{Start: 15 * time.Second, End: 100 * time.Second, Action: model.ActionKeep},
			},
		},
		{
			name: "Middle cut",
			cuts: []model.CutInterval{
				{Start: 30 * time.Second, End: 40 * time.Second, Action: model.ActionCut},
			},
			expected: []model.CutInterval{
				{Start: 0, End: 30 * time.Second, Action: model.ActionKeep},
				{Start: 40 * time.Second, End: 100 * time.Second, Action: model.ActionKeep},
			},
		},
		{
			name: "Intro and outro cuts",
			cuts: []model.CutInterval{
				{Start: 0, End: 10 * time.Second, Action: model.ActionCut},
				{Start: 90 * time.Second, End: 100 * time.Second, Action: model.ActionCut},
			},
			expected: []model.CutInterval{
				{Start: 10 * time.Second, End: 90 * time.Second, Action: model.ActionKeep},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetKeptIntervals(total, tt.cuts)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d intervals, got %d", len(tt.expected), len(got))
			}
			for i := range got {
				if got[i].Start != tt.expected[i].Start || got[i].End != tt.expected[i].End {
					t.Errorf("interval %d mismatch: got %v-%v, want %v-%v",
						i, got[i].Start, got[i].End, tt.expected[i].Start, tt.expected[i].End)
				}
			}
		})
	}
}

func TestCutVideoSynthetic(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed in environment, skipping synthetic video test")
		return
	}

	tempDir := t.TempDir()
	sourceVideo := filepath.Join(tempDir, "source.mp4")
	outputVideo := filepath.Join(tempDir, "output.mp4")

	// Generate a 6-second synthetic test video with audio and 1-second keyframe GOPs
	createCmd := exec.Command(
		"ffmpeg",
		"-y",
		"-f", "lavfi", "-i", "testsrc=duration=6:size=320x240:rate=25",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono",
		"-t", "6",
		"-c:v", "libx264",
		"-g", "25",
		"-c:a", "aac",
		sourceVideo,
	)
	if err := createCmd.Run(); err != nil {
		t.Fatalf("failed to create synthetic source video: %v", err)
	}

	// Cut out 2s-3s (mid-video cut) -> kept: 0s-2s and 3s-6s (total ~5 seconds)
	cuts := []model.CutInterval{
		{Start: 2 * time.Second, End: 3 * time.Second, Action: model.ActionCut},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	progressCalls := 0
	opts := CutOptions{
		InputPath:  sourceVideo,
		OutputPath: outputVideo,
		Cuts:       cuts,
		Mode:       ModeLossless,
		OnProgress: func(_ CutProgress) {
			progressCalls++
		},
	}

	if err := CutVideo(ctx, opts); err != nil {
		t.Fatalf("CutVideo failed: %v", err)
	}

	if _, err := os.Stat(outputVideo); err != nil {
		t.Fatalf("output video was not created: %v", err)
	}

	// Verify output video with ffprobe
	info, err := ProbeMedia(ctx, outputVideo)
	if err != nil {
		t.Fatalf("ProbeMedia on output failed: %v", err)
	}

	if info.Duration < 4*time.Second || info.Duration > 6*time.Second {
		t.Errorf("unexpected output duration: %v (expected ~5s)", info.Duration)
	}
}
