// Package cutter implements video probing, cut calculation, and FFmpeg execution.
package cutter

import (
	"sort"
	"time"

	"talk_cut/internal/model"
)

// AdjustTime maps an original media timestamp to its post-cut timeline position.
// If the timestamp falls inside a cut interval, it snaps to the start of that excision.
func AdjustTime(t time.Duration, cuts []model.CutInterval) time.Duration {
	if t <= 0 {
		return 0
	}

	var elapsedKept time.Duration
	var lastKeptEnd time.Duration

	// Filter and sort active cuts chronologically
	activeCuts := filterAndSortCuts(cuts)

	for _, cut := range activeCuts {
		if cut.End <= cut.Start {
			continue
		}

		if t < cut.Start {
			// Target timestamp occurs before this cut starts
			elapsedKept += t - lastKeptEnd
			return elapsedKept
		}

		// Add the kept segment between lastKeptEnd and cut.Start
		if cut.Start > lastKeptEnd {
			elapsedKept += cut.Start - lastKeptEnd
		}

		if t <= cut.End {
			// Timestamp falls inside this cut segment -> snap to excision point
			return elapsedKept
		}

		lastKeptEnd = cut.End
	}

	// Remaining time after all cuts
	if t > lastKeptEnd {
		elapsedKept += t - lastKeptEnd
	}

	return elapsedKept
}

// AdjustChapters recalculates chapter marker timings based on excised intervals.
// It guarantees that chapter 1 starts at 00:00 (required by YouTube).
func AdjustChapters(original []model.ChapterMarker, cuts []model.CutInterval, defaultFirstTitle string) []model.ChapterMarker {
	if len(original) == 0 {
		return nil
	}

	var adjusted []model.ChapterMarker
	seenTimes := make(map[time.Duration]bool)

	for _, ch := range original {
		adjTime := AdjustTime(ch.OriginalTime, cuts)

		// Round to nearest second for clean YouTube chapter formatting
		adjTime = adjTime.Round(time.Second)

		// Avoid duplicate chapter markers at the same second
		if seenTimes[adjTime] {
			continue
		}
		seenTimes[adjTime] = true

		adjusted = append(adjusted, model.ChapterMarker{
			OriginalTime: ch.OriginalTime,
			AdjustedTime: adjTime,
			Title:        ch.Title,
		})
	}

	if len(adjusted) == 0 {
		return nil
	}

	// Ensure the first chapter starts strictly at 00:00
	if adjusted[0].AdjustedTime > 0 {
		title := "Introduction"
		if defaultFirstTitle != "" {
			title = defaultFirstTitle
		}
		adjusted = append([]model.ChapterMarker{{
			OriginalTime: 0,
			AdjustedTime: 0,
			Title:        title,
		}}, adjusted...)
	}

	return adjusted
}

// filterAndSortCuts extracts only ActionCut intervals and sorts them by Start time.
func filterAndSortCuts(cuts []model.CutInterval) []model.CutInterval {
	var filtered []model.CutInterval
	for _, c := range cuts {
		if c.Action == model.ActionCut && c.End > c.Start {
			filtered = append(filtered, c)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Start < filtered[j].Start
	})

	return filtered
}
