package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"talk_cut/internal/bundle"
	"talk_cut/internal/config"
	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
	"talk_cut/internal/vtt"
	"talk_cut/internal/youtube"
)

// runUploadPipeline executes the direct CLI rendering and YouTube upload pipeline.
func runUploadPipeline(ctx context.Context, opts *cliOptions, cfg config.Config) error {
	info, err := os.Stat(opts.dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%q is not a valid directory", opts.dir)
	}

	channel := resolveTargetChannel(opts, cfg)
	client, err := authenticateChannelClient(ctx, cfg, channel)
	if err != nil {
		return err
	}

	b, _, cues, err := loadBundleAndMedia(ctx, opts, cfg)
	if err != nil {
		return err
	}

	talkMeta, cuts := prepareUploadMetadataAndCuts(ctx, opts, cfg, cues)
	outPath := resolveOutputPath(opts.dir, b.PrimaryVideo, opts.output)
	outBase := strings.TrimSuffix(outPath, filepath.Ext(outPath))
	chaptersPath := outBase + "_chapters.txt"
	vttPath := outBase + ".vtt"

	if renderErr := renderVideoIfNeeded(ctx, b.PrimaryVideo, outPath, cuts, opts.dir); renderErr != nil {
		return renderErr
	}

	writeChaptersFile(chaptersPath, talkMeta.Chapters)
	writeAdjustedVTT(vttPath, cues, cuts)

	printUploadPreflight(channel, talkMeta, outPath)

	result, err := performVideoUpload(ctx, client, outPath, talkMeta)
	if err != nil {
		return fmt.Errorf("uploading video to YouTube: %w", err)
	}

	talkMeta.YouTubeID = result.VideoID
	_ = model.SaveMetaFile(opts.dir, talkMeta)

	performCaptionUpload(ctx, client, result.VideoID, vttPath)

	var targetPlaylists []string
	if opts.playlist != "" {
		for _, p := range strings.Split(opts.playlist, ",") {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				targetPlaylists = append(targetPlaylists, trimmed)
			}
		}
	}
	for _, pl := range talkMeta.AllPlaylists() {
		targetPlaylists = append(targetPlaylists, pl.ID)
	}
	if len(targetPlaylists) == 0 && cfg.DefaultPlaylist != "" {
		targetPlaylists = append(targetPlaylists, cfg.DefaultPlaylist)
	}
	if len(targetPlaylists) > 0 {
		syncVideoPlaylists(ctx, client, targetPlaylists, result.VideoID, &talkMeta, opts.dir)
	}

	printUploadSuccess(talkMeta, channel, result.VideoURL, len(talkMeta.Chapters))
	return nil
}

// resolveTargetChannel determines the active YouTube channel profile name.
func resolveTargetChannel(opts *cliOptions, cfg config.Config) string {
	if opts.channel != "" {
		return opts.channel
	}
	if cfg.DefaultChannel != "" {
		return cfg.DefaultChannel
	}
	return "default"
}

// authenticateChannelClient verifies authentication and returns an authorized HTTP client.
func authenticateChannelClient(ctx context.Context, cfg config.Config, channel string) (*http.Client, error) {
	client, err := youtube.GetAuthenticatedClient(ctx, cfg, channel)
	if err != nil {
		return nil, fmt.Errorf("authenticating with YouTube channel %q: %w\nRun 'talk_cut youtube setup --channel %s' to authorize", channel, err, channel)
	}
	return client, nil
}

