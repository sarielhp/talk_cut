package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"talk_cut/internal/config"
	"talk_cut/internal/youtube"

	"github.com/sarielhp/clihelp"
)

// buildApp declares the command line. It replaces a hand-written dispatcher and
// four hand-maintained usage blocks: three levels of "switch args[0]", twenty-one
// flags registered by hand, and help text that had to be kept in step with them
// by whoever remembered.
//
// The root's flags are App.Options rather than PersistentOptions because they
// belong to the root alone. The youtube subcommands declare --channel nine times
// over, plus --output, --dry-run and --url; inherited, those would collide.
func buildApp(opts *cliOptions) *clihelp.App {
	return &clihelp.App{
		Name:        "talk_cut",
		Description: "Interactive talk trimmer and YouTube publisher.",
		// The "v" is part of what this program has always printed.
		Version:   "v" + Version,
		UsageLine: "talk_cut [options] <recording-directory> [announcement-url]",
		GlobalNote: "Give it a recording directory and it finds the talk, cuts it, and — with " +
			"`--upload` — publishes it.",
		Pager: true,
		// AbbrevCommands is deliberately off: the program did not accept
		// abbreviated command names before this conversion, and a conversion
		// should not quietly add behaviour. Turning it on is a one-line change.
		// The root takes positional arguments of its own, so an unrecognised
		// first word is a path, not a mistyped command — which is what the old
		// dispatcher did by falling through to the pipeline.
		Args: clihelp.MaximumNArgs(3),
		Options: []clihelp.Option{
			clihelp.String(&opts.output, "-o, --output <path>", "", "Custom output destination for the sliced video"),
			clihelp.String(&opts.url, "-u, --url <url>", "", "Seminar announcement URL; speaker, title and abstract are read from it"),
			clihelp.String(&opts.layout, "--layout <type>", "", "Preferred layout: slides (default), clean, speaker, gallery"),
			clihelp.String(&opts.channel, "--channel <name>", "", "Target YouTube channel profile"),
			clihelp.String(&opts.playlist, "--playlist <name|id>", "", "Add the video to this YouTube playlist"),
			clihelp.String(&opts.keyFile, "--key-file <path>", "", "OpenRouter API key file (default: ~/.config/auth/openrouter_api_key)"),
			clihelp.String(&opts.model, "--model <name>", "", "OpenRouter model name (default: google/gemini-2.5-flash-lite)"),
			clihelp.Bool(&opts.metaOnly, "--meta-only", false, "Fetch talk metadata from the URL, save talk_meta.json, and exit"),
			// One option, three spellings — the two aliases were separate
			// BoolVar calls writing to the same target, and were documented
			// nowhere.
			clihelp.Bool(&opts.updateYouTube, "--update-youtube, --update-info, --update-details", false,
				"Update talk details on YouTube for an already uploaded talk"),
			clihelp.Bool(&opts.noAI, "--no-ai", false, "Skip AI cut and chapter detection"),
			clihelp.Bool(&opts.reDetect, "--re-detect", false, "Re-run AI cut and chapter detection even if saved files exist"),
			clihelp.Bool(&opts.dryRun, "--dry-run", false, "Analyse and print the plan without opening the TUI or touching YouTube"),
			clihelp.Bool(&opts.upload, "--upload", false, "Upload the cut video to YouTube when it is finished"),
			clihelp.Bool(&opts.showVersion, "-v, --version", false, "Print version information"),
		},
		Examples: []clihelp.Example{
			{Line: "talk_cut ~/recordings/2026-09-19", Description: "Cut a recording, choosing the cuts in the TUI."},
			{Line: "talk_cut --upload --playlist Seminars ~/recordings/2026-09-19", Description: "Cut it and publish it to a playlist."},
			{Line: "talk_cut --meta-only -u https://example.org/seminar ~/recordings/2026-09-19", Description: "Only fetch the announcement metadata."},
		},
		Run: func(ctx *clihelp.Context) error { return runRoot(ctx, opts) },
		Commands: []clihelp.Command{
			authCmd(),
			youtubeCmd(),
			clihelp.CompletionCommand(),
		},
	}
}

