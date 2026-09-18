package ui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/config"
	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
	"talk_cut/internal/youtube"
)

// handleYouTubeKey processes keys on the YouTube publish and upload screen.
func (a AppModel) handleYouTubeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.isUploading {
		return a, nil
	}

	if a.youtubeView.IsSelectingPlaylist() {
		return a.handlePlaylistSelectKey(msg)
	}

	switch msg.String() {
	case "q":
		a.cutsView.Close()
		return a, tea.Quit
	case "esc":
		a.switchTab(ScreenCuts)
		return a, nil
	case "r":
		if !a.youtubeView.IsDone() && a.youtubeView.VideoID() == "" {
			a.switchTab(ScreenProg)
			return a.startRender()
		}
		return a.startYouTubeUpload()
	case "c", "y":
		if a.youtubeView.YouTubeURL() != "" {
			return a, a.copyShortURL()
		}
		a.youtubeView.NextChannel()
		return a, nil
	case "o":
		if a.youtubeView.YouTubeURL() != "" {
			url := a.youtubeView.YouTubeURL()
			_ = exec.Command("xdg-open", url).Start()
			return a, nil
		}
	case "p":
		if a.youtubeView.VideoID() != "" {
			return a.openPlaylistSelector()
		}
		return a, a.launchCutPreview()
	case "P":
		return a, a.launchCutPreview()
	case "d", "U":
		return a.startYouTubeUpdateDetails()
	case "enter":
		if a.youtubeView.VideoID() != "" {
			return a.startYouTubeUpdateDetails()
		}
		if !a.youtubeView.IsDone() {
			return a.startYouTubeUpload()
		}
	case "u":
		return a.startYouTubeUpload()
	case "s", "f":
		return a, a.checkYouTubeExistingUpload()
	}

	var cmd tea.Cmd
	a.youtubeView, cmd = a.youtubeView.Update(msg)
	return a, cmd
}

// startYouTubeUpload initiates the background upload pipeline to YouTube.
func (a AppModel) startYouTubeUpload() (tea.Model, tea.Cmd) {
	if a.isUploading {
		return a, nil
	}

	channel := a.youtubeView.CurrentChannel()
	client, err := youtube.GetAuthenticatedClient(context.Background(), a.cfg, channel)
	if err != nil {
		a.youtubeView.SetDone(nil, false, err)
		return a, nil
	}

	outVideo := a.metaView.OutputPath()
	if stat, statErr := os.Stat(outVideo); statErr != nil || stat.Size() == 0 {
		a.youtubeView.SetDone(nil, false, fmt.Errorf("cut video %q not found (render it in Tab 4 first)", outVideo))
		return a, nil
	}

	a.isUploading = true
	a.youtubeView.SetUploading(true)
	a.screen = ScreenYouTube

	outBase := strings.TrimSuffix(outVideo, filepath.Ext(outVideo))
	vttPath := outBase + ".vtt"

	meta := a.metaView.Metadata()
	cues := a.cutsView.Cues()
	cuts := model.BuildCutIntervals(cues)
	aligned := cutter.AlignChaptersToKeptCues(meta.Chapters, cues)
	meta.Chapters = cutter.AdjustChapters(aligned, cuts, "Introduction")

	var targetPlaylists []string
	for _, pl := range meta.AllPlaylists() {
		targetPlaylists = append(targetPlaylists, pl.ID)
	}
	if len(targetPlaylists) == 0 && a.cfg.DefaultPlaylist != "" {
		targetPlaylists = append(targetPlaylists, a.cfg.DefaultPlaylist)
	}

	ytProgChan := a.ytProgChan
	ytDoneChan := a.ytDoneChan

	return a, func() tea.Msg {
		go runUploadWorker(client, outVideo, vttPath, meta, targetPlaylists, ytProgChan, ytDoneChan)
		return a.listenNextYtEvent()()
	}
}

// startYouTubeUpdateDetails initiates background update of video details on YouTube.
func (a AppModel) startYouTubeUpdateDetails() (tea.Model, tea.Cmd) {
	if a.isUploading {
		return a, nil
	}

	videoID := a.youtubeView.VideoID()
	if videoID == "" {
		a.youtubeView.SetDone(nil, false, fmt.Errorf("no YouTube video ID found to update (set 'youtube_id' in talk_meta.json or upload first)"))
		return a, nil
	}

	channel := a.youtubeView.CurrentChannel()
	client, err := youtube.GetAuthenticatedClient(context.Background(), a.cfg, channel)
	if err != nil {
		a.youtubeView.SetDone(nil, false, err)
		return a, nil
	}

	a.isUploading = true
	a.youtubeView.SetUpdatingDetails(true)
	a.screen = ScreenYouTube

	meta := a.metaView.Metadata()
	cues := a.cutsView.Cues()
	cuts := model.BuildCutIntervals(cues)
	aligned := cutter.AlignChaptersToKeptCues(meta.Chapters, cues)
	meta.Chapters = cutter.AdjustChapters(aligned, cuts, "Introduction")

	var targetPlaylists []string
	for _, pl := range meta.AllPlaylists() {
		targetPlaylists = append(targetPlaylists, pl.ID)
	}
	if len(targetPlaylists) == 0 && a.cfg.DefaultPlaylist != "" {
		targetPlaylists = append(targetPlaylists, a.cfg.DefaultPlaylist)
	}

	ytDoneChan := a.ytDoneChan

	return a, func() tea.Msg {
		go runUpdateDetailsWorker(client, videoID, meta, targetPlaylists, ytDoneChan)
		return a.listenNextYtEvent()()
	}
}

