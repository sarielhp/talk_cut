// Package youtube implements YouTube OAuth2 authorization, resumable video upload, and caption synchronization.
package youtube

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"talk_cut/internal/model"
)

func TestUploadVideo(t *testing.T) {
	// Create temporary dummy video file
	tmpDir := t.TempDir()
	dummyVideo := filepath.Join(tmpDir, "test_video.mp4")
	content := []byte("fake mp4 video binary stream content 1234567890")
	if err := os.WriteFile(dummyVideo, content, 0o644); err != nil {
		t.Fatalf("writing dummy video: %v", err)
	}

	var uploadURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			// Session initiation
			w.Header().Set("Location", uploadURL)
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			// File payload upload
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"id": "mock_youtube_id_999"}`)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer ts.Close()

	uploadURL = ts.URL + "/resumable_upload"
	origEndpoint := videosEndpoint
	videosEndpoint = ts.URL + "/initiate"
	defer func() { videosEndpoint = origEndpoint }()

	opts := UploadOptions{
		VideoPath: dummyVideo,
		Metadata: model.TalkMetadata{
			Title:   "Test Lecture",
			Privacy: "unlisted",
		},
	}

	res, err := UploadVideo(context.Background(), ts.Client(), opts)
	if err != nil {
		t.Fatalf("UploadVideo failed: %v", err)
	}

	if res.VideoID != "mock_youtube_id_999" {
		t.Errorf("expected VideoID mock_youtube_id_999, got %q", res.VideoID)
	}
	if res.VideoURL != "https://youtu.be/mock_youtube_id_999" {
		t.Errorf("expected URL https://youtu.be/mock_youtube_id_999, got %q", res.VideoURL)
	}
}

func TestUploadCaption(t *testing.T) {
	tmpDir := t.TempDir()
	dummyVTT := filepath.Join(tmpDir, "test.vtt")
	if err := os.WriteFile(dummyVTT, []byte("WEBVTT\n\n1\n00:00:00.000 --> 00:00:05.000\nHello world\n"), 0o644); err != nil {
		t.Fatalf("writing dummy vtt: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "expected post", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"id": "caption_id_123"}`)
	}))
	defer ts.Close()

	origCap := captionsEndpoint
	captionsEndpoint = ts.URL + "/captions"
	defer func() { captionsEndpoint = origCap }()

	err := UploadCaption(context.Background(), ts.Client(), "mock_vid", dummyVTT, "en", "English")
	if err != nil {
		t.Fatalf("UploadCaption failed: %v", err)
	}
}