// loadBundleAndMedia loads the bundle, ffprobe media info, and subtitle cues.
func loadBundleAndMedia(ctx context.Context, opts *cliOptions, cfg config.Config) (*bundle.RecordingBundle, cutter.MediaInfo, []model.SubtitleCue, error) {
	b, err := bundle.DiscoverBundle(opts.dir, cfg.PreferredLayout)
	if err != nil {
		return nil, cutter.MediaInfo{}, nil, fmt.Errorf("discovering bundle: %w", err)
	}

	mediaInfo, err := cutter.ProbeMedia(ctx, b.PrimaryVideo)
	if err != nil {
		return nil, mediaInfo, nil, fmt.Errorf("probing video %q: %w", b.PrimaryVideo, err)
	}

	cues, err := vtt.ParseFile(b.TranscriptPath)
	if err != nil {
		return nil, mediaInfo, nil, fmt.Errorf("parsing transcript %q: %w", b.TranscriptPath, err)
	}

	return b, mediaInfo, cues, nil
}

// prepareUploadMetadataAndCuts resolves cuts and metadata and aligns chapters.
func prepareUploadMetadataAndCuts(ctx context.Context, opts *cliOptions, cfg config.Config, cues []model.SubtitleCue) (model.TalkMetadata, []model.CutInterval) {
	talkMeta := initialMetadata(ctx, opts, cfg)

	if model.HasSavedCuts(opts.dir) && !opts.reDetect {
		if savedCuts, loadErr := model.LoadCutsFile(opts.dir); loadErr == nil {
			model.ApplyCutsToCues(cues, savedCuts)
			fmt.Printf("Loaded %d saved cut intervals from %s\n", len(savedCuts), filepath.Join(opts.dir, model.CutsFileName))
		}
	} else if !opts.noAI {
		cues = runAICutDetection(ctx, cfg, cues)
		_ = model.SaveCutsFile(opts.dir, model.BuildCutIntervals(cues))
	}

	if !opts.noAI && (len(talkMeta.Chapters) == 0 || opts.reDetect) {
		runAIChapterDetection(ctx, opts.dir, cfg, cues, &talkMeta)
	}

	cuts := model.BuildCutIntervals(cues)
	aligned := cutter.AlignChaptersToKeptCues(talkMeta.Chapters, cues)
	normalized := cutter.AdjustChapters(aligned, cuts, "Introduction")
	talkMeta.Chapters = normalized
	_ = model.SaveMetaFile(opts.dir, talkMeta)

	return talkMeta, cuts
}

// renderVideoIfNeeded checks if the cut video already exists and is up to date; otherwise renders it.
func renderVideoIfNeeded(ctx context.Context, inVideo, outVideo string, cuts []model.CutInterval, recDir string) error {
	outStat, outErr := os.Stat(outVideo)
	cutsPath := filepath.Join(recDir, model.CutsFileName)
	cutsStat, cutsErr := os.Stat(cutsPath)

	needsRender := outErr != nil || outStat.Size() == 0
	if !needsRender && cutsErr == nil && cutsStat.ModTime().After(outStat.ModTime()) {
		needsRender = true
	}

	if !needsRender {
		fmt.Printf("✔ Using existing cut video: %s (%s)\n", outVideo, formatBytes(outStat.Size()))
		return nil
	}

	fmt.Printf("Slicing video losslessly with FFmpeg: %s -> %s\n", filepath.Base(inVideo), filepath.Base(outVideo))
	cutOpts := cutter.CutOptions{
		InputPath:  inVideo,
		OutputPath: outVideo,
		Cuts:       cuts,
		Mode:       cutter.ModeLossless,
		OnProgress: func(p cutter.CutProgress) {
			pct := p.Percent * 100.0
			if pct > 100.0 {
				pct = 100.0
			}
			fmt.Printf("\r  Rendering: %.0f%% [%s]   ", pct, p.Stage)
		},
	}

	if err := cutter.CutVideo(ctx, cutOpts); err != nil {
		fmt.Println()
		return fmt.Errorf("cutting video: %w", err)
	}
	fmt.Println()
	return nil
}

