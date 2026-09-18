package model

import (
	"testing"
)

func TestExtractYouTubeID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"GqfuUGTR12Y", "GqfuUGTR12Y"},
		{"https://youtu.be/GqfuUGTR12Y", "GqfuUGTR12Y"},
		{"https://youtu.be/GqfuUGTR12Y?si=12345", "GqfuUGTR12Y"},
		{"https://www.youtube.com/watch?v=GqfuUGTR12Y", "GqfuUGTR12Y"},
		{"https://www.youtube.com/watch?v=GqfuUGTR12Y&feature=youtu.be", "GqfuUGTR12Y"},
		{"", ""},
		{"https://example.com/other", ""},
	}

	for _, tc := range tests {
		got := ExtractYouTubeID(tc.input)
		if got != tc.expected {
			t.Errorf("ExtractYouTubeID(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestTalkMetadata_YouTubeWatchURL(t *testing.T) {
	m1 := TalkMetadata{YouTubeURL: "https://youtu.be/abc"}
	if m1.YouTubeWatchURL() != "https://youtu.be/abc" {
		t.Errorf("expected YouTubeURL, got %q", m1.YouTubeWatchURL())
	}

	m2 := TalkMetadata{YouTubeID: "GqfuUGTR12Y"}
	if m2.YouTubeWatchURL() != "https://youtu.be/GqfuUGTR12Y" {
		t.Errorf("expected generated short URL, got %q", m2.YouTubeWatchURL())
	}

	m3 := TalkMetadata{YouTubeURL: "https://www.youtube.com/watch?v=GqfuUGTR12Y"}
	if m3.EffectiveYouTubeID() != "GqfuUGTR12Y" {
		t.Errorf("expected effective ID GqfuUGTR12Y, got %q", m3.EffectiveYouTubeID())
	}
}

func TestTalkMetadata_Playlists(t *testing.T) {
	var m TalkMetadata
	if m.HasPlaylist("pl1") {
		t.Error("expected false for empty playlists")
	}

	m.AddPlaylist("pl1", "Seminar One")
	if !m.HasPlaylist("pl1") || !m.HasPlaylist("Seminar One") {
		t.Error("expected pl1 / Seminar One to be present")
	}
	if m.PlaylistID != "pl1" || m.PlaylistTitle != "Seminar One" {
		t.Errorf("legacy fields not synchronized: %s, %s", m.PlaylistID, m.PlaylistTitle)
	}

	m.AddPlaylist("pl2", "Seminar Two")
	if len(m.AllPlaylists()) != 2 {
		t.Fatalf("expected 2 playlists, got %d", len(m.AllPlaylists()))
	}

	// Idempotent addition
	m.AddPlaylist("pl1", "Seminar One")
	if len(m.AllPlaylists()) != 2 {
		t.Fatalf("expected 2 playlists after duplicate addition, got %d", len(m.AllPlaylists()))
	}

	// Remove pl1
	m.RemovePlaylist("pl1")
	if m.HasPlaylist("pl1") {
		t.Error("expected pl1 removed")
	}
	if len(m.AllPlaylists()) != 1 {
		t.Fatalf("expected 1 playlist remaining, got %d", len(m.AllPlaylists()))
	}
	if m.PlaylistID != "pl2" || m.PlaylistTitle != "Seminar Two" {
		t.Errorf("legacy fields not updated after removal: %s, %s", m.PlaylistID, m.PlaylistTitle)
	}

	// Remove remaining
	m.RemovePlaylist("pl2")
	if len(m.AllPlaylists()) != 0 {
		t.Errorf("expected 0 playlists, got %d", len(m.AllPlaylists()))
	}
	if m.PlaylistID != "" || m.PlaylistTitle != "" {
		t.Errorf("legacy fields not cleared: %s, %s", m.PlaylistID, m.PlaylistTitle)
	}
}
