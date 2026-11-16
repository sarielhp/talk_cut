package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"talk_cut/internal/readonly"
	"time"

	"talk_cut/internal/ctxsleep"
)

var (
	playlistItemsDetailEndpoint = "https://www.googleapis.com/youtube/v3/playlistItems?part=snippet,contentDetails"
	playlistItemsUpdateEndpoint = "https://www.googleapis.com/youtube/v3/playlistItems?part=snippet"
)

// ErrManualSortRequired reports that YouTube refused a reorder because the playlist is sorted automatically.
var ErrManualSortRequired = errors.New("the playlist is not sorted manually; in YouTube Studio set its ordering to \"Manual\" and run again")

// PlaylistItem is one entry of a playlist, in playlist order.
type PlaylistItem struct {
	ItemID  string
	VideoID string
	Title   string
	// VideoPublishedAt is when the video itself was published (RFC 3339); empty for private or deleted videos.
	VideoPublishedAt string
}

// PlaylistMove places an item at a zero-based position.
type PlaylistMove struct {
	Item     PlaylistItem
	Position int
}

// TidyPlan lists the writes that leave a playlist with each video once, newest first.
type TidyPlan struct {
	Duplicates []PlaylistItem
	Moves      []PlaylistMove
	Final      []PlaylistItem
}

// TidyResult reports what a tidy did, or would do on a dry run.
type TidyResult struct {
	Items      int
	Duplicates []PlaylistItem
	Moves      []PlaylistMove
	Final      []PlaylistItem
}

type playlistItemsDetailPage struct {
	NextPageToken string `json:"nextPageToken"`
	Items         []struct {
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
	} `json:"items"`
}

// ListPlaylistItems returns every item of a playlist in playlist order.
func ListPlaylistItems(ctx context.Context, client *http.Client, playlistID string) ([]PlaylistItem, error) {
	playlistID = strings.TrimSpace(playlistID)
	if playlistID == "" {
		return nil, fmt.Errorf("playlist ID cannot be empty")
	}
	var items []PlaylistItem
	token := ""
	for range maxPlaylistPages {
		page, err := fetchPlaylistItemsDetailPage(ctx, client, playlistID, token)
		if err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			items = append(items, PlaylistItem{
				ItemID:           it.ID,
				VideoID:          it.Snippet.ResourceID.VideoID,
				Title:            it.Snippet.Title,
				VideoPublishedAt: it.ContentDetails.VideoPublishedAt,
			})
		}
		token = page.NextPageToken
		if token == "" {
			return items, nil
		}
	}
	return nil, fmt.Errorf("playlist %s has more than %d items", playlistID, maxPlaylistPages*50)
}

func fetchPlaylistItemsDetailPage(ctx context.Context, client *http.Client, playlistID, pageToken string) (*playlistItemsDetailPage, error) {
	q := url.Values{}
	q.Set("playlistId", playlistID)
	q.Set("maxResults", "50")
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, playlistItemsDetailEndpoint+"&"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating playlist items request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching playlist items: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("playlist items request failed (status %d): %s", resp.StatusCode, string(body))
	}
	var page playlistItemsDetailPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decoding playlist items: %w", err)
	}
	return &page, nil
}

// SplitDuplicates keeps the first item of each video and returns the kept items and the later copies.
func SplitDuplicates(items []PlaylistItem) (kept, dups []PlaylistItem) {
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		if it.VideoID != "" && seen[it.VideoID] {
			dups = append(dups, it)
			continue
		}
		seen[it.VideoID] = true
		kept = append(kept, it)
	}
	return kept, dups
}

// NewestFirst returns items ordered by video publish day (UTC), newest first.
// Videos published on the same day keep their current order, so a day of numbered talks
// stays in sequence; items without a date (private or deleted videos) go last.
func NewestFirst(items []PlaylistItem) []PlaylistItem {
	sorted := slices.Clone(items)
	slices.SortStableFunc(sorted, func(a, b PlaylistItem) int {
		ta, okA := publishDay(a)
		tb, okB := publishDay(b)
		switch {
		case okA && okB:
			return strings.Compare(tb, ta)
		case okA:
			return -1
		case okB:
			return 1
		}
		return 0
	})
	return sorted
}

// publishDay is the date the video was published, as YYYY-MM-DD in UTC.
func publishDay(it PlaylistItem) (string, bool) {
	t, err := time.Parse(time.RFC3339, it.VideoPublishedAt)
	if err != nil {
		return "", false
	}
	return t.UTC().Format(time.DateOnly), true
}

// PlanMoves returns the fewest moves that turn current into target, which holds the same items.
// Items on a longest run already in target order stay put; each other item is placed,
// in target order, right after its target predecessor.
func PlanMoves(current, target []PlaylistItem) []PlaylistMove {
	pos := make(map[string]int, len(current))
	for i, it := range current {
		pos[it.ItemID] = i
	}
	seq := make([]int, len(target))
	for i, it := range target {
		seq[i] = pos[it.ItemID]
	}
	stay := longestIncreasing(seq)

	order := slices.Clone(current)
	var moves []PlaylistMove
	for i, it := range target {
		if stay[i] {
			continue
		}
		order = slices.DeleteFunc(order, func(o PlaylistItem) bool { return o.ItemID == it.ItemID })
		at := 0
		if i > 0 {
			at = slices.IndexFunc(order, func(o PlaylistItem) bool { return o.ItemID == target[i-1].ItemID }) + 1
		}
		order = slices.Insert(order, at, it)
		moves = append(moves, PlaylistMove{Item: it, Position: at})
	}
	return moves
}

