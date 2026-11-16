package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

func item(id, video, date string) PlaylistItem {
	return PlaylistItem{ItemID: id, VideoID: video, Title: "t" + video, VideoPublishedAt: date}
}

func itemIDs(items []PlaylistItem) []string {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ItemID
	}
	return ids
}

func TestSplitDuplicates(t *testing.T) {
	items := []PlaylistItem{item("a", "v1", ""), item("b", "v2", ""), item("c", "v1", ""), item("d", "", ""), item("e", "", "")}
	kept, dups := SplitDuplicates(items)
	if got := itemIDs(kept); !slices.Equal(got, []string{"a", "b", "d", "e"}) {
		t.Errorf("kept = %v", got)
	}
	if got := itemIDs(dups); !slices.Equal(got, []string{"c"}) {
		t.Errorf("dups = %v", got)
	}
}

func TestNewestFirst(t *testing.T) {
	items := []PlaylistItem{
		item("old", "v1", "2024-01-01T00:00:00Z"),
		item("nodate", "v2", ""),
		item("new", "v3", "2026-09-08T10:00:00Z"),
		item("mid", "v4", "2025-05-05T00:00:00+02:00"),
		item("sameday", "v5", "2026-09-08T18:00:00Z"),
		item("earlier", "v6", "2026-09-08T01:00:00Z"),
	}
	got := itemIDs(NewestFirst(items))
	// Same-day videos keep their playlist order even when published at different times.
	want := []string{"new", "sameday", "earlier", "mid", "old", "nodate"}
	if !slices.Equal(got, want) {
		t.Errorf("NewestFirst = %v, want %v", got, want)
	}
}

// simulate applies moves the way YouTube does: remove the item, then insert it at Position.
func simulate(current []PlaylistItem, moves []PlaylistMove) []PlaylistItem {
	order := slices.Clone(current)
	for _, m := range moves {
		order = slices.DeleteFunc(order, func(o PlaylistItem) bool { return o.ItemID == m.Item.ItemID })
		order = slices.Insert(order, m.Position, m.Item)
	}
	return order
}

func TestPlanMovesReachesTargetMinimally(t *testing.T) {
	letters := func(s string) []PlaylistItem {
		var out []PlaylistItem
		for _, r := range s {
			out = append(out, item(string(r), string(r), ""))
		}
		return out
	}
	for _, tt := range []struct {
		current, target string
		moves           int
	}{
		{"abcde", "abcde", 0},
		{"eabcd", "abcde", 1},
		{"bcdea", "abcde", 1},
		{"edcba", "abcde", 4},
		{"acbed", "abcde", 2},
		{"", "", 0},
	} {
		cur, tgt := letters(tt.current), letters(tt.target)
		moves := PlanMoves(cur, tgt)
		if len(moves) != tt.moves {
			t.Errorf("%s→%s: %d moves, want %d", tt.current, tt.target, len(moves), tt.moves)
		}
		if got := itemIDs(simulate(cur, moves)); !slices.Equal(got, itemIDs(tgt)) {
			t.Errorf("%s→%s: reached %v", tt.current, tt.target, got)
		}
	}
}

func TestPlanMovesRandom(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for n := range 30 {
		cur := make([]PlaylistItem, n)
		for i := range cur {
			cur[i] = item(fmt.Sprint(i), fmt.Sprint(i), "")
		}
		tgt := slices.Clone(cur)
		rng.Shuffle(len(tgt), func(i, j int) { tgt[i], tgt[j] = tgt[j], tgt[i] })
		if got := itemIDs(simulate(cur, PlanMoves(cur, tgt))); !slices.Equal(got, itemIDs(tgt)) {
			t.Fatalf("n=%d: reached %v, want %v", n, got, itemIDs(tgt))
		}
	}
}

// fakePlaylist is an in-memory playlistItems endpoint supporting list, delete and update.
type fakePlaylist struct {
	mu         sync.Mutex
	items      []PlaylistItem
	writes     int
	manualSort bool
}

func (f *fakePlaylist) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case http.MethodGet:
		f.list(w)
	case http.MethodDelete:
		f.writes++
		id := r.URL.Query().Get("id")
		f.items = slices.DeleteFunc(f.items, func(it PlaylistItem) bool { return it.ItemID == id })
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		f.writes++
		f.update(w, r)
	}
}

