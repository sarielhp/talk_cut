package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// maxPlaylistPages covers YouTube's 5000-item playlist limit at 50 items per page.
const maxPlaylistPages = 100

// ChannelVideo is one video from a playlist listing, such as the channel's uploads.
type ChannelVideo struct {
	ID          string
	Title       string
	PublishedAt string
}

type playlistItemsPage struct {
	NextPageToken string `json:"nextPageToken"`
	Items         []struct {
		Snippet struct {
			Title       string `json:"title"`
			PublishedAt string `json:"publishedAt"`
			ResourceID  struct {
				VideoID string `json:"videoId"`
			} `json:"resourceId"`
		} `json:"snippet"`
	} `json:"items"`
}

// ListRecentUploads returns up to limit of the channel's most recent uploads, newest first.
func ListRecentUploads(ctx context.Context, client *http.Client, limit int) ([]ChannelVideo, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	uploadsID, err := getUploadsPlaylistID(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("retrieving uploads playlist: %w", err)
	}
	page, err := fetchPlaylistItemsPage(ctx, client, uploadsID, "", limit)
	if err != nil {
		return nil, err
	}
	videos := make([]ChannelVideo, 0, len(page.Items))
	for _, item := range page.Items {
		if item.Snippet.ResourceID.VideoID == "" {
			continue
		}
		videos = append(videos, ChannelVideo{
			ID:          item.Snippet.ResourceID.VideoID,
			Title:       item.Snippet.Title,
			PublishedAt: item.Snippet.PublishedAt,
		})
	}
	return videos, nil
}

// ListPlaylistVideoIDs returns the set of video IDs contained in a playlist.
func ListPlaylistVideoIDs(ctx context.Context, client *http.Client, playlistID string) (map[string]bool, error) {
	playlistID = strings.TrimSpace(playlistID)
	if playlistID == "" {
		return nil, fmt.Errorf("playlist ID cannot be empty")
	}
	ids := make(map[string]bool)
	token := ""
	for range maxPlaylistPages {
		page, err := fetchPlaylistItemsPage(ctx, client, playlistID, token, 50)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if vid := item.Snippet.ResourceID.VideoID; vid != "" {
				ids[vid] = true
			}
		}
		token = page.NextPageToken
		if token == "" {
			return ids, nil
		}
	}
	return nil, fmt.Errorf("playlist %s has more than %d items; membership would be incomplete", playlistID, maxPlaylistPages*50)
}

// fetchPlaylistItemsPage reads one page of a playlist's items.
func fetchPlaylistItemsPage(ctx context.Context, client *http.Client, playlistID, pageToken string, limit int) (*playlistItemsPage, error) {
	q := url.Values{}
	q.Set("playlistId", playlistID)
	q.Set("maxResults", fmt.Sprint(limit))
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, playlistItemsEndpoint+"&"+q.Encode(), nil)
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
	var page playlistItemsPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decoding playlist items: %w", err)
	}
	return &page, nil
}
