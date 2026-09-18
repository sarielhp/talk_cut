package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"talk_cut/internal/config"
	"talk_cut/internal/model"
	"talk_cut/internal/youtube"
)

// runYouTubePlaylist routes playlist subcommands (list, create, add, set-default).
func runYouTubePlaylist(args []string) error {
	if len(args) == 0 {
		printPlaylistHelp()
		return nil
	}

	switch args[0] {
	case "list", "ls":
		return runPlaylistList(args[1:])
	case "create", "new":
		return runPlaylistCreate(args[1:])
	case "set-default", "default":
		return runPlaylistSetDefault(args[1:])
	case "add":
		return runPlaylistAdd(args[1:])
	case "remove", "rm":
		return runPlaylistRemove(args[1:])
	case "-h", "--help", "help":
		printPlaylistHelp()
		return nil
	default:
		return fmt.Errorf("unknown playlist subcommand %q; run 'talk_cut youtube playlist --help' for usage", args[0])
	}
}

// runPlaylistList queries and displays all playlists owned by the authenticated channel.
func runPlaylistList(args []string) error {
	fs := flag.NewFlagSet("talk_cut youtube playlist list", flag.ContinueOnError)
	var channel string
	fs.StringVar(&channel, "channel", "", "Target YouTube channel profile name")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	chName := channel
	if chName == "" {
		chName = cfg.DefaultChannel
	}

	ctx := context.Background()
	client, err := youtube.GetAuthenticatedClient(ctx, cfg, chName)
	if err != nil {
		return fmt.Errorf("authenticating for channel %q: %w", chName, err)
	}

	playlists, err := youtube.ListPlaylists(ctx, client)
	if err != nil {
		return fmt.Errorf("listing playlists: %w", err)
	}

	if len(playlists) == 0 {
		fmt.Printf("No playlists found on YouTube channel %q.\n", chName)
		return nil
	}

	fmt.Printf("=== YouTube Playlists (%s) ===\n", chName)
	for _, pl := range playlists {
		defaultBadge := ""
		if cfg.DefaultPlaylist != "" && (cfg.DefaultPlaylist == pl.ID || strings.EqualFold(cfg.DefaultPlaylist, pl.Title)) {
			defaultBadge = " [DEFAULT]"
		}
		fmt.Printf("  • [%s] %s (%d videos, %s)%s\n", pl.ID, pl.Title, pl.ItemCount, pl.PrivacyStatus, defaultBadge)
	}
	return nil
}

// runPlaylistCreate creates a new playlist on YouTube and optionally sets it as default in config.
func runPlaylistCreate(args []string) error {
	fs := flag.NewFlagSet("talk_cut youtube playlist create", flag.ContinueOnError)
	var title, desc, privacy, channel string
	var makeDefault bool

	fs.StringVar(&title, "title", "", "Playlist title")
	fs.StringVar(&desc, "description", "", "Playlist description")
	fs.StringVar(&desc, "desc", "", "Playlist description")
	fs.StringVar(&privacy, "privacy", "public", "Playlist privacy (public, unlisted, private)")
	fs.StringVar(&channel, "channel", "", "Target YouTube channel profile name")
	fs.BoolVar(&makeDefault, "default", false, "Set as default playlist in config")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if title == "" && fs.NArg() > 0 {
		title = fs.Arg(0)
	}
	if title == "" {
		return fmt.Errorf("playlist title is required; specify --title \"<name>\" or pass as argument")
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	chName := channel
	if chName == "" {
		chName = cfg.DefaultChannel
	}

	ctx := context.Background()
	client, err := youtube.GetAuthenticatedClient(ctx, cfg, chName)
	if err != nil {
		return fmt.Errorf("authenticating for channel %q: %w", chName, err)
	}

	pl, err := youtube.CreatePlaylist(ctx, client, title, desc, privacy)
	if err != nil {
		return fmt.Errorf("creating playlist %q: %w", title, err)
	}

	fmt.Printf("✔ Playlist created on YouTube:\n")
	fmt.Printf("  ID:      %s\n", pl.ID)
	fmt.Printf("  Title:   %s\n", pl.Title)
	fmt.Printf("  Privacy: %s\n", pl.PrivacyStatus)
	fmt.Printf("  URL:     https://www.youtube.com/playlist?list=%s\n", pl.ID)

	if makeDefault {
		cfg.DefaultPlaylist = pl.ID
		if err := config.SaveConfig(cfg); err != nil {
			return fmt.Errorf("saving default playlist to config: %w", err)
		}
		fmt.Printf("✔ Set %q as default playlist in ~/.config/talk_cut/config.json\n", pl.Title)
	}

	return nil
}

// runPlaylistSetDefault searches channel playlists and writes the chosen playlist to ~/.config/talk_cut/config.json.
func runPlaylistSetDefault(args []string) error {
	fs := flag.NewFlagSet("talk_cut youtube playlist set-default", flag.ContinueOnError)
	var channel string
	fs.StringVar(&channel, "channel", "", "Target YouTube channel profile name")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("please provide a playlist ID or title: talk_cut youtube playlist set-default <id|title>")
	}
	target := fs.Arg(0)

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	chName := channel
	if chName == "" {
		chName = cfg.DefaultChannel
	}

	ctx := context.Background()
	client, err := youtube.GetAuthenticatedClient(ctx, cfg, chName)
	if err != nil {
		return fmt.Errorf("authenticating for channel %q: %w", chName, err)
	}

	pl, err := youtube.FindPlaylist(ctx, client, target)
	if err != nil {
		return fmt.Errorf("finding playlist %q: %w", target, err)
	}

	cfg.DefaultPlaylist = pl.ID
	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("saving default playlist to config: %w", err)
	}

	fmt.Printf("✔ Default playlist set to %q [%s] in ~/.config/talk_cut/config.json\n", pl.Title, pl.ID)
	return nil
}

