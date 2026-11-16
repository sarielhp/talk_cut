package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"talk_cut/internal/config"
	"talk_cut/internal/model"
	"talk_cut/internal/ui"
	"talk_cut/internal/youtube"

	tea "github.com/charmbracelet/bubbletea"
)

// runPlaylistList queries and displays all playlists owned by the authenticated channel.
func runPlaylistListWith(channel string) error {

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
func runPlaylistCreateWith(title, desc, privacy, channel string, makeDefault bool) error {

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
func runPlaylistSetDefaultWith(target, channel string) error {

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
func runPlaylistAddWith(playlistTarget, videoTarget, channel string) error {

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
func runPlaylistRemoveWith(playlistTarget, videoTarget, channel string) error {

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

// runPlaylistTidyWith removes repeated videos from playlists and orders them newest first on YouTube.
func runPlaylistTidyWith(targets []string, all, dryRun bool, channel string) error {
	if len(targets) == 0 && !all {
		return fmt.Errorf("name a playlist, or pass --all to tidy every playlist on the channel")
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
	playlists, err := tidyTargets(ctx, client, targets, all)
	if err != nil {
		return err
	}
	for _, pl := range playlists {
		if err := tidyOne(ctx, client, pl, dryRun); err != nil {
			return fmt.Errorf("tidying playlist %q: %w", pl.Title, err)
		}
	}
	return nil
}

func tidyTargets(ctx context.Context, client *http.Client, targets []string, all bool) ([]youtube.Playlist, error) {
	if all {
		return youtube.ListPlaylists(ctx, client)
	}
	var out []youtube.Playlist
	for _, t := range targets {
		pl, err := youtube.FindPlaylist(ctx, client, t)
		if err != nil {
			return nil, fmt.Errorf("finding playlist %q: %w", t, err)
		}
		out = append(out, *pl)
	}
	return out, nil
}

func tidyOne(ctx context.Context, client *http.Client, pl youtube.Playlist, dryRun bool) error {
	fmt.Printf("=== %s [%s] ===\n", pl.Title, pl.ID)
	report := func(line string) { fmt.Printf("  … %s\n", line) }
	res, err := youtube.TidyPlaylist(ctx, client, pl.ID, dryRun, report)
	if err != nil {
		return err
	}
	if len(res.Duplicates) == 0 && len(res.Moves) == 0 {
		fmt.Printf("  ✔ Already tidy (%d videos)\n", res.Items)
		return nil
	}
	if !dryRun {
		fmt.Printf("  ✔ Removed %d duplicate(s), moved %d item(s); %d videos, newest first\n",
			len(res.Duplicates), len(res.Moves), len(res.Final))
		return nil
	}
	fmt.Printf("  Would remove %d duplicate(s) and move %d item(s):\n", len(res.Duplicates), len(res.Moves))
	for _, d := range res.Duplicates {
		fmt.Printf("    - duplicate: %s\n", d.Title)
	}
	for i, it := range res.Final {
		fmt.Printf("    %3d. %s  %s\n", i+1, dateOnly(it.VideoPublishedAt), it.Title)
	}
	return nil
}

func dateOnly(ts string) string {
	if len(ts) < 10 {
		return "----------"
	}
	return ts[:10]
}

// runPlaylistEditor opens the playlists panel on its own.
func runPlaylistEditor(channel string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	_, err = tea.NewProgram(ui.NewPlaylistEditor(cfg, channel), tea.WithAltScreen()).Run()
	return err
}
