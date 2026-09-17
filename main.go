// Package main is the entry point for the talk_cut CLI tool.
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/ai"
	"talk_cut/internal/bundle"
	"talk_cut/internal/config"
	"talk_cut/internal/cutter"
	"talk_cut/internal/metadata"
	"talk_cut/internal/model"
	"talk_cut/internal/ui"
	"talk_cut/internal/vtt"
)

//go:embed VERSION
var rawVersion string

// Version is the current semantic version of talk_cut.
var Version = strings.TrimSpace(rawVersion)

type cliOptions struct {
	dir         string
	output      string
	url         string
	layout      string
	noAI        bool
	dryRun      bool
	keyFile     string
	model       string
	showVersion bool
	showHelp    bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	opts, err := parseCLIFlags(args)
	if err != nil {
		return err
	}

	if opts.showVersion {
		fmt.Printf("talk_cut v%s\n", Version)
		return nil
	}
	if opts.showHelp || opts.dir == "" {
		printHelp()
		return nil
	}

	return executePipeline(opts)
}

// parseCLIFlags parses command-line arguments into cliOptions.
func parseCLIFlags(args []string) (*cliOptions, error) {
	fs := flag.NewFlagSet("talk_cut", flag.ContinueOnError)
	opts := &cliOptions{}

	fs.StringVar(&opts.output, "o", "", "Output cut video path")
	fs.StringVar(&opts.output, "output", "", "Output cut video path")
	fs.StringVar(&opts.url, "u", "", "Seminar announcement web URL")
	fs.StringVar(&opts.url, "url", "", "Seminar announcement web URL")
	fs.StringVar(&opts.layout, "layout", "", "Preferred video layout (slides, clean, speaker, gallery)")
	fs.BoolVar(&opts.noAI, "no-ai", false, "Disable OpenRouter AI cut detection")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "Analyze and print cut plan without opening TUI")
	fs.StringVar(&opts.keyFile, "key-file", "", "Custom path to auth key file")
	fs.StringVar(&opts.model, "model", "", "OpenRouter model name")
	fs.BoolVar(&opts.showVersion, "v", false, "Print version and exit")
	fs.BoolVar(&opts.showVersion, "version", false, "Print version and exit")
	fs.BoolVar(&opts.showHelp, "h", false, "Print help and exit")
	fs.BoolVar(&opts.showHelp, "help", false, "Print help and exit")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if fs.NArg() > 0 {
		opts.dir = fs.Arg(0)
	}
	return opts, nil
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
	if !opts.noAI {
		cues = runAICutDetection(ctx, cfg, cues, &talkMeta)
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

// initialMetadata constructs baseline metadata, optionally scraping the seminar web page.
func initialMetadata(ctx context.Context, opts *cliOptions, cfg config.Config) model.TalkMetadata {
	meta := model.TalkMetadata{
		Title:   filepath.Base(opts.dir),
		Privacy: cfg.DefaultPrivacy,
	}

	if opts.url != "" {
		meta.URL = opts.url
		if page, err := metadata.FetchTalkPage(ctx, opts.url); err == nil {
			if page.Title != "" {
				meta.Title = page.Title
			}
			if page.Description != "" {
				meta.Abstract = page.Description
			}
		}
	}
	return meta
}

// runAICutDetection queries OpenRouter to detect candidate cuts and enrich metadata.
func runAICutDetection(ctx context.Context, cfg config.Config, cues []model.SubtitleCue, meta *model.TalkMetadata) []model.SubtitleCue {
	client, err := ai.NewClient(cfg)
	if err != nil {
		return cues
	}

	cuts, cutErr := ai.DetectCuts(ctx, client, cues)
	if cutErr == nil {
		model.ApplyCutsToCues(cues, cuts)
	}

	if meta.Speaker == "" {
		if aiMeta, metaErr := ai.ExtractTalkMetadata(ctx, client, meta.Abstract, cues); metaErr == nil {
			if aiMeta.Title != "" && meta.Title == "" {
				meta.Title = aiMeta.Title
			}
			if aiMeta.Speaker != "" {
				meta.Speaker = aiMeta.Speaker
			}
			if aiMeta.Affiliation != "" {
				meta.Affiliation = aiMeta.Affiliation
			}
			if aiMeta.Abstract != "" && meta.Abstract == "" {
				meta.Abstract = aiMeta.Abstract
			}
			if len(aiMeta.Tags) > 0 {
				meta.Tags = aiMeta.Tags
			}
			if len(aiMeta.Chapters) > 0 {
				meta.Chapters = aiMeta.Chapters
			}
		}
	}

	return cues
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
}

// printHelp outputs CLI usage instructions.
func printHelp() {
	fmt.Printf("talk_cut v%s - Interactive Talk Trimmer & YouTube Publisher\n\n", Version)
	fmt.Println("Usage:")
	fmt.Println("  talk_cut [options] <recording-directory>")
	fmt.Println("\nOptions:")
	fmt.Println("  -o, --output <path>    Custom output destination for sliced video")
	fmt.Println("  -u, --url <url>        Seminar announcement URL (extracts speaker, title, abstract)")
	fmt.Println("  --layout <type>        Preferred layout: slides (default), clean, speaker, gallery")
	fmt.Println("  --no-ai                Skip AI LLM cut detection")
	fmt.Println("  --dry-run              Analyze and print cut plan without opening TUI")
	fmt.Println("  --key-file <path>      Path to OpenRouter API key file (default: ~/.config/auth/openrouter_api_key)")
	fmt.Println("  --model <name>         OpenRouter model name (default: google/gemini-2.5-flash-lite)")
	fmt.Println("  -v, --version          Print version information")
	fmt.Println("  -h, --help             Show this help screen")
}
