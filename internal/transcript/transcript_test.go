package transcript

import (
	"strings"
	"testing"
	"time"

	"talk_cut/internal/model"
)

func mkCue(id int, speaker, text string, action model.CutAction) model.SubtitleCue {
	return model.SubtitleCue{
		ID:      id,
		Start:   time.Duration(id) * time.Second,
		End:     time.Duration(id+1) * time.Second,
		Speaker: speaker,
		Text:    text,
		Action:  action,
	}
}

func TestBuildSkipsCutCuesAndTimestamps(t *testing.T) {
	cues := []model.SubtitleCue{
		mkCue(1, "Alice", "Hello there.", model.ActionKeep),
		mkCue(2, "Alice", "This should be removed.", model.ActionCut),
		mkCue(3, "Bob", "Hi Alice.", model.ActionKeep),
	}

	got := Build(cues, Options{})
	if strings.Contains(got, "removed") {
		t.Errorf("cut cue leaked into transcript: %q", got)
	}
	if !strings.Contains(got, "Alice: Hello there.") {
		t.Errorf("expected Alice paragraph, got %q", got)
	}
	if !strings.Contains(got, "Bob: Hi Alice.") {
		t.Errorf("expected Bob paragraph, got %q", got)
	}
	if strings.Contains(got, "00:") {
		t.Errorf("transcript should not contain timestamps: %q", got)
	}
}

func TestBuildMergesSameSpeaker(t *testing.T) {
	cues := []model.SubtitleCue{
		mkCue(1, "Alice", "First part", model.ActionKeep),
		mkCue(2, "Alice", "second part.", model.ActionKeep),
	}

	if got := Build(cues, Options{}); got != "Alice: First part second part." {
		t.Errorf("unexpected merged paragraph: %q", got)
	}
}

func TestBuildWithChaptersInsertsHeadingsAndBreaks(t *testing.T) {
	cues := []model.SubtitleCue{
		mkCue(1, "Alice", "Intro one.", model.ActionKeep),
		mkCue(2, "Alice", "Intro two.", model.ActionKeep),
		mkCue(3, "Alice", "Body one.", model.ActionKeep),
	}
	chapters := []Chapter{
		{Start: 0, Title: "Introduction"},
		{Start: 3 * time.Second, Title: "Main Results"},
	}

	got := BuildWithChapters(cues, chapters, Options{})
	want := "### Introduction\n\nAlice: Intro one. Intro two.\n\n### Main Results\n\nAlice: Body one."
	if got != want {
		t.Errorf("unexpected chaptered transcript:\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildDropsFillersAndRepeats(t *testing.T) {
	cues := []model.SubtitleCue{mkCue(1, "", "Um, so the the graph is is connected.", model.ActionKeep)}

	got := strings.ToLower(Build(cues, Options{DropFillers: true}))
	if strings.Contains(got, "um") {
		t.Errorf("filler leaked: %q", got)
	}
	if strings.Contains(got, "the the") || strings.Contains(got, "is is") {
		t.Errorf("repeated word leaked: %q", got)
	}
	if !strings.Contains(got, "so the graph is connected.") {
		t.Errorf("unexpected cleanup result: %q", got)
	}
}

func TestBuildKeepsFillersWhenDisabled(t *testing.T) {
	cues := []model.SubtitleCue{mkCue(1, "", "Um the graph.", model.ActionKeep)}

	if got := Build(cues, Options{}); !strings.Contains(got, "Um") {
		t.Errorf("expected filler preserved without DropFillers: %q", got)
	}
}

func TestBuildIncludeCutCues(t *testing.T) {
	cues := []model.SubtitleCue{
		mkCue(1, "", "Keep me.", model.ActionKeep),
		mkCue(2, "", "Cut me too.", model.ActionCut),
	}

	got := Build(cues, Options{IncludeCutCues: true})
	if !strings.Contains(got, "Cut me too.") {
		t.Errorf("expected cut cue included, got %q", got)
	}
}

func TestBuildEmptyInput(t *testing.T) {
	if got := Build(nil, Options{}); got != "" {
		t.Errorf("expected empty output, got %q", got)
	}
}