// runUpdateDetailsWorker updates YouTube video details (title, description, chapters, tags, privacy).
func runUpdateDetailsWorker(
	client *http.Client,
	videoID string,
	meta model.TalkMetadata,
	targetPlaylists []string,
	doneChan chan<- ytDoneMsg,
) {
	ctx := context.Background()
	ver, err := youtube.UpdateVideoMetadata(ctx, client, videoID, meta)
	if err != nil {
		doneChan <- ytDoneMsg{err: fmt.Errorf("updating YouTube video details: %w", err)}
		return
	}
	var addedPlaylists []model.PlaylistRef
	for _, target := range targetPlaylists {
		if pl, plErr := youtube.FindPlaylist(ctx, client, target); plErr == nil {
			if addErr := youtube.AddVideoToPlaylist(ctx, client, pl.ID, videoID); addErr == nil {
				addedPlaylists = append(addedPlaylists, model.PlaylistRef{ID: pl.ID, Title: pl.Title})
			}
		}
	}
	doneChan <- ytDoneMsg{
		verification:    ver,
		captionUploaded: false,
		playlists:       addedPlaylists,
	}
}

// runUploadWorker streams the video to YouTube, uploads closed captions, and verifies status.
func runUploadWorker(
	client *http.Client,
	videoPath, vttPath string,
	meta model.TalkMetadata,
	targetPlaylists []string,
	progChan chan<- ytProgressMsg,
	doneChan chan<- ytDoneMsg,
) {
	ctx := context.Background()

	opts := youtube.UploadOptions{
		VideoPath: videoPath,
		Metadata:  meta,
		OnProgress: func(bytesSent, totalBytes int64, percent float64) {
			progChan <- ytProgressMsg{
				bytesSent:  bytesSent,
				totalBytes: totalBytes,
				percent:    percent,
				stage:      "uploading_video",
			}
		},
	}

	res, err := youtube.UploadVideo(ctx, client, opts)
	if err != nil {
		doneChan <- ytDoneMsg{err: fmt.Errorf("uploading video: %w", err)}
		return
	}

	captionUploaded := false
	if _, statErr := os.Stat(vttPath); statErr == nil {
		progChan <- ytProgressMsg{
			percent: 100.0,
			stage:   "uploading_captions",
		}
		if capErr := youtube.UploadCaption(ctx, client, res.VideoID, vttPath, "en", "English"); capErr == nil {
			captionUploaded = true
		}
	}

	progChan <- ytProgressMsg{
		percent: 100.0,
		stage:   "verifying",
	}

	ver, err := youtube.VerifyVideo(ctx, client, res.VideoID)
	if err != nil {
		ver = &youtube.VideoVerification{
			VideoID:       res.VideoID,
			Title:         meta.Title,
			UploadStatus:  "uploaded",
			PrivacyStatus: meta.Privacy,
			ShortURL:      res.VideoURL,
			WatchURL:      fmt.Sprintf("https://www.youtube.com/watch?v=%s", res.VideoID),
		}
	}

	var addedPlaylists []model.PlaylistRef
	for _, target := range targetPlaylists {
		if pl, plErr := youtube.FindPlaylist(ctx, client, target); plErr == nil {
			if addErr := youtube.AddVideoToPlaylist(ctx, client, pl.ID, res.VideoID); addErr == nil {
				addedPlaylists = append(addedPlaylists, model.PlaylistRef{ID: pl.ID, Title: pl.Title})
			}
		}
	}

	doneChan <- ytDoneMsg{
		verification:    ver,
		captionUploaded: captionUploaded,
		playlists:       addedPlaylists,
	}
}

// listenNextYtEvent returns a tea.Cmd waiting on YouTube upload progress or completion.
func (a AppModel) listenNextYtEvent() tea.Cmd {
	progChan := a.ytProgChan
	doneChan := a.ytDoneChan
	return func() tea.Msg {
		select {
		case p, ok := <-progChan:
			if ok {
				return p
			}
		case d := <-doneChan:
			return d
		}
		return nil
	}
}

