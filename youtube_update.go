package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"talk_cut/internal/bundle"
	"talk_cut/internal/config"
	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
	"talk_cut/internal/youtube"
)

// maxDurationDelta is the maximum allowed difference between local and YouTube video duration (+/- 1s).
const maxDurationDelta = 1050 * time.Millisecond

// runYouTubeUpdate performs pre-flight verification (video existence and length agreement)
// and updates the YouTube video metadata (title, description with abstract & chapters, tags, privacy).
func runYouTubeUpdate(ctx context.Context, opts *cliOptions, cfg config.Config) error {
	info, err := os.Stat(opts.dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%q is not a valid directory", opts.dir)
	}

	channel := resolveTargetChannel(opts, cfg)
	client, err := authenticateChannelClient(ctx, cfg, channel)
	if err != nil {
		return err
	}

	meta := initialMetadata(ctx, opts, cfg)
	videoID, err := resolveTalkVideoID(ctx, client, opts.dir, meta)
	if err != nil {
		return err
	}

	ytVer, err := youtube.VerifyVideo(ctx, client, videoID)
	if err != nil {
		return fmt.Errorf("verifying video %s on YouTube: %w", videoID, err)
	}

	localDur, localSource, err := resolveLocalVideoDuration(ctx, opts, cfg)
	if err != nil {
		return fmt.Errorf("determining local video duration: %w", err)
	}

	if err := verifyVideoLengthAgreement(localDur, localSource, ytVer.Duration, videoID); err != nil {
		return err
	}

	printUpdatePreflight(channel, videoID, ytVer, localDur, localSource)

	if opts.dryRun {
		fmt.Println("\n[Dry run] Pre-flight verification passed; skipping YouTube update.")
		return nil
	}

	fmt.Println("\nUpdating YouTube video details...")
	updatedVer, err := youtube.UpdateVideoMetadata(ctx, client, videoID, meta)
	if err != nil {
		return fmt.Errorf("updating YouTube video details: %w", err)
	}

	meta.YouTubeID = updatedVer.VideoID
	meta.YouTubeURL = updatedVer.ShortURL
	if meta.YouTubeURL == "" {
		meta.YouTubeURL = fmt.Sprintf("https://youtu.be/%s", updatedVer.VideoID)
	}
	_ = model.SaveMetaFile(opts.dir, meta)

	var targetPlaylists []string
	if opts.playlist != "" {
		for _, p := range strings.Split(opts.playlist, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				targetPlaylists = append(targetPlaylists, trimmed)
			}
		}
	}
	for _, pl := range meta.AllPlaylists() {
		targetPlaylists = append(targetPlaylists, pl.ID)
	}
	if len(targetPlaylists) == 0 && cfg.DefaultPlaylist != "" {
		targetPlaylists = append(targetPlaylists, cfg.DefaultPlaylist)
	}
	if len(targetPlaylists) > 0 {
		syncVideoPlaylists(ctx, client, targetPlaylists, videoID, &meta, opts.dir)
	}

	printUpdateSuccess(meta, channel, updatedVer)
	return nil
}

// syncVideoPlaylists adds the video to all targeted playlists.
func syncVideoPlaylists(ctx context.Context, client *http.Client, targets []string, videoID string, meta *model.TalkMetadata, dir string) {
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		pl, err := youtube.FindPlaylist(ctx, client, target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: playlist %q: %v\n", target, err)
			continue
		}
		if err := youtube.AddVideoToPlaylist(ctx, client, pl.ID, videoID); err != nil {
			fmt.Fprintf(os.Stderr, "warning: adding to playlist %q: %v\n", pl.Title, err)
			continue
		}
		meta.AddPlaylist(pl.ID, pl.Title)
		fmt.Printf("✔ Added video to playlist: %s [%s]\n", pl.Title, pl.ID)
	}
	if dir != "" {
		_ = model.SaveMetaFile(dir, *meta)
	}
}

// resolveTalkVideoID locates the video ID from metadata or by querying recent uploads on the channel.
func resolveTalkVideoID(ctx context.Context, client *http.Client, dir string, meta model.TalkMetadata) (string, error) {
	if vid := meta.EffectiveYouTubeID(); vid != "" {
		return vid, nil
	}

	ver, err := youtube.FindChannelVideoForTalk(ctx, client, meta.Title, filepath.Base(dir))
	if err == nil && ver != nil && ver.VideoID != "" {
		return ver.VideoID, nil
	}

	return "", fmt.Errorf("no YouTube video ID found in %s and no matching upload discovered on channel", filepath.Join(dir, model.MetaFileName))
}

