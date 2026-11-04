// Package youtube implements YouTube OAuth2 authorization, resumable video upload, and playlist synchronization.
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
	"strings"
	"talk_cut/internal/ctxsleep"
	"time"
)

var (
	playlistsListEndpoint       = "https://www.googleapis.com/youtube/v3/playlists?part=snippet,contentDetails,status&mine=true&maxResults=50"
	playlistsCreateEndpoint     = "https://www.googleapis.com/youtube/v3/playlists?part=snippet,status"
	playlistItemsInsertEndpoint = "https://www.googleapis.com/youtube/v3/playlistItems?part=snippet"
	playlistItemsRemoveEndpoint = "https://www.googleapis.com/youtube/v3/playlistItems"
)

// Playlist represents a YouTube playlist owned by the authenticated channel.
type Playlist struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description,omitempty"`
	ItemCount     int64  `json:"item_count"`
	PrivacyStatus string `json:"privacy_status"`
}

// ListPlaylists queries YouTube Data API for all playlists owned by the authenticated user.
func ListPlaylists(ctx context.Context, client *http.Client) ([]Playlist, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, playlistsListEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating playlist list request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching playlists: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("playlist list failed (status %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"snippet"`
			ContentDetails struct {
				ItemCount int64 `json:"itemCount"`
			} `json:"contentDetails"`
			Status struct {
				PrivacyStatus string `json:"privacyStatus"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding playlists: %w", err)
	}

	var playlists []Playlist
	for _, item := range data.Items {
		playlists = append(playlists, Playlist{
			ID:            item.ID,
			Title:         item.Snippet.Title,
			Description:   item.Snippet.Description,
			ItemCount:     item.ContentDetails.ItemCount,
			PrivacyStatus: item.Status.PrivacyStatus,
		})
	}

	return playlists, nil
}

// CreatePlaylist creates a new YouTube playlist for the authenticated user.
func CreatePlaylist(ctx context.Context, client *http.Client, title, description, privacy string) (*Playlist, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("playlist title cannot be empty")
	}

	privacy = strings.ToLower(strings.TrimSpace(privacy))
	if privacy != "unlisted" && privacy != "private" {
		privacy = "public"
	}

	payload := map[string]any{
		"snippet": map[string]any{
			"title":       sanitizeYouTubeText(title),
			"description": sanitizeYouTubeText(description),
		},
		"status": map[string]any{
			"privacyStatus": privacy,
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling playlist payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, playlistsCreateEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating playlist request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("creating playlist on YouTube: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("playlist creation failed (status %d): %s", resp.StatusCode, string(body))
	}

	var created struct {
		ID      string `json:"id"`
		Snippet struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"snippet"`
		Status struct {
			PrivacyStatus string `json:"privacyStatus"`
		} `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("decoding created playlist: %w", err)
	}

	return &Playlist{
		ID:            created.ID,
		Title:         created.Snippet.Title,
		Description:   created.Snippet.Description,
		PrivacyStatus: created.Status.PrivacyStatus,
		ItemCount:     0,
	}, nil
}

// Right after a write, YouTube briefly rejects or hides it: a new playlist answers inserts with
// 409 ABORTED or 404 playlistNotFound for ~2s, and lookups miss a just-added item for ~1s.
var propagationRetryDelay = time.Second

const propagationRetries = 4

// errPlaylistNotFound marks a lookup that YouTube rejected as playlistNotFound, which a just-created playlist briefly is.
var errPlaylistNotFound = errors.New("playlist not found")

// AddVideoToPlaylist inserts a video into a YouTube playlist, retrying while a new playlist propagates.
func AddVideoToPlaylist(ctx context.Context, client *http.Client, playlistID, videoID string) error {
	playlistID = strings.TrimSpace(playlistID)
	videoID = strings.TrimSpace(videoID)
	if playlistID == "" {
		return fmt.Errorf("playlist ID cannot be empty")
	}
	if videoID == "" {
		return fmt.Errorf("video ID cannot be empty")
	}

	for attempt := 0; ; attempt++ {
		retryable, err := insertPlaylistItem(ctx, client, playlistID, videoID)
		if err == nil || !retryable || attempt == propagationRetries {
			return err
		}
		if err := ctxsleep.Sleep(ctx, propagationRetryDelay); err != nil {
			return err
		}
	}
}

// insertPlaylistItem makes one insert call and reports whether a failure is worth retrying.
func insertPlaylistItem(ctx context.Context, client *http.Client, playlistID, videoID string) (bool, error) {
	payload := map[string]any{
		"snippet": map[string]any{
			"playlistId": playlistID,
			"resourceId": map[string]any{
				"kind":    "youtube#video",
				"videoId": videoID,
			},
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("marshaling playlist item payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, playlistItemsInsertEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return false, fmt.Errorf("creating playlist item request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("adding video to playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return false, nil
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if strings.Contains(bodyStr, "videoAlreadyInPlaylist") {
		return false, nil // Already in playlist, treat as success
	}
	retryable := resp.StatusCode == http.StatusConflict ||
		(resp.StatusCode == http.StatusNotFound && strings.Contains(bodyStr, "playlistNotFound"))
	return retryable, fmt.Errorf("adding video to playlist failed (status %d): %s", resp.StatusCode, bodyStr)
}

// FindPlaylist searches channel playlists by ID, exact title, or title substring.
func FindPlaylist(ctx context.Context, client *http.Client, idOrTitle string) (*Playlist, error) {
	idOrTitle = strings.TrimSpace(idOrTitle)
	if idOrTitle == "" {
		return nil, fmt.Errorf("playlist identifier cannot be empty")
	}

	playlists, err := ListPlaylists(ctx, client)
	if err != nil {
		return nil, err
	}

	for i := range playlists {
		if playlists[i].ID == idOrTitle {
			return &playlists[i], nil
		}
	}

	for i := range playlists {
		if strings.EqualFold(playlists[i].Title, idOrTitle) {
			return &playlists[i], nil
		}
	}

	target := strings.ToLower(idOrTitle)
	for i := range playlists {
		if strings.Contains(strings.ToLower(playlists[i].Title), target) {
			return &playlists[i], nil
		}
	}

	return nil, fmt.Errorf("playlist %q not found on channel", idOrTitle)
}

// RemoveVideoFromPlaylist removes all occurrences of a video from a YouTube playlist.
// A video that is not in the playlist is not an error; any failed lookup or deletion is.
// An empty or playlistNotFound lookup is retried briefly, since YouTube lags just-made writes.
func RemoveVideoFromPlaylist(ctx context.Context, client *http.Client, playlistID, videoID string) error {
	playlistID = strings.TrimSpace(playlistID)
	videoID = strings.TrimSpace(videoID)
	if playlistID == "" {
		return fmt.Errorf("playlist ID cannot be empty")
	}
	if videoID == "" {
		return fmt.Errorf("video ID cannot be empty")
	}

	var itemIDs []string
	for attempt := 0; ; attempt++ {
		ids, err := findPlaylistItemIDs(ctx, client, playlistID, videoID)
		if err != nil && (!errors.Is(err, errPlaylistNotFound) || attempt == propagationRetries) {
			return err
		}
		itemIDs = ids
		if err == nil && (len(itemIDs) > 0 || attempt == propagationRetries) {
			break
		}
		if err := ctxsleep.Sleep(ctx, propagationRetryDelay); err != nil {
			return err
		}
	}
	for _, itemID := range itemIDs {
		if err := deletePlaylistItem(ctx, client, itemID); err != nil {
			return err
		}
	}
	return nil
}

// findPlaylistItemIDs returns the playlist item IDs under which a video appears in a playlist.
func findPlaylistItemIDs(ctx context.Context, client *http.Client, playlistID, videoID string) ([]string, error) {
	q := url.Values{}
	q.Set("part", "id")
	q.Set("playlistId", playlistID)
	q.Set("videoId", videoID)
	q.Set("maxResults", "50")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, playlistItemsRemoveEndpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating playlistItems list request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying playlist item: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("looking up video in playlist failed (status %d): %s", resp.StatusCode, string(body))
		if resp.StatusCode == http.StatusNotFound && strings.Contains(string(body), "playlistNotFound") {
			err = fmt.Errorf("%w: %w", errPlaylistNotFound, err)
		}
		return nil, err
	}

	var data struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding playlist items: %w", err)
	}
	ids := make([]string, 0, len(data.Items))
	for _, item := range data.Items {
		ids = append(ids, item.ID)
	}
	return ids, nil
}

// deletePlaylistItem deletes one playlist item; an item that is already gone counts as deleted.
func deletePlaylistItem(ctx context.Context, client *http.Client, itemID string) error {
	delURL := playlistItemsRemoveEndpoint + "?id=" + url.QueryEscape(itemID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil)
	if err != nil {
		return fmt.Errorf("creating playlist item delete request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("deleting playlist item: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("removing video from playlist failed (status %d): %s", resp.StatusCode, string(body))
}
