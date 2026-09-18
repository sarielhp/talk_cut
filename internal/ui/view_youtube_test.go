package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"talk_cut/internal/config"
	"talk_cut/internal/model"
	"talk_cut/internal/youtube"
)

func TestYouTubeModelPreflight(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels = map[string]string{
		"seminar": "~/.config/auth/youtube_seminar.json",
		"course":  "~/.config/auth/youtube_course.json",
	}
	cfg.DefaultChannel = "seminar"

	meta := model.TalkMetadata{
		Title:   "Circle Graphs Lecture",
		Speaker: "Karim",
		Privacy: "unlisted",
		Chapters: []model.ChapterMarker{
			{Title: "Intro", AdjustedTime: 0},
			{Title: "Results", AdjustedTime: 10 * time.Minute},
		},
	}

	m := NewYouTubeModel("/nonexistent/video.mp4", "/nonexistent/video.vtt", meta, cfg)
	m.SetDimensions(100, 30)

	if m.CurrentChannel() != "seminar" {
		t.Fatalf("expected current channel 'seminar', got %q", m.CurrentChannel())
	}

	m.NextChannel()
	if m.CurrentChannel() != "course" {
		t.Fatalf("expected current channel 'course' after NextChannel, got %q", m.CurrentChannel())
	}

	view := m.View()
	if !strings.Contains(view, "YOUTUBE PUBLISH & UPLOAD PRE-FLIGHT") {
		t.Errorf("expected view to contain preflight header, got:\n%s", view)
	}
	if !strings.Contains(view, "Circle Graphs Lecture") {
		t.Errorf("expected view to contain title, got:\n%s", view)
	}
}

func TestYouTubeModelSuccessView(t *testing.T) {
	cfg := config.DefaultConfig()
	meta := model.TalkMetadata{
		Title:   "Circle Graphs Lecture",
		Privacy: "public",
	}

	m := NewYouTubeModel("/tmp/test.mp4", "/tmp/test.vtt", meta, cfg)
	m.SetDimensions(100, 30)

	ver := &youtube.VideoVerification{
		VideoID:          "mock_123",
		Title:            "Circle Graphs Lecture",
		UploadStatus:     "uploaded",
		PrivacyStatus:    "public",
		ProcessingStatus: "processing",
		ShortURL:         "https://youtu.be/mock_123",
		WatchURL:         "https://www.youtube.com/watch?v=mock_123",
	}

	m.SetDone(ver, true, nil)
	if !m.IsDone() {
		t.Fatal("expected IsDone to be true")
	}

	view := m.View()
	if !strings.Contains(view, "VIDEO PUBLISHED & VERIFIED ON YOUTUBE") {
		t.Errorf("expected view to contain success title, got:\n%s", view)
	}
	if !strings.Contains(view, "https://youtu.be/mock_123") {
		t.Errorf("expected view to contain short URL, got:\n%s", view)
	}
	if !strings.Contains(view, "Closed captions uploaded") {
		t.Errorf("expected view to indicate captions uploaded, got:\n%s", view)
	}
}

func TestYouTubeModelErrorView(t *testing.T) {
	cfg := config.DefaultConfig()
	m := NewYouTubeModel("", "", model.TalkMetadata{}, cfg)
	m.SetDimensions(100, 30)

	m.SetDone(nil, false, errors.New("network timeout reaching googleapis"))
	view := m.View()
	if !strings.Contains(view, "YOUTUBE UPLOAD FAILED") {
		t.Errorf("expected failure header, got:\n%s", view)
	}
	if !strings.Contains(view, "network timeout reaching googleapis") {
		t.Errorf("expected error message in view, got:\n%s", view)
	}
}

