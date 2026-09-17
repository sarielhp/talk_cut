// Package model defines core domain data types for talk_cut.
package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// MetaFileName is the standard filename for persisted talk metadata in a recording directory.
const MetaFileName = "talk_meta.json"

// SavedMetaFile represents the JSON structure persisted to talk_meta.json.
type SavedMetaFile struct {
	UpdatedAt   string          `json:"updated_at"`
	URL         string          `json:"url,omitempty"`
	Title       string          `json:"title"`
	Speaker     string          `json:"speaker"`
	Affiliation string          `json:"affiliation,omitempty"`
	Abstract    string          `json:"abstract"`
	Tags        []string        `json:"tags,omitempty"`
	Privacy     string          `json:"privacy"`
	Chapters    []ChapterMarker `json:"chapters,omitempty"`
}

// SaveMetaFile writes the talk metadata to <dir>/talk_meta.json.
func SaveMetaFile(dir string, meta TalkMetadata) error {
	path := filepath.Join(dir, MetaFileName)
	smf := SavedMetaFile{
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		URL:         meta.URL,
		Title:       meta.Title,
		Speaker:     meta.Speaker,
		Affiliation: meta.Affiliation,
		Abstract:    meta.Abstract,
		Tags:        meta.Tags,
		Privacy:     meta.Privacy,
		Chapters:    meta.Chapters,
	}

	data, err := json.MarshalIndent(smf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing metadata file %q: %w", path, err)
	}

	return nil
}

// HasSavedMetadata checks if <dir>/talk_meta.json exists.
func HasSavedMetadata(dir string) bool {
	path := filepath.Join(dir, MetaFileName)
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// LoadMetaFile loads persisted talk metadata from <dir>/talk_meta.json.
func LoadMetaFile(dir string) (TalkMetadata, error) {
	path := filepath.Join(dir, MetaFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return TalkMetadata{}, fmt.Errorf("reading metadata file %q: %w", path, err)
	}

	var smf SavedMetaFile
	if err := json.Unmarshal(data, &smf); err != nil {
		return TalkMetadata{}, fmt.Errorf("unmarshaling metadata file %q: %w", path, err)
	}

	privacy := smf.Privacy
	if privacy == "" {
		privacy = "unlisted"
	}

	return TalkMetadata{
		Title:       smf.Title,
		Speaker:     smf.Speaker,
		Affiliation: smf.Affiliation,
		Abstract:    smf.Abstract,
		URL:         smf.URL,
		Tags:        smf.Tags,
		Privacy:     privacy,
		Chapters:    smf.Chapters,
	}, nil
}