func (f *fakePlaylist) list(w http.ResponseWriter) {
	var page playlistItemsDetailPage
	for _, it := range f.items {
		var e = struct {
			ID      string `json:"id"`
			Snippet struct {
				Title      string `json:"title"`
				ResourceID struct {
					VideoID string `json:"videoId"`
				} `json:"resourceId"`
			} `json:"snippet"`
			ContentDetails struct {
				VideoPublishedAt string `json:"videoPublishedAt"`
			} `json:"contentDetails"`
		}{ID: it.ItemID}
		e.Snippet.Title = it.Title
		e.Snippet.ResourceID.VideoID = it.VideoID
		e.ContentDetails.VideoPublishedAt = it.VideoPublishedAt
		page.Items = append(page.Items, e)
	}
	_ = json.NewEncoder(w).Encode(page)
}

func (f *fakePlaylist) update(w http.ResponseWriter, r *http.Request) {
	if !f.manualSort {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, `{"error":{"errors":[{"reason":"manualSortRequired"}]}}`)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		ID      string `json:"id"`
		Snippet struct {
			Position int `json:"position"`
		} `json:"snippet"`
	}
	_ = json.Unmarshal(body, &req)
	i := slices.IndexFunc(f.items, func(it PlaylistItem) bool { return it.ItemID == req.ID })
	it := f.items[i]
	f.items = slices.Delete(f.items, i, i+1)
	f.items = slices.Insert(f.items, req.Snippet.Position, it)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, `{}`)
}

func serveFake(t *testing.T, f *fakePlaylist) *http.Client {
	ts := httptest.NewServer(f)
	t.Cleanup(ts.Close)
	override(t, &playlistItemsDetailEndpoint, ts.URL+"/playlistItems?part=snippet,contentDetails")
	override(t, &playlistItemsUpdateEndpoint, ts.URL+"/playlistItems?part=snippet")
	override(t, &playlistItemsRemoveEndpoint, ts.URL+"/playlistItems")
	override(t, &propagationRetryDelay, 0)
	return ts.Client()
}

func tidyFixture() []PlaylistItem {
	return []PlaylistItem{
		item("i1", "old", "2024-01-01T00:00:00Z"),
		item("i2", "new", "2026-09-08T00:00:00Z"),
		item("i3", "old", "2024-01-01T00:00:00Z"),
		item("i4", "mid", "2025-01-01T00:00:00Z"),
		item("i5", "new", "2026-09-08T00:00:00Z"),
	}
}

func TestTidyPlaylist(t *testing.T) {
	f := &fakePlaylist{items: tidyFixture(), manualSort: true}
	client := serveFake(t, f)

	res, err := TidyPlaylist(context.Background(), client, "PL1", false, nil)
	if err != nil {
		t.Fatalf("TidyPlaylist: %v", err)
	}
	if got := itemIDs(f.items); !slices.Equal(got, []string{"i2", "i4", "i1"}) {
		t.Errorf("remote order = %v", got)
	}
	if len(res.Duplicates) != 2 || res.Items != 5 {
		t.Errorf("result = %+v", res)
	}

	// A tidy playlist needs no writes.
	f.writes = 0
	if _, err := TidyPlaylist(context.Background(), client, "PL1", false, nil); err != nil || f.writes != 0 {
		t.Errorf("second tidy: err=%v writes=%d", err, f.writes)
	}
}

func TestTidyPlaylistDryRunWritesNothing(t *testing.T) {
	f := &fakePlaylist{items: tidyFixture(), manualSort: true}
	client := serveFake(t, f)
	res, err := TidyPlaylist(context.Background(), client, "PL1", true, nil)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if f.writes != 0 || len(res.Duplicates) != 2 || len(res.Moves) == 0 {
		t.Errorf("writes=%d result=%+v", f.writes, res)
	}
}

func TestTidyPlaylistManualSortRequired(t *testing.T) {
	f := &fakePlaylist{items: tidyFixture()}
	client := serveFake(t, f)
	_, err := TidyPlaylist(context.Background(), client, "PL1", false, nil)
	if !errors.Is(err, ErrManualSortRequired) {
		t.Fatalf("err = %v, want ErrManualSortRequired", err)
	}
	if !strings.Contains(err.Error(), "Manual") || !strings.Contains(err.Error(), "2 repeated video(s) were already removed") {
		t.Errorf("message does not say how to fix it: %v", err)
	}
}
