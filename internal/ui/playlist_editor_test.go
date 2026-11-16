package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"talk_cut/internal/config"
	"talk_cut/internal/youtube"
)

func runeKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func loadedEditor(t *testing.T) PlaylistEditor {
	t.Helper()
	e := NewPlaylistEditor(config.Config{}, "seminar")
	pls := []youtube.Playlist{{ID: "PL1", Title: "Seminars", ItemCount: 5}}
	m, _ := e.Update(plPanelLoadedMsg{channel: "seminar", playlists: pls})
	return m.(PlaylistEditor)
}

func TestPlaylistEditorLoadsWithoutRecording(t *testing.T) {
	e := loadedEditor(t)
	view := e.View()
	if !strings.Contains(view, "Seminars") || !strings.Contains(view, "playlist editor") {
		t.Errorf("view lacks the playlist or the standalone header:\n%s", view)
	}
	if strings.Contains(view, "Cut Review") {
		t.Errorf("standalone view shows the recording tabs:\n%s", view)
	}
}

func TestPlaylistEditorTidyConfirmFlow(t *testing.T) {
	e := loadedEditor(t)
	m, cmd := e.Update(runeKey("t"))
	e = m.(PlaylistEditor)
	if cmd == nil {
		t.Fatal("t did not start a tidy plan")
	}
	if _, again := e.Update(runeKey("t")); again != nil {
		t.Error("a second t started another plan while one was in flight")
	}

	pl := youtube.Playlist{ID: "PL1", Title: "Seminars"}
	plan := youtube.TidyResult{Items: 5, Duplicates: make([]youtube.PlaylistItem, 2), Moves: make([]youtube.PlaylistMove, 1)}
	m, _ = e.Update(plTidyMsg{playlist: pl, dryRun: true, result: plan})
	e = m.(PlaylistEditor)
	if !e.view.AwaitingTidyConfirm() || !strings.Contains(e.View(), "remove 2 repeated") {
		t.Fatalf("plan was not offered for confirmation:\n%s", e.View())
	}

	// q answers the confirmation (cancel) instead of quitting.
	m, cmd = e.Update(runeKey("q"))
	e = m.(PlaylistEditor)
	if cmd != nil || e.view.AwaitingTidyConfirm() {
		t.Fatal("a non-y key did not cancel the tidy")
	}

	m, _ = e.Update(plTidyMsg{playlist: pl, dryRun: true, result: plan})
	e = m.(PlaylistEditor)
	m, cmd = e.Update(runeKey("y"))
	e = m.(PlaylistEditor)
	if cmd == nil {
		t.Fatal("y did not apply the tidy")
	}

	done := youtube.TidyResult{Duplicates: plan.Duplicates, Moves: plan.Moves, Final: make([]youtube.PlaylistItem, 3)}
	m, _ = e.Update(plTidyMsg{playlist: pl, result: done})
	e = m.(PlaylistEditor)
	if got := e.view.SelectedPlaylist().ItemCount; got != 3 {
		t.Errorf("item count after tidy = %d, want 3", got)
	}
	if !strings.Contains(e.View(), "Tidied") {
		t.Errorf("no success message:\n%s", e.View())
	}
}

func TestPlaylistEditorTidyAlreadyTidyAndError(t *testing.T) {
	e := loadedEditor(t)
	pl := youtube.Playlist{ID: "PL1", Title: "Seminars"}
	m, _ := e.Update(plTidyMsg{playlist: pl, dryRun: true, result: youtube.TidyResult{Items: 5}})
	e = m.(PlaylistEditor)
	if e.view.AwaitingTidyConfirm() || !strings.Contains(e.View(), "already tidy") {
		t.Errorf("a tidy playlist asked for confirmation:\n%s", e.View())
	}
	m, _ = e.Update(plTidyMsg{playlist: pl, err: errors.New("boom")})
	e = m.(PlaylistEditor)
	if !strings.Contains(e.View(), "boom") {
		t.Errorf("error not shown:\n%s", e.View())
	}
}

func TestPlaylistEditorQuits(t *testing.T) {
	e := loadedEditor(t)
	_, cmd := e.Update(runeKey("q"))
	if cmd == nil {
		t.Fatal("q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q did not quit")
	}
}

func TestPlaylistEditorTidyPlanDuringCreateDoesNotArm(t *testing.T) {
	e := loadedEditor(t)
	m, _ := e.Update(runeKey("t"))
	e = m.(PlaylistEditor)
	m, _ = e.Update(runeKey("n"))
	e = m.(PlaylistEditor)
	pl := youtube.Playlist{ID: "PL1", Title: "Seminars"}
	plan := youtube.TidyResult{Items: 5, Moves: make([]youtube.PlaylistMove, 1)}
	m, _ = e.Update(plTidyMsg{playlist: pl, dryRun: true, result: plan})
	e = m.(PlaylistEditor)
	if e.view.AwaitingTidyConfirm() {
		t.Fatal("a plan arriving while the title input has focus armed the confirmation")
	}
}
