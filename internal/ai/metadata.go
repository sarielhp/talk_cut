// Package ai provides an OpenRouter client and structured prompts for cut detection and metadata extraction.
package ai

import (
	"context"
	"fmt"
	"strings"

	"talk_cut/internal/model"
	"talk_cut/internal/vtt"
)

type rawMetadataResponse struct {
	Title             string   `json:"title"`
	Speaker           string   `json:"speaker"`
	Affiliation       string   `json:"affiliation"`
	Abstract          string   `json:"abstract"`
	Tags              []string `json:"tags"`
	SuggestedChapters []struct {
		Timestamp string `json:"timestamp"`
		Title     string `json:"title"`
	} `json:"suggested_chapters"`
}

const metadataSystemPrompt = `You are a seminar coordinator and YouTube publishing specialist.
Extract concise talk metadata from the provided seminar webpage text and initial transcript speech.

Produce a JSON object matching this schema:
{
  "title": "Clean talk title without seminar prefixes",
  "speaker": "Primary speaker name",
  "affiliation": "Speaker's institution or university",
  "abstract": "Clean 1-3 paragraph summary of the talk topic and results",
  "tags": ["algorithm", "computational geometry", "seminar"],
  "suggested_chapters": [
    {"timestamp": "00:00:28.750", "title": "Introduction & Motivation"}
  ]
}`

const emailMetadataSystemPrompt = `You are a seminar coordinator and YouTube publishing specialist.
Extract talk metadata from the provided seminar announcement email.

CRITICAL REQUIREMENTS:
1. Follow the email details VERY CLOSELY and faithfully:
   - Title: Use the exact talk title from the email. Do NOT alter, summarize, or generalize it.
   - Speaker: Use the exact speaker name from the email.
   - Affiliation: Use the speaker's institution or university as given in the email.
   - Abstract: Use the abstract text from the email. Preserve all mathematical notation, formulas, and bullet points closely.
2. Tags: Extract 3-6 relevant academic tags based on the topic.

Produce a JSON object matching this schema:
{
  "title": "Exact talk title",
  "speaker": "Speaker name",
  "affiliation": "Speaker's institution or university",
  "abstract": "Abstract text following the email closely",
  "tags": ["algorithm", "computational geometry", "seminar"],
  "suggested_chapters": [
    {"timestamp": "00:00:28.750", "title": "Introduction & Motivation"}
  ]
}`

// ExtractTalkMetadata queries the AI model to construct enriched YouTube metadata from web and transcript context.
func ExtractTalkMetadata(ctx context.Context, client *Client, pageText string, cues []model.SubtitleCue) (model.TalkMetadata, error) {
	var sb strings.Builder

	if strings.TrimSpace(pageText) != "" {
		sb.WriteString("--- SEMINAR ANNOUNCEMENT WEBPAGE ---\n")
		sb.WriteString(strings.TrimSpace(pageText))
		sb.WriteString("\n\n")
	}

	sb.WriteString("--- OPENING TRANSCRIPT SPEECH ---\n")
	openingLimit := 25
	if openingLimit > len(cues) {
		openingLimit = len(cues)
	}
	sb.WriteString(formatCueSlice(cues[:openingLimit]))

	var raw rawMetadataResponse
	if err := client.CompleteJSON(ctx, metadataSystemPrompt, sb.String(), &raw); err != nil {
		return model.TalkMetadata{}, fmt.Errorf("extracting talk metadata via AI: %w", err)
	}

	return buildMetadataFromRaw(raw), nil
}

// ExtractTalkMetadataFromEmail queries the AI model to extract metadata closely matching an announcement email.
func ExtractTalkMetadataFromEmail(ctx context.Context, client *Client, subject, body string, cues []model.SubtitleCue) (model.TalkMetadata, error) {
	var sb strings.Builder
	if strings.TrimSpace(subject) != "" {
		sb.WriteString("EMAIL SUBJECT: ")
		sb.WriteString(strings.TrimSpace(subject))
		sb.WriteString("\n\n")
	}
	sb.WriteString("EMAIL BODY:\n")
	sb.WriteString(strings.TrimSpace(body))
	sb.WriteString("\n\n")

	if len(cues) > 0 {
		sb.WriteString("--- OPENING TRANSCRIPT SPEECH ---\n")
		openingLimit := 25
		if openingLimit > len(cues) {
			openingLimit = len(cues)
		}
		sb.WriteString(formatCueSlice(cues[:openingLimit]))
	}

	var raw rawMetadataResponse
	if err := client.CompleteJSON(ctx, emailMetadataSystemPrompt, sb.String(), &raw); err != nil {
		return model.TalkMetadata{}, fmt.Errorf("extracting talk metadata from email via AI: %w", err)
	}

	return buildMetadataFromRaw(raw), nil
}

// buildMetadataFromRaw converts rawMetadataResponse into domain model.TalkMetadata.
func buildMetadataFromRaw(raw rawMetadataResponse) model.TalkMetadata {
	meta := model.TalkMetadata{
		Title:       raw.Title,
		Speaker:     raw.Speaker,
		Affiliation: raw.Affiliation,
		Abstract:    raw.Abstract,
		Tags:        raw.Tags,
		Privacy:     "public",
	}

	for _, ch := range raw.SuggestedChapters {
		t, err := vtt.ParseTimestamp(ch.Timestamp)
		if err != nil || ch.Title == "" {
			continue
		}
		meta.Chapters = append(meta.Chapters, model.ChapterMarker{
			OriginalTime: t,
			AdjustedTime: t,
			Title:        ch.Title,
		})
	}
	return meta
}
