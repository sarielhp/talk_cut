// Package ai provides an OpenRouter client and structured prompts for cut detection, metadata extraction, and chapter detection.
package ai

import (
	"context"
	"fmt"
	"strings"
)

type rawPolishedTranscript struct {
	Transcript string `json:"transcript"`
}

const transcriptPolishSystemPrompt = `You are an expert transcript editor for recorded academic seminars and technical talks.
Rewrite the raw transcript into clean, readable prose while preserving meaning and order exactly.

Rules:
1. Remove filler words, false starts, stutters, and accidental repetitions.
2. Fix punctuation, capitalization, and sentence boundaries.
3. Break long stretches of speech into readable paragraphs of a few sentences. Start a new paragraph whenever the topic shifts, the speaker pauses, or a new section begins. Never return the whole talk as one giant block of text.
4. Keep speaker turns: start each turn with the speaker name followed by a colon when a speaker is known.
5. The input may contain "### " section headings on their own lines. Keep every heading exactly as written and in order, each on its own line. Do not invent, rename, merge, reorder, or drop headings, and keep the text that follows each heading inside that section.
6. Never add facts, commentary, or timestamps. Do not summarize.
7. Preserve technical terms, names, and numbers exactly.

Return a JSON object matching this schema:
{
  "transcript": "the cleaned transcript text"
}`

// PolishTranscript rewrites a raw transcript into clean, readable prose using the AI model.
func PolishTranscript(ctx context.Context, client *Client, raw, title string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}

	var out rawPolishedTranscript
	if err := client.CompleteJSON(ctx, transcriptPolishSystemPrompt, buildPolishPrompt(raw, title), &out); err != nil {
		return "", fmt.Errorf("polishing transcript via AI: %w", err)
	}
	return strings.TrimSpace(out.Transcript), nil
}

// buildPolishPrompt frames the raw transcript with optional talk context.
func buildPolishPrompt(raw, title string) string {
	var sb strings.Builder
	if strings.TrimSpace(title) != "" {
		sb.WriteString("TALK TITLE: ")
		sb.WriteString(strings.TrimSpace(title))
		sb.WriteString("\n\n")
	}
	sb.WriteString("RAW TRANSCRIPT:\n")
	sb.WriteString(raw)
	sb.WriteString("\n\nRewrite the transcript above following the system rules.")
	return sb.String()
}
