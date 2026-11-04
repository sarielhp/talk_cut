package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/youtube"
)

func loadedPlaylistsModel() PlaylistsModel {
	m := NewPlaylistsModel()
	m.SetDimensions(120, 40)
	m.SetLoading("seminar")
	m.SetLoaded(
		"seminar",
		[]youtube.Playlist{
			{ID: "PL1", Title: "Seminar", ItemCount: 1, PrivacyStatus: "public"},
			{ID: "PL2", Title: "Course", ItemCount: 0, PrivacyStatus: "unlisted"},
		},
		[]youtube.ChannelVideo{
			{ID: "v1", Title: "Talk One", PublishedAt: "2026-10-30T10:00:00Z"},
			{ID: "v2", Title: "Talk Two", PublishedAt: "2026-10-01T10:00:00Z"},
		},
		nil,
	)
	return m
}

func TestPlaylistsModelToggleMembership(t *testing.T) {
	m := loadedPlaylistsModel()
	if m.NeedsLoad("seminar") {
		t.Fatal("expected loaded model not to need a load")
	}

	seq := m.OpenPlaylist()
	if m.MembershipKnown() {
		t.Fatal("membership must be unknown until loaded")
	}
	m.SetMembers(seq, "PL2", map[string]bool{"v2": true}, nil) // stale response for another playlist
	if m.MembershipKnown() {
		t.Fatal("membership for another playlist must be ignored")
	}
	m.SetMembers(seq, "PL1", map[string]bool{"v1": true}, nil)
	if !m.MembershipKnown() || !m.IsMember("v1") || m.IsMember("v2") {
		t.Fatalf("unexpected membership: %v", m.members)
	}

	view := m.View()
	if !strings.Contains(view, "[✔] 2026-10-30  Talk One") || !strings.Contains(view, "[ ] 2026-10-01  Talk Two") {
		t.Fatalf("membership markers missing from view:\n%s", view)
	}

	m.MoveCursor(1)
	m.SetPending("v2", "Talk Two", true)
	if !strings.Contains(m.View(), "[…]") {
		t.Error("expected pending marker in view")
	}
	m.ApplyToggle("PL1", "v2", true, nil)
	if !m.IsMember("v2") || m.IsPending("v2") || m.playlists[0].ItemCount != 2 {
		t.Fatalf("add not applied: members=%v count=%d", m.members, m.playlists[0].ItemCount)
	}

	m.ApplyToggle("PL1", "v1", false, nil)
	if m.IsMember("v1") || m.playlists[0].ItemCount != 1 {
		t.Fatalf("remove not applied: members=%v", m.members)
	}

	m.ApplyToggle("PL1", "v2", false, errors.New("quota"))
	if !m.IsMember("v2") || !m.isError {
		t.Fatal("failed toggle must keep membership and show an error")
	}
}

func TestPlaylistsModelCreate(t *testing.T) {
	m := loadedPlaylistsModel()
	m.BeginCreate()
	if !m.IsEditing() {
		t.Fatal("expected create form to capture text")
	}
	if _, _, ok := m.CreateRequest(); ok {
		t.Fatal("empty title must not be submittable")
	}

	for _, r := range "Geometry 2026" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.CyclePrivacy()
	title, privacy, ok := m.CreateRequest()
	if !ok || title != "Geometry 2026" || privacy != "unlisted" {
		t.Fatalf("unexpected request: %q %q %v", title, privacy, ok)
	}

	m.SetCreating(title)
	if _, _, ok := m.CreateRequest(); ok {
		t.Fatal("must not resubmit while creating")
	}
	m.ApplyCreated("seminar", &youtube.Playlist{ID: "PL9", Title: title, PrivacyStatus: privacy}, nil)
	if m.panelMode() != plModeVideos || m.SelectedPlaylist().ID != "PL9" || !m.MembershipKnown() {
		t.Fatalf("expected new playlist to open with empty membership, mode=%d", m.panelMode())
	}
	if len(m.playlists) != 3 {
		t.Fatalf("expected 3 playlists, got %d", len(m.playlists))
	}
}

func TestAppPlaylistsTabIsolatesTextEntry(t *testing.T) {
	app := AppModel{screen: ScreenPlaylists, playlistsView: loadedPlaylistsModel()}
	res, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	app = res.(AppModel)
	res, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	app = res.(AppModel)
	if app.screen != ScreenPlaylists {
		t.Fatalf("typing a digit in the title must not switch tabs, got screen %d", app.screen)
	}
	if got := app.playlistsView.titleInput.Value(); got != "2" {
		t.Fatalf("expected title %q, got %q", "2", got)
	}
	res, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = res.(AppModel)
	if app.playlistsView.panelMode() != plModeList {
		t.Fatal("esc must cancel the create form")
	}
}

