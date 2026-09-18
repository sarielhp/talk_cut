// Package model defines core domain data types for talk_cut.
package model

import (
	"fmt"
	"strings"
	"time"
)

// ChapterMarker represents a video chapter checkpoint.
type ChapterMarker struct {
	OriginalTime time.Duration `json:"original_time"`
	AdjustedTime time.Duration `json:"adjusted_time"`
	Title        string        `json:"title"`
}

// FormatYouTubeLine returns the chapter line in YouTube format: "MM:SS Title" or "H:MM:SS Title".
func (c ChapterMarker) FormatYouTubeLine() string {
	totalSeconds := int(c.AdjustedTime.Seconds())
	if totalSeconds < 0 {
		totalSeconds = 0
	}

	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d %s", hours, minutes, seconds, c.Title)
	}

	return fmt.Sprintf("%02d:%02d %s", minutes, seconds, c.Title)
}

// PlaylistRef represents an association to a YouTube playlist.
type PlaylistRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// TalkMetadata holds all descriptive metadata for the talk and YouTube upload.
type TalkMetadata struct {
	Title         string          `json:"title"`
	Speaker       string          `json:"speaker"`
	Affiliation   string          `json:"affiliation,omitempty"`
	Abstract      string          `json:"abstract"`
	URL           string          `json:"url,omitempty"`
	Tags          []string        `json:"tags,omitempty"`
	Privacy       string          `json:"privacy"` // "unlisted", "public", "private"
	Chapters      []ChapterMarker `json:"chapters,omitempty"`
	YouTubeID     string          `json:"youtube_id,omitempty"`
	YouTubeURL    string          `json:"youtube_url,omitempty"`
	PlaylistID    string          `json:"playlist_id,omitempty"`
	PlaylistTitle string          `json:"playlist_title,omitempty"`
	Playlists     []PlaylistRef   `json:"playlists,omitempty"`
}

// BuildYouTubeDescription generates the complete YouTube video description with abstract and chapters.
func (m TalkMetadata) BuildYouTubeDescription() string {
	var sb strings.Builder

	if m.Speaker != "" {
		sb.WriteString("Speaker: ")
		sb.WriteString(m.Speaker)
		if m.Affiliation != "" {
			sb.WriteString(" (")
			sb.WriteString(m.Affiliation)
			sb.WriteString(")")
		}
		sb.WriteString("\n\n")
	}

	if strings.TrimSpace(m.Abstract) != "" {
		sb.WriteString("Abstract:\n")
		sb.WriteString(strings.TrimSpace(m.Abstract))
		sb.WriteString("\n\n")
	}

	if m.URL != "" && !strings.Contains(m.Abstract, m.URL) {
		sb.WriteString("Talk announcement: ")
		sb.WriteString(m.URL)
		sb.WriteString("\n\n")
	}

	if len(m.Chapters) > 0 {
		sb.WriteString("Chapters:\n")
		for _, ch := range m.Chapters {
			sb.WriteString(ch.FormatYouTubeLine())
			sb.WriteString("\n")
		}
	}

	return strings.TrimSpace(sb.String())
}

// YouTubeWatchURL returns the canonical youtube.com watch URL or short URL.
func (m TalkMetadata) YouTubeWatchURL() string {
	if m.YouTubeURL != "" {
		return m.YouTubeURL
	}
	if m.YouTubeID != "" {
		return fmt.Sprintf("https://youtu.be/%s", m.YouTubeID)
	}
	return ""
}

// EffectiveYouTubeID returns the video ID from YouTubeID or parsed from YouTubeURL.
func (m TalkMetadata) EffectiveYouTubeID() string {
	if m.YouTubeID != "" {
		return m.YouTubeID
	}
	return ExtractYouTubeID(m.YouTubeURL)
}

// ExtractYouTubeID parses an 11-character YouTube video ID from various URL formats or raw ID.
func ExtractYouTubeID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) == 11 && !strings.ContainsAny(raw, "/?=&") {
		return raw
	}
	if strings.Contains(raw, "youtu.be/") {
		parts := strings.Split(raw, "youtu.be/")
		if len(parts) > 1 {
			id := parts[1]
			if idx := strings.IndexAny(id, "?&"); idx != -1 {
				id = id[:idx]
			}
			return strings.TrimSpace(id)
		}
	}
	if strings.Contains(raw, "v=") {
		parts := strings.Split(raw, "v=")
		if len(parts) > 1 {
			id := parts[1]
			if idx := strings.IndexAny(id, "?&#"); idx != -1 {
				id = id[:idx]
			}
			return strings.TrimSpace(id)
		}
	}
	return ""
}

// AllPlaylists returns all associated playlists, synthesizing from legacy fields if needed.
func (m TalkMetadata) AllPlaylists() []PlaylistRef {
	if len(m.Playlists) > 0 {
		return m.Playlists
	}
	if m.PlaylistID != "" {
		return []PlaylistRef{{ID: m.PlaylistID, Title: m.PlaylistTitle}}
	}
	return nil
}

// HasPlaylist returns true if the given playlist ID or title is associated.
func (m TalkMetadata) HasPlaylist(idOrTitle string) bool {
	idOrTitle = strings.TrimSpace(idOrTitle)
	if idOrTitle == "" {
		return false
	}
	for _, pl := range m.AllPlaylists() {
		if pl.ID == idOrTitle || strings.EqualFold(pl.Title, idOrTitle) {
			return true
		}
	}
	return false
}

// AddPlaylist adds an association to a playlist if not already present.
func (m *TalkMetadata) AddPlaylist(id, title string) {
	id = strings.TrimSpace(id)
	title = strings.TrimSpace(title)
	if id == "" {
		return
	}
	if title == "" {
		title = id
	}

	if len(m.Playlists) == 0 && m.PlaylistID != "" {
		m.Playlists = []PlaylistRef{{ID: m.PlaylistID, Title: m.PlaylistTitle}}
	}

	for i, pl := range m.Playlists {
		if pl.ID == id {
			if title != "" && m.Playlists[i].Title != title {
				m.Playlists[i].Title = title
			}
			m.syncLegacyPlaylist()
			return
		}
	}

	m.Playlists = append(m.Playlists, PlaylistRef{ID: id, Title: title})
	m.syncLegacyPlaylist()
}

// RemovePlaylist removes an association to a playlist.
func (m *TalkMetadata) RemovePlaylist(idOrTitle string) {
	idOrTitle = strings.TrimSpace(idOrTitle)
	if idOrTitle == "" {
		return
	}

	if len(m.Playlists) == 0 && m.PlaylistID != "" {
		m.Playlists = []PlaylistRef{{ID: m.PlaylistID, Title: m.PlaylistTitle}}
	}

	var filtered []PlaylistRef
	for _, pl := range m.Playlists {
		if pl.ID != idOrTitle && !strings.EqualFold(pl.Title, idOrTitle) {
			filtered = append(filtered, pl)
		}
	}
	m.Playlists = filtered
	m.syncLegacyPlaylist()
}

func (m *TalkMetadata) syncLegacyPlaylist() {
	if len(m.Playlists) > 0 {
		m.PlaylistID = m.Playlists[0].ID
		m.PlaylistTitle = m.Playlists[0].Title
	} else {
		m.PlaylistID = ""
		m.PlaylistTitle = ""
	}
}
