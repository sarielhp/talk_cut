// Package transcript renders cleaned, timestamp-free transcripts from subtitle cues.
package transcript

import (
	"strings"
	"time"
	"unicode"

	"talk_cut/internal/model"
)

// Options controls deterministic transcript cleanup.
type Options struct {
	// IncludeCutCues keeps cues marked for cutting (default false).
	IncludeCutCues bool
	// DropFillers removes disfluencies and accidental word repetitions.
	DropFillers bool
}

// Chapter marks a section boundary in the rendered transcript. Start is the
// post-cut (adjusted) timestamp at which the section begins.
type Chapter struct {
	Start time.Duration
	Title string
}

// Build renders kept cues into a clean, timestamp-free prose transcript.
// Consecutive cues from the same speaker are merged into a single paragraph.
func Build(cues []model.SubtitleCue, opts Options) string {
	return BuildWithChapters(cues, nil, opts)
}

// BuildWithChapters renders kept cues into a chaptered, timestamp-free
// transcript. Each chapter emits a "### Title" heading and forces a new
// paragraph; consecutive cues from the same speaker within a chapter are merged.
// Cues must be ordered by ascending Start and carry adjusted timestamps.
func BuildWithChapters(cues []model.SubtitleCue, chapters []Chapter, opts Options) string {
	var blocks []string
	var curWords []string
	var curSpeaker string

	flush := func() {
		if len(curWords) == 0 {
			return
		}
		blocks = append(blocks, formatParagraph(curSpeaker, curWords))
		curWords = nil
	}

	chIdx := 0
	emitChapters := func(at time.Duration) {
		for chIdx < len(chapters) && chapters[chIdx].Start <= at {
			flush()
			if title := strings.TrimSpace(chapters[chIdx].Title); title != "" {
				blocks = append(blocks, "### "+title)
			}
			chIdx++
		}
	}

	for _, cue := range cues {
		if !opts.IncludeCutCues && cue.Action == model.ActionCut {
			continue
		}
		emitChapters(cue.Start)

		text := cleanCueText(cue.Text, opts.DropFillers)
		if text == "" {
			continue
		}
		if len(curWords) > 0 && cue.Speaker != curSpeaker {
			flush()
		}
		curSpeaker = cue.Speaker
		curWords = append(curWords, text)
	}
	flush()

	return strings.Join(blocks, "\n\n")
}

// formatParagraph joins cleaned chunks under an optional speaker label.
func formatParagraph(speaker string, chunks []string) string {
	body := normalizeSpacing(strings.Join(chunks, " "))
	if strings.TrimSpace(speaker) == "" {
		return body
	}
	return speaker + ": " + body
}

// cleanCueText folds whitespace and optionally removes disfluencies.
func cleanCueText(raw string, dropFillers bool) string {
	fields := strings.Fields(raw)
	if dropFillers {
		fields = dropDisfluencies(fields)
	}
	return normalizeSpacing(strings.Join(fields, " "))
}

// fillerWords is a conservative set of spoken disfluencies removed on cleanup.
var fillerWords = map[string]struct{}{
	"um": {}, "uh": {}, "uhm": {}, "erm": {}, "er": {}, "hmm": {}, "hm": {}, "ah": {}, "eh": {},
}

// dropDisfluencies removes filler words and immediately repeated words.
func dropDisfluencies(words []string) []string {
	out := words[:0]
	for _, w := range words {
		if _, ok := fillerWords[strings.ToLower(trimPunct(w))]; ok {
			continue
		}
		if len(out) > 0 && strings.EqualFold(out[len(out)-1], w) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// trimPunct strips surrounding punctuation and whitespace from a token.
func trimPunct(w string) string {
	return strings.TrimFunc(w, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r)
	})
}

// normalizeSpacing collapses runs of whitespace and removes spaces before punctuation.
func normalizeSpacing(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(fixSpaceBeforePunctuation(s))
}

// fixSpaceBeforePunctuation deletes stray spaces preceding common punctuation.
func fixSpaceBeforePunctuation(s string) string {
	const punct = ",.!?;:"
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if r == ' ' && i+1 < len(runes) && strings.ContainsRune(punct, runes[i+1]) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
