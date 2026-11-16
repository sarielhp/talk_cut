package ui

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/config"
	"talk_cut/internal/model"
	"talk_cut/internal/youtube"
)

// playlistCallTimeout bounds a single playlist read or write.
const playlistCallTimeout = 30 * time.Second

// playlistTidyTimeout bounds a tidy, which makes one write per duplicate and per moved item.
const playlistTidyTimeout = 10 * time.Minute

type plPanelLoadedMsg struct {
	channel   string
	playlists []youtube.Playlist
	videos    []youtube.ChannelVideo
	err       error
}

type plMembersLoadedMsg struct {
	seq        int
	playlistID string
	ids        map[string]bool
	err        error
}

type plToggledMsg struct {
	playlistID    string
	playlistTitle string
	videoID       string
	added         bool
	err           error
}

type plCopiedMsg struct {
	playlist youtube.Playlist
	err      error
}

type plCreatedMsg struct {
	channel  string
	playlist *youtube.Playlist
	err      error
}

// plTidyMsg carries a tidy plan (dryRun) or the result of applying one.
type plTidyMsg struct {
	playlist youtube.Playlist
	dryRun   bool
	result   youtube.TidyResult
	err      error
}

// plSession is what the playlists panel needs to talk to YouTube; it does not depend on a recording.
type plSession struct {
	cfg     config.Config
	channel string
}

// playlistSession returns the session for the YouTube tab's current channel.
func (a AppModel) playlistSession() plSession {
	return plSession{cfg: a.cfg, channel: a.youtubeView.CurrentChannel()}
}

// handlePlaylistsKey processes keys on the playlists panel.
func (a AppModel) handlePlaylistsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.playlistsView.panelMode() == plModeList && !a.playlistsView.AwaitingTidyConfirm() {
		switch msg.String() {
		case "q":
			a.cutsView.Close()
			return a, tea.Quit
		case "esc":
			a.switchTab(ScreenCuts)
			return a, nil
		}
	}
	cmd := handlePlaylistPanelKey(&a.playlistsView, a.playlistSession(), msg)
	return a, cmd
}

// handlePlaylistPanelKey processes the panel keys shared by the TUI tab and the standalone editor.
func handlePlaylistPanelKey(v *PlaylistsModel, s plSession, msg tea.KeyMsg) tea.Cmd {
	if v.AwaitingTidyConfirm() {
		pl, ok := v.ConfirmTidy(msg.String() == "y")
		if !ok {
			return nil
		}
		return s.tidyCmd(pl, false)
	}
	switch v.panelMode() {
	case plModeCreate:
		return handlePlaylistCreateKey(v, s, msg)
	case plModeVideos:
		return handlePlaylistVideosKey(v, s, msg)
	}
	switch msg.String() {
	case "up", "k":
		v.MoveCursor(-1)
	case "down", "j":
		v.MoveCursor(1)
	case "enter", "l":
		return openSelectedPlaylist(v, s)
	case "n":
		return v.BeginCreate()
	case "t":
		return startTidy(v, s)
	case "o":
		openSelectedPlaylistInBrowser(v.SelectedPlaylist())
	case "c":
		return copyPlaylistLinkCmd(v.SelectedPlaylist())
	case "r":
		return s.loadPanelCmd(v)
	}
	return nil
}

// handlePlaylistVideosKey processes keys in the membership view of one playlist.
func handlePlaylistVideosKey(v *PlaylistsModel, s plSession, msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "h", "backspace":
		v.BackToList()
	case "up", "k":
		v.MoveCursor(-1)
	case "down", "j":
		v.MoveCursor(1)
	case " ", "x", "enter":
		return s.toggleSelectedVideoCmd(v)
	case "t":
		return startTidy(v, s)
	case "o":
		openSelectedPlaylistInBrowser(v.SelectedPlaylist())
	case "c":
		return copyPlaylistLinkCmd(v.SelectedPlaylist())
	case "r":
		return openSelectedPlaylist(v, s)
	}
	return nil
}

