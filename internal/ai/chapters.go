// Package ai provides an OpenRouter client and structured prompts for cut detection, metadata extraction, and chapter detection.
package ai

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
	"talk_cut/internal/vtt"
)

type rawChapterProposal struct {
	Chapters []struct {
		Timestamp string `json:"timestamp"`
		Title     string `json:"title"`
	} `json:"chapters"`
}

const chapterDetectionSystemPrompt = `You are an expert video editor and technical seminar curator.
Your task is to analyze the timestamped transcript of a recorded talk and identify natural, topical chapters for YouTube.

Guidelines:
1. Identify major semantic and topical shifts in the presentation (e.g. Introduction & Motivation, Problem Definition, Related Work, Key Theoretical Results, Algorithm Details, Experiments & Empirical Results, Conclusions & Open Problems).
2. Produce between 4 and 10 natural chapters (at least 3 chapters).
3. Chapter timestamps must match the exact timestamp in the transcript where the speaker begins discussing that topic.
4. Titles must be concise (2-6 words), descriptive, and title-cased. Avoid generic titles like "Part 1" or "Slide 5".
5. The first chapter must mark the beginning of the presentation.

Return a JSON object matching this schema:
{
  "chapters": [
    {
      "timestamp": "HH:MM:SS",
      "title": "Concise Chapter Title"
    }
  ]
}`

// DetectChapters queries the AI model to identify natural topical chapters across the talk.
func DetectChapters(ctx context.Context, client *Client, cues []model.SubtitleCue, title, abstract string) ([]model.ChapterMarker, error) {
	if len(cues) == 0 {
		return nil, nil
	}

	userPrompt := buildChapterPrompt(cues, title, abstract)

	var proposal rawChapterProposal
	if err := client.CompleteJSON(ctx, chapterDetectionSystemPrompt, userPrompt, &proposal); err != nil {
		return nil, fmt.Errorf("detecting chapters via AI: %w", err)
	}

	proposalChapters, err := parseChapterProposal(proposal)
	if err != nil {
		return nil, err
	}
	return cutter.SnapChaptersToCues(proposalChapters, cues), nil
}

// buildChapterPrompt constructs a contextual prompt with talk info and compact transcript.
func buildChapterPrompt(cues []model.SubtitleCue, title, abstract string) string {
	var sb strings.Builder

	if strings.TrimSpace(title) != "" {
		sb.WriteString("TALK TITLE: ")
		sb.WriteString(strings.TrimSpace(title))
		sb.WriteString("\n")
	}
	if strings.TrimSpace(abstract) != "" {
		sb.WriteString("ABSTRACT:\n")
		sb.WriteString(strings.TrimSpace(abstract))
		sb.WriteString("\n\n")
	}

	sb.WriteString("TRANSCRIPT:\n")
	sb.WriteString(buildChapterTranscript(cues))
	sb.WriteString("\n\nIdentify the natural chapters for this talk.")

	return sb.String()
}

// buildChapterTranscript condenses subtitle cues into compact, time-stamped paragraphs.
func buildChapterTranscript(cues []model.SubtitleCue) string {
	var sb strings.Builder
	var curSpeaker string
	var blockStart time.Duration
	var curWords []string

	flushBlock := func() {
		if len(curWords) == 0 {
			return
		}
		prefix := ""
		if curSpeaker != "" {
			prefix = curSpeaker + ": "
		}
		ts := vtt.FormatTimestampShort(blockStart)
		sb.WriteString(fmt.Sprintf("[%s] %s%s\n", ts, prefix, strings.Join(curWords, " ")))
		curWords = curWords[:0]
	}

	for i, c := range cues {
		text := strings.TrimSpace(c.Text)
		if text == "" {
			continue
		}

		if len(curWords) == 0 {
			curSpeaker = c.Speaker
			blockStart = c.Start
			curWords = append(curWords, text)
			continue
		}

		isSpeakerChange := c.Speaker != curSpeaker && c.Speaker != ""
		isTimeExceeded := c.Start-blockStart >= 30*time.Second

		if isSpeakerChange || isTimeExceeded {
			flushBlock()
			curSpeaker = c.Speaker
			blockStart = c.Start
		}

		curWords = append(curWords, text)

		if i == len(cues)-1 {
			flushBlock()
		}
	}

	flushBlock()
	return sb.String()
}

// parseChapterProposal converts raw AI proposal into validated, sorted ChapterMarkers.
func parseChapterProposal(proposal rawChapterProposal) ([]model.ChapterMarker, error) {
	var chapters []model.ChapterMarker

	for _, raw := range proposal.Chapters {
		rawTitle := strings.TrimSpace(raw.Title)
		if rawTitle == "" {
			continue
		}

		t, err := vtt.ParseTimestamp(raw.Timestamp)
		if err != nil {
			continue
		}

		chapters = append(chapters, model.ChapterMarker{
			OriginalTime: t,
			AdjustedTime: t,
			Title:        rawTitle,
		})
	}

	sort.Slice(chapters, func(i, j int) bool {
		return chapters[i].OriginalTime < chapters[j].OriginalTime
	})

	return deduplicateChapters(chapters), nil
}

// deduplicateChapters filters markers occurring within 10 seconds of the preceding marker.
func deduplicateChapters(chapters []model.ChapterMarker) []model.ChapterMarker {
	if len(chapters) <= 1 {
		return chapters
	}

	var filtered []model.ChapterMarker
	filtered = append(filtered, chapters[0])

	for i := 1; i < len(chapters); i++ {
		prev := filtered[len(filtered)-1]
		if chapters[i].OriginalTime-prev.OriginalTime >= 10*time.Second {
			filtered = append(filtered, chapters[i])
		}
	}

	return filtered
}
