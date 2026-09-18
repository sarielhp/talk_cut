// Package main is the entry point for the talk_cut CLI tool.
package main

import (
	"context"
	_ "embed"
	"errors"
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
	"talk_cut/internal/youtube"
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
	metaOnly    bool
	upload      bool
	reDetect    bool
	channel     string
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
	if len(args) > 0 {
		switch args[0] {
		case "auth":
			return runAuth(args[1:])
		case "youtube":
			return runYouTube(args[1:])
		}
	}

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

	if opts.metaOnly {
		ctx := context.Background()
		cfg, cfgErr := config.LoadConfig()
		if cfgErr != nil {
			return fmt.Errorf("loading config: %w", cfgErr)
		}
		return runMetaOnly(ctx, opts, cfg)
	}

	if opts.upload {
		ctx := context.Background()
		cfg, cfgErr := config.LoadConfig()
		if cfgErr != nil {
			return fmt.Errorf("loading config: %w", cfgErr)
		}
		applyConfigOverrides(&cfg, opts)
		return runUploadPipeline(ctx, opts, cfg)
	}

	return executePipeline(opts)
}

// runAuth executes the interactive YouTube OAuth authorization flow.
func runAuth(args []string) error {
	fs := flag.NewFlagSet("talk_cut auth", flag.ContinueOnError)
	var secretsPath, tokenPath, channel string
	fs.StringVar(&channel, "channel", "", "YouTube channel name (e.g. seminar, course, personal)")
	fs.StringVar(&secretsPath, "secrets", "", "Path to Google Cloud client secrets JSON")
	fs.StringVar(&tokenPath, "token", "", "Path to store OAuth token (default: ~/.config/auth/youtube_<channel>.json)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if fs.NArg() > 0 && secretsPath == "" {
		secretsPath = fs.Arg(0)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx := context.Background()
	if err := youtube.Authorize(ctx, cfg, channel, secretsPath, tokenPath); err != nil {
		return fmt.Errorf("authorization failed: %w", err)
	}

	targetToken := cfg.ResolveChannelTokenFile(channel)
	chDisplay := channel
	if chDisplay == "" {
		chDisplay = "default"
	}

	fmt.Println("\n✔ Successfully authorized with YouTube!")
	fmt.Printf("Channel:     %s\n", chDisplay)
	fmt.Printf("Token saved: %s\n", targetToken)
	fmt.Println("Config updated: ~/.config/talk_cut/config.json")
	return nil
}

// runYouTube dispatches youtube subcommands (setup, auth, status).
func runYouTube(args []string) error {
	if len(args) == 0 {
		printYouTubeUsage()
		return nil
	}

	switch args[0] {
	case "setup":
		return runYouTubeSetup(args[1:])
	case "auth":
		return runAuth(args[1:])
	case "status":
		return runYouTubeStatus(args[1:])
	case "-H", "--guide":
		youtube.PrintDetailedSetupGuide(os.Stdout)
		return nil
	case "-h", "--help", "help":
		printYouTubeUsage()
		return nil
	default:
		return fmt.Errorf("unknown youtube command %q. Run 'talk_cut youtube --help' for usage", args[0])
	}
}

// runYouTubeSetup runs the interactive guided YouTube setup flow or prints detailed help.
func runYouTubeSetup(args []string) error {
	fs := flag.NewFlagSet("talk_cut youtube setup", flag.ContinueOnError)
	var showDetailedHelp, showHelp, nonInteractive bool
	var channel, secretsPath, tokenPath string

	fs.BoolVar(&showDetailedHelp, "H", false, "Display comprehensive step-by-step YouTube setup guide")
	fs.BoolVar(&showHelp, "h", false, "Display brief help")
	fs.BoolVar(&showHelp, "help", false, "Display brief help")
	fs.BoolVar(&nonInteractive, "non-interactive", false, "Fail instead of prompting if input is missing")
	fs.StringVar(&channel, "channel", "", "Target YouTube channel profile name")
	fs.StringVar(&secretsPath, "secrets", "", "Path to Google Cloud client secrets JSON")
	fs.StringVar(&tokenPath, "token", "", "Path to store OAuth token")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if showDetailedHelp {
		youtube.PrintDetailedSetupGuide(os.Stdout)
		return nil
	}
	if showHelp {
		printYouTubeSetupUsage()
		return nil
	}

	if fs.NArg() > 0 && secretsPath == "" {
		secretsPath = fs.Arg(0)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	opts := youtube.SetupOptions{
		Channel:        channel,
		SecretsPath:    secretsPath,
		TokenPath:      tokenPath,
		NonInteractive: nonInteractive,
	}

	return youtube.RunGuidedSetup(context.Background(), os.Stdin, os.Stdout, cfg, opts, youtube.Authorize)
}

// runYouTubeStatus displays the current YouTube credentials and channel tokens.
func runYouTubeStatus(args []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	return youtube.RunYouTubeStatus(os.Stdout, cfg)
}

// printYouTubeUsage outputs CLI usage for the youtube subcommand group.
func printYouTubeUsage() {
	fmt.Println("Usage:")
	fmt.Println("  talk_cut youtube setup [-H]      Interactive guided YouTube setup (-H for detailed guide)")
	fmt.Println("  talk_cut youtube auth [options]  Direct OAuth browser authorization")
	fmt.Println("  talk_cut youtube status          Inspect configured YouTube channels and tokens")
	fmt.Println("\nRun 'talk_cut youtube setup -H' for the full step-by-step setup guide.")
}

// printYouTubeSetupUsage outputs CLI usage for the youtube setup command.
func printYouTubeSetupUsage() {
	fmt.Println("Usage: talk_cut youtube setup [options] [client_secrets.json]")
	fmt.Println("\nOptions:")
	fmt.Println("  -H                     Display comprehensive step-by-step YouTube setup guide")
	fmt.Println("  --channel <name>       Target YouTube channel profile name (default: default)")
	fmt.Println("  --secrets <path>       Path to Google Cloud client secrets JSON")
	fmt.Println("  --token <path>         Path to store OAuth token")
	fmt.Println("  --non-interactive      Fail instead of prompting if input is missing")
	fmt.Println("  -h, --help             Show this help screen")
}

// isURL checks if a string begins with http:// or https://.
func isURL(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
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
	fs.BoolVar(&opts.metaOnly, "meta-only", false, "Fetch talk metadata from URL, save talk_meta.json, and exit")
	fs.BoolVar(&opts.upload, "upload", false, "Upload cut video to YouTube upon completion")
	fs.BoolVar(&opts.reDetect, "re-detect", false, "Force re-running AI cut detection even if talk_cuts.json exists")
	fs.StringVar(&opts.channel, "channel", "", "Target YouTube channel profile name")
	fs.StringVar(&opts.keyFile, "key-file", "", "Custom path to auth key file")
	fs.StringVar(&opts.model, "model", "", "OpenRouter model name")
	fs.BoolVar(&opts.showVersion, "v", false, "Print version and exit")
	fs.BoolVar(&opts.showVersion, "version", false, "Print version and exit")
	fs.BoolVar(&opts.showHelp, "h", false, "Print help and exit")
	fs.BoolVar(&opts.showHelp, "help", false, "Print help and exit")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	for i := 0; i < fs.NArg(); i++ {
		arg := fs.Arg(i)
		if isURL(arg) && opts.url == "" {
			opts.url = arg
		} else if opts.dir == "" {
			opts.dir = arg
		} else if opts.output == "" && !isURL(arg) {
			opts.output = arg
		}
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
	if model.HasSavedCuts(opts.dir) && !opts.reDetect {
		if savedCuts, loadErr := model.LoadCutsFile(opts.dir); loadErr == nil {
			model.ApplyCutsToCues(cues, savedCuts)
			cutsPath := filepath.Join(opts.dir, model.CutsFileName)
			fmt.Printf("Loaded %d saved cut intervals from %s\n", len(savedCuts), cutsPath)
		}
	} else if !opts.noAI {
		cues = runAICutDetection(ctx, opts.dir, cfg, cues, &talkMeta)
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

// runMetaOnly fetches metadata from URL and saves talk_meta.json without launching TUI.
func runMetaOnly(ctx context.Context, opts *cliOptions, cfg config.Config) error {
	info, err := os.Stat(opts.dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%q is not a valid directory", opts.dir)
	}
	if opts.url == "" && !model.HasSavedMetadata(opts.dir) {
		return fmt.Errorf("no URL provided to update metadata (usage: talk_cut --meta-only <dir> <url>)")
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

// initialMetadata constructs baseline metadata, optionally scraping the seminar web page.
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
		info, err := metadata.FetchTalkInfo(ctx, opts.url)
		if err == nil {
			applyTalkInfoToMetadata(&meta, info)
			fmt.Printf("✔ Fetched talk metadata from %s\n", opts.url)
			if meta.Speaker != "" {
				fmt.Printf("  Speaker: %s", meta.Speaker)
				if meta.Affiliation != "" {
					fmt.Printf(" (%s)", meta.Affiliation)
				}
				fmt.Println()
			}
			if meta.Title != "" {
				fmt.Printf("  Title:   %s\n", meta.Title)
			}
		} else {
			fmt.Fprintf(os.Stderr, "warning: fetching %s: %v\n", opts.url, err)
		}
		if saveErr := model.SaveMetaFile(opts.dir, meta); saveErr == nil {
			metaPath := filepath.Join(opts.dir, model.MetaFileName)
			fmt.Printf("  Saved talk metadata to %s\n", metaPath)
		}
	}
	return meta
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

// runAICutDetection queries OpenRouter to detect candidate cuts and enrich metadata.
func runAICutDetection(ctx context.Context, dir string, cfg config.Config, cues []model.SubtitleCue, meta *model.TalkMetadata) []model.SubtitleCue {
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
			if len(aiMeta.Tags) > 0 && len(meta.Tags) == 0 {
				meta.Tags = aiMeta.Tags
			}
			if len(aiMeta.Chapters) > 0 {
				meta.Chapters = aiMeta.Chapters
			}
			_ = model.SaveMetaFile(dir, *meta)
		}
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
func printHelp() {
	fmt.Printf("talk_cut v%s - Interactive Talk Trimmer & YouTube Publisher\n\n", Version)
	fmt.Println("Usage:")
	fmt.Println("  talk_cut [options] <recording-directory> [announcement-url]")
	fmt.Println("  talk_cut youtube setup [-H]            Interactive guided setup (-H for detailed guide)")
	fmt.Println("  talk_cut youtube status                Inspect configured YouTube channels and tokens")
	fmt.Println("  talk_cut auth [options] [secrets.json] Direct OAuth browser authorization")
	fmt.Println("\nOptions:")
	fmt.Println("  -o, --output <path>    Custom output destination for sliced video")
	fmt.Println("  -u, --url <url>        Seminar announcement URL (extracts speaker, title, abstract)")
	fmt.Println("  --meta-only            Fetch talk metadata from URL, save talk_meta.json, and exit")
	fmt.Println("  --layout <type>        Preferred layout: slides (default), clean, speaker, gallery")
	fmt.Println("  --no-ai                Skip AI LLM cut and chapter detection")
	fmt.Println("  --re-detect            Force re-running AI cut & chapter detection even if saved files exist")
	fmt.Println("  --dry-run              Analyze and print cut plan without opening TUI")
	fmt.Println("  --upload               Upload cut video to YouTube upon completion")
	fmt.Println("  --channel <name>       Target YouTube channel (stores/loads ~/.config/auth/youtube_<channel>.json)")
	fmt.Println("  --key-file <path>      Path to OpenRouter API key file (default: ~/.config/auth/openrouter_api_key)")
	fmt.Println("  --model <name>         OpenRouter model name (default: google/gemini-2.5-flash-lite)")
	fmt.Println("  -v, --version          Print version information")
	fmt.Println("  -h, --help             Show this help screen")
}
