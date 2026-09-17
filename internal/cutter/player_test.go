package cutter

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		player   string
		start    time.Duration
		path     string
		expected string
	}{
		{
			player:   "mpv",
			start:    15500 * time.Millisecond,
			path:     "/video/test.mp4",
			expected: "--start=15.50",
		},
		{
			player:   "vlc",
			start:    20 * time.Second,
			path:     "/video/test.mp4",
			expected: "--start-time=20.00",
		},
		{
			player:   "ffplay",
			start:    10 * time.Second,
			path:     "/video/test.mp4",
			expected: "10.00",
		},
		{
			player:   "totem",
			start:    5 * time.Second,
			path:     "/video/test.mp4",
			expected: "--seek=5000",
		},
	}

	for _, tt := range tests {
		var found *playerCandidate
		for _, p := range supportedPlayers {
			if p.name == tt.player {
				cand := p
				found = &cand
				break
			}
		}
		if found == nil {
			t.Fatalf("player %q not found in supportedPlayers", tt.player)
		}
		args := found.buildArgs(tt.start, tt.path)
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, tt.expected) {
			t.Errorf("%s: expected args to contain %q, got: %s", tt.player, tt.expected, joined)
		}
	}
}

func TestDetectPlayer(t *testing.T) {
	// Should at least detect ffplay which is installed on host
	player, err := DetectPlayer()
	if err != nil {
		t.Skipf("no media player installed on host test environment: %v", err)
	}
	if player.Name == "" || player.ExePath == "" {
		t.Errorf("expected detected player to have name and path, got %+v", player)
	}
}

func TestDetectPlayerCustom(t *testing.T) {
	orig := os.Getenv("TALK_CUT_PLAYER")
	defer func() {
		if orig != "" {
			_ = os.Setenv("TALK_CUT_PLAYER", orig)
		} else {
			_ = os.Unsetenv("TALK_CUT_PLAYER")
		}
	}()

	_ = os.Setenv("TALK_CUT_PLAYER", "ffplay")
	player, err := DetectPlayer()
	if err != nil {
		t.Skipf("ffplay not found: %v", err)
	}
	if player.Name != "ffplay" {
		t.Errorf("expected player name ffplay, got %q", player.Name)
	}
}

func TestKillPreviewNil(_ *testing.T) {
	// Should safely not panic on nil
	KillPreview(nil)
}