func TestVisibleWindow(t *testing.T) {
	cases := []struct{ cursor, n, rows, start, end int }{
		{0, 5, 10, 0, 5},
		{0, 20, 5, 0, 5},
		{10, 20, 5, 8, 13},
		{19, 20, 5, 15, 20},
	}
	for _, c := range cases {
		s, e := visibleWindow(c.cursor, c.n, c.rows)
		if s != c.start || e != c.end {
			t.Errorf("visibleWindow(%d,%d,%d) = %d,%d; want %d,%d", c.cursor, c.n, c.rows, s, e, c.start, c.end)
		}
	}
}

func TestPlaylistsModelChannelSwitchReloads(t *testing.T) {
	m := loadedPlaylistsModel()
	m.SetMembers(m.OpenPlaylist(), "PL1", map[string]bool{"v1": true}, nil)

	if !m.NeedsLoad("course") {
		t.Fatal("switching channel must require a reload")
	}
	m.SetLoading("course")
	if m.panelMode() != plModeList || len(m.playlists) != 0 || m.MembershipKnown() {
		t.Fatal("old channel data must be discarded when loading another channel")
	}

	// Late responses for the previous channel must not land in the new channel's panel.
	m.SetLoaded("seminar", []youtube.Playlist{{ID: "PL1", Title: "Seminar"}}, nil, nil)
	m.ApplyCreated("seminar", &youtube.Playlist{ID: "PL9", Title: "Late"}, nil)
	if len(m.playlists) != 0 {
		t.Fatalf("stale responses applied: %+v", m.playlists)
	}

	m.SetLoaded("course", []youtube.Playlist{{ID: "PC1", Title: "Course"}}, nil, nil)
	if m.NeedsLoad("course") || m.SelectedPlaylist().ID != "PC1" {
		t.Fatal("expected course playlists to be loaded")
	}
	if !strings.Contains(m.View(), "YOUTUBE PLAYLISTS: course") {
		t.Error("expected the panel title to name the channel")
	}
}

func TestPlaylistsModelDropsSupersededMemberFetch(t *testing.T) {
	m := loadedPlaylistsModel()
	first := m.OpenPlaylist()
	m.BackToList()
	second := m.OpenPlaylist()

	m.SetMembers(second, "PL1", map[string]bool{}, nil)
	m.SetPending("v1", "Talk One", true)
	m.ApplyToggle("PL1", "v1", true, nil)

	// The slower first fetch carries pre-add data and must not erase the add.
	m.SetMembers(first, "PL1", map[string]bool{}, nil)
	if !m.IsMember("v1") {
		t.Fatal("a superseded member fetch overwrote newer membership")
	}
}

func TestPlaylistsModelCreateAfterUserMovedOn(t *testing.T) {
	m := loadedPlaylistsModel()
	m.BeginCreate()
	m.SetCreating("New")
	m.CancelCreate()
	m.MoveCursor(1) // now on PL2

	m.ApplyCreated("seminar", &youtube.Playlist{ID: "PL9", Title: "New"}, nil)
	if m.panelMode() != plModeList {
		t.Fatalf("create response must not navigate after cancel, mode=%d", m.panelMode())
	}
	if m.SelectedPlaylist().ID != "PL2" {
		t.Fatalf("cursor moved off PL2 to %s", m.SelectedPlaylist().ID)
	}

	// A reload that already contains the playlist must not lead to a duplicate entry.
	m.ApplyCreated("seminar", &youtube.Playlist{ID: "PL9", Title: "New"}, nil)
	if len(m.playlists) != 3 {
		t.Fatalf("expected 3 playlists without duplicates, got %d", len(m.playlists))
	}
}

func TestTabBarFitsEightyColumns(t *testing.T) {
	for screen := 0; screen < numScreens; screen++ {
		bar := RenderTabBar(screen, 80, DefaultTheme())
		if lines := strings.Count(bar, "\n") + 1; lines != 1 {
			t.Fatalf("tab bar wraps to %d lines at 80 columns:\n%s", lines, bar)
		}
	}
}

func TestPlaylistsModelCopiedFeedback(t *testing.T) {
	m := loadedPlaylistsModel()
	url := playlistURL("PL1")
	if url != "https://www.youtube.com/playlist?list=PL1" {
		t.Fatalf("unexpected playlist URL %q", url)
	}

	m.SetCopied(youtube.Playlist{ID: "PL1", Title: "Seminar", PrivacyStatus: "public"}, url, nil)
	if m.isError || !strings.Contains(m.feedback, url) {
		t.Fatalf("expected success feedback with the link, got %q", m.feedback)
	}

	m.SetCopied(youtube.Playlist{ID: "PL3", Title: "Drafts", PrivacyStatus: "private"}, playlistURL("PL3"), nil)
	if !m.isError || !strings.Contains(m.feedback, "PRIVATE") {
		t.Fatalf("expected a private-playlist warning, got %q", m.feedback)
	}

	m.SetCopied(youtube.Playlist{ID: "PL1"}, url, errors.New("no clipboard"))
	if !m.isError || !strings.Contains(m.feedback, url) {
		t.Fatalf("a failed copy must show the link so it can be copied by hand, got %q", m.feedback)
	}
}
