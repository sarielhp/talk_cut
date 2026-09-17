// Package model defines core domain data types for talk_cut.
package model

import (
	"fmt"
	"strings"
	"time"
)

// CutAction defines what action should be performed on a cue or interval.
type CutAction int

const (
	// ActionKeep indicates the segment will be retained in the final output.
	ActionKeep CutAction = iota
	// ActionCut indicates the segment will be excised from the final output.
	ActionCut
	// ActionReview indicates an AI-suggested cut that awaits user confirmation.
	ActionReview
)

// String returns a human-readable representation of the cut action.
func (a CutAction) String() string {
	switch a {
	case ActionKeep:
		return "KEEP"
	case ActionCut:
		return "CUT"
	case ActionReview:
		return "REVIEW"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(a))
	}
}

// SubtitleCue represents a single timestamped speech utterance.
type SubtitleCue struct {
	ID        int           `json:"id"`
	Start     time.Duration `json:"start"`
	End       time.Duration `json:"end"`
	Speaker   string        `json:"speaker"`
	Text      string        `json:"text"`
	Action    CutAction     `json:"action"`
	CutReason string        `json:"cut_reason,omitempty"`
}

// Duration returns the total length of the subtitle cue.
func (c SubtitleCue) Duration() time.Duration {
	if c.End < c.Start {
		return 0
	}
	return c.End - c.Start
}

// FullText returns speaker and text formatted as "Speaker: Text" or just "Text".
func (c SubtitleCue) FullText() string {
	if strings.TrimSpace(c.Speaker) == "" {
		return c.Text
	}
	return fmt.Sprintf("%s: %s", c.Speaker, c.Text)
}