// runRoot is the old run(): the positional heuristic, then the pipeline the
// flags selected.
func runRoot(ctx *clihelp.Context, opts *cliOptions) error {
	// Positional arguments are assigned by what they look like, not by
	// position — a URL is a URL wherever it appears. Kept exactly as it was.
	for _, arg := range ctx.Args {
		switch {
		case isURL(arg) && opts.url == "":
			opts.url = arg
		case opts.dir == "":
			opts.dir = arg
		case opts.output == "" && !isURL(arg):
			opts.output = arg
		}
	}

	if opts.showVersion {
		fmt.Fprintf(ctx.Stdout, "talk_cut v%s\n", Version)
		return nil
	}
	if opts.dir == "" {
		ctx.App.RenderGlobal(clihelp.Options{Writer: ctx.Stdout})
		return nil
	}

	if opts.metaOnly || opts.upload || opts.updateYouTube {
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		if opts.metaOnly {
			return runMetaOnly(context.Background(), opts, cfg)
		}
		applyConfigOverrides(&cfg, opts)
		if opts.upload {
			return runUploadPipeline(context.Background(), opts, cfg)
		}
		return runYouTubeUpdate(context.Background(), opts, cfg)
	}

	return executePipeline(opts)
}

func authCmd() clihelp.Command {
	var channel, secrets, token string
	return clihelp.Command{
		Name:        "auth",
		Description: "Authorize with YouTube through a browser.",
		UsageLine:   "talk_cut auth [options] [secrets.json]",
		Parameters: []clihelp.Param{
			{Name: "[secrets.json]", Description: "Google Cloud client secrets file; the same as --secrets."},
		},
		Args: clihelp.MaximumNArgs(1),
		Options: []clihelp.Option{
			clihelp.String(&channel, "--channel <name>", "", "YouTube channel name (e.g. seminar, course, personal)"),
			clihelp.String(&secrets, "--secrets <path>", "", "Path to the Google Cloud client secrets JSON"),
			clihelp.String(&token, "--token <path>", "", "Where to store the OAuth token (default: ~/.config/auth/youtube_<channel>.json)"),
		},
		Examples: []clihelp.Example{
			{Line: "talk_cut auth --channel seminar", Description: "Authorize the seminar channel."},
		},
		Run: func(ctx *clihelp.Context) error {
			s := secrets
			if len(ctx.Args) > 0 && s == "" {
				s = ctx.Args[0]
			}
			return runAuthWith(ctx.Stdout, channel, s, token)
		},
	}
}

