package youtube

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withPlaylistItemsServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(handler)
	origItems, origChannels := playlistItemsEndpoint, channelsEndpoint
	playlistItemsEndpoint = ts.URL + "/playlistItems?part=snippet"
	channelsEndpoint = ts.URL + "/channels?part=contentDetails&mine=true"
	t.Cleanup(func() {
		playlistItemsEndpoint, channelsEndpoint = origItems, origChannels
		ts.Close()
	})
	return ts
}

func TestListRecentUploads(t *testing.T) {
	ts := withPlaylistItemsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/channels" {
			fmt.Fprint(w, `{"items":[{"contentDetails":{"relatedPlaylists":{"uploads":"UU42"}}}]}`)
			return
		}
		if got := r.URL.Query().Get("playlistId"); got != "UU42" {
			t.Errorf("expected uploads playlist UU42, got %q", got)
		}
		if got := r.URL.Query().Get("maxResults"); got != "20" {
			t.Errorf("expected maxResults=20, got %q", got)
		}
		fmt.Fprint(w, `{"items":[
			{"snippet":{"title":"Newest","publishedAt":"2026-10-30T10:00:00Z","resourceId":{"videoId":"v1"}}},
			{"snippet":{"title":"Deleted","resourceId":{"videoId":""}}},
			{"snippet":{"title":"Older","publishedAt":"2026-10-01T10:00:00Z","resourceId":{"videoId":"v2"}}}
		]}`)
	})

	videos, err := ListRecentUploads(context.Background(), ts.Client(), 20)
	if err != nil {
		t.Fatalf("ListRecentUploads failed: %v", err)
	}
	if len(videos) != 2 || videos[0].ID != "v1" || videos[1].Title != "Older" {
		t.Fatalf("unexpected videos: %+v", videos)
	}
	if videos[0].PublishedAt != "2026-10-30T10:00:00Z" {
		t.Errorf("unexpected publishedAt: %q", videos[0].PublishedAt)
	}
}

func TestListPlaylistVideoIDsPaginates(t *testing.T) {
	ts := withPlaylistItemsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			fmt.Fprint(w, `{"nextPageToken":"P2","items":[{"snippet":{"resourceId":{"videoId":"a"}}}]}`)
			return
		}
		fmt.Fprint(w, `{"items":[{"snippet":{"resourceId":{"videoId":"b"}}}]}`)
	})

	ids, err := ListPlaylistVideoIDs(context.Background(), ts.Client(), "PL1")
	if err != nil {
		t.Fatalf("ListPlaylistVideoIDs failed: %v", err)
	}
	if len(ids) != 2 || !ids["a"] || !ids["b"] {
		t.Fatalf("expected {a, b}, got %v", ids)
	}
}

func TestListPlaylistVideoIDsErrors(t *testing.T) {
	ts := withPlaylistItemsServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	})

	if _, err := ListPlaylistVideoIDs(context.Background(), ts.Client(), " "); err == nil {
		t.Error("expected error for empty playlist ID")
	}
	if _, err := ListPlaylistVideoIDs(context.Background(), ts.Client(), "PL1"); err == nil {
		t.Error("expected error for HTTP 403")
	}
}

func TestListPlaylistVideoIDsRefusesIncompleteResult(t *testing.T) {
	ts := withPlaylistItemsServer(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"nextPageToken":"more","items":[{"snippet":{"resourceId":{"videoId":"a"}}}]}`)
	})
	if _, err := ListPlaylistVideoIDs(context.Background(), ts.Client(), "PL1"); err == nil {
		t.Fatal("expected an error instead of a silently truncated membership set")
	}
}
