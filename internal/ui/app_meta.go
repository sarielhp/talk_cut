package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/ai"
	"talk_cut/internal/config"
	"talk_cut/internal/eml"
	"talk_cut/internal/metadata"
	"talk_cut/internal/model"
)

type emlMetadataMsg struct {
	meta model.TalkMetadata
	err  error
}

type urlMetadataMsg struct {
	meta model.TalkMetadata
	err  error
}

// triggerEMLMetadata dispatches background parsing and AI extraction of metadata from an announcement email.
func (a *AppModel) triggerEMLMetadata() tea.Cmd {
	emlPath := a.bundle.EmlPath
	if emlPath == "" {
		discovered, err := eml.FindUniqueEML(a.bundle.Dir)
		if err != nil {
			a.metaView.SetFeedback(fmt.Sprintf("Email discovery error: %v", err), true)
			return clearStatusCmd()
		}
		if discovered == "" {
			a.metaView.SetFeedback("No unique .eml file found in directory", true)
			return clearStatusCmd()
		}
		emlPath = discovered
		a.bundle.EmlPath = discovered
	}

	a.metaView.SetFeedback(fmt.Sprintf("Extracting metadata from %s...", filepath.Base(emlPath)), false)
	cfg := a.cfg

	return func() tea.Msg {
		parsed, err := eml.ParseEMLFile(emlPath)
		if err != nil {
			return emlMetadataMsg{err: fmt.Errorf("parsing %s: %w", filepath.Base(emlPath), err)}
		}

		client, err := ai.NewClient(cfg)
		if err != nil {
			return emlMetadataMsg{err: fmt.Errorf("initializing AI client: %w", err)}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		extracted, err := ai.ExtractTalkMetadataFromEmail(ctx, client, parsed.Subject, parsed.Body, nil)
		if err != nil {
			return emlMetadataMsg{err: fmt.Errorf("AI extraction failed: %w", err)}
		}

		return emlMetadataMsg{meta: extracted}
	}
}

// handleEMLMetadataMsg processes the asynchronous email metadata extraction result.
func (a *AppModel) handleEMLMetadataMsg(msg emlMetadataMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		errStr := fmt.Sprintf("Email extraction error: %v", msg.err)
		a.metaView.SetFeedback(errStr, true)
		return *a, clearStatusCmd()
	}

	a.metaView.ApplyMetadata(msg.meta)
	meta := a.metaView.Metadata()
	_ = model.SaveMetaFile(a.bundle.Dir, meta)

	feedback := fmt.Sprintf("Updated talk metadata from %s", filepath.Base(a.bundle.EmlPath))
	a.metaView.SetFeedback(feedback, false)
	return *a, clearStatusCmd()
}

// triggerURLMetadata dispatches background fetching and extraction of metadata from the seminar URL.
func (a *AppModel) triggerURLMetadata() tea.Cmd {
	meta := a.metaView.Metadata()
	pageURL := strings.TrimSpace(meta.URL)
	if pageURL == "" {
		a.metaView.SetFeedback("No URL specified in metadata form to fetch from", true)
		return clearStatusCmd()
	}

	a.metaView.SetFeedback(fmt.Sprintf("Fetching metadata from %s...", pageURL), false)
	cfg := a.cfg

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		extracted, err := fetchAndExtractFromURL(ctx, pageURL, cfg)
		if err != nil {
			return urlMetadataMsg{err: err}
		}

		return urlMetadataMsg{meta: extracted}
	}
}

// fetchAndExtractFromURL fetches page info and supplements with AI if needed.
func fetchAndExtractFromURL(ctx context.Context, pageURL string, cfg config.Config) (model.TalkMetadata, error) {
	info, err := metadata.FetchTalkInfo(ctx, pageURL)
	if err != nil {
		return model.TalkMetadata{}, fmt.Errorf("fetching %s: %w", pageURL, err)
	}

	meta := model.TalkMetadata{
		URL:         pageURL,
		Title:       info.Title,
		Speaker:     info.Speaker,
		Affiliation: info.Affiliation,
		Abstract:    info.Abstract,
		Tags:        info.Tags,
	}

	if meta.Speaker == "" || meta.Abstract == "" {
		pageContent, pageErr := metadata.FetchTalkPage(ctx, pageURL)
		if pageErr == nil && pageContent.BodyText != "" {
			if client, clientErr := ai.NewClient(cfg); clientErr == nil {
				if aiMeta, aiErr := ai.ExtractTalkMetadata(ctx, client, pageContent.BodyText, nil); aiErr == nil {
					enrichMetadataFromAI(&meta, aiMeta)
				}
			}
		}
	}

	return meta, nil
}

// enrichMetadataFromAI supplements empty metadata fields from AI extraction.
func enrichMetadataFromAI(meta *model.TalkMetadata, aiMeta model.TalkMetadata) {
	if meta.Title == "" && aiMeta.Title != "" {
		meta.Title = aiMeta.Title
	}
	if meta.Speaker == "" && aiMeta.Speaker != "" {
		meta.Speaker = aiMeta.Speaker
	}
	if meta.Affiliation == "" && aiMeta.Affiliation != "" {
		meta.Affiliation = aiMeta.Affiliation
	}
	if meta.Abstract == "" && aiMeta.Abstract != "" {
		meta.Abstract = aiMeta.Abstract
	}
	if len(meta.Tags) == 0 && len(aiMeta.Tags) > 0 {
		meta.Tags = aiMeta.Tags
	}
}

// handleURLMetadataMsg processes the asynchronous URL metadata fetch result.
func (a *AppModel) handleURLMetadataMsg(msg urlMetadataMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		errStr := fmt.Sprintf("URL fetch error: %v", msg.err)
		a.metaView.SetFeedback(errStr, true)
		return *a, clearStatusCmd()
	}

	a.metaView.ApplyMetadata(msg.meta)
	meta := a.metaView.Metadata()
	_ = model.SaveMetaFile(a.bundle.Dir, meta)

	feedback := "Updated talk metadata from URL"
	if msg.meta.Title != "" {
		feedback = fmt.Sprintf("Loaded metadata: %s", msg.meta.Title)
	}
	a.metaView.SetFeedback(feedback, false)
	return *a, clearStatusCmd()
}
