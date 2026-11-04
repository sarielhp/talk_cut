package youtube

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListPlaylists(t *testing.T) {
	ts := jsonServer(t, http.MethodGet, `{
			"items": [
				{
					"id": "PL123",
					"snippet": {"title": "Geometry Seminar", "description": "Fall talks"},
					"contentDetails": {"itemCount": 5},
					"status": {"privacyStatus": "public"}
				},
				{
					"id": "PL456",
					"snippet": {"title": "SoCG 2026", "description": "Conference"},
					"contentDetails": {"itemCount": 12},
					"status": {"privacyStatus": "unlisted"}
				}
			]
		}`)

	override(t, &playlistsListEndpoint, ts.URL)

	playlists, err := ListPlaylists(context.Background(), ts.Client())
	if err != nil {
		t.Fatalf("ListPlaylists failed: %v", err)
	}

	if len(playlists) != 2 {
		t.Fatalf("expected 2 playlists, got %d", len(playlists))
	}
	if playlists[0].ID != "PL123" || playlists[0].Title != "Geometry Seminar" || playlists[0].ItemCount != 5 {
		t.Errorf("unexpected playlist 0: %+v", playlists[0])
	}
	if playlists[1].ID != "PL456" || playlists[1].Title != "SoCG 2026" {
		t.Errorf("unexpected playlist 1: %+v", playlists[1])
	}
}

func TestCreatePlaylist(t *testing.T) {
	ts := jsonServer(t, http.MethodPost, `{
			"id": "PL_new_789",
			"snippet": {"title": "Theory Seminar 2026", "description": "New talks"},
			"status": {"privacyStatus": "public"}
		}`)

	override(t, &playlistsCreateEndpoint, ts.URL)

	pl, err := CreatePlaylist(context.Background(), ts.Client(), "Theory Seminar 2026", "New talks", "public")
	if err != nil {
		t.Fatalf("CreatePlaylist failed: %v", err)
	}

	if pl.ID != "PL_new_789" {
		t.Errorf("expected ID PL_new_789, got %q", pl.ID)
	}
	if pl.Title != "Theory Seminar 2026" {
		t.Errorf("expected Title 'Theory Seminar 2026', got %q", pl.Title)
	}
}

func TestAddVideoToPlaylist(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "expected POST", http.StatusMethodNotAllowed)
			return
		}
		if strings.Contains(r.URL.Path, "already") {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, `{"error": {"errors": [{"reason": "videoAlreadyInPlaylist"}]}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"id": "item_123"}`)
	}))
	defer ts.Close()

	override(t, &playlistItemsInsertEndpoint, ts.URL)

	// Standard add
	if err := AddVideoToPlaylist(context.Background(), ts.Client(), "PL123", "vid999"); err != nil {
		t.Fatalf("AddVideoToPlaylist failed: %v", err)
	}

	// Idempotent duplicate add
	playlistItemsInsertEndpoint = ts.URL + "/already"
	if err := AddVideoToPlaylist(context.Background(), ts.Client(), "PL123", "vid999"); err != nil {
		t.Fatalf("AddVideoToPlaylist duplicate should not error, got: %v", err)
	}
}

