// Package ui implements the interactive True-Color Bubble Tea terminal interface for talk_cut.
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"talk_cut/internal/model"
	"talk_cut/internal/vtt"
)

// renderSidebar renders the right-hand panel scaled to fit bodyHeight.
func (m CutsModel) renderSidebar(width, bodyHeight int, stats model.CutStats, intervals []model.CutInterval) string {
	statsBox := m.renderStatsBox(width, stats)
	helpBox := m.renderHelpBox(width)

	statsLines := 4
	helpLines := 5
	availForCuts := bodyHeight - statsLines - helpLines
	var cutsBox string
	if availForCuts >= 3 {
		cutsBox = m.renderCutsBox(width, intervals, availForCuts-2)
	}

	var boxes []string
	boxes = append(boxes, statsBox)
	if cutsBox != "" {
		boxes = append(boxes, cutsBox)
	}
	boxes = append(boxes, helpBox)

	content := lipgloss.JoinVertical(lipgloss.Left, boxes...)
	return lipgloss.NewStyle().Width(width).Height(bodyHeight).MarginLeft(1).Render(content)
}

// renderStatsBox formats the duration and cut statistics.
func (m CutsModel) renderStatsBox(width int, stats model.CutStats) string {
	orig := vtt.FormatTimestampShort(stats.TotalOriginal)
	cut := vtt.FormatTimestampShort(stats.TotalCut)
	kept := vtt.FormatTimestampShort(stats.TotalKept)
	pct := fmt.Sprintf("%.1f%%", stats.KeptPercent())

	content := fmt.Sprintf(
		"Original: %s  Cut: %s\nKept:     %s (%s kept)",
		m.theme.StatsValue.Render(orig),
		m.theme.DangerText.Render("-"+cut),
		m.theme.SuccessText.Render(kept),
		pct,
	)

	return m.theme.SidebarBox.Width(width - 2).Render("STATISTICS\n" + content)
}

// renderCutsBox lists active cut intervals fitted to maxItems.
func (m CutsModel) renderCutsBox(width int, intervals []model.CutInterval, maxItems int) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("CUT REGIONS (%d)", len(intervals)))

	if len(intervals) == 0 {
		lines = append(lines, "  (no cuts marked)")
	} else {
		if maxItems < 1 {
			maxItems = 1
		}
		for i, cut := range intervals {
			if i >= maxItems {
				lines = append(lines, fmt.Sprintf("  ...and %d more", len(intervals)-maxItems))
				break
			}
			durSec := fmt.Sprintf("%.1fs", cut.Duration().Seconds())
			lines = append(lines, fmt.Sprintf("  %d. %s-%s (%s)",
				i+1,
				vtt.FormatTimestampShort(cut.Start),
				vtt.FormatTimestampShort(cut.End),
				durSec,
			))
		}
	}

	return m.theme.SidebarBox.Width(width - 2).Render(strings.Join(lines, "\n"))
}

// renderHelpBox renders keyboard shortcut hints.
func (m CutsModel) renderHelpBox(width int) string {
	hints := "[Space] Cut/Keep   [s] Save\n" +
		"[n/N]   Jump Cut   [p] Preview\n" +
		"[Tab]   Metadata   [F1/?] Help\n" +
		"[q]     Quit"
	return m.theme.SidebarBox.Width(width - 2).Render(hints)
}

