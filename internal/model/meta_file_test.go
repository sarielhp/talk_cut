package model

import (
	"testing"
	"time"
)

func TestSaveAndLoadMetaFile(t *testing.T) {
	tmpDir := t.TempDir()

	if HasSavedMetadata(tmpDir) {
		t.Fatalf("expected HasSavedMetadata to be false for empty temp dir")
	}

	meta := TalkMetadata{
		Title:         "The Minimum Dominating Set Problem",
		Speaker:       "Karim Abu-Affash",
		Affiliation:   "Shamoon College of Engineering",
		Abstract:      "A circle graph is the intersection graph of chords.\n\nTalk announcement: https://example.com/talk",
		URL:           "https://example.com/talk",
		Tags:          []string{"Geometry Seminar", "Algorithms"},
		Privacy:       "public",
		YouTubeID:     "dQw4w9WgXcQ",
		YouTubeURL:    "https://youtu.be/dQw4w9WgXcQ",
		PlaylistID:    "PL_test_123",
		PlaylistTitle: "Geometry Talks 2026",
		Chapters: []ChapterMarker{
			{OriginalTime: 0, AdjustedTime: 0, Title: "Introduction"},
			{OriginalTime: 10 * time.Second, AdjustedTime: 10 * time.Second, Title: "Main Results"},
		},
	}

	if err := SaveMetaFile(tmpDir, meta); err != nil {
		t.Fatalf("SaveMetaFile failed: %v", err)
	}

	if !HasSavedMetadata(tmpDir) {
		t.Fatalf("expected HasSavedMetadata to be true after saving")
	}

	loaded, err := LoadMetaFile(tmpDir)
	if err != nil {
		t.Fatalf("LoadMetaFile failed: %v", err)
	}

	if loaded.Title != meta.Title {
		t.Errorf("title: got %q, want %q", loaded.Title, meta.Title)
	}
	if loaded.Speaker != meta.Speaker {
		t.Errorf("speaker: got %q, want %q", loaded.Speaker, meta.Speaker)
	}
	if loaded.Affiliation != meta.Affiliation {
		t.Errorf("affiliation: got %q, want %q", loaded.Affiliation, meta.Affiliation)
	}
	if loaded.URL != meta.URL {
		t.Errorf("url: got %q, want %q", loaded.URL, meta.URL)
	}
	if loaded.Abstract != meta.Abstract {
		t.Errorf("abstract: got %q, want %q", loaded.Abstract, meta.Abstract)
	}
	if len(loaded.Tags) != len(meta.Tags) {
		t.Errorf("tags length: got %d, want %d", len(loaded.Tags), len(meta.Tags))
	}
	if loaded.YouTubeID != meta.YouTubeID {
		t.Errorf("youtube_id: got %q, want %q", loaded.YouTubeID, meta.YouTubeID)
	}
	if loaded.YouTubeURL != meta.YouTubeURL {
		t.Errorf("youtube_url: got %q, want %q", loaded.YouTubeURL, meta.YouTubeURL)
	}
	if loaded.PlaylistID != meta.PlaylistID {
		t.Errorf("playlist_id: got %q, want %q", loaded.PlaylistID, meta.PlaylistID)
	}
	if loaded.PlaylistTitle != meta.PlaylistTitle {
		t.Errorf("playlist_title: got %q, want %q", loaded.PlaylistTitle, meta.PlaylistTitle)
	}
}
