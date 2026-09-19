// Package main is the entry point for the talk_cut CLI tool.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/ai"
	"talk_cut/internal/bundle"
	"talk_cut/internal/config"
	"talk_cut/internal/cutter"
	"talk_cut/internal/eml"
	"talk_cut/internal/metadata"
	"talk_cut/internal/model"
	"talk_cut/internal/ui"
	"talk_cut/internal/vtt"
	"talk_cut/internal/youtube"
)

//go:embed VERSION
var rawVersion string

// Version is the current semantic version of talk_cut.
var Version = strings.TrimSpace(rawVersion)

type cliOptions struct {
	dir           string
	output        string
	url           string
	layout        string
	noAI          bool
	dryRun        bool
	metaOnly      bool
	upload        bool
	reDetect      bool
	updateYouTube bool
	channel       string
	playlist      string
	keyFile       string
	model         string
	showVersion   bool
	showHelp      bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	opts := &cliOptions{}
	return buildApp(opts).Execute(args)
}

func isURL(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// executePipeline loads bundle, analyzes talk, triggers AI cuts, and starts TUI.
func executePipeline(opts *cliOptions) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	applyConfigOverrides(&cfg, opts)

	b, err := bundle.DiscoverBundle(opts.dir, cfg.PreferredLayout)
	if err != nil {
		return fmt.Errorf("discovering bundle: %w", err)
	}

	ctx := context.Background()
	mediaInfo, err := cutter.ProbeMedia(ctx, b.PrimaryVideo)
	if err != nil {
		return fmt.Errorf("probing video %q: %w", b.PrimaryVideo, err)
	}

	cues, err := vtt.ParseFile(b.TranscriptPath)
	if err != nil {
		return fmt.Errorf("parsing transcript %q: %w", b.TranscriptPath, err)
	}

	talkMeta := initialMetadata(ctx, opts, cfg)
	if model.HasSavedCuts(opts.dir) && !opts.reDetect {
		if savedCuts, loadErr := model.LoadCutsFile(opts.dir); loadErr == nil {
			model.ApplyCutsToCues(cues, savedCuts)
			cutsPath := filepath.Join(opts.dir, model.CutsFileName)
			fmt.Printf("Loaded %d saved cut intervals from %s\n", len(savedCuts), cutsPath)
		}
	} else if !opts.noAI {
		cues = runAICutDetection(ctx, cfg, cues)
		_ = model.SaveCutsFile(opts.dir, model.BuildCutIntervals(cues))
	}

	if !opts.noAI && (len(talkMeta.Chapters) == 0 || opts.reDetect) {
		runAIChapterDetection(ctx, opts.dir, cfg, cues, &talkMeta)
	}

	outPath := resolveOutputPath(opts.dir, b.PrimaryVideo, opts.output)
	if opts.dryRun {
		printDryRunReport(b, mediaInfo, cues, talkMeta, outPath)
		return nil
	}

	app := ui.NewAppModel(*b, mediaInfo, cues, talkMeta, outPath)

	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui runtime error: %w", err)
	}
	return nil
}

// applyConfigOverrides overlays CLI flag values onto the loaded configuration.
func applyConfigOverrides(cfg *config.Config, opts *cliOptions) {
	if opts.keyFile != "" {
		cfg.KeyFile = opts.keyFile
	}
	if opts.model != "" {
		cfg.Model = opts.model
	}
	if opts.layout != "" {
		cfg.PreferredLayout = opts.layout
	}
}

// runMetaOnly fetches metadata from URL or announcement email and saves talk_meta.json without launching TUI.
func runMetaOnly(ctx context.Context, opts *cliOptions, cfg config.Config) error {
	info, err := os.Stat(opts.dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%q is not a valid directory", opts.dir)
	}

	emlPath, emlErr := eml.FindUniqueEML(opts.dir)
	if emlErr != nil {
		return emlErr
	}

	if opts.url == "" && emlPath == "" && !model.HasSavedMetadata(opts.dir) {
		return fmt.Errorf("no URL or unique .eml file found to update metadata (usage: talk_cut --meta-only <dir> [url])")
	}

	meta := initialMetadata(ctx, opts, cfg)
	fmt.Printf("✔ Talk metadata in %s is updated\n", opts.dir)
	if meta.Title != "" {
		fmt.Printf("  Title:    %s\n", meta.Title)
	}
	if meta.Speaker != "" {
		fmt.Printf("  Speaker:  %s", meta.Speaker)
		if meta.Affiliation != "" {
			fmt.Printf(" (%s)", meta.Affiliation)
		}
		fmt.Println()
	}
	if meta.URL != "" {
		fmt.Printf("  URL:      %s\n", meta.URL)
	}
	return nil
}

