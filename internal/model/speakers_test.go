package model

import (
	"testing"
	"time"
)

func cue(id int, speaker string, start, dur time.Duration) SubtitleCue {
	return SubtitleCue{ID: id, Speaker: speaker, Start: start, End: start + dur}
}

func TestHasSyntheticSpeakers(t *testing.T) {
	cases := []struct {
		name string
		cues []SubtitleCue
		want bool
	}{
		{"all synthetic", []SubtitleCue{cue(1, "SPEAKER_00", 0, time.Second), cue(2, "SPEAKER_01", time.Second, time.Second)}, true},
		{"real names", []SubtitleCue{cue(1, "Boris Aronov", 0, time.Second), cue(2, "Michiel Smid", time.Second, time.Second)}, false},
		{"mixed", []SubtitleCue{cue(1, "SPEAKER_00", 0, time.Second), cue(2, "Michiel Smid", time.Second, time.Second)}, false},
		{"none set", []SubtitleCue{cue(1, "", 0, time.Second)}, false},
		{"empty slice", nil, false},
		{"supplementary blanks allowed", []SubtitleCue{cue(1, "SPEAKER_00", 0, time.Second), cue(2, "", time.Second, time.Second)}, true},
	}
	for _, tc := range cases {
		if got := HasSyntheticSpeakers(tc.cues); got != tc.want {
			t.Errorf("%s: HasSyntheticSpeakers = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestNameDominantSpeakerReplacesSynthetic(t *testing.T) {
	cues := []SubtitleCue{
		cue(1, "SPEAKER_00", 0, 5*time.Second),
		cue(2, "SPEAKER_01", 5*time.Second, 30*time.Second),
		cue(3, "SPEAKER_01", 35*time.Second, 10*time.Second),
	}

	NameDominantSpeaker(cues, "Michiel Smid")

	if cues[0].Speaker != "SPEAKER_00" {
		t.Errorf("non-dominant label should be unchanged, got %q", cues[0].Speaker)
	}
	if cues[1].Speaker != "Michiel Smid" || cues[2].Speaker != "Michiel Smid" {
		t.Errorf("dominant label should become the known name, got %q/%q", cues[1].Speaker, cues[2].Speaker)
	}
}

func TestNameDominantSpeakerLeavesRealNames(t *testing.T) {
	cues := []SubtitleCue{
		cue(1, "Boris Aronov", 0, 10*time.Second),
		cue(2, "Michiel Smid", 10*time.Second, 30*time.Second),
	}

	NameDominantSpeaker(cues, "Someone Else")

	if cues[0].Speaker != "Boris Aronov" || cues[1].Speaker != "Michiel Smid" {
		t.Errorf("real names must never be overwritten, got %q/%q", cues[0].Speaker, cues[1].Speaker)
	}
}

func TestNameDominantSpeakerWithoutKnownSpeaker(t *testing.T) {
	cues := []SubtitleCue{cue(1, "SPEAKER_00", 0, 10*time.Second)}

	NameDominantSpeaker(cues, "   ")

	if cues[0].Speaker != "SPEAKER_00" {
		t.Errorf("expected labels untouched without a known speaker, got %q", cues[0].Speaker)
	}
}

func TestDominantSpeakerTieBreak(t *testing.T) {
	cues := []SubtitleCue{
		cue(1, "SPEAKER_01", 0, 10*time.Second),
		cue(2, "SPEAKER_00", 10*time.Second, 10*time.Second),
	}
	if got := dominantSpeaker(cues); got != "SPEAKER_00" {
		t.Errorf("expected deterministic tie-break to SPEAKER_00, got %q", got)
	}
}
