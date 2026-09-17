package ai

import (
	"context"
	"os"
	"testing"
	"time"

	"talk_cut/internal/config"
	"talk_cut/internal/vtt"
)

func TestDetectCutsLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live API integration test in short mode")
	}

	cfg, err := config.LoadConfig()
	if err != nil || cfg.OpenRouterKey == "" {
		t.Skip("no OpenRouter API key found in environment or opencode-switcher, skipping live test")
		return
	}

	vttPath := "../../examples/26_09_08/GMT20260908-180238_Recording.transcript.vtt"
	file, err := os.Open(vttPath)
	if err != nil {
		t.Skip("example vtt not present, skipping live test")
		return
	}
	defer file.Close()

	cues, err := vtt.Parse(file)
	if err != nil {
		t.Fatalf("parsing cues failed: %v", err)
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cuts, err := DetectCuts(ctx, client, cues)
	if err != nil {
		t.Fatalf("DetectCuts live call failed: %v", err)
	}

	t.Logf("Live AI detected %d cuts on real Zoom recording:", len(cuts))
	for i, c := range cuts {
		t.Logf("  Cut #%d: [%s -> %s] (Duration: %v) Reason: %s",
			i+1, vtt.FormatTimestamp(c.Start), vtt.FormatTimestamp(c.End), c.Duration(), c.Reason)
	}

	if len(cuts) == 0 {
		t.Errorf("expected at least 1 cut detected on real recording")
	}
}
