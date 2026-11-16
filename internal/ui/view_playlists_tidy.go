package ui

import (
	"fmt"

	"talk_cut/internal/youtube"
)

// BeginTidy marks a tidy plan as being computed; it reports false while another tidy is in flight.
func (m *PlaylistsModel) BeginTidy() bool {
	if m.tidyBusy || m.tidyAsk != nil {
		return false
	}
	m.tidyBusy = true
	m.setFeedback("Checking the playlist for repeated videos and order...", false)
	return true
}

// AwaitingTidyConfirm reports whether the next key answers a tidy confirmation.
func (m PlaylistsModel) AwaitingTidyConfirm() bool {
	return m.tidyAsk != nil
}

// ConfirmTidy answers the pending confirmation and returns the playlist to tidy when yes.
func (m *PlaylistsModel) ConfirmTidy(yes bool) (youtube.Playlist, bool) {
	pl := *m.tidyAsk
	m.tidyAsk = nil
	if !yes {
		m.setFeedback("Tidy cancelled; nothing was changed.", false)
		return pl, false
	}
	m.tidyBusy = true
	m.setFeedback(fmt.Sprintf("Tidying %q on YouTube...", pl.Title), false)
	return pl, true
}

// ApplyTidy records a tidy plan (dryRun), asking for confirmation when it would change anything, or a finished tidy.
func (m *PlaylistsModel) ApplyTidy(pl youtube.Playlist, dryRun bool, res youtube.TidyResult, err error) {
	m.tidyBusy = false
	if err != nil {
		m.setFeedback(fmt.Sprintf("Tidy of %q failed: %v", pl.Title, err), true)
		return
	}
	dups, moves := len(res.Duplicates), len(res.Moves)
	if dryRun && dups == 0 && moves == 0 {
		m.setFeedback(fmt.Sprintf("✔ %q is already tidy: %d videos, each once, newest first", pl.Title, res.Items), false)
		return
	}
	if dryRun && m.mode == plModeCreate {
		// The title input has focus, so a typed "y" must not answer a confirmation.
		m.setFeedback(fmt.Sprintf("%q can be tidied; press t on it to review the changes", pl.Title), false)
		return
	}
	if dryRun {
		m.tidyAsk = &pl
		m.setFeedback(fmt.Sprintf("Tidy %q: remove %d repeated video(s) and move %d item(s) on YouTube? [y] apply, any other key cancels",
			pl.Title, dups, moves), false)
		return
	}
	if i := m.indexOf(pl.ID); i >= 0 {
		m.playlists[i].ItemCount = int64(len(res.Final))
	}
	m.setFeedback(fmt.Sprintf("✔ Tidied %q: removed %d repeated video(s), moved %d item(s); now newest first",
		pl.Title, dups, moves), false)
}
