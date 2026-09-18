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

// FormatYouTubeLine returns the chapter line in YouTube format: "MM:SS Title".
func (c ChapterMarker) FormatYouTubeLine() string {
	totalSeconds := int(c.AdjustedTime.Seconds())
	if totalSeconds < 0 {
		totalSeconds = 0
	}

	minutes := totalSeconds / 60
	seconds := totalSeconds % 60

	return fmt.Sprintf("%02d:%02d %s", minutes, seconds, c.Title)
}

// TalkMetadata holds all descriptive metadata for the talk and YouTube upload.
type TalkMetadata struct {
	Title       string          `json:"title"`
	Speaker     string          `json:"speaker"`
	Affiliation string          `json:"affiliation,omitempty"`
	Abstract    string          `json:"abstract"`
	URL         string          `json:"url,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Privacy     string          `json:"privacy"` // "unlisted", "public", "private"
	Chapters    []ChapterMarker `json:"chapters,omitempty"`
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