// initialMetadata constructs baseline metadata, optionally scraping web page or parsing announcement email.
func initialMetadata(ctx context.Context, opts *cliOptions, cfg config.Config) model.TalkMetadata {
	meta := model.TalkMetadata{
		Title:   filepath.Base(opts.dir),
		Privacy: cfg.DefaultPrivacy,
	}

	if model.HasSavedMetadata(opts.dir) {
		if saved, err := model.LoadMetaFile(opts.dir); err == nil {
			meta = saved
			metaPath := filepath.Join(opts.dir, model.MetaFileName)
			fmt.Printf("Loaded saved talk metadata from %s\n", metaPath)
		}
	}

	if opts.url != "" {
		meta.URL = opts.url
	}

	if opts.metaOnly {
		populateMetaOnly(ctx, opts, cfg, &meta)
		_ = model.SaveMetaFile(opts.dir, meta)
	}

	return meta
}

// populateMetaOnly handles CLI metadata generation from URL or EML when --meta-only is passed.
func populateMetaOnly(ctx context.Context, opts *cliOptions, cfg config.Config, meta *model.TalkMetadata) {
	if opts.url != "" {
		info, err := metadata.FetchTalkInfo(ctx, opts.url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: fetching %s: %v\n", opts.url, err)
			return
		}
		applyTalkInfoToMetadata(meta, info)
		fmt.Printf("✔ Fetched talk metadata from %s\n", opts.url)
		discoverChannelUpload(ctx, opts, cfg, meta)
		printMetaSummary(*meta)
		return
	}

	emlPath, emlErr := eml.FindUniqueEML(opts.dir)
	if emlErr != nil {
		fmt.Fprintf(os.Stderr, "warning: checking announcement email: %v\n", emlErr)
		return
	}
	if emlPath != "" && !opts.noAI {
		if err := extractMetadataFromEML(ctx, emlPath, cfg, meta); err != nil {
			fmt.Fprintf(os.Stderr, "warning: extracting metadata from %s: %v\n", emlPath, err)
			return
		}
		fmt.Printf("✔ Extracted talk metadata from email: %s\n", filepath.Base(emlPath))
	}

	discoverChannelUpload(ctx, opts, cfg, meta)
	printMetaSummary(*meta)
}

// discoverChannelUpload checks if a video for this talk is already uploaded on the YouTube channel.
func discoverChannelUpload(ctx context.Context, opts *cliOptions, cfg config.Config, meta *model.TalkMetadata) {
	channel := opts.channel
	if channel == "" {
		channel = cfg.DefaultChannel
	}
	if !cfg.HasValidChannelToken(channel) || (meta.YouTubeID != "" && meta.YouTubeURL != "") {
		return
	}
	client, err := youtube.GetAuthenticatedClient(ctx, cfg, channel)
	if err != nil {
		return
	}
	ver, err := youtube.FindChannelVideoForTalk(ctx, client, meta.Title, filepath.Base(opts.dir))
	if err == nil && ver != nil && ver.VideoID != "" {
		meta.YouTubeID = ver.VideoID
		meta.YouTubeURL = ver.ShortURL
		if meta.YouTubeURL == "" {
			meta.YouTubeURL = fmt.Sprintf("https://youtu.be/%s", ver.VideoID)
		}
		fmt.Printf("✔ Discovered existing YouTube upload: %s\n", meta.YouTubeURL)
	}
}

// printMetaSummary prints formatted speaker and title summary to stdout.
func printMetaSummary(meta model.TalkMetadata) {
	if meta.Title != "" {
		fmt.Printf("  Title:   %s\n", meta.Title)
	}
	if meta.Speaker != "" {
		fmt.Printf("  Speaker: %s", meta.Speaker)
		if meta.Affiliation != "" {
			fmt.Printf(" (%s)", meta.Affiliation)
		}
		fmt.Println()
	}
	if meta.YouTubeURL != "" {
		fmt.Printf("  YouTube: %s\n", meta.YouTubeURL)
	} else if meta.YouTubeID != "" {
		fmt.Printf("  YouTube: https://youtu.be/%s\n", meta.YouTubeID)
	}
}

// hasEmptyMetadataFields returns true if key talk metadata fields (speaker, abstract, and custom title) are empty.
func hasEmptyMetadataFields(meta model.TalkMetadata, dir string) bool {
	titleEmpty := strings.TrimSpace(meta.Title) == "" || meta.Title == filepath.Base(dir)
	speakerEmpty := strings.TrimSpace(meta.Speaker) == ""
	abstractEmpty := strings.TrimSpace(meta.Abstract) == ""
	return titleEmpty && speakerEmpty && abstractEmpty
}

// extractMetadataFromEML parses an announcement email and queries AI to extract talk details.
func extractMetadataFromEML(ctx context.Context, emlPath string, cfg config.Config, meta *model.TalkMetadata) error {
	parsed, err := eml.ParseEMLFile(emlPath)
	if err != nil {
		return fmt.Errorf("parsing email %q: %w", emlPath, err)
	}

	client, err := ai.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("initializing AI client: %w", err)
	}

	extracted, err := ai.ExtractTalkMetadataFromEmail(ctx, client, parsed.Subject, parsed.Body, nil)
	if err != nil {
		return fmt.Errorf("extracting metadata via AI: %w", err)
	}

	if extracted.Title != "" {
		meta.Title = extracted.Title
	}
	if extracted.Speaker != "" {
		meta.Speaker = extracted.Speaker
	}
	if extracted.Affiliation != "" {
		meta.Affiliation = extracted.Affiliation
	}
	if extracted.Abstract != "" {
		meta.Abstract = extracted.Abstract
	}
	if len(extracted.Tags) > 0 {
		meta.Tags = extracted.Tags
	}
	return nil
}

