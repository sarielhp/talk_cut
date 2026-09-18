// Package youtube implements YouTube OAuth2 authorization, resumable video upload, and caption synchronization.
package youtube

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestVerifyVideo(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "expected get", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.RawQuery, "vid_not_found") {
			fmt.Fprintln(w, `{"items": []}`)
			return
		}
		fmt.Fprintln(w, `{
			"items": [{
				"id": "vid_abc123",
				"snippet": {"title": "Verified Talk"},
				"status": {"uploadStatus": "uploaded", "privacyStatus": "unlisted"},
				"processingDetails": {"processingStatus": "processing"},
				"contentDetails": {"duration": "PT44M58S"}
			}]
		}`)
	}))
	defer ts.Close()

	origEndpoint := videoStatusEndpoint
	videoStatusEndpoint = ts.URL + "/videos?part=snippet,status,processingDetails,contentDetails"
	defer func() { videoStatusEndpoint = origEndpoint }()

	ver, err := VerifyVideo(context.Background(), ts.Client(), "vid_abc123")
	if err != nil {
		t.Fatalf("VerifyVideo failed: %v", err)
	}

	if ver.VideoID != "vid_abc123" {
		t.Errorf("expected VideoID vid_abc123, got %q", ver.VideoID)
	}
	if ver.Title != "Verified Talk" {
		t.Errorf("expected Title 'Verified Talk', got %q", ver.Title)
	}
	if ver.UploadStatus != "uploaded" {
		t.Errorf("expected UploadStatus 'uploaded', got %q", ver.UploadStatus)
	}
	if ver.PrivacyStatus != "unlisted" {
		t.Errorf("expected PrivacyStatus 'unlisted', got %q", ver.PrivacyStatus)
	}
	if ver.ProcessingStatus != "processing" {
		t.Errorf("expected ProcessingStatus 'processing', got %q", ver.ProcessingStatus)
	}
	if ver.ShortURL != "https://youtu.be/vid_abc123" {
		t.Errorf("expected ShortURL 'https://youtu.be/vid_abc123', got %q", ver.ShortURL)
	}
	if ver.DurationRaw != "PT44M58S" {
		t.Errorf("expected DurationRaw 'PT44M58S', got %q", ver.DurationRaw)
	}
	if expectedDur := 44*time.Minute + 58*time.Second; ver.Duration != expectedDur {
		t.Errorf("expected Duration %v, got %v", expectedDur, ver.Duration)
	}

	// Verify not found returns error
	_, notFoundErr := VerifyVideo(context.Background(), ts.Client(), "vid_not_found")
	if notFoundErr == nil {
		t.Errorf("expected error for non-existent video, got nil")
	}
}

func TestUpdateVideoMetadata(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{"id": "vid_abc123"}`)
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{
				"items": [{
					"id": "vid_abc123",
					"snippet": {"title": "Updated Title"},
					"status": {"uploadStatus": "uploaded", "privacyStatus": "public"},
					"processingDetails": {"processingStatus": "succeeded"}
				}]
			}`)
		default:
			http.Error(w, "expected put or get", http.StatusMethodNotAllowed)
		}
	}))
	defer ts.Close()

	origEndpoint := videoStatusEndpoint
	origUpdate := videoUpdateEndpoint
	videoStatusEndpoint = ts.URL + "/videos?part=snippet,status,processingDetails"
	videoUpdateEndpoint = ts.URL + "/videos"
	defer func() {
		videoStatusEndpoint = origEndpoint
		videoUpdateEndpoint = origUpdate
	}()

	meta := model.TalkMetadata{
		Title:    "Updated Title",
		Speaker:  "Updated Speaker",
		Abstract: "Updated Abstract",
		Privacy:  "public",
	}

	ver, err := UpdateVideoMetadata(context.Background(), ts.Client(), "vid_abc123", meta)
	if err != nil {
		t.Fatalf("UpdateVideoMetadata failed: %v", err)
	}

	if ver.VideoID != "vid_abc123" {
		t.Errorf("expected VideoID vid_abc123, got %q", ver.VideoID)
	}
	if ver.Title != "Updated Title" {
		t.Errorf("expected Title 'Updated Title', got %q", ver.Title)
	}
}

func TestMatchVideoTitle(t *testing.T) {
	if !matchVideoTitle("15", "Oriented Spanners", "15") {
		t.Errorf("expected match for directory name 15")
	}
	if !matchVideoTitle("Oriented Spanners in Metric Spaces", "Oriented Spanners in Metric Spaces", "15") {
		t.Errorf("expected exact match for talk title")
	}
	if !matchVideoTitle("Sariel: Oriented Spanners in Metric Spaces", "Oriented Spanners in Metric Spaces", "15") {
		t.Errorf("expected substring match for talk title")
	}
	if matchVideoTitle("Completely Unrelated Video", "Oriented Spanners", "15") {
		t.Errorf("expected no match for unrelated video")
	}
}

func TestFindChannelVideoForTalk(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/channels"):
			fmt.Fprintln(w, `{
				"items": [{
					"contentDetails": {
						"relatedPlaylists": {"uploads": "UU_test_uploads"}
					}
				}]
			}`)
		case strings.Contains(r.URL.Path, "/playlistItems"):
			fmt.Fprintln(w, `{
				"items": [
					{
						"snippet": {
							"title": "15",
							"resourceId": {"videoId": "GqfuUGTR12Y"}
						}
					},
					{
						"snippet": {
							"title": "Unrelated Talk",
							"resourceId": {"videoId": "other_vid_999"}
						}
					}
				]
			}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	origChannels := channelsEndpoint
	origPlaylist := playlistItemsEndpoint
	channelsEndpoint = ts.URL + "/channels?part=contentDetails&mine=true"
	playlistItemsEndpoint = ts.URL + "/playlistItems?part=snippet"
	defer func() {
		channelsEndpoint = origChannels
		playlistItemsEndpoint = origPlaylist
	}()

	ver, err := FindChannelVideoForTalk(context.Background(), ts.Client(), "Oriented Spanners", "15")
	if err != nil {
		t.Fatalf("FindChannelVideoForTalk failed: %v", err)
	}
	if ver == nil {
		t.Fatalf("expected video match, got nil")
	}
	if ver.VideoID != "GqfuUGTR12Y" {
		t.Errorf("expected VideoID GqfuUGTR12Y, got %q", ver.VideoID)
	}
	if ver.ShortURL != "https://youtu.be/GqfuUGTR12Y" {
		t.Errorf("expected short URL, got %q", ver.ShortURL)
	}
}

func TestSanitizeYouTubeText(t *testing.T) {
	input := "For any constant ε > 0 and 0 < x < 1"
	expected := "For any constant ε ＞ 0 and 0 ＜ x ＜ 1"
	if got := sanitizeYouTubeText(input); got != expected {
		t.Errorf("sanitizeYouTubeText(%q) = %q, want %q", input, got, expected)
	}
}
