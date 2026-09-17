// Package model defines core domain data types for talk_cut.
package model

import (
	"fmt"
	"time"
)

// CutInterval represents a continuous time range to keep or cut.
type CutInterval struct {
	Start  time.Duration `json:"start"`
	End    time.Duration `json:"end"`
	Action CutAction     `json:"action"`
	Reason string        `json:"reason,omitempty"`
}

// Duration returns the length of the interval.
func (i CutInterval) Duration() time.Duration {
	if i.End < i.Start {
		return 0
	}
	return i.End - i.Start
}

// String returns a compact representation of the interval.
func (i CutInterval) String() string {
	return fmt.Sprintf("[%s - %s] %s (%s)", i.Start, i.End, i.Action, i.Reason)
}

// CutStats summarizes the overall impact of active cuts.
type CutStats struct {
	TotalOriginal time.Duration
	TotalKept     time.Duration
	TotalCut      time.Duration
	CutCount      int
}

// KeptPercent returns the percentage of total media duration retained.
func (s CutStats) KeptPercent() float64 {
	if s.TotalOriginal <= 0 {
		return 100.0
	}
	return (float64(s.TotalKept) / float64(s.TotalOriginal)) * 100.0
}

// ComputeStats calculates aggregate cut statistics across a slice of cues.
func ComputeStats(cues []SubtitleCue, mediaDuration time.Duration) CutStats {
	stats := CutStats{
		TotalOriginal: mediaDuration,
	}

	if len(cues) == 0 {
		stats.TotalKept = mediaDuration
		return stats
	}

	intervals := BuildCutIntervals(cues)
	var cutTotal time.Duration
	cutCount := 0

	for _, inv := range intervals {
		if inv.Action == ActionCut {
			cutTotal += inv.Duration()
			cutCount++
		}
	}

	stats.CutCount = cutCount
	stats.TotalCut = cutTotal
	if cutTotal <= mediaDuration {
		stats.TotalKept = mediaDuration - cutTotal
		return stats
	}

	stats.TotalKept = 0
	return stats
}

// BuildCutIntervals merges contiguous cues with the same action into continuous intervals.
func BuildCutIntervals(cues []SubtitleCue) []CutInterval {
	if len(cues) == 0 {
		return nil
	}

	var intervals []CutInterval
	current := CutInterval{
		Start:  cues[0].Start,
		End:    cues[0].End,
		Action: cues[0].Action,
		Reason: cues[0].CutReason,
	}

	for i := 1; i < len(cues); i++ {
		cue := cues[i]
		if cue.Action == current.Action {
			current.End = cue.End
			continue
		}

		intervals = append(intervals, current)
		current = CutInterval{
			Start:  cue.Start,
			End:    cue.End,
			Action: cue.Action,
			Reason: cue.CutReason,
		}
	}

	intervals = append(intervals, current)
	return intervals
}

// ApplyCutsToCues updates the Action and CutReason on each cue whose midpoint falls within a cut interval.
func ApplyCutsToCues(cues []SubtitleCue, cuts []CutInterval) {
	for i := range cues {
		mid := (cues[i].Start + cues[i].End) / 2
		for _, cut := range cuts {
			if cut.Action == ActionCut && mid >= cut.Start && mid <= cut.End {
				cues[i].Action = ActionCut
				cues[i].CutReason = cut.Reason
				break
			}
		}
	}
}