func youtubeCmd() clihelp.Command {
	var setupChannel, setupSecrets, setupToken string
	var setupGuide, setupNonInteractive bool
	var groupGuide bool
	var updChannel, updOutput string
	var updDryRun bool

	return clihelp.Command{
		Name:        "youtube",
		Description: "Manage YouTube credentials, uploads and playlists.",
		UsageLine:   "talk_cut youtube <subcommand> [options]",
		Args:        clihelp.NoArgs,
		// "-H" and "--guide" answer here as well as on "youtube setup": the old
		// dispatcher handled them at this level, and a conversion that dropped
		// them would take a working spelling away from whoever types it.
		Options: []clihelp.Option{
			clihelp.Bool(&groupGuide, "-H, --guide", false, "Print the step-by-step YouTube setup guide"),
		},
		Run: func(ctx *clihelp.Context) error {
			if groupGuide {
				youtube.PrintDetailedSetupGuide(ctx.Stdout)
				return nil
			}
			ctx.App.RenderCommand(clihelp.Options{Writer: ctx.Stdout}, "youtube")
			return nil
		},
		Subcommands: []clihelp.Command{
			{
				Name:        "setup",
				Description: "Interactive guided setup for a YouTube channel.",
				UsageLine:   "talk_cut youtube setup [options] [secrets.json]",
				Parameters: []clihelp.Param{
					{Name: "[secrets.json]", Description: "Google Cloud client secrets file; the same as --secrets."},
				},
				Args: clihelp.MaximumNArgs(1),
				Options: []clihelp.Option{
					clihelp.Bool(&setupGuide, "-H, --guide", false, "Print the step-by-step setup guide instead of running setup"),
					clihelp.Bool(&setupNonInteractive, "--non-interactive", false, "Fail instead of prompting when something is missing"),
					clihelp.String(&setupChannel, "--channel <name>", "", "Target YouTube channel profile"),
					clihelp.String(&setupSecrets, "--secrets <path>", "", "Path to the Google Cloud client secrets JSON"),
					clihelp.String(&setupToken, "--token <path>", "", "Where to store the OAuth token"),
				},
				Run: func(ctx *clihelp.Context) error {
					if setupGuide {
						youtube.PrintDetailedSetupGuide(ctx.Stdout)
						return nil
					}
					s := setupSecrets
					if len(ctx.Args) > 0 && s == "" {
						s = ctx.Args[0]
					}
					return runYouTubeSetupWith(setupChannel, s, setupToken, setupNonInteractive)
				},
			},
			{
				Name:        "auth",
				Description: "Authorize with YouTube through a browser.",
				UsageLine:   "talk_cut youtube auth [options] [secrets.json]",
				Args:        clihelp.MaximumNArgs(1),
				Options: []clihelp.Option{
					clihelp.String(&setupChannel, "--channel <name>", "", "YouTube channel name"),
					clihelp.String(&setupSecrets, "--secrets <path>", "", "Path to the Google Cloud client secrets JSON"),
					clihelp.String(&setupToken, "--token <path>", "", "Where to store the OAuth token"),
				},
				Run: func(ctx *clihelp.Context) error {
					s := setupSecrets
					if len(ctx.Args) > 0 && s == "" {
						s = ctx.Args[0]
					}
					return runAuthWith(ctx.Stdout, setupChannel, s, setupToken)
				},
			},
			{
				Name:        "status",
				Description: "Show the configured YouTube channels and their tokens.",
				UsageLine:   "talk_cut youtube status",
				Args:        clihelp.NoArgs,
				Run: func(ctx *clihelp.Context) error {
					cfg, err := config.LoadConfig()
					if err != nil {
						return fmt.Errorf("loading config: %w", err)
					}
					return youtube.RunYouTubeStatus(ctx.Stdout, cfg)
				},
			},
			{
				Name:        "update",
				Description: "Update a talk's details on YouTube after it has been uploaded.",
				UsageLine:   "talk_cut youtube update [options] <recording-directory>",
				Parameters: []clihelp.Param{
					{Name: "<recording-directory>", Description: "The recording directory holding the talk."},
				},
				Args: clihelp.ExactArgs(1),
				Options: []clihelp.Option{
					clihelp.String(&updChannel, "--channel <name>", "", "Target YouTube channel profile"),
					clihelp.String(&updOutput, "-o, --output <path>", "", "Path to the cut video"),
					clihelp.Bool(&updDryRun, "--dry-run", false, "Check the video exists and its duration without touching YouTube"),
				},
				Examples: []clihelp.Example{
					{Line: "talk_cut youtube update ~/recordings/2026-09-19", Description: "Refresh the title, description and chapters."},
				},
				Run: func(ctx *clihelp.Context) error {
					return runYouTubeUpdateWith(ctx.Args[0], updOutput, updChannel, updDryRun)
				},
			},
			playlistCmd(),
		},
	}
}

// runAuthWith is the body of the old runAuth, with the flag parsing taken out.
func runAuthWith(w io.Writer, channel, secretsPath, tokenPath string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if err := youtube.Authorize(context.Background(), cfg, channel, secretsPath, tokenPath); err != nil {
		return fmt.Errorf("authorization failed: %w", err)
	}

	chDisplay := channel
	if chDisplay == "" {
		chDisplay = "default"
	}
	fmt.Fprintln(w, "\n✔ Successfully authorized with YouTube!")
	fmt.Fprintf(w, "Channel:     %s\n", chDisplay)
	fmt.Fprintf(w, "Token saved: %s\n", cfg.ResolveChannelTokenFile(channel))
	fmt.Fprintln(w, "Config updated: ~/.config/talk_cut/config.json")
	return nil
}

func runYouTubeSetupWith(channel, secretsPath, tokenPath string, nonInteractive bool) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	return youtube.RunGuidedSetup(context.Background(), os.Stdin, os.Stdout, cfg, youtube.SetupOptions{
		Channel:        channel,
		SecretsPath:    secretsPath,
		TokenPath:      tokenPath,
		NonInteractive: nonInteractive,
	}, youtube.Authorize)
}

func runYouTubeUpdateWith(dir, output, channel string, dryRun bool) error {
	opts := &cliOptions{dir: dir, output: output, channel: channel, dryRun: dryRun, updateYouTube: true}
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	applyConfigOverrides(&cfg, opts)
	return runYouTubeUpdate(context.Background(), opts, cfg)
}