func TestFindPlaylist(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{
			"items": [
				{
					"id": "PL_abc",
					"snippet": {"title": "CompGeom Seminar Fall 2026"},
					"contentDetails": {"itemCount": 3},
					"status": {"privacyStatus": "public"}
				}
			]
		}`)
	}))
	defer ts.Close()

	override(t, &playlistsListEndpoint, ts.URL)

	// 1. By exact ID
	pl, err := FindPlaylist(context.Background(), ts.Client(), "PL_abc")
	if err != nil || pl.Title != "CompGeom Seminar Fall 2026" {
		t.Errorf("FindPlaylist by ID failed: %v, %v", pl, err)
	}

	// 2. By substring
	plSub, err := FindPlaylist(context.Background(), ts.Client(), "Fall 2026")
	if err != nil || plSub.ID != "PL_abc" {
		t.Errorf("FindPlaylist by substring failed: %v, %v", plSub, err)
	}

	// 3. Not found
	_, errNotFound := FindPlaylist(context.Background(), ts.Client(), "NonExistent")
	if errNotFound == nil {
		t.Errorf("expected error for non-existent playlist, got nil")
	}
}

func TestRemoveVideoFromPlaylist(t *testing.T) {
	deleted := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `{
				"items": [
					{"id": "item_to_delete_1"}
				]
			}`)
			return
		}
		if r.Method == http.MethodDelete {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "bad method", http.StatusMethodNotAllowed)
	}))
	defer ts.Close()

	override(t, &playlistItemsRemoveEndpoint, ts.URL)

	err := RemoveVideoFromPlaylist(context.Background(), ts.Client(), "PL123", "vid999")
	if err != nil {
		t.Fatalf("RemoveVideoFromPlaylist failed: %v", err)
	}
	if !deleted {
		t.Error("expected DELETE request to be issued for item_to_delete_1")
	}
}

func TestRemoveVideoFromPlaylistReportsFailures(t *testing.T) {
	override(t, &propagationRetryDelay, 0)

	cases := []struct {
		name         string
		listStatus   int
		listBody     string
		deleteStatus int
		wantErr      bool
	}{
		{"not in playlist", http.StatusOK, `{"items": []}`, http.StatusNoContent, false},
		{"lookup forbidden", http.StatusForbidden, `{"error": "quotaExceeded"}`, http.StatusNoContent, true},
		{"lookup bad json", http.StatusOK, `{not json`, http.StatusNoContent, true},
		{"delete forbidden", http.StatusOK, `{"items": [{"id": "it1"}]}`, http.StatusForbidden, true},
		{"delete already gone", http.StatusOK, `{"items": [{"id": "it1"}]}`, http.StatusNotFound, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.WriteHeader(tc.listStatus)
					fmt.Fprint(w, tc.listBody)
					return
				}
				w.WriteHeader(tc.deleteStatus)
			}))
			defer ts.Close()

			override(t, &playlistItemsRemoveEndpoint, ts.URL)

			err := RemoveVideoFromPlaylist(context.Background(), ts.Client(), "PL123", "vid999")
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestAddVideoToPlaylistRetriesWhileNewPlaylistPropagates(t *testing.T) {
	override(t, &propagationRetryDelay, 0)

	cases := []struct {
		name     string
		statuses []int
		body     string
		wantErr  bool
		wantHits int
	}{
		{"409 then ok", []int{http.StatusConflict, http.StatusOK}, `{"error":{"status":"ABORTED"}}`, false, 2},
		{"404 playlistNotFound then ok", []int{http.StatusNotFound, http.StatusOK}, `{"reason":"playlistNotFound"}`, false, 2},
		{"403 is not retried", []int{http.StatusForbidden}, `{"reason":"forbidden"}`, true, 1},
		{"gives up", []int{409, 409, 409, 409, 409, 409}, `{}`, true, propagationRetries + 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.statuses[min(hits, len(tc.statuses)-1)])
				hits++
				fmt.Fprint(w, tc.body)
			}))
			defer ts.Close()
			override(t, &playlistItemsInsertEndpoint, ts.URL)

			err := AddVideoToPlaylist(context.Background(), ts.Client(), "PL1", "v1")
			if (err != nil) != tc.wantErr || hits != tc.wantHits {
				t.Fatalf("err=%v hits=%d; want err=%v hits=%d", err, hits, tc.wantErr, tc.wantHits)
			}
		})
	}
}

func TestRemoveVideoFromPlaylistWaitsForJustAddedItem(t *testing.T) {
	override(t, &propagationRetryDelay, 0)

	lookups, deleted := 0, false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		lookups++
		if lookups < 3 {
			fmt.Fprint(w, `{"items": []}`) // the add has not propagated yet
			return
		}
		fmt.Fprint(w, `{"items": [{"id": "it1"}]}`)
	}))
	defer ts.Close()
	override(t, &playlistItemsRemoveEndpoint, ts.URL)

	if err := RemoveVideoFromPlaylist(context.Background(), ts.Client(), "PL1", "v1"); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("remove gave up before the just-added item became visible")
	}
}

func TestRemoveVideoFromPlaylistWaitsForNewPlaylist(t *testing.T) {
	override(t, &propagationRetryDelay, 0)

	lookups, deleted := 0, false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		lookups++
		if lookups == 1 {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":{"errors":[{"reason":"playlistNotFound"}]}}`)
			return
		}
		fmt.Fprint(w, `{"items": [{"id": "it1"}]}`)
	}))
	defer ts.Close()
	override(t, &playlistItemsRemoveEndpoint, ts.URL)

	if err := RemoveVideoFromPlaylist(context.Background(), ts.Client(), "PL1", "v1"); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected the item to be deleted once the playlist became visible")
	}
}
