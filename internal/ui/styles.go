// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme defines the True-Color Lip Gloss styling palette.
type Theme struct {
	Primary    lipgloss.Color
	Secondary  lipgloss.Color
	Success    lipgloss.Color
	Danger     lipgloss.Color
	Warning    lipgloss.Color
	Muted      lipgloss.Color
	Subtle     lipgloss.Color
	Cursor     lipgloss.Color
	Background lipgloss.Color
	Surface    lipgloss.Color
	Highlight  lipgloss.Color

	TitleStyle     lipgloss.Style
	SubtitleStyle  lipgloss.Style
	CueNormal      lipgloss.Style
	CueSelected    lipgloss.Style
	RowSelected    lipgloss.Style
	CueCut         lipgloss.Style
	CueCutSelected lipgloss.Style
	CueReview      lipgloss.Style
	SpeakerStyle   lipgloss.Style
	TimestampStyle lipgloss.Style
	SidebarBox     lipgloss.Style
	StatsLabel     lipgloss.Style
	StatsValue     lipgloss.Style
	PrimaryText    lipgloss.Style
	DangerText     lipgloss.Style
	SuccessText    lipgloss.Style
	WarningText    lipgloss.Style
	HelpKey        lipgloss.Style
	HelpDesc       lipgloss.Style
	BadgeKept      lipgloss.Style
	BadgeCut       lipgloss.Style
	BadgeReview    lipgloss.Style
	IconKept       lipgloss.Style
	IconCut        lipgloss.Style
	IconReview     lipgloss.Style
}

// DefaultTheme returns a polished 24-bit dark theme.
func DefaultTheme() Theme {
	t := Theme{
		Primary:    lipgloss.Color("#38BDF8"), // Sky blue
		Secondary:  lipgloss.Color("#818CF8"), // Indigo
		Success:    lipgloss.Color("#34D399"), // Emerald
		Danger:     lipgloss.Color("#F87171"), // Coral Red
		Warning:    lipgloss.Color("#FBBF24"), // Amber
		Muted:      lipgloss.Color("#94A3B8"), // Slate
		Subtle:     lipgloss.Color("#475569"), // Dark Slate
		Cursor:     lipgloss.Color("#F43F5E"), // Rose
		Background: lipgloss.Color("#0F172A"),
		Surface:    lipgloss.Color("#1E293B"),
		Highlight:  lipgloss.Color("#064E3B"), // Dark green for all active highlights
	}

	t.TitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(t.Primary).
		Padding(0, 1)

	t.SubtitleStyle = lipgloss.NewStyle().
		Foreground(t.Muted)

	initCueStyles(&t)

	t.SpeakerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Secondary)

	t.TimestampStyle = lipgloss.NewStyle().
		Foreground(t.Muted)

	t.SidebarBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Subtle).
		Padding(0, 1)

	t.StatsLabel = lipgloss.NewStyle().
		Foreground(t.Muted)

	t.StatsValue = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF"))

	t.PrimaryText = lipgloss.NewStyle().Foreground(t.Primary)
	t.DangerText = lipgloss.NewStyle().Foreground(t.Danger)
	t.SuccessText = lipgloss.NewStyle().Foreground(t.Success)
	t.WarningText = lipgloss.NewStyle().Foreground(t.Warning)

	t.HelpKey = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Primary)

	t.HelpDesc = lipgloss.NewStyle().
		Foreground(t.Muted)

	initBadgesAndIcons(&t)

	return t
}

func initCueStyles(t *Theme) {
	t.CueNormal = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E2E8F0"))

	t.CueSelected = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(t.Highlight)

	t.RowSelected = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(t.Highlight)

	t.CueCut = lipgloss.NewStyle().
		Foreground(t.Danger)

	t.CueCutSelected = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Danger).
		Background(lipgloss.Color("#451A1A"))

	t.CueReview = lipgloss.NewStyle().
		Foreground(t.Warning)
}

func initBadgesAndIcons(t *Theme) {
	t.BadgeKept = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#065F46")).
		Background(t.Success).
		Padding(0, 1)

	t.BadgeCut = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7F1D1D")).
		Background(t.Danger).
		Padding(0, 1)

	t.BadgeReview = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#78350F")).
		Background(t.Warning).
		Padding(0, 1)

	t.IconKept = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Success)

	t.IconCut = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Danger)

	t.IconReview = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Warning)
}

// RenderTabBar formats the persistent 4-tab navigation bar for any screen.
func RenderTabBar(activeTab, width int, theme Theme) string {
	tabs := []string{
		"[1] Cut Review",
		"[2] Metadata",
		"[3] Chapters",
		"[4] Export & Render",
	}

	var parts []string
	for i, t := range tabs {
		if i == activeTab {
			parts = append(parts, theme.TitleStyle.Render(" "+t+" "))
		} else {
			parts = append(parts, theme.HelpDesc.Render(" "+t+" "))
		}
	}

	appTitle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(" talk_cut: ")
	tabBar := appTitle + strings.Join(parts, " ")
	return lipgloss.NewStyle().Width(width).Background(lipgloss.Color("#1E293B")).Render(tabBar)
}