// runPlaylistAdd inserts a talk video into a playlist.
func runPlaylistAdd(args []string) error {
	fs := flag.NewFlagSet("talk_cut youtube playlist add", flag.ContinueOnError)
	var channel string
	fs.StringVar(&channel, "channel", "", "Target YouTube channel profile name")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if fs.NArg() < 2 {
		return fmt.Errorf("usage: talk_cut youtube playlist add <playlist-id-or-title> <recording-dir-or-videoid>")
	}
	playlistTarget := fs.Arg(0)
	videoTarget := fs.Arg(1)

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	chName := channel
	if chName == "" {
		chName = cfg.DefaultChannel
	}

	videoID, dirToUpdate := resolveVideoIDFromTarget(videoTarget)
	if videoID == "" {
		return fmt.Errorf("could not determine YouTube video ID from %q", videoTarget)
	}

	ctx := context.Background()
	client, err := youtube.GetAuthenticatedClient(ctx, cfg, chName)
	if err != nil {
		return fmt.Errorf("authenticating for channel %q: %w", chName, err)
	}

	pl, err := youtube.FindPlaylist(ctx, client, playlistTarget)
	if err != nil {
		return fmt.Errorf("finding playlist %q: %w", playlistTarget, err)
	}

	if err := youtube.AddVideoToPlaylist(ctx, client, pl.ID, videoID); err != nil {
		return fmt.Errorf("adding video %s to playlist %q: %w", videoID, pl.Title, err)
	}

	fmt.Printf("✔ Video %s added to playlist %q [%s]\n", videoID, pl.Title, pl.ID)

	if dirToUpdate != "" {
		if meta, err := model.LoadMetaFile(dirToUpdate); err == nil {
			meta.AddPlaylist(pl.ID, pl.Title)
			_ = model.SaveMetaFile(dirToUpdate, meta)
			fmt.Printf("✔ Saved playlist to %s\n", model.MetaFileName)
		}
	}

	return nil
}

// runPlaylistRemove removes a talk video from a playlist.
func runPlaylistRemove(args []string) error {
	fs := flag.NewFlagSet("talk_cut youtube playlist remove", flag.ContinueOnError)
	var channel string
	fs.StringVar(&channel, "channel", "", "Target YouTube channel profile name")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if fs.NArg() < 2 {
		return fmt.Errorf("usage: talk_cut youtube playlist remove <playlist-id-or-title> <recording-dir-or-videoid>")
	}
	playlistTarget := fs.Arg(0)
	videoTarget := fs.Arg(1)

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	chName := channel
	if chName == "" {
		chName = cfg.DefaultChannel
	}

	videoID, dirToUpdate := resolveVideoIDFromTarget(videoTarget)
	if videoID == "" {
		return fmt.Errorf("could not determine YouTube video ID from %q", videoTarget)
	}

	ctx := context.Background()
	client, err := youtube.GetAuthenticatedClient(ctx, cfg, chName)
	if err != nil {
		return fmt.Errorf("authenticating for channel %q: %w", chName, err)
	}

	pl, err := youtube.FindPlaylist(ctx, client, playlistTarget)
	if err != nil {
		return fmt.Errorf("finding playlist %q: %w", playlistTarget, err)
	}

	if err := youtube.RemoveVideoFromPlaylist(ctx, client, pl.ID, videoID); err != nil {
		return fmt.Errorf("removing video %s from playlist %q: %w", videoID, pl.Title, err)
	}

	fmt.Printf("✔ Video %s removed from playlist %q [%s]\n", videoID, pl.Title, pl.ID)

	if dirToUpdate != "" {
		if meta, err := model.LoadMetaFile(dirToUpdate); err == nil {
			meta.RemovePlaylist(pl.ID)
			_ = model.SaveMetaFile(dirToUpdate, meta)
			fmt.Printf("✔ Updated playlists in %s\n", model.MetaFileName)
		}
	}

	return nil
}

// resolveVideoIDFromTarget extracts the video ID from an ID string, URL, or recording directory.
func resolveVideoIDFromTarget(target string) (videoID string, dir string) {
	target = strings.TrimSpace(target)
	if stat, err := os.Stat(target); err == nil && stat.IsDir() {
		dir = target
		if meta, err := model.LoadMetaFile(target); err == nil {
			videoID = meta.EffectiveYouTubeID()
		}
		return videoID, dir
	}
	videoID = model.ExtractYouTubeID(target)
	return videoID, ""
}

// printPlaylistHelp outputs usage instructions for youtube playlist commands.
func printPlaylistHelp() {
	fmt.Println("Usage:")
	fmt.Println("  talk_cut youtube playlist list [options]                        List channel playlists")
	fmt.Println("  talk_cut youtube playlist create [options] <title>              Create a new playlist on YouTube")
	fmt.Println("  talk_cut youtube playlist set-default [options] <id|title>      Set default playlist in config")
	fmt.Println("  talk_cut youtube playlist add [options] <id|title> <dir|id>     Add video to a playlist")
	fmt.Println("  talk_cut youtube playlist remove [options] <id|title> <dir|id>  Remove video from a playlist")
	fmt.Println("\nOptions:")
	fmt.Println("  --channel <name>       Target YouTube channel profile name")
	fmt.Println("  --privacy <type>       Privacy for new playlist (public, unlisted, private)")
	fmt.Println("  --description <desc>   Description for new playlist")
	fmt.Println("  --default              Set newly created playlist as default in config")
}