// copyShortURL copies the verified short URL to the system clipboard using wl-copy or xclip.
func (a AppModel) copyShortURL() tea.Cmd {
	url := a.youtubeView.YouTubeURL()
	if url == "" {
		return nil
	}
	return func() tea.Msg {
		cmd := exec.Command("wl-copy", url)
		if err := cmd.Run(); err != nil {
			cmd2 := exec.Command("xclip", "-selection", "clipboard")
			cmd2.Stdin = strings.NewReader(url)
			_ = cmd2.Run()
		}
		return ytCopiedMsg{}
	}
}

// launchCutPreview opens the rendered cut video in ffplay.
func (a AppModel) launchCutPreview() tea.Cmd {
	outVideo := a.metaView.OutputPath()
	return func() tea.Msg {
		cmd := exec.Command("ffplay", "-autoexit", outVideo)
		cmd.Stdin = nil
		cmd.Stdout = nil
		cmd.Stderr = nil
		_ = cmd.Start()
		return nil
	}
}

type ytDiscoveredMsg struct {
	verification *youtube.VideoVerification
	err          error
}

// checkYouTubeExistingUpload queries the YouTube channel in background to detect if video was already uploaded.
func (a AppModel) checkYouTubeExistingUpload() tea.Cmd {
	channel := a.youtubeView.CurrentChannel()
	if !a.cfg.HasValidChannelToken(channel) {
		return nil
	}

	title := a.metaView.Metadata().Title
	dirName := filepath.Base(a.bundle.Dir)
	cfg := a.cfg

	return func() tea.Msg {
		client, err := youtube.GetAuthenticatedClient(context.Background(), cfg, channel)
		if err != nil {
			return ytDiscoveredMsg{err: err}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		ver, err := youtube.FindChannelVideoForTalk(ctx, client, title, dirName)
		return ytDiscoveredMsg{verification: ver, err: err}
	}
}

// handleYTDiscoveredMsg synchronizes newly detected YouTube upload details into the app and disk.
func (a *AppModel) handleYTDiscoveredMsg(msg ytDiscoveredMsg) (tea.Model, tea.Cmd) {
	if msg.err == nil && msg.verification != nil && msg.verification.VideoID != "" {
		meta := a.metaView.Metadata()
		meta.YouTubeID = msg.verification.VideoID
		meta.YouTubeURL = msg.verification.ShortURL
		if meta.YouTubeURL == "" {
			meta.YouTubeURL = fmt.Sprintf("https://youtu.be/%s", msg.verification.VideoID)
		}
		a.metaView.ApplyMetadata(meta)

		outVideo := a.metaView.OutputPath()
		outBase := strings.TrimSuffix(outVideo, filepath.Ext(outVideo))
		a.youtubeView.SetPreflight(outVideo, outBase+".vtt", meta)
		_ = model.SaveMetaFile(a.bundle.Dir, meta)
	}
	return *a, nil
}

// handlePlaylistSelectKey processes keys while the interactive playlist picker is open.
func (a AppModel) handlePlaylistSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		a.youtubeView.SetSelectingPlaylist(false)
		return a, nil
	case "up", "k":
		a.youtubeView.PrevPlaylist()
		return a, nil
	case "down", "j":
		a.youtubeView.NextPlaylist()
		return a, nil
	case "enter", " ", "x":
		pl := a.youtubeView.SelectedPlaylist()
		if pl == nil {
			return a, nil
		}
		videoID := a.youtubeView.VideoID()
		if videoID == "" {
			a.youtubeView.SetPlaylistFeedback("No uploaded video found to update")
			return a, nil
		}
		if a.metaView.Metadata().HasPlaylist(pl.ID) {
			a.youtubeView.SetPlaylistFeedback(fmt.Sprintf("Removing from %q...", pl.Title))
			return a, a.removeVideoFromPlaylistCmd(pl.ID, pl.Title, videoID)
		}
		a.youtubeView.SetPlaylistFeedback(fmt.Sprintf("Adding to %q...", pl.Title))
		return a, a.addVideoToPlaylistCmd(pl.ID, pl.Title, videoID)
	case "d":
		pl := a.youtubeView.SelectedPlaylist()
		if pl == nil {
			return a, nil
		}
		a.cfg.DefaultPlaylist = pl.ID
		if err := config.SaveConfig(a.cfg); err != nil {
			a.youtubeView.SetPlaylistFeedback(fmt.Sprintf("Failed to save default playlist: %v", err))
		} else {
			a.youtubeView.SetPlaylistFeedback(fmt.Sprintf("✔ Default playlist set to %q in config", pl.Title))
		}
		return a, nil
	}
	return a, nil
}

