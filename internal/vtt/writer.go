// Package vtt provides WebVTT subtitle parsing and timestamp formatting.
package vtt

import (
	"fmt"
	"io"

	"talk_cut/internal/model"
)

// Write writes a slice of SubtitleCue models as a valid WebVTT file to an io.Writer.
func Write(w io.Writer, cues []model.SubtitleCue) error {
	if _, err := io.WriteString(w, "WEBVTT\n\n"); err != nil {
		return fmt.Errorf("writing vtt header: %w", err)
	}

	for i, cue := range cues {
		cueNum := cue.ID
		if cueNum <= 0 {
			cueNum = i + 1
		}

		timeLine := fmt.Sprintf("%s --> %s\n", FormatTimestamp(cue.Start), FormatTimestamp(cue.End))
		body := cue.FullText() + "\n\n"

		if _, err := fmt.Fprintf(w, "%d\n%s%s", cueNum, timeLine, body); err != nil {
			return fmt.Errorf("writing cue #%d: %w", cueNum, err)
		}
	}

	return nil
}
