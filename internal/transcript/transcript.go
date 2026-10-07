// Package transcript renders cleaned, timestamp-free transcripts from subtitle cues.
package transcript

import (
	"strings"
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

// Build renders kept cues into a clean, timestamp-free prose transcript.
// Consecutive cues from the same speaker are merged into a single paragraph.
func Build(cues []model.SubtitleCue, opts Options) string {
	var paras, curWords []string
	var curSpeaker string

	flush := func() {
		if len(curWords) == 0 {
			return
		}
		paras = append(paras, formatParagraph(curSpeaker, curWords))
		curWords = nil
	}

	for _, cue := range cues {
		if !opts.IncludeCutCues && cue.Action == model.ActionCut {
			continue
		}
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

	return strings.Join(paras, "\n\n")
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
