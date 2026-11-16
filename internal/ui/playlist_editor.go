package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/config"
)

// PlaylistEditor is the playlists panel on its own, for editing playlists without a recording.
type PlaylistEditor struct {
	view    PlaylistsModel
	session plSession
}

// NewPlaylistEditor creates a standalone playlist editor for a channel ("" means the configured default).
func NewPlaylistEditor(cfg config.Config, channel string) PlaylistEditor {
	if channel == "" {
		channel = cfg.DefaultChannel
	}
	if channel == "" {
		channel = "default"
	}
	view := NewPlaylistsModel()
	view.standalone = true
	view.SetLoading(channel) // Init runs on a copy, so the channel is recorded here
	return PlaylistEditor{view: view, session: plSession{cfg: cfg, channel: channel}}
}

// Init loads the channel's playlists.
func (e PlaylistEditor) Init() tea.Cmd {
	return e.session.loadPanelCmd(&e.view)
}

// Update handles keys, window size and YouTube results.
func (e PlaylistEditor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if applyPlaylistPanelMsg(&e.view, msg) {
		return e, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		e.view.SetDimensions(msg.Width, msg.Height)
	case plToggledMsg:
		e.view.ApplyToggle(msg.playlistID, msg.videoID, msg.added, msg.err)
	case tea.KeyMsg:
		return e.handleKey(msg)
	}
	return e, nil
}

func (e PlaylistEditor) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return e, tea.Quit
	}
	if e.view.panelMode() == plModeList && !e.view.AwaitingTidyConfirm() && (msg.String() == "q" || msg.String() == "esc") {
		return e, tea.Quit
	}
	cmd := handlePlaylistPanelKey(&e.view, e.session, msg)
	return e, cmd
}

// View renders the panel.
func (e PlaylistEditor) View() string {
	return e.view.View()
}