// handlePlaylistCreateKey processes keys in the new-playlist form.
func handlePlaylistCreateKey(v *PlaylistsModel, s plSession, msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		v.CancelCreate()
		return nil
	case "tab":
		v.CyclePrivacy()
		return nil
	case "enter":
		title, privacy, ok := v.CreateRequest()
		if !ok {
			return nil
		}
		v.SetCreating(title)
		return s.createCmd(title, privacy)
	}
	var cmd tea.Cmd
	*v, cmd = v.Update(msg)
	return cmd
}

// applyPlaylistPanelMsg applies the results shared by both hosts of the panel; it reports whether msg was one.
func applyPlaylistPanelMsg(v *PlaylistsModel, msg tea.Msg) bool {
	switch msg := msg.(type) {
	case plPanelLoadedMsg:
		v.SetLoaded(msg.channel, msg.playlists, msg.videos, msg.err)
	case plMembersLoadedMsg:
		v.SetMembers(msg.seq, msg.playlistID, msg.ids, msg.err)
	case plCreatedMsg:
		v.ApplyCreated(msg.channel, msg.playlist, msg.err)
	case plCopiedMsg:
		v.SetCopied(msg.playlist, playlistURL(msg.playlist.ID), msg.err)
	case plTidyMsg:
		v.ApplyTidy(msg.playlist, msg.dryRun, msg.result, msg.err)
	default:
		return false
	}
	return true
}

// openSelectedPlaylist enters the membership view and fetches the playlist's contents.
func openSelectedPlaylist(v *PlaylistsModel, s plSession) tea.Cmd {
	pl := v.SelectedPlaylist()
	if pl == nil {
		return nil
	}
	seq := v.OpenPlaylist()
	return s.fetchMembersCmd(seq, pl.ID)
}

// startTidy plans a tidy of the selected playlist; the user confirms before anything is written.
func startTidy(v *PlaylistsModel, s plSession) tea.Cmd {
	pl := v.SelectedPlaylist()
	if pl == nil || !v.BeginTidy() {
		return nil
	}
	return s.tidyCmd(*pl, true)
}

// toggleSelectedVideoCmd adds or removes the video under the cursor from the open playlist.
func (s plSession) toggleSelectedVideoCmd(v *PlaylistsModel) tea.Cmd {
	pl := v.SelectedPlaylist()
	vid := v.SelectedVideo()
	if pl == nil || vid == nil || !v.MembershipKnown() || v.IsPending(vid.ID) {
		return nil
	}
	adding := !v.IsMember(vid.ID)
	v.SetPending(vid.ID, vid.Title, adding)

	playlistID, playlistTitle, videoID := pl.ID, pl.Title, vid.ID
	return s.run(playlistCallTimeout, func(ctx context.Context, client *http.Client) tea.Msg {
		var err error
		if adding {
			err = youtube.AddVideoToPlaylist(ctx, client, playlistID, videoID)
		} else {
			err = youtube.RemoveVideoFromPlaylist(ctx, client, playlistID, videoID)
		}
		return plToggledMsg{playlistID: playlistID, playlistTitle: playlistTitle, videoID: videoID, added: adding, err: err}
	}, func(err error) tea.Msg {
		return plToggledMsg{playlistID: playlistID, videoID: videoID, added: adding, err: err}
	})
}

// loadPanelCmd fetches the channel's playlists and its most recent uploads.
func (s plSession) loadPanelCmd(v *PlaylistsModel) tea.Cmd {
	channel := s.channel
	v.SetLoading(channel)
	return s.run(playlistCallTimeout, func(ctx context.Context, client *http.Client) tea.Msg {
		pls, err := youtube.ListPlaylists(ctx, client)
		if err != nil {
			return plPanelLoadedMsg{channel: channel, err: err}
		}
		videos, err := youtube.ListRecentUploads(ctx, client, recentVideoLimit)
		return plPanelLoadedMsg{channel: channel, playlists: pls, videos: videos, err: err}
	}, func(err error) tea.Msg {
		return plPanelLoadedMsg{channel: channel, err: err}
	})
}

// fetchMembersCmd reads the set of videos contained in a playlist.
func (s plSession) fetchMembersCmd(seq int, playlistID string) tea.Cmd {
	return s.run(playlistCallTimeout, func(ctx context.Context, client *http.Client) tea.Msg {
		ids, err := youtube.ListPlaylistVideoIDs(ctx, client, playlistID)
		return plMembersLoadedMsg{seq: seq, playlistID: playlistID, ids: ids, err: err}
	}, func(err error) tea.Msg {
		return plMembersLoadedMsg{seq: seq, playlistID: playlistID, err: err}
	})
}