// performVideoUpload handles streaming the video payload to YouTube with progress reporting.
func performVideoUpload(ctx context.Context, client *http.Client, videoPath string, meta model.TalkMetadata) (*youtube.UploadResult, error) {
	stat, err := os.Stat(videoPath)
	if err != nil {
		return nil, fmt.Errorf("stat video %q: %w", videoPath, err)
	}

	fmt.Printf("\n[YouTube Upload] Uploading %s (%s)...\n", filepath.Base(videoPath), formatBytes(stat.Size()))
	uploadOpts := youtube.UploadOptions{
		VideoPath: videoPath,
		Metadata:  meta,
		OnProgress: func(bytesSent, totalBytes int64, percent float64) {
			fmt.Printf("\r  Uploading: %.1f%% (%s / %s)   ", percent, formatBytes(bytesSent), formatBytes(totalBytes))
		},
	}

	res, err := youtube.UploadVideo(ctx, client, uploadOpts)
	fmt.Println()
	if err != nil {
		return nil, err
	}
	return res, nil
}

// performCaptionUpload uploads the retimed WebVTT subtitles to YouTube as closed captions.
func performCaptionUpload(ctx context.Context, client *http.Client, videoID, vttPath string) {
	if _, err := os.Stat(vttPath); err != nil {
		return
	}
	fmt.Printf("Uploading closed captions (%s)...\n", filepath.Base(vttPath))
	if err := youtube.UploadCaption(ctx, client, videoID, vttPath, "en", "English"); err != nil {
		fmt.Fprintf(os.Stderr, "warning: uploading closed captions: %v\n", err)
		return
	}
	fmt.Println("✔ Closed captions uploaded successfully.")
}

// printUploadPreflight displays a summary of parameters before initiating upload.
func printUploadPreflight(channel string, meta model.TalkMetadata, videoPath string) {
	privacy := strings.ToUpper(meta.Privacy)
	if privacy == "" {
		privacy = "PUBLIC"
	}
	fmt.Println("\n=== Pre-Flight YouTube Upload ===")
	fmt.Printf("  Channel:     %s\n", channel)
	fmt.Printf("  Title:       %s\n", meta.Title)
	if meta.Speaker != "" {
		fmt.Printf("  Speaker:     %s\n", meta.Speaker)
	}
	fmt.Printf("  Privacy:     %s\n", privacy)
	fmt.Printf("  File:        %s\n", videoPath)
	fmt.Printf("  Chapters:    %d markers\n", len(meta.Chapters))
}

// printUploadSuccess formats the final completion confirmation.
func printUploadSuccess(meta model.TalkMetadata, channel, url string, chaptersCount int) {
	fmt.Println("\n================================================================================")
	fmt.Println("✔ SUCCESS: Talk published to YouTube!")
	fmt.Println("================================================================================")
	fmt.Printf("  Title:      %s\n", meta.Title)
	fmt.Printf("  Channel:    %s\n", channel)
	fmt.Printf("  Privacy:    %s\n", strings.ToLower(meta.Privacy))
	fmt.Printf("  Video URL:  %s\n", url)
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
	fmt.Printf("  Chapters:   %d markers synchronized\n", chaptersCount)
	fmt.Println("================================================================================")
}

// formatBytes formats an integer byte count into a human-readable size string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// writeChaptersFile writes adjusted YouTube chapters to disk.
func writeChaptersFile(path string, chapters []model.ChapterMarker) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	for _, ch := range chapters {
		fmt.Fprintln(f, ch.FormatYouTubeLine())
	}
}

// writeAdjustedVTT writes excised & retimed WebVTT subtitles to disk.
func writeAdjustedVTT(path string, cues []model.SubtitleCue, cuts []model.CutInterval) {
	var kept []model.SubtitleCue
	for _, c := range cues {
		if c.Action == model.ActionCut {
			continue
		}
		adjCue := c
		adjCue.Start = cutter.AdjustTime(c.Start, cuts)
		adjCue.End = cutter.AdjustTime(c.End, cuts)
		kept = append(kept, adjCue)
	}

	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	_ = vtt.Write(f, kept)
}
