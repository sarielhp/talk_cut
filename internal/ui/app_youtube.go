package ui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/cutter"
	"talk_cut/internal/model"
	"talk_cut/internal/youtube"
)

// handleYouTubeKey processes keys on the YouTube publish and upload screen.
func (a AppModel) handleYouTubeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.isUploading {
		return a, nil
	}

	switch msg.String() {
	case "q":
		a.cutsView.Close()
		return a, tea.Quit
	case "esc":
		a.switchTab(ScreenCuts)
		return a, nil
	case "r":
		if !a.youtubeView.IsDone() {
			a.switchTab(ScreenProg)
			return a.startRender()
		}
		return a.startYouTubeUpload()
	case "c":
		if a.youtubeView.IsDone() && a.youtubeView.Verification() != nil {
			return a, a.copyShortURL()
		}
		a.youtubeView.NextChannel()
		return a, nil
	case "y":
		if a.youtubeView.IsDone() && a.youtubeView.Verification() != nil {
			return a, a.copyShortURL()
		}
	case "o":
		if a.youtubeView.IsDone() && a.youtubeView.Verification() != nil {
			ver := a.youtubeView.Verification()
			_ = exec.Command("xdg-open", ver.ShortURL).Start()
			return a, nil
		}
	case "p":
		return a, a.launchCutPreview()
	case "u", "enter":
		if !a.youtubeView.IsDone() {
			return a.startYouTubeUpload()
		}
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

	ytProgChan := a.ytProgChan
	ytDoneChan := a.ytDoneChan

	return a, func() tea.Msg {
		go runUploadWorker(client, outVideo, vttPath, meta, ytProgChan, ytDoneChan)
		return a.listenNextYtEvent()()
	}
}

// runUploadWorker streams the video to YouTube, uploads closed captions, and verifies status.
func runUploadWorker(
	client *http.Client,
	videoPath, vttPath string,
	meta model.TalkMetadata,
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

	doneChan <- ytDoneMsg{
		verification:    ver,
		captionUploaded: captionUploaded,
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
	ver := a.youtubeView.Verification()
	if ver == nil {
		return nil
	}
	url := ver.ShortURL
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
