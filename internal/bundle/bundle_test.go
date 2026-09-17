package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverBundleRealExample(t *testing.T) {
	exampleDir := "../../examples/26_09_08"
	if _, err := os.Stat(exampleDir); err != nil {
		t.Skip("sample directory not present, skipping bundle discovery test")
		return
	}

	b, err := DiscoverBundle(exampleDir, "slides")
	if err != nil {
		t.Fatalf("DiscoverBundle failed: %v", err)
	}

	if !strings.HasSuffix(b.TranscriptPath, "GMT20260908-180238_Recording.transcript.vtt") {
		t.Errorf("unexpected transcript: %q", b.TranscriptPath)
	}

	if len(b.AllVideos) != 4 {
		t.Errorf("expected 4 videos, got %d", len(b.AllVideos))
	}

	if !strings.HasSuffix(b.PrimaryVideo, "GMT20260908-180238_Recording_1920x1200.mp4") {
		t.Errorf("unexpected default primary video: %q", b.PrimaryVideo)
	}

	// Test "clean" layout selects _as_
	cleanBundle, err := DiscoverBundle(exampleDir, "clean")
	if err != nil {
		t.Fatalf("DiscoverBundle clean layout failed: %v", err)
	}
	if !strings.Contains(cleanBundle.PrimaryVideo, "_as_") {
		t.Errorf("expected _as_ video for clean layout, got %q", cleanBundle.PrimaryVideo)
	}
}

func TestDiscoverBundleSynthetic(t *testing.T) {
	tempDir := t.TempDir()

	// Write mock transcript and video
	if err := os.WriteFile(filepath.Join(tempDir, "talk.transcript.vtt"), []byte("WEBVTT\n"), 0o644); err != nil {
		t.Fatalf("failed writing vtt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "talk_1920x1080.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatalf("failed writing mp4: %v", err)
	}

	b, err := DiscoverBundle(tempDir, "slides")
	if err != nil {
		t.Fatalf("DiscoverBundle failed: %v", err)
	}

	if filepath.Base(b.TranscriptPath) != "talk.transcript.vtt" {
		t.Errorf("unexpected transcript: %q", b.TranscriptPath)
	}
	if filepath.Base(b.PrimaryVideo) != "talk_1920x1080.mp4" {
		t.Errorf("unexpected primary video: %q", b.PrimaryVideo)
	}
}
