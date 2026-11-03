package ui

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/model"
	"talk_cut/internal/youtube"
)

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

type plCreatedMsg struct {
	channel  string
	playlist *youtube.Playlist
	err      error
}

// handlePlaylistsKey processes keys on the playlists panel.
func (a AppModel) handlePlaylistsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch a.playlistsView.Mode() {
	case plModeCreate:
		return a.handlePlaylistCreateKey(msg)
	case plModeVideos:
		return a.handlePlaylistVideosKey(msg)
	}
	switch msg.String() {
	case "q":
		a.cutsView.Close()
		return a, tea.Quit
	case "esc":
		a.switchTab(ScreenCuts)
		return a, nil
	case "up", "k":
		a.playlistsView.MoveCursor(-1)
	case "down", "j":
		a.playlistsView.MoveCursor(1)
	case "enter", "l":
		return a.openSelectedPlaylist()
	case "n":
		cmd := a.playlistsView.BeginCreate()
		return a, cmd
	case "o":
		openSelectedPlaylistInBrowser(a.playlistsView.SelectedPlaylist())
	case "r":
		cmd := a.loadPlaylistPanelCmd()
		return a, cmd
	}
	return a, nil
}

// handlePlaylistVideosKey processes keys in the membership view of one playlist.
func (a AppModel) handlePlaylistVideosKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "h", "backspace":
		a.playlistsView.BackToList()
	case "up", "k":
		a.playlistsView.MoveCursor(-1)
	case "down", "j":
		a.playlistsView.MoveCursor(1)
	case " ", "x", "enter":
		cmd := a.toggleSelectedVideo()
		return a, cmd
	case "o":
		openSelectedPlaylistInBrowser(a.playlistsView.SelectedPlaylist())
	case "r":
		return a.openSelectedPlaylist()
	}
	return a, nil
}

// handlePlaylistCreateKey processes keys in the new-playlist form.
func (a AppModel) handlePlaylistCreateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.playlistsView.CancelCreate()
		return a, nil
	case "tab":
		a.playlistsView.CyclePrivacy()
		return a, nil
	case "enter":
		title, privacy, ok := a.playlistsView.CreateRequest()
		if !ok {
			return a, nil
		}
		a.playlistsView.SetCreating(title)
		return a, a.createPlaylistCmd(title, privacy)
	}
	var cmd tea.Cmd
	a.playlistsView, cmd = a.playlistsView.Update(msg)
	return a, cmd
}

// openSelectedPlaylist enters the membership view and fetches the playlist's contents.
func (a AppModel) openSelectedPlaylist() (tea.Model, tea.Cmd) {
	pl := a.playlistsView.SelectedPlaylist()
	if pl == nil {
		return a, nil
	}
	seq := a.playlistsView.OpenPlaylist()
	return a, a.fetchPlaylistMembersCmd(seq, pl.ID)
}

// toggleSelectedVideo adds or removes the video under the cursor from the open playlist.
func (a *AppModel) toggleSelectedVideo() tea.Cmd {
	pl := a.playlistsView.SelectedPlaylist()
	v := a.playlistsView.SelectedVideo()
	if pl == nil || v == nil || !a.playlistsView.MembershipKnown() || a.playlistsView.IsPending(v.ID) {
		return nil
	}
	adding := !a.playlistsView.IsMember(v.ID)
	a.playlistsView.SetPending(v.ID, v.Title, adding)

	playlistID, playlistTitle, videoID := pl.ID, pl.Title, v.ID
	return a.withYouTubeClient(func(ctx context.Context, client *http.Client) tea.Msg {
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

// loadPlaylistPanelCmd fetches the channel's playlists and its most recent uploads.
func (a *AppModel) loadPlaylistPanelCmd() tea.Cmd {
	channel := a.youtubeView.CurrentChannel()
	a.playlistsView.SetLoading(channel)
	return a.withYouTubeClient(func(ctx context.Context, client *http.Client) tea.Msg {
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

// fetchPlaylistMembersCmd reads the set of videos contained in a playlist.
func (a AppModel) fetchPlaylistMembersCmd(seq int, playlistID string) tea.Cmd {
	return a.withYouTubeClient(func(ctx context.Context, client *http.Client) tea.Msg {
		ids, err := youtube.ListPlaylistVideoIDs(ctx, client, playlistID)
		return plMembersLoadedMsg{seq: seq, playlistID: playlistID, ids: ids, err: err}
	}, func(err error) tea.Msg {
		return plMembersLoadedMsg{seq: seq, playlistID: playlistID, err: err}
	})
}

// createPlaylistCmd creates a playlist on the current channel.
func (a AppModel) createPlaylistCmd(title, privacy string) tea.Cmd {
	channel := a.youtubeView.CurrentChannel()
	return a.withYouTubeClient(func(ctx context.Context, client *http.Client) tea.Msg {
		pl, err := youtube.CreatePlaylist(ctx, client, title, "", privacy)
		return plCreatedMsg{channel: channel, playlist: pl, err: err}
	}, func(err error) tea.Msg {
		return plCreatedMsg{channel: channel, err: err}
	})
}

// withYouTubeClient runs fn in the background with an authenticated client for the current channel.
func (a AppModel) withYouTubeClient(fn func(context.Context, *http.Client) tea.Msg, onAuthErr func(error) tea.Msg) tea.Cmd {
	channel := a.youtubeView.CurrentChannel()
	cfg := a.cfg
	return func() tea.Msg {
		client, err := youtube.GetAuthenticatedClient(context.Background(), cfg, channel)
		if err != nil {
			return onAuthErr(fmt.Errorf("authenticating for channel %q: %w", channel, err))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return fn(ctx, client)
	}
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
	_ = exec.Command("xdg-open", "https://www.youtube.com/playlist?list="+pl.ID).Start()
}
