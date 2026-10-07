package model

import (
	"regexp"
	"strings"
	"time"
)

// syntheticSpeakerRe matches WhisperX-style diarization labels, e.g. SPEAKER_00.
var syntheticSpeakerRe = regexp.MustCompile(`^SPEAKER_[0-9]+$`)

// HasSyntheticSpeakers reports whether every non-empty speaker label looks like
// WhisperX diarization output. A transcript carrying real names (for example a
// Zoom-generated VTT) returns false, letting callers avoid overwriting them.
func HasSyntheticSpeakers(cues []SubtitleCue) bool {
	found := false
	for _, cue := range cues {
		label := strings.TrimSpace(cue.Speaker)
		if label == "" {
			continue
		}
		found = true
		if !syntheticSpeakerRe.MatchString(label) {
			return false
		}
	}
	return found
}

// NameDominantSpeaker replaces the most talkative synthetic speaker label with
// the known speaker's name. It does nothing unless every label is synthetic and
// knownSpeaker is non-empty, so genuine speaker names are never overwritten.
func NameDominantSpeaker(cues []SubtitleCue, knownSpeaker string) {
	knownSpeaker = strings.TrimSpace(knownSpeaker)
	if knownSpeaker == "" || !HasSyntheticSpeakers(cues) {
		return
	}

	dominant := dominantSpeaker(cues)
	if dominant == "" {
		return
	}
	for i := range cues {
		if cues[i].Speaker == dominant {
			cues[i].Speaker = knownSpeaker
		}
	}
}

// dominantSpeaker returns the synthetic label with the greatest speaking time,
// breaking ties by label order for determinism.
func dominantSpeaker(cues []SubtitleCue) string {
	totals := make(map[string]time.Duration)
	for _, cue := range cues {
		if cue.Speaker == "" {
			continue
		}
		totals[cue.Speaker] += cue.Duration()
	}

	best := ""
	var bestTotal time.Duration
	for label, total := range totals {
		if total > bestTotal || (total == bestTotal && (best == "" || label < best)) {
			best, bestTotal = label, total
		}
	}
	return best
}
