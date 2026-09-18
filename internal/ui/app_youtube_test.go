package ui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/config"
	"talk_cut/internal/model"
	"talk_cut/internal/youtube"
)

func TestHandlePlaylistSelectKeys(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	meta := model.TalkMetadata{
		Title:     "Test Talk",
		YouTubeID: "vid123",
	}

	app := AppModel{
		screen:      ScreenYouTube,
		cfg:         cfg,
		youtubeView: NewYouTubeModel("", "", meta, cfg),
	}
	app.metaView = NewMetaModel(meta, nil, filepath.Join(tempDir, "out.mp4"))

	playlists := []youtube.Playlist{
		{ID: "pl1", Title: "Playlist One"},
		{ID: "pl2", Title: "Playlist Two"},
	}
	app.youtubeView.SetPlaylists(playlists, "")
	app.youtubeView.SetSelectingPlaylist(true)

	// Test navigation down
	m, _ := app.handlePlaylistSelectKey(tea.KeyMsg{Type: tea.KeyDown})
	updated := m.(AppModel)
	if updated.youtubeView.SelectedPlaylist().ID != "pl2" {
		t.Errorf("expected pl2 selected, got %s", updated.youtubeView.SelectedPlaylist().ID)
	}

	// Test navigation up
	m, _ = updated.handlePlaylistSelectKey(tea.KeyMsg{Type: tea.KeyUp})
	updated = m.(AppModel)
	if updated.youtubeView.SelectedPlaylist().ID != "pl1" {
		t.Errorf("expected pl1 selected, got %s", updated.youtubeView.SelectedPlaylist().ID)
	}

	// Test setting default playlist with 'd'
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	_ = os.MkdirAll(filepath.Join(homeDir, ".config", "talk_cut"), 0755)

	m, _ = updated.handlePlaylistSelectKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	updated = m.(AppModel)
	if updated.cfg.DefaultPlaylist != "pl1" {
		t.Errorf("expected default playlist pl1, got %s", updated.cfg.DefaultPlaylist)
	}

	// Test esc closes modal
	m, _ = updated.handlePlaylistSelectKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated = m.(AppModel)
	if updated.youtubeView.IsSelectingPlaylist() {
		t.Error("expected playlist selector to be closed after esc")
	}
}

func TestHandleYTPlaylistMessages(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	meta := model.TalkMetadata{
		Title: "Test Talk",
	}

	app := AppModel{
		screen:      ScreenYouTube,
		cfg:         cfg,
		youtubeView: NewYouTubeModel("", "", meta, cfg),
	}
	app.bundle.Dir = tempDir
	app.metaView = NewMetaModel(meta, nil, filepath.Join(tempDir, "out.mp4"))

	// Test playlists loaded with error
	m, _ := app.handleYTPlaylistsLoadedMsg(ytPlaylistsLoadedMsg{err: errors.New("network error")})
	updated := m.(AppModel)
	if updated.youtubeView.playlistFeedback == "" {
		t.Error("expected error feedback on failure")
	}

	// Test playlists loaded with success
	pls := []youtube.Playlist{{ID: "pl_loaded", Title: "Loaded Pl"}}
	m, _ = updated.handleYTPlaylistsLoadedMsg(ytPlaylistsLoadedMsg{playlists: pls})
	updated = m.(AppModel)
	if len(updated.youtubeView.Playlists()) != 1 {
		t.Fatalf("expected 1 playlist loaded, got %d", len(updated.youtubeView.Playlists()))
	}

	// Test video added to playlist
	m, _ = updated.handleYTVideoAddedToPlaylistMsg(ytVideoAddedToPlaylistMsg{
		playlistID: "pl_loaded",
		title:      "Loaded Pl",
	})
	updated = m.(AppModel)
	if updated.metaView.Metadata().PlaylistID != "pl_loaded" {
		t.Errorf("expected meta playlist pl_loaded, got %s", updated.metaView.Metadata().PlaylistID)
	}
	if updated.metaView.Metadata().PlaylistTitle != "Loaded Pl" {
		t.Errorf("expected meta playlist title 'Loaded Pl', got %s", updated.metaView.Metadata().PlaylistTitle)
	}

	// Test adding a second playlist
	m, _ = updated.handleYTVideoAddedToPlaylistMsg(ytVideoAddedToPlaylistMsg{
		playlistID: "pl_second",
		title:      "Second Pl",
	})
	updated = m.(AppModel)
	if len(updated.metaView.Metadata().AllPlaylists()) != 2 {
		t.Fatalf("expected 2 playlists, got %d", len(updated.metaView.Metadata().AllPlaylists()))
	}

	// Verify talk_meta.json was written to disk
	saved, err := model.LoadMetaFile(tempDir)
	if err != nil {
		t.Fatalf("failed to load saved meta file: %v", err)
	}
	if len(saved.AllPlaylists()) != 2 {
		t.Errorf("saved meta does not have 2 playlists: %+v", saved.AllPlaylists())
	}

	// Test removing a playlist
	m, _ = updated.handleYTVideoRemovedFromPlaylistMsg(ytVideoRemovedFromPlaylistMsg{
		playlistID: "pl_loaded",
		title:      "Loaded Pl",
	})
	updated = m.(AppModel)
	if len(updated.metaView.Metadata().AllPlaylists()) != 1 {
		t.Fatalf("expected 1 playlist after removal, got %d", len(updated.metaView.Metadata().AllPlaylists()))
	}
	if updated.metaView.Metadata().HasPlaylist("pl_loaded") {
		t.Errorf("expected pl_loaded removed")
	}
}