// renderBottomCueCard renders the active cue card spanning full width.
func (m CutsModel) renderBottomCueCard(width, innerLines int) string {
	if len(m.cues) == 0 || m.cursor < 0 || m.cursor >= len(m.cues) {
		return ""
	}
	cue := m.cues[m.cursor]

	statusBadge := m.theme.BadgeKept.Render(" ✔ KEEP ")
	if cue.Action == model.ActionCut {
		statusBadge = m.theme.BadgeCut.Render(" ✂ CUT ")
	} else if cue.Action == model.ActionReview {
		statusBadge = m.theme.BadgeReview.Render(" ? REVIEW ")
	}

	hasFeedback := !m.savedAt.IsZero() && time.Since(m.savedAt) < 4*time.Second && m.saveFeedback != ""
	if hasFeedback {
		if m.saveIsError {
			statusBadge += " " + m.theme.BadgeCut.Render(" ✗ ERROR ")
		} else if strings.Contains(m.saveFeedback, "Playing") {
			statusBadge += " " + m.theme.TitleStyle.Render(" ▶ PLAYING ")
		} else {
			statusBadge += " " + m.theme.BadgeKept.Render(" ✔ SAVED ")
		}
	}

	durSec := fmt.Sprintf("%.2fs", cue.Duration().Seconds())
	line1 := fmt.Sprintf(
		"Cue #%d of %d  [%s -> %s] (%s)  %s",
		m.cursor+1,
		len(m.cues),
		vtt.FormatTimestampShort(cue.Start),
		vtt.FormatTimestampShort(cue.End),
		durSec,
		statusBadge,
	)

	innerWidth := width - 4
	if innerWidth < 20 {
		innerWidth = 20
	}

	if cue.CutReason != "" {
		avail := innerWidth - len(durSec) - 45
		if avail > 10 {
			reason := cue.CutReason
			if len(reason) > avail {
				reason = reason[:avail-3] + "..."
			}
			line1 += "  " + m.theme.HelpDesc.Render("Reason: "+reason)
		}
	}

	speakerPrefix := ""
	if cue.Speaker != "" {
		speakerPrefix = m.theme.SpeakerStyle.Render(cue.Speaker+": ") + " "
	}
	fullText := speakerPrefix + "\"" + cue.Text + "\""

	wrapped := lipgloss.NewStyle().Width(innerWidth).Render(fullText)
	textLines := strings.Split(wrapped, "\n")

	maxTextLines := innerLines - 1
	if maxTextLines < 1 {
		maxTextLines = 1
	}
	if len(textLines) > maxTextLines {
		textLines = textLines[:maxTextLines]
		lastIdx := maxTextLines - 1
		if len(textLines[lastIdx]) > 3 {
			textLines[lastIdx] = textLines[lastIdx][:len(textLines[lastIdx])-3] + "..."
		}
	}

	contentLines := append([]string{line1}, textLines...)
	if len(contentLines) > innerLines {
		contentLines = contentLines[:innerLines]
	}

	cardContent := strings.Join(contentLines, "\n")
	box := m.theme.SidebarBox.
		Width(width - 2).
		Height(innerLines).
		Render(cardContent)

	boxLines := strings.Split(box, "\n")
	cardTotalHeight := innerLines + 2
	if len(boxLines) > cardTotalHeight {
		res := make([]string, 0, cardTotalHeight)
		res = append(res, boxLines[0])
		res = append(res, boxLines[1:cardTotalHeight-1]...)
		res = append(res, boxLines[len(boxLines)-1])
		box = strings.Join(res, "\n")
	}
	return box
}

// renderHelpModal renders keyboard shortcuts cheat sheet.
func (m CutsModel) renderHelpModal(width, innerLines int) string {
	title := m.theme.TitleStyle.Render(" KEYBOARD SHORTCUTS ") + "  " + m.theme.HelpDesc.Render("(Press F1, ?, or Esc to close)")
	rows := []string{
		title,
		"  j / k       Navigate cues              Space / x   Toggle Cut / Keep (auto-saves)",
		"  g / G       Jump top / bottom          s / Ctrl+S  Save cut decisions to talk_cuts.json",
		"  pgdn / pgup Page down / up             p           Preview cue at timestamp (ffplay)",
		"  n / N       Jump next / prev cut       Tab / Enter Metadata & YouTube chapters",
		"  F1 / ?      Toggle help                q / Ctrl+C  Quit",
	}
	if len(rows) > innerLines {
		rows = rows[:innerLines]
	}
	for len(rows) < innerLines {
		rows = append(rows, "")
	}
	return m.theme.SidebarBox.
		Width(width - 2).
		Height(innerLines).
		Render(strings.Join(rows, "\n"))
}
