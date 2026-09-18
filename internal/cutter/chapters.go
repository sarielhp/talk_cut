// Package cutter implements video probing, cut calculation, and FFmpeg execution.
package cutter

import (
	"sort"
	"strings"
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
		// Avoid consecutive duplicate chapter titles
		if len(adjusted) > 0 && strings.EqualFold(adjusted[len(adjusted)-1].Title, ch.Title) {
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
		if strings.EqualFold(adjusted[0].Title, title) || strings.Contains(strings.ToLower(adjusted[0].Title), "intro") {
			adjusted[0].AdjustedTime = 0
		} else {
			adjusted = append([]model.ChapterMarker{{
				OriginalTime: 0,
				AdjustedTime: 0,
				Title:        title,
			}}, adjusted...)
		}
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

// maxSnapDiff is the maximum distance a chapter will shift to align with a cue start.
const maxSnapDiff = 45 * time.Second

// SnapChaptersToCues aligns each chapter marker to the start time of the closest cue that is not deleted.
func SnapChaptersToCues(chapters []model.ChapterMarker, cues []model.SubtitleCue) []model.ChapterMarker {
	if len(cues) == 0 || len(chapters) == 0 {
		return chapters
	}
	res := make([]model.ChapterMarker, len(chapters))
	copy(res, chapters)
	for i := range res {
		cueTime, ok := closestCueStart(res[i].OriginalTime, cues, maxSnapDiff)
		if ok {
			res[i].OriginalTime = cueTime
			res[i].AdjustedTime = cueTime
		}
	}
	return AlignChaptersToKeptCues(res, cues)
}

// AlignChaptersToKeptCues ensures chapter starts automatically move to the first cue that is not deleted (ActionCut).
func AlignChaptersToKeptCues(chapters []model.ChapterMarker, cues []model.SubtitleCue) []model.ChapterMarker {
	if len(cues) == 0 || len(chapters) == 0 {
		return chapters
	}

	res := make([]model.ChapterMarker, 0, len(chapters))
	seenCues := make(map[time.Duration]bool)

	for _, ch := range chapters {
		cutIdx := findCutCueIndex(ch.OriginalTime, cues)
		if cutIdx >= 0 {
			keptIdx := findNextKeptCue(cutIdx, cues)
			if keptIdx >= 0 {
				ch.OriginalTime = cues[keptIdx].Start
				ch.AdjustedTime = cues[keptIdx].Start
			}
		}

		if seenCues[ch.OriginalTime] {
			continue
		}
		seenCues[ch.OriginalTime] = true
		res = append(res, ch)
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].OriginalTime < res[j].OriginalTime
	})
	return res
}

// findCutCueIndex returns the index of a cue matching timestamp t if it is marked ActionCut.
func findCutCueIndex(t time.Duration, cues []model.SubtitleCue) int {
	for i, c := range cues {
		if c.Start == t {
			if c.Action == model.ActionCut {
				return i
			}
			return -1
		}
	}
	for i, c := range cues {
		if c.Start <= t && t < c.End {
			if c.Action == model.ActionCut {
				return i
			}
			return -1
		}
	}
	bestIdx := -1
	minDiff := 2 * time.Second
	for i, c := range cues {
		diff := c.Start - t
		if diff < 0 {
			diff = -diff
		}
		if diff < minDiff {
			minDiff = diff
			bestIdx = i
		}
	}
	if bestIdx >= 0 && cues[bestIdx].Action == model.ActionCut {
		return bestIdx
	}
	return -1
}

// findNextKeptCue searches for the first non-deleted subtitle cue at or after startIdx.
func findNextKeptCue(startIdx int, cues []model.SubtitleCue) int {
	for i := startIdx; i < len(cues); i++ {
		if cues[i].Action != model.ActionCut {
			return i
		}
	}
	for i := startIdx - 1; i >= 0; i-- {
		if cues[i].Action != model.ActionCut {
			return i
		}
	}
	return -1
}

// closestCueStart finds the start timestamp of the subtitle cue closest to target time t within maxDiff.
func closestCueStart(t time.Duration, cues []model.SubtitleCue, maxDiff time.Duration) (time.Duration, bool) {
	bestIdx := -1
	minDiff := time.Duration(1<<63 - 1)
	for j, c := range cues {
		diff := c.Start - t
		if diff < 0 {
			diff = -diff
		}
		if diff < minDiff {
			minDiff = diff
			bestIdx = j
		}
	}
	if bestIdx >= 0 && minDiff <= maxDiff {
		return cues[bestIdx].Start, true
	}
	return t, false
}