// longestIncreasing marks the indices of one longest strictly increasing subsequence of seq.
func longestIncreasing(seq []int) []bool {
	tails := []int{} // tails[k] is the index ending the best run of length k+1
	prev := make([]int, len(seq))
	for i, v := range seq {
		k, _ := slices.BinarySearchFunc(tails, v, func(j, target int) int { return seq[j] - target })
		prev[i] = -1
		if k > 0 {
			prev[i] = tails[k-1]
		}
		if k == len(tails) {
			tails = append(tails, i)
		} else {
			tails[k] = i
		}
	}
	in := make([]bool, len(seq))
	if len(tails) == 0 {
		return in
	}
	for i := tails[len(tails)-1]; i >= 0; i = prev[i] {
		in[i] = true
	}
	return in
}

// PlanTidy plans the deletes and moves that leave each video once, newest first.
func PlanTidy(items []PlaylistItem) TidyPlan {
	kept, dups := SplitDuplicates(items)
	final := NewestFirst(kept)
	return TidyPlan{Duplicates: dups, Moves: PlanMoves(kept, final), Final: final}
}

// TidyPlaylist removes repeated videos from a playlist and orders it newest first on YouTube.
// With dryRun it only reports the plan. report, if not nil, receives progress lines.
func TidyPlaylist(ctx context.Context, client *http.Client, playlistID string, dryRun bool, report func(string)) (TidyResult, error) {
	if !dryRun {
		if roErr := readonly.Check("TidyPlaylist"); roErr != nil {
			return TidyResult{}, roErr
		}
	}
	if report == nil {
		report = func(string) {}
	}
	items, err := ListPlaylistItems(ctx, client, playlistID)
	if err != nil {
		return TidyResult{}, err
	}
	plan := PlanTidy(items)
	res := TidyResult{Items: len(items), Duplicates: plan.Duplicates, Moves: plan.Moves, Final: plan.Final}
	if dryRun || (len(plan.Duplicates) == 0 && len(plan.Moves) == 0) {
		return res, nil
	}
	for _, d := range plan.Duplicates {
		report(fmt.Sprintf("removing duplicate %q", d.Title))
		if err := deletePlaylistItem(ctx, client, d.ItemID); err != nil {
			return res, err
		}
	}
	if len(plan.Duplicates) > 0 {
		// Positions are absolute, so plan the moves against what YouTube now lists.
		kept, err := awaitDeduplicated(ctx, client, playlistID)
		if err != nil {
			return res, err
		}
		res.Final = NewestFirst(kept)
		res.Moves = PlanMoves(kept, res.Final)
	}
	if err := applyMoves(ctx, client, playlistID, res.Moves, report); err != nil {
		if len(res.Duplicates) > 0 {
			err = fmt.Errorf("%w (the %d repeated video(s) were already removed)", err, len(res.Duplicates))
		}
		return res, err
	}
	return res, nil
}

// awaitDeduplicated re-reads the playlist until the deleted copies no longer show.
func awaitDeduplicated(ctx context.Context, client *http.Client, playlistID string) ([]PlaylistItem, error) {
	for attempt := 0; ; attempt++ {
		items, err := ListPlaylistItems(ctx, client, playlistID)
		if err != nil {
			return nil, err
		}
		kept, dups := SplitDuplicates(items)
		if len(dups) == 0 {
			return kept, nil
		}
		if attempt == propagationRetries {
			return nil, fmt.Errorf("YouTube still lists %d duplicate(s) after deleting them; run again", len(dups))
		}
		if err := ctxsleep.Sleep(ctx, propagationRetryDelay); err != nil {
			return nil, err
		}
	}
}

func applyMoves(ctx context.Context, client *http.Client, playlistID string, moves []PlaylistMove, report func(string)) error {
	for _, m := range moves {
		report(fmt.Sprintf("moving %q to position %d", m.Item.Title, m.Position+1))
		if err := movePlaylistItem(ctx, client, playlistID, m); err != nil {
			return err
		}
	}
	return nil
}

// movePlaylistItem sets one item's position with playlistItems.update.
func movePlaylistItem(ctx context.Context, client *http.Client, playlistID string, m PlaylistMove) error {
	if roErr := readonly.Check("movePlaylistItem"); roErr != nil {
		return roErr
	}
	payload := map[string]any{
		"id": m.Item.ItemID,
		"snippet": map[string]any{
			"playlistId": playlistID,
			"position":   m.Position,
			"resourceId": map[string]any{"kind": "youtube#video", "videoId": m.Item.VideoID},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling playlist item update: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, playlistItemsUpdateEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating playlist item update request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("moving playlist item: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(respBody), "manualSortRequired") {
		return ErrManualSortRequired
	}
	return fmt.Errorf("moving %q failed (status %d): %s", m.Item.Title, resp.StatusCode, string(respBody))
}
