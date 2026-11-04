package cutter

import (
	"fmt"
	"os"

	"talk_cut/internal/model"
	"talk_cut/internal/vtt"
)

// WriteChaptersFile writes YouTube chapter lines to path.
func WriteChaptersFile(path string, chapters []model.ChapterMarker) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	for _, ch := range chapters {
		fmt.Fprintln(f, ch.FormatYouTubeLine())
	}
	return f.Close()
}

// WriteAdjustedVTT writes the kept cues, retimed onto the post-cut timeline, as WebVTT.
func WriteAdjustedVTT(path string, cues []model.SubtitleCue, cuts []model.CutInterval) error {
	var kept []model.SubtitleCue
	for _, c := range cues {
		if c.Action == model.ActionCut {
			continue
		}
		adjCue := c
		adjCue.Start = AdjustTime(c.Start, cuts)
		adjCue.End = AdjustTime(c.End, cuts)
		kept = append(kept, adjCue)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err = vtt.Write(f, kept); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
