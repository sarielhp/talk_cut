// Package ai provides an OpenRouter client and structured prompts for cut detection and metadata extraction.
package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"talk_cut/internal/model"
	"talk_cut/internal/vtt"
)

type rawCutProposal struct {
	Cuts []struct {
		Start  string `json:"start"`
		End    string `json:"end"`
		Reason string `json:"reason"`
	} `json:"cuts"`
}

const cutDetectionSystemPrompt = `You are an expert video editor analyzing a timestamped talk transcript.
Your task is to identify candidate sections of the recording to cut out:
1. Preamble/Intro fluff: Sound checks, mic checks, waiting for attendees, and introductory remarks by the seminar organizer/host before the main speaker begins their talk presentation. The cut should start at 00:00:00 and end right when the main speaker takes the floor.
2. Post-talk/Outro fluff: After the speaker delivers their final conclusion slide or says "Thank you", cut out the subsequent Q&A session, housekeeping remarks, or casual sign-offs.
3. Mid-talk dead pauses: Any substantial pauses or technical disruptions.

Return a JSON object matching this schema:
{
  "cuts": [
    {
      "start": "HH:MM:SS.mmm",
      "end": "HH:MM:SS.mmm",
      "reason": "Brief explanation"
    }
  ]
}`

// DetectCuts queries the AI model to detect preamble, dead pauses, and post-talk outro cuts.
func DetectCuts(ctx context.Context, client *Client, cues []model.SubtitleCue) ([]model.CutInterval, error) {
	if len(cues) == 0 {
		return nil, nil
	}

	sampledTranscript := buildSampledTranscript(cues)
	userPrompt := fmt.Sprintf("Here is the timestamped transcript of the recording:\n\n%s\n\nIdentify the cuts to make.", sampledTranscript)

	var proposal rawCutProposal
	if err := client.CompleteJSON(ctx, cutDetectionSystemPrompt, userPrompt, &proposal); err != nil {
		return nil, fmt.Errorf("detecting cuts via AI: %w", err)
	}

	var intervals []model.CutInterval
	for _, c := range proposal.Cuts {
		start, startErr := vtt.ParseTimestamp(c.Start)
		end, endErr := vtt.ParseTimestamp(c.End)
		if startErr != nil || endErr != nil || end <= start {
			continue
		}

		intervals = append(intervals, model.CutInterval{
			Start:  start,
			End:    end,
			Action: model.ActionCut,
			Reason: c.Reason,
		})
	}

	return intervals, nil
}

// buildSampledTranscript selects head, tail, and silence-gap cues to minimize token usage.
func buildSampledTranscript(cues []model.SubtitleCue) string {
	if len(cues) <= 80 {
		return formatCueSlice(cues)
	}

	var sb strings.Builder
	sb.WriteString("--- BEGINNING OF RECORDING ---\n")
	headLimit := 40
	if headLimit > len(cues) {
		headLimit = len(cues)
	}
	sb.WriteString(formatCueSlice(cues[:headLimit]))

	sb.WriteString("\n... [MAIN TALK PRESENTATION CONTINUES] ...\n\n")

	// Scan middle for silence gaps >= 12s
	for i := headLimit; i < len(cues)-40; i++ {
		gap := cues[i].Start - cues[i-1].End
		if gap >= 12*time.Second {
			sb.WriteString(fmt.Sprintf("[Long silence gap: %.1fs]\n", gap.Seconds()))
			sb.WriteString(formatCue(cues[i]))
		}
	}

	sb.WriteString("\n--- END OF RECORDING ---\n")
	tailStart := len(cues) - 40
	if tailStart < headLimit {
		tailStart = headLimit
	}
	sb.WriteString(formatCueSlice(cues[tailStart:]))

	return sb.String()
}

// formatCueSlice formats multiple cues with timestamps.
func formatCueSlice(cues []model.SubtitleCue) string {
	var sb strings.Builder
	for _, c := range cues {
		sb.WriteString(formatCue(c))
	}
	return sb.String()
}

// formatCue formats a single cue as "[Start -> End] Speaker: Text".
func formatCue(c model.SubtitleCue) string {
	start := vtt.FormatTimestamp(c.Start)
	end := vtt.FormatTimestamp(c.End)
	return fmt.Sprintf("[%s -> %s] %s\n", start, end, c.FullText())
}