func TestYouTubeModelUpdateDetailsView(t *testing.T) {
	cfg := config.DefaultConfig()
	meta := model.TalkMetadata{
		Title:     "Spanners",
		YouTubeID: "vid_test_123",
	}
	m := NewYouTubeModel("", "", meta, cfg)
	m.SetDimensions(100, 30)
	m.hasToken = true

	if m.VideoID() != "vid_test_123" {
		t.Errorf("expected VideoID vid_test_123, got %q", m.VideoID())
	}

	view := m.View()
	if !strings.Contains(view, "vid_test_123") {
		t.Errorf("expected view to contain YouTube video ID, got:\n%s", view)
	}
	if !strings.Contains(view, "Update Details") {
		t.Errorf("expected view to contain Update Details button, got:\n%s", view)
	}

	m.SetUpdatingDetails(true)
	updatingView := m.View()
	if !strings.Contains(updatingView, "Updating video title, description, and chapters") {
		t.Errorf("expected updating description in view, got:\n%s", updatingView)
	}
}

func TestYouTubeModelPlaylistSelection(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DefaultPlaylist = "pl_default"

	meta := model.TalkMetadata{
		Title:      "Oriented Spanners",
		PlaylistID: "pl_existing",
	}

	m := NewYouTubeModel("", "", meta, cfg)
	m.SetDimensions(100, 30)

	playlists := []youtube.Playlist{
		{ID: "pl_other", Title: "Other Talks", ItemCount: 5},
		{ID: "pl_default", Title: "Default Seminar", ItemCount: 12},
		{ID: "pl_existing", Title: "Geometry Seminar", ItemCount: 30},
	}

	m.SetPlaylists(playlists, cfg.DefaultPlaylist)
	if m.SelectedPlaylist() == nil || m.SelectedPlaylist().ID != "pl_existing" {
		t.Fatalf("expected existing playlist selected by default, got %+v", m.SelectedPlaylist())
	}

	m.NextPlaylist()
	if m.SelectedPlaylist().ID != "pl_other" {
		t.Errorf("expected wrap-around to pl_other, got %s", m.SelectedPlaylist().ID)
	}

	m.PrevPlaylist()
	if m.SelectedPlaylist().ID != "pl_existing" {
		t.Errorf("expected wrap-back to pl_existing, got %s", m.SelectedPlaylist().ID)
	}

	m.SetSelectingPlaylist(true)
	view := m.View()
	if !strings.Contains(view, "SELECT YOUTUBE PLAYLIST") {
		t.Errorf("expected playlist selector title in view, got:\n%s", view)
	}
	if !strings.Contains(view, "[DEFAULT]") {
		t.Errorf("expected [DEFAULT] badge in view, got:\n%s", view)
	}
	if !strings.Contains(view, "[ADDED]") {
		t.Errorf("expected [ADDED] badge in view, got:\n%s", view)
	}

	m.SetPlaylistAdded("pl_default", "Default Seminar")
	if m.SelectedPlaylist() == nil {
		t.Fatal("expected selected playlist not nil")
	}
	m.SetPlaylistFeedback("Added to Default Seminar")
	viewWithFeedback := m.View()
	if !strings.Contains(viewWithFeedback, "Added to Default Seminar") {
		t.Errorf("expected feedback in view, got:\n%s", viewWithFeedback)
	}

	// Test multiple playlists rendering in preflight
	m.SetPlaylistAdded("pl_other", "Other Talks")
	if len(m.talkMeta.AllPlaylists()) != 3 {
		t.Fatalf("expected 3 playlists, got %d", len(m.talkMeta.AllPlaylists()))
	}
	m.SetSelectingPlaylist(false)
	m.hasToken = true
	preflightView := m.View()
	if !strings.Contains(preflightView, "Playlists:") {
		t.Errorf("expected 'Playlists:' in preflight view for multiple playlists, got:\n%s", preflightView)
	}

	// Test playlist removal
	m.SetPlaylistRemoved("pl_other", "Other Talks")
	if m.talkMeta.HasPlaylist("pl_other") {
		t.Errorf("expected pl_other removed")
	}
}
