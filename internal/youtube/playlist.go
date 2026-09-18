// Package youtube implements YouTube OAuth2 authorization, resumable video upload, and playlist synchronization.
package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// AddVideoToPlaylist inserts a video into a YouTube playlist.
func AddVideoToPlaylist(ctx context.Context, client *http.Client, playlistID, videoID string) error {
	playlistID = strings.TrimSpace(playlistID)
	videoID = strings.TrimSpace(videoID)
	if playlistID == "" {
		return fmt.Errorf("playlist ID cannot be empty")
	}
	if videoID == "" {
		return fmt.Errorf("video ID cannot be empty")
	}

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
		return fmt.Errorf("marshaling playlist item payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, playlistItemsInsertEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("creating playlist item request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("adding video to playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		if strings.Contains(bodyStr, "videoAlreadyInPlaylist") {
			return nil // Already in playlist, treat as success
		}
		return fmt.Errorf("adding video to playlist failed (status %d): %s", resp.StatusCode, bodyStr)
	}

	return nil
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
func RemoveVideoFromPlaylist(ctx context.Context, client *http.Client, playlistID, videoID string) error {
	playlistID = strings.TrimSpace(playlistID)
	videoID = strings.TrimSpace(videoID)
	if playlistID == "" {
		return fmt.Errorf("playlist ID cannot be empty")
	}
	if videoID == "" {
		return fmt.Errorf("video ID cannot be empty")
	}

	url := fmt.Sprintf("%s?part=id&playlistId=%s&videoId=%s&maxResults=50", playlistItemsRemoveEndpoint, playlistID, videoID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating playlistItems list request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("querying playlist item: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var data struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil || len(data.Items) == 0 {
		return nil
	}

	for _, item := range data.Items {
		delURL := fmt.Sprintf("%s?id=%s", playlistItemsRemoveEndpoint, item.ID)
		delReq, delErr := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil)
		if delErr != nil {
			continue
		}
		delResp, doErr := client.Do(delReq)
		if doErr == nil {
			delResp.Body.Close()
		}
	}

	return nil
}
