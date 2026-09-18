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