// createCmd creates a playlist on the session's channel.
func (s plSession) createCmd(title, privacy string) tea.Cmd {
	channel := s.channel
	return s.run(playlistCallTimeout, func(ctx context.Context, client *http.Client) tea.Msg {
		pl, err := youtube.CreatePlaylist(ctx, client, title, "", privacy)
		return plCreatedMsg{channel: channel, playlist: pl, err: err}
	}, func(err error) tea.Msg {
		return plCreatedMsg{channel: channel, err: err}
	})
}

// tidyCmd plans (dryRun) or applies the removal of repeated videos and the newest-first order.
func (s plSession) tidyCmd(pl youtube.Playlist, dryRun bool) tea.Cmd {
	timeout := playlistTidyTimeout
	if dryRun {
		timeout = playlistCallTimeout
	}
	return s.run(timeout, func(ctx context.Context, client *http.Client) tea.Msg {
		res, err := youtube.TidyPlaylist(ctx, client, pl.ID, dryRun, nil)
		return plTidyMsg{playlist: pl, dryRun: dryRun, result: res, err: err}
	}, func(err error) tea.Msg {
		return plTidyMsg{playlist: pl, dryRun: dryRun, err: err}
	})
}

// run calls fn in the background with an authenticated client for the session's channel.
func (s plSession) run(timeout time.Duration, fn func(context.Context, *http.Client) tea.Msg, onAuthErr func(error) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		client, err := youtube.GetAuthenticatedClient(context.Background(), s.cfg, s.channel)
		if err != nil {
			return onAuthErr(fmt.Errorf("authenticating for channel %q: %w", s.channel, err))
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return fn(ctx, client)
	}
}

// withYouTubeClient runs fn in the background with an authenticated client for the current channel.
func (a AppModel) withYouTubeClient(fn func(context.Context, *http.Client) tea.Msg, onAuthErr func(error) tea.Msg) tea.Cmd {
	return a.playlistSession().run(playlistCallTimeout, fn, onAuthErr)
}

// loadPlaylistPanelCmd fetches the panel's data for the YouTube tab's current channel.
func (a *AppModel) loadPlaylistPanelCmd() tea.Cmd {
	return a.playlistSession().loadPanelCmd(&a.playlistsView)
}

// handlePlaylistToggled records a membership change and mirrors it into the talk's metadata.
func (a *AppModel) handlePlaylistToggled(msg plToggledMsg) (tea.Model, tea.Cmd) {
	a.playlistsView.ApplyToggle(msg.playlistID, msg.videoID, msg.added, msg.err)
	if msg.err != nil || msg.videoID != a.youtubeView.VideoID() {
		return *a, nil
	}
	meta := a.metaView.Metadata()
	if msg.added {
		meta.AddPlaylist(msg.playlistID, msg.playlistTitle)
		a.youtubeView.SetPlaylistAdded(msg.playlistID, msg.playlistTitle)
	} else {
		meta.RemovePlaylist(msg.playlistID)
		a.youtubeView.SetPlaylistRemoved(msg.playlistID, msg.playlistTitle)
	}
	a.metaView.ApplyMetadata(meta)
	_ = model.SaveMetaFile(a.bundle.Dir, meta)
	return *a, nil
}

// openSelectedPlaylistInBrowser opens the playlist page with xdg-open.
func openSelectedPlaylistInBrowser(pl *youtube.Playlist) {
	if pl == nil {
		return
	}
	_ = exec.Command("xdg-open", playlistURL(pl.ID)).Start()
}

// copyPlaylistLinkCmd copies the playlist's share link to the clipboard in the background.
func copyPlaylistLinkCmd(pl *youtube.Playlist) tea.Cmd {
	if pl == nil {
		return nil
	}
	p := *pl
	return func() tea.Msg {
		return plCopiedMsg{playlist: p, err: copyToClipboard(playlistURL(p.ID))}
	}
}

// playlistURL is the share link for a playlist; YouTube has no youtu.be form for playlists.
func playlistURL(id string) string {
	return "https://www.youtube.com/playlist?list=" + id
}
