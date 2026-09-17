package cutter

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestProbeMediaRealExample(t *testing.T) {
	examplePath := "../../examples/26_09_08/GMT20260908-180238_Recording_1920x1200.mp4"
	if _, err := os.Stat(examplePath); err != nil {
		t.Skip("sample example video not found on disk, skipping probe test")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := ProbeMedia(ctx, examplePath)
	if err != nil {
		t.Fatalf("ProbeMedia failed: %v", err)
	}

	if info.Width != 1920 || info.Height != 1200 {
		t.Errorf("unexpected resolution: %dx%d (expected 1920x1200)", info.Width, info.Height)
	}

	if info.VideoCodec != "h264" {
		t.Errorf("unexpected video codec: %q (expected h264)", info.VideoCodec)
	}

	if info.Duration < 1*time.Hour {
		t.Errorf("unexpected duration: %v (expected > 1h)", info.Duration)
	}
}
