// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the keybindings available in the TUI.
type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding
	Toggle   key.Binding
	Preview  key.Binding
	NextCut  key.Binding
	PrevCut  key.Binding
	NextTab  key.Binding
	PrevTab  key.Binding
	Render   key.Binding
	Help     key.Binding
	Quit     key.Binding
}

// DefaultKeyMap returns the default keybindings for talk_cut.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("pgdn", "page down"),
		),
		Home: key.NewBinding(
			key.WithKeys("g", "home"),
			key.WithHelp("g", "top"),
		),
		End: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G", "bottom"),
		),
		Toggle: key.NewBinding(
			key.WithKeys(" ", "x"),
			key.WithHelp("space", "toggle cut"),
		),
		Preview: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "ffplay preview"),
		),
		NextCut: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next cut"),
		),
		PrevCut: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "prev cut"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("tab", "enter"),
			key.WithHelp("tab", "metadata screen"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("shift+tab", "esc"),
			key.WithHelp("esc", "back to cuts"),
		),
		Render: key.NewBinding(
			key.WithKeys("c", "ctrl+r"),
			key.WithHelp("c", "commit & cut video"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}