func playlistCmd() clihelp.Command {
	var listChannel string
	var createTitle, createDesc, createPrivacy, createChannel string
	var createDefault bool
	var setChannel, addChannel, removeChannel string

	return clihelp.Command{
		Name:        "playlist",
		Description: "List, create and populate YouTube playlists.",
		UsageLine:   "talk_cut youtube playlist <subcommand> [options]",
		Args:        clihelp.NoArgs,
		Subcommands: []clihelp.Command{
			{
				Name: "list", Aliases: []string{"ls"},
				Description: "List the playlists owned by the channel.",
				UsageLine:   "talk_cut youtube playlist list [options]",
				Args:        clihelp.NoArgs,
				Options: []clihelp.Option{
					clihelp.String(&listChannel, "--channel <name>", "", "Target YouTube channel profile"),
				},
				Examples: []clihelp.Example{{Line: "talk_cut youtube playlist list", Description: "Show every playlist."}},
				Run:      func(ctx *clihelp.Context) error { return runPlaylistListWith(listChannel) },
			},
			{
				Name: "create", Aliases: []string{"new"},
				Description: "Create a playlist on YouTube.",
				UsageLine:   "talk_cut youtube playlist create [options] <title>",
				Parameters: []clihelp.Param{
					{Name: "<title>", Description: "The playlist title; the same as --title."},
				},
				Args: clihelp.MaximumNArgs(1),
				Options: []clihelp.Option{
					clihelp.String(&createTitle, "--title <title>", "", "Playlist title"),
					clihelp.String(&createDesc, "--description, --desc <text>", "", "Playlist description"),
					clihelp.String(&createPrivacy, "--privacy <level>", "public", "Playlist privacy: public, unlisted or private"),
					clihelp.String(&createChannel, "--channel <name>", "", "Target YouTube channel profile"),
					clihelp.Bool(&createDefault, "--default", false, "Also set it as the default playlist in the config"),
				},
				Examples: []clihelp.Example{
					{Line: "talk_cut youtube playlist create Seminars --default", Description: "Create it and make it the default."},
				},
				Run: func(ctx *clihelp.Context) error {
					title := createTitle
					if len(ctx.Args) > 0 && title == "" {
						title = ctx.Args[0]
					}
					return runPlaylistCreateWith(title, createDesc, createPrivacy, createChannel, createDefault)
				},
			},
			{
				Name: "set-default", Aliases: []string{"default"},
				Description: "Record a playlist as the default in the config.",
				UsageLine:   "talk_cut youtube playlist set-default [options] <id|title>",
				Parameters: []clihelp.Param{
					{Name: "<id|title>", Description: "The playlist, by identifier or by title."},
				},
				Args: clihelp.ExactArgs(1),
				Options: []clihelp.Option{
					clihelp.String(&setChannel, "--channel <name>", "", "Target YouTube channel profile"),
				},
				Run: func(ctx *clihelp.Context) error { return runPlaylistSetDefaultWith(ctx.Args[0], setChannel) },
			},
			{
				Name:        "add",
				Description: "Add a video to a playlist.",
				UsageLine:   "talk_cut youtube playlist add [options] <playlist> <dir|id>",
				Parameters: []clihelp.Param{
					{Name: "<playlist>", Description: "The playlist, by identifier or by title."},
					{Name: "<dir|id>", Description: "A recording directory, or a YouTube video identifier."},
				},
				Args: clihelp.ExactArgs(2),
				Options: []clihelp.Option{
					clihelp.String(&addChannel, "--channel <name>", "", "Target YouTube channel profile"),
				},
				Run: func(ctx *clihelp.Context) error {
					return runPlaylistAddWith(ctx.Args[0], ctx.Args[1], addChannel)
				},
			},
			{
				Name: "remove", Aliases: []string{"rm"},
				Description: "Remove a video from a playlist.",
				UsageLine:   "talk_cut youtube playlist remove [options] <playlist> <dir|id>",
				Parameters: []clihelp.Param{
					{Name: "<playlist>", Description: "The playlist, by identifier or by title."},
					{Name: "<dir|id>", Description: "A recording directory, or a YouTube video identifier."},
				},
				Args: clihelp.ExactArgs(2),
				Options: []clihelp.Option{
					clihelp.String(&removeChannel, "--channel <name>", "", "Target YouTube channel profile"),
				},
				Run: func(ctx *clihelp.Context) error {
					return runPlaylistRemoveWith(ctx.Args[0], ctx.Args[1], removeChannel)
				},
			},
		},
	}
}
