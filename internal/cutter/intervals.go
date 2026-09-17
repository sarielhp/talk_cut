// Package cutter implements video probing, cut calculation, and FFmpeg execution.
package cutter

import (
	"time"

	"talk_cut/internal/model"
)

// GetKeptIntervals calculates the inverse of active cuts across total media duration.
func GetKeptIntervals(totalDuration time.Duration, cuts []model.CutInterval) []model.CutInterval {
	if totalDuration <= 0 {
		return nil
	}

	activeCuts := filterAndSortCuts(cuts)
	if len(activeCuts) == 0 {
		return []model.CutInterval{
			{Start: 0, End: totalDuration, Action: model.ActionKeep},
		}
	}

	var kept []model.CutInterval
	var currentPos time.Duration

	for _, cut := range activeCuts {
		if cut.Start > currentPos {
			end := cut.Start
			if end > totalDuration {
				end = totalDuration
			}
			kept = append(kept, model.CutInterval{
				Start:  currentPos,
				End:    end,
				Action: model.ActionKeep,
			})
		}

		if cut.End > currentPos {
			currentPos = cut.End
		}

		if currentPos >= totalDuration {
			break
		}
	}

	if currentPos < totalDuration {
		kept = append(kept, model.CutInterval{
			Start:  currentPos,
			End:    totalDuration,
			Action: model.ActionKeep,
		})
	}

	return kept
}