// openPlaylistSelector opens the playlist chooser and queries channel playlists.
func (a AppModel) openPlaylistSelector() (tea.Model, tea.Cmd) {
	a.youtubeView.SetSelectingPlaylist(true)
	if len(a.youtubeView.Playlists()) == 0 {
		a.youtubeView.SetLoadingPlaylists(true)
	}
	return a, a.fetchPlaylistsCmd()
}

// fetchPlaylistsCmd queries the YouTube API for the channel's playlists.
func (a AppModel) fetchPlaylistsCmd() tea.Cmd {
	channel := a.youtubeView.CurrentChannel()
	cfg := a.cfg
	return func() tea.Msg {
		client, err := youtube.GetAuthenticatedClient(context.Background(), cfg, channel)
		if err != nil {
			return ytPlaylistsLoadedMsg{err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		pls, err := youtube.ListPlaylists(ctx, client)
		return ytPlaylistsLoadedMsg{playlists: pls, err: err}
	}
}

// addVideoToPlaylistCmd adds the video to the target playlist via the YouTube API.
func (a AppModel) addVideoToPlaylistCmd(playlistID, playlistTitle, videoID string) tea.Cmd {
	channel := a.youtubeView.CurrentChannel()
	cfg := a.cfg
	return func() tea.Msg {
		client, err := youtube.GetAuthenticatedClient(context.Background(), cfg, channel)
		if err != nil {
			return ytVideoAddedToPlaylistMsg{playlistID: playlistID, title: playlistTitle, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err = youtube.AddVideoToPlaylist(ctx, client, playlistID, videoID)
		return ytVideoAddedToPlaylistMsg{playlistID: playlistID, title: playlistTitle, err: err}
	}
}

// removeVideoFromPlaylistCmd removes the video from the target playlist via the YouTube API.
func (a AppModel) removeVideoFromPlaylistCmd(playlistID, playlistTitle, videoID string) tea.Cmd {
	channel := a.youtubeView.CurrentChannel()
	cfg := a.cfg
	return func() tea.Msg {
		client, err := youtube.GetAuthenticatedClient(context.Background(), cfg, channel)
		if err != nil {
			return ytVideoRemovedFromPlaylistMsg{playlistID: playlistID, title: playlistTitle, err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err = youtube.RemoveVideoFromPlaylist(ctx, client, playlistID, videoID)
		return ytVideoRemovedFromPlaylistMsg{playlistID: playlistID, title: playlistTitle, err: err}
	}
}

type ytPlaylistsLoadedMsg struct {
	playlists []youtube.Playlist
	err       error
}

type ytVideoAddedToPlaylistMsg struct {
	playlistID string
	title      string
	err        error
}

type ytVideoRemovedFromPlaylistMsg struct {
	playlistID string
	title      string
	err        error
}

// handleYTPlaylistsLoadedMsg processes loaded channel playlists.
func (a *AppModel) handleYTPlaylistsLoadedMsg(msg ytPlaylistsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.youtubeView.SetLoadingPlaylists(false)
		a.youtubeView.SetPlaylistFeedback(fmt.Sprintf("Failed to load playlists: %v", msg.err))
		return *a, nil
	}
	a.youtubeView.SetPlaylists(msg.playlists, a.cfg.DefaultPlaylist)
	return *a, nil
}

// handleYTVideoAddedToPlaylistMsg persists playlist association upon successful YouTube addition.
func (a *AppModel) handleYTVideoAddedToPlaylistMsg(msg ytVideoAddedToPlaylistMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.youtubeView.SetPlaylistFeedback(fmt.Sprintf("Failed to add video: %v", msg.err))
		return *a, nil
	}
	a.youtubeView.SetPlaylistAdded(msg.playlistID, msg.title)
	meta := a.metaView.Metadata()
	meta.AddPlaylist(msg.playlistID, msg.title)
	a.metaView.ApplyMetadata(meta)
	_ = model.SaveMetaFile(a.bundle.Dir, meta)
	return *a, nil
}

// handleYTVideoRemovedFromPlaylistMsg updates metadata when video is removed from a playlist.
func (a *AppModel) handleYTVideoRemovedFromPlaylistMsg(msg ytVideoRemovedFromPlaylistMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.youtubeView.SetPlaylistFeedback(fmt.Sprintf("Failed to remove video: %v", msg.err))
		return *a, nil
	}
	a.youtubeView.SetPlaylistRemoved(msg.playlistID, msg.title)
	meta := a.metaView.Metadata()
	meta.RemovePlaylist(msg.playlistID)
	a.metaView.ApplyMetadata(meta)
	_ = model.SaveMetaFile(a.bundle.Dir, meta)
	return *a, nil
}