// applyTalkInfoToMetadata applies extracted web info onto TalkMetadata.
func applyTalkInfoToMetadata(meta *model.TalkMetadata, info metadata.TalkPageInfo) {
	if info.Title != "" {
		meta.Title = info.Title
	}
	if info.Speaker != "" {
		meta.Speaker = info.Speaker
	}
	if info.Affiliation != "" {
		meta.Affiliation = info.Affiliation
	}
	if info.Abstract != "" {
		meta.Abstract = info.Abstract
	}
	if len(info.Tags) > 0 {
		meta.Tags = info.Tags
	}
}

// runAICutDetection queries OpenRouter to detect candidate cuts from transcript cues.
func runAICutDetection(ctx context.Context, cfg config.Config, cues []model.SubtitleCue) []model.SubtitleCue {
	client, err := ai.NewClient(cfg)
	if err != nil {
		return cues
	}

	cuts, cutErr := ai.DetectCuts(ctx, client, cues)
	if cutErr == nil {
		model.ApplyCutsToCues(cues, cuts)
	}

	return cues
}

// runAIChapterDetection queries OpenRouter to detect natural talk chapters if not already set.
func runAIChapterDetection(ctx context.Context, dir string, cfg config.Config, cues []model.SubtitleCue, meta *model.TalkMetadata) {
	client, err := ai.NewClient(cfg)
	if err != nil {
		return
	}

	chapters, err := ai.DetectChapters(ctx, client, cues, meta.Title, meta.Abstract)
	if err != nil || len(chapters) == 0 {
		return
	}

	meta.Chapters = chapters
	fmt.Printf("✔ AI detected %d natural chapters\n", len(chapters))
	for _, ch := range chapters {
		fmt.Printf("  %s %s\n", vtt.FormatTimestampShort(ch.OriginalTime), ch.Title)
	}
	_ = model.SaveMetaFile(dir, *meta)
}

// resolveOutputPath computes the default destination path for the cut video.
func resolveOutputPath(dir, primaryVideo, userOutput string) string {
	if userOutput != "" {
		return userOutput
	}

	base := filepath.Base(primaryVideo)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	return filepath.Join(dir, fmt.Sprintf("%s_cut%s", stem, ext))
}

// printDryRunReport formats an inspection summary of discovered bundle and detected cuts.
func printDryRunReport(b *bundle.RecordingBundle, media cutter.MediaInfo, cues []model.SubtitleCue, meta model.TalkMetadata, outPath string) {
	stats := model.ComputeStats(cues, media.Duration)
	intervals := model.BuildCutIntervals(cues)

	fmt.Println("=== Dry Run Inspection ===")
	fmt.Printf("Video Input:     %s (%s, %dx%d, %s)\n", b.PrimaryVideo, vtt.FormatTimestampShort(media.Duration), media.Width, media.Height, media.VideoCodec)
	fmt.Printf("Transcript:      %s (%d cues)\n", b.TranscriptPath, len(cues))
	fmt.Printf("Planned Output:  %s\n", outPath)
	fmt.Printf("Original Time:   %s\n", vtt.FormatTimestampShort(stats.TotalOriginal))
	fmt.Printf("Total Cut:       %s\n", vtt.FormatTimestampShort(stats.TotalCut))
	fmt.Printf("Total Kept:      %s (%.1f%%)\n\n", vtt.FormatTimestampShort(stats.TotalKept), stats.KeptPercent())

	fmt.Println("Cut Intervals:")
	cutFound := false
	for i, inv := range intervals {
		if inv.Action == model.ActionCut {
			cutFound = true
			fmt.Printf("  %d. %s - %s (%.1fs) - %s\n",
				i+1,
				vtt.FormatTimestampShort(inv.Start),
				vtt.FormatTimestampShort(inv.End),
				inv.Duration().Seconds(),
				inv.Reason,
			)
		}
	}
	if !cutFound {
		fmt.Println("  (No cuts marked)")
	}

	if meta.Title != "" || meta.Speaker != "" {
		fmt.Printf("\nMetadata:\n  Title:   %s\n  Speaker: %s\n", meta.Title, meta.Speaker)
	}

	if len(meta.Chapters) > 0 {
		fmt.Println("\nAdjusted YouTube Chapters:")
		adj := cutter.AdjustChapters(meta.Chapters, intervals, "Introduction")
		for _, ch := range adj {
			fmt.Printf("  %s\n", ch.FormatYouTubeLine())
		}
	}
}

// printHelp outputs CLI usage instructions.
