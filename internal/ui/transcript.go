// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/ai"
	"talk_cut/internal/config"
	"talk_cut/internal/model"
	"talk_cut/internal/transcript"
)

// handleTranscriptMsg reports the result of a talk transcript export.
func (a *AppModel) handleTranscriptMsg(msg transcriptMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.progView.SetFeedback(fmt.Sprintf("Transcript error: %v", msg.err), true)
		return *a, nil
	}
	note := "local cleanup"
	if msg.polished {
		note = "local + AI polish"
	}
	a.progView.SetFeedback(fmt.Sprintf("Transcript saved: %s (%s)", msg.path, note), false)
	return *a, nil
}

// triggerTranscript generates the talk transcript document on demand from the
// current kept cues and metadata. It is the keyboard shortcut for talks that
// were already rendered or uploaded.
func (a *AppModel) triggerTranscript() tea.Cmd {
	cues := a.cutsView.Cues()
	meta := a.metaView.Metadata()
	outBase := outputBase(a.metaView.OutputPath())

	if len(keptCues(cues)) == 0 {
		a.progView.SetFeedback("Cannot export transcript: all cues cut", true)
		return nil
	}
	a.progView.SetFeedback("Cleaning transcript and polishing with AI...", false)

	return func() tea.Msg {
		path, polished, err := generateTranscript(outBase, cues, meta, true)
		if err != nil {
			return transcriptMsg{err: err}
		}
		return transcriptMsg{path: path, polished: polished}
	}
}

// polishTranscriptCmd best-effort polishes the transcript written during render.
// The local document already exists on disk; this only upgrades it with the AI
// pass, so the render completion is never blocked on the model.
func (a *AppModel) polishTranscriptCmd(videoPath string) tea.Cmd {
	cues := a.cutsView.Cues()
	meta := a.metaView.Metadata()
	outBase := outputBase(videoPath)

	return func() tea.Msg {
		path, polished, err := generateTranscript(outBase, cues, meta, true)
		if err != nil {
			return transcriptMsg{err: err}
		}
		return transcriptMsg{path: path, polished: polished}
	}
}

// generateTranscript writes the talk document (YouTube metadata plus the
// kept-only cleaned transcript) to <outBase>_transcript.md. When polish is true
// it also runs the best-effort AI polish pass and rewrites the file.
func generateTranscript(outBase string, cues []model.SubtitleCue, meta model.TalkMetadata, polish bool) (string, bool, error) {
	kept := keptCues(cues)
	if len(kept) == 0 {
		return "", false, fmt.Errorf("no kept cues to export")
	}
	model.NameDominantSpeaker(kept, meta.Speaker)

	body := transcript.Build(kept, transcript.Options{DropFillers: true})
	path := outBase + transcriptSuffix
	if err := writeTalkDoc(path, meta, body); err != nil {
		return path, false, err
	}

	if !polish {
		return path, false, nil
	}
	if out, ok := polishCleanTranscript(body, meta.Title); ok {
		if err := writeTalkDoc(path, meta, out); err != nil {
			return path, false, err
		}
		return path, true, nil
	}
	return path, false, nil
}

// writeTalkDoc writes the assembled talk document to path.
func writeTalkDoc(path string, meta model.TalkMetadata, body string) error {
	return os.WriteFile(path, []byte(buildTalkDoc(meta, body)), 0o644)
}

// buildTalkDoc assembles the markdown document: the standard YouTube metadata
// (title, speaker, URL, tags, abstract, chapters) followed by the transcript.
func buildTalkDoc(meta model.TalkMetadata, body string) string {
	var sb strings.Builder

	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = "Talk Transcript"
	}
	sb.WriteString("# " + title + "\n\n")

	if meta.Speaker != "" {
		sb.WriteString("**Speaker:** " + meta.Speaker)
		if meta.Affiliation != "" {
			sb.WriteString(" (" + meta.Affiliation + ")")
		}
		sb.WriteString("\n")
	}
	if meta.URL != "" {
		sb.WriteString("**Talk URL:** " + meta.URL + "\n")
	}
	if p := strings.TrimSpace(meta.Privacy); p != "" {
		sb.WriteString("**Privacy:** " + p + "\n")
	}
	if len(meta.Tags) > 0 {
		sb.WriteString("**Tags:** " + strings.Join(meta.Tags, ", ") + "\n")
	}

	if abstract := strings.TrimSpace(meta.Abstract); abstract != "" {
		sb.WriteString("\n## Abstract\n\n" + abstract + "\n")
	}
	if len(meta.Chapters) > 0 {
		sb.WriteString("\n## Chapters\n\n")
		for _, ch := range meta.Chapters {
			sb.WriteString(ch.FormatYouTubeLine() + "\n")
		}
	}

	sb.WriteString("\n## Transcript\n\n" + strings.TrimSpace(body) + "\n")
	return sb.String()
}

// keptCues returns the cues that survive cutting.
func keptCues(cues []model.SubtitleCue) []model.SubtitleCue {
	kept := make([]model.SubtitleCue, 0, len(cues))
	for _, c := range cues {
		if c.Action != model.ActionCut {
			kept = append(kept, c)
		}
	}
	return kept
}

// outputBase strips the file extension from a rendered output path.
func outputBase(output string) string {
	return strings.TrimSuffix(output, filepath.Ext(output))
}

// polishCleanTranscript best-effort runs the AI polish step, returning false on any failure.
func polishCleanTranscript(body, title string) (string, bool) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return body, false
	}
	client, err := ai.NewClient(cfg)
	if err != nil {
		return body, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	out, err := ai.PolishTranscript(ctx, client, body, title)
	if err != nil || strings.TrimSpace(out) == "" {
		return body, false
	}
	return out, true
}
