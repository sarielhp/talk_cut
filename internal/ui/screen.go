package ui

import (
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"
)

// screen is a full-tab view made of a header, a body, and a footer.
type screen interface {
	renderHeader() string
	renderBody() string
	renderFooter() string
}

// renderScreen stacks the parts of s, or returns placeholder before the
// terminal size is known.
func renderScreen(s screen, width, height int, placeholder string) string {
	if width <= 0 || height <= 0 {
		return placeholder
	}
	return lipgloss.JoinVertical(lipgloss.Left, s.renderHeader(), s.renderBody(), s.renderFooter())
}

// resizeProgressBar fits bar to a terminal of width w, leaving a 20-column
// margin and capping it at 80 columns; narrow terminals keep the old width.
func resizeProgressBar(bar *progress.Model, w int) {
	if w <= 20 {
		return
	}
	bar.Width = min(w-20, 80)
}