// resolveLocalVideoDuration probes local video files to obtain the talk's runtime duration.
func resolveLocalVideoDuration(ctx context.Context, opts *cliOptions, cfg config.Config) (time.Duration, string, error) {
	if opts.output != "" {
		if stat, err := os.Stat(opts.output); err == nil && !stat.IsDir() {
			pInfo, err := cutter.ProbeMedia(ctx, opts.output)
			if err != nil {
				return 0, "", fmt.Errorf("probing %q: %w", opts.output, err)
			}
			return pInfo.Duration, filepath.Base(opts.output), nil
		}
	}

	b, bErr := bundle.DiscoverBundle(opts.dir, cfg.PreferredLayout)
	if bErr == nil && b != nil && b.PrimaryVideo != "" {
		cutPath := resolveOutputPath(opts.dir, b.PrimaryVideo, opts.output)
		if stat, err := os.Stat(cutPath); err == nil && !stat.IsDir() && stat.Size() > 0 {
			pInfo, err := cutter.ProbeMedia(ctx, cutPath)
			if err != nil {
				return 0, "", fmt.Errorf("probing %q: %w", cutPath, err)
			}
			return pInfo.Duration, filepath.Base(cutPath), nil
		}

		pInfo, err := cutter.ProbeMedia(ctx, b.PrimaryVideo)
		if err == nil && pInfo.Duration > 0 {
			if model.HasSavedCuts(opts.dir) {
				cuts, err := model.LoadCutsFile(opts.dir)
				if err == nil && len(cuts) > 0 {
					kept := calculateKeptDuration(pInfo.Duration, cuts)
					return kept, fmt.Sprintf("%s (minus cuts)", filepath.Base(b.PrimaryVideo)), nil
				}
			}
			return pInfo.Duration, filepath.Base(b.PrimaryVideo), nil
		}
	}

	return findFallbackCutVideo(ctx, opts.dir)
}

// calculateKeptDuration calculates the total duration retained after subtracting cut intervals.
func calculateKeptDuration(totalDur time.Duration, cuts []model.CutInterval) time.Duration {
	intervals := cutter.GetKeptIntervals(totalDur, cuts)
	var sum time.Duration
	for _, inv := range intervals {
		sum += inv.End - inv.Start
	}
	return sum
}

// findFallbackCutVideo looks for any cut video in directory.
func findFallbackCutVideo(ctx context.Context, dir string) (time.Duration, string, error) {
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), "_cut.mp4") {
				p := filepath.Join(dir, e.Name())
				pInfo, err := cutter.ProbeMedia(ctx, p)
				if err == nil && pInfo.Duration > 0 {
					return pInfo.Duration, e.Name(), nil
				}
			}
		}
	}
	return 0, "", fmt.Errorf("no cut video or media file found in %q to verify duration", dir)
}

// verifyVideoLengthAgreement ensures local and YouTube durations match within the allowed delta (+/- 1s).
func verifyVideoLengthAgreement(localDur time.Duration, localSource string, ytDur time.Duration, videoID string) error {
	if ytDur <= 0 {
		return fmt.Errorf("YouTube video %s reports zero duration; cannot verify length agreement", videoID)
	}
	if localDur <= 0 {
		return fmt.Errorf("local video reports zero duration; cannot verify length agreement")
	}

	diff := time.Duration(math.Abs(float64(localDur - ytDur)))
	if diff > maxDurationDelta {
		return fmt.Errorf(
			"video length mismatch: local video (%s) is %s (%.2fs), but YouTube video %s is %s (%.2fs) [diff: %.2fs > 1s threshold]; aborting update",
			localSource,
			formatMMSS(localDur),
			localDur.Seconds(),
			videoID,
			formatMMSS(ytDur),
			ytDur.Seconds(),
			diff.Seconds(),
		)
	}

	return nil
}

// formatMMSS formats a time.Duration into MM:SS.
func formatMMSS(d time.Duration) string {
	totalSec := int(math.Round(d.Seconds()))
	if totalSec < 0 {
		totalSec = 0
	}
	m := totalSec / 60
	s := totalSec % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

// printUpdatePreflight displays the verification report before applying updates.
func printUpdatePreflight(channel, videoID string, ytVer *youtube.VideoVerification, localDur time.Duration, localSource string) {
	diff := time.Duration(math.Abs(float64(localDur - ytVer.Duration)))
	fmt.Println("=== YouTube Video Verification ===")
	fmt.Printf("  Channel:        %s\n", channel)
	fmt.Printf("  Video ID:       %s\n", videoID)
	fmt.Printf("  YouTube Title:  %s\n", ytVer.Title)
	fmt.Printf("  YouTube Length: %s (%.2fs)\n", formatMMSS(ytVer.Duration), ytVer.Duration.Seconds())
	fmt.Printf("  Local Source:   %s\n", localSource)
	fmt.Printf("  Local Length:   %s (%.2fs)\n", formatMMSS(localDur), localDur.Seconds())
	fmt.Printf("  Difference:     %.2fs (within +/- 1s threshold)\n", diff.Seconds())
	fmt.Printf("  URL:            %s\n", ytVer.ShortURL)
	fmt.Println("✔ Video verified on YouTube and length matches.")
}

// printUpdateSuccess displays the final update confirmation.
func printUpdateSuccess(meta model.TalkMetadata, channel string, ver *youtube.VideoVerification) {
	fmt.Println("\n================================================================================")
	fmt.Println("✔ SUCCESS: YouTube video details updated!")
	fmt.Println("================================================================================")
	fmt.Printf("  Title:      %s\n", meta.Title)
	if meta.Speaker != "" {
		fmt.Printf("  Speaker:    %s\n", meta.Speaker)
	}
	fmt.Printf("  Channel:    %s\n", channel)
	fmt.Printf("  Privacy:    %s\n", strings.ToLower(meta.Privacy))
	fmt.Printf("  Video URL:  %s\n", ver.ShortURL)
	pls := meta.AllPlaylists()
	if len(pls) == 1 {
		fmt.Printf("  Playlist:   %s\n", pls[0].Title)
	} else if len(pls) > 1 {
		var titles []string
		for _, pl := range pls {
			titles = append(titles, pl.Title)
		}
		fmt.Printf("  Playlists:  %s\n", strings.Join(titles, ", "))
	}
	fmt.Printf("  Chapters:   %d markers synchronized\n", len(meta.Chapters))
	fmt.Println("================================================================================")
}
