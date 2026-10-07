package cutter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExtractAudio(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "tone.m4a")
	out := filepath.Join(dir, "tone.wav")

	gen := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "aac", src)
	if b, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generating source audio: %v: %s", err, b)
	}

	if err := ExtractAudio(context.Background(), src, out); err != nil {
		t.Fatalf("ExtractAudio: %v", err)
	}

	fi, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatalf("expected non-empty wav output")
	}
}

func TestExtractAudioRejectsEmptyPaths(t *testing.T) {
	if err := ExtractAudio(context.Background(), "", "out.wav"); err == nil {
		t.Fatalf("expected error for empty input path")
	}
	if err := ExtractAudio(context.Background(), "in.m4a", ""); err == nil {
		t.Fatalf("expected error for empty output path")
	}
}
