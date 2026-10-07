package cutter

import (
	"context"
	"fmt"
	"os/exec"
)

// ExtractAudio decodes inputPath to the 16 kHz mono PCM WAV that whisper
// expects, writing it to outputPath and overwriting any existing file.
func ExtractAudio(ctx context.Context, inputPath, outputPath string) error {
	if inputPath == "" || outputPath == "" {
		return fmt.Errorf("input and output paths must be specified")
	}

	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", inputPath,
		"-vn", "-ac", "1", "-ar", "16000",
		"-c:a", "pcm_s16le",
		outputPath,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extracting audio via ffmpeg: %w: %s", err, string(out))
	}
	return nil
}
