// Package bundle detects and organizes media and transcript files in a Zoom recording directory.
package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"talk_cut/internal/eml"
)

// RecordingBundle holds paths to all detected recording artifacts in a directory.
type RecordingBundle struct {
	Dir            string   `json:"dir"`
	TranscriptPath string   `json:"transcript_path"`
	PrimaryVideo   string   `json:"primary_video"`
	AllVideos      []string `json:"all_videos"`
	AudioPath      string   `json:"audio_path,omitempty"`
	ChatPath       string   `json:"chat_path,omitempty"`
	EmlPath        string   `json:"eml_path,omitempty"`
}

// DiscoverBundle inspects the directory and resolves recording files.
// preferredLayout can be "slides" (default), "clean", "speaker", or "gallery".
func DiscoverBundle(dir string, preferredLayout string) (*RecordingBundle, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("reading recording directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("listing directory %q: %w", dir, err)
	}

	b := &RecordingBundle{
		Dir: dir,
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		fullPath := filepath.Join(dir, name)
		classifyFile(b, name, fullPath)
	}

	if b.TranscriptPath == "" {
		return nil, fmt.Errorf("no WebVTT transcript (.vtt) found in %q", dir)
	}

	if len(b.AllVideos) == 0 {
		return nil, fmt.Errorf("no video files (.mp4/.mkv) found in %q", dir)
	}

	emlPath, err := eml.FindUniqueEML(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving announcement email in %q: %w", dir, err)
	}
	b.EmlPath = emlPath

	b.PrimaryVideo = selectPrimaryVideo(b.AllVideos, preferredLayout)
	return b, nil
}

// classifyFile categorizes an individual file into bundle slots.
func classifyFile(b *RecordingBundle, name, fullPath string) {
	lower := strings.ToLower(name)

	// Ignore output artifacts from previous cutting runs
	if strings.HasSuffix(lower, "_cut.mp4") || strings.HasSuffix(lower, "_cut.mkv") || strings.HasSuffix(lower, "_cut.vtt") {
		return
	}

	if strings.HasSuffix(lower, ".vtt") {
		// Prefer files containing ".transcript.vtt" if multiple exist
		if b.TranscriptPath == "" || strings.Contains(lower, "transcript") {
			b.TranscriptPath = fullPath
		}
		return
	}

	if strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".mkv") || strings.HasSuffix(lower, ".webm") {
		b.AllVideos = append(b.AllVideos, fullPath)
		return
	}

	if strings.HasSuffix(lower, ".m4a") || strings.HasSuffix(lower, ".aac") || strings.HasSuffix(lower, ".mp3") {
		b.AudioPath = fullPath
		return
	}

	if strings.HasSuffix(lower, "chat.txt") {
		b.ChatPath = fullPath
	}
}

// selectPrimaryVideo chooses the most appropriate video according to preferred layout.
func selectPrimaryVideo(videos []string, layout string) string {
	if len(videos) == 1 {
		return videos[0]
	}

	sort.Strings(videos)

	switch strings.ToLower(layout) {
	case "clean":
		if match := findMatchingVideo(videos, "_as_"); match != "" {
			return match
		}
	case "speaker":
		if match := findMatchingVideo(videos, "_avo_"); match != "" {
			return match
		}
	case "gallery":
		if match := findMatchingVideo(videos, "_gvo_"); match != "" {
			return match
		}
	default: // "slides" (default)
		// Prioritize standard presentation view (without _as_, _avo_, _gvo_)
		for _, v := range videos {
			base := filepath.Base(v)
			if !strings.Contains(base, "_as_") && !strings.Contains(base, "_avo_") && !strings.Contains(base, "_gvo_") {
				return v
			}
		}
	}

	// Fallback to first video
	return videos[0]
}

// findMatchingVideo returns the first path whose filename contains the given substring.
func findMatchingVideo(videos []string, substr string) string {
	for _, v := range videos {
		if strings.Contains(filepath.Base(v), substr) {
			return v
		}
	}
	return ""
}
