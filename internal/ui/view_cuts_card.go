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

	statsLines := lipgloss.Height(statsBox)
	availForCuts := bodyHeight - statsLines
	var cutsBox string
	if availForCuts >= 3 {
		cutsBox = m.renderCutsBox(width, intervals, availForCuts-2)
	}

	var boxes []string
	boxes = append(boxes, statsBox)
	if cutsBox != "" {
		boxes = append(boxes, cutsBox)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, boxes...)
	content = clampHeight(content, bodyHeight)
	return lipgloss.NewStyle().Width(width).Height(bodyHeight).MarginLeft(1).Render(content)
}

// clampHeight trims rendered content to at most maxLines lines.
func clampHeight(content string, maxLines int) string {
	if maxLines < 1 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content
	}
	return strings.Join(lines[:maxLines], "\n")
}

const (
	// minTranscriptBody keeps a minimal number of transcript rows visible so
	// navigation stays usable even when the bottom card grows.
	minTranscriptBody = 3
)

// cardLayout computes the transcript body height and the bottom cue card
// dimensions for the active cue. The card grows to show the cue's full text and
// takes those lines from the transcript pane, keeping only minTranscriptBody
// rows available above.
func (m CutsModel) cardLayout() (bodyHeight, cardHeight, innerLines int) {
	if len(m.cues) == 0 || m.cursor < 0 || m.cursor >= len(m.cues) {
		bodyHeight = m.height - 2
		if bodyHeight < minTranscriptBody {
			bodyHeight = minTranscriptBody
		}
		return bodyHeight, 0, 0
	}

	// Size the card from the actual wrapped content (metadata header plus full
	// text) so the height budget always matches what renderBottomCueCard draws.
	innerLines = len(m.bottomCardContent(m.cues[m.cursor]))

	// Header and footer occupy 2 lines, the card border 2 more; keep a minimal
	// transcript pane so the card cannot starve the list entirely.
	maxInner := m.height - 4 - minTranscriptBody
	if maxInner < 3 {
		maxInner = 3
	}
	if innerLines > maxInner {
		innerLines = maxInner
	}
	if innerLines < 3 {
		innerLines = 3
	}

	cardHeight = innerLines + 2
	bodyHeight = m.height - 2 - cardHeight
	if bodyHeight < minTranscriptBody {
		bodyHeight = minTranscriptBody
	}
	return bodyHeight, cardHeight, innerLines
}

// bottomCardInnerWidth returns the usable content width inside the bottom card,
// i.e. the terminal width minus the box border and horizontal padding.
func (m CutsModel) bottomCardInnerWidth() int {
	innerWidth := m.width - 4
	if innerWidth < 20 {
		innerWidth = 20
	}
	return innerWidth
}

// ellipsize shortens s so the result, including a trailing "...", fits in width
// display columns. It always appends the ellipsis.
func ellipsize(s string, width int) string {
	if width <= 3 {
		return "..."
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)+"...") > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "..."
}

// wrapCueText wraps the active cue's full text (speaker prefix and quotes) to
// the bottom card's inner width.
func (m CutsModel) wrapCueText(cue model.SubtitleCue) []string {
	innerWidth := m.bottomCardInnerWidth()

	speakerPrefix := ""
	if cue.Speaker != "" {
		speakerPrefix = m.theme.SpeakerStyle.Render(cue.Speaker+": ") + " "
	}
	fullText := speakerPrefix + "\"" + cue.Text + "\""

	return strings.Split(lipgloss.NewStyle().Width(innerWidth).Render(fullText), "\n")
}

// bottomCardContent returns the exact wrapped content lines of the bottom cue
// card: the metadata header followed by the cue's full text. cardLayout and
// renderBottomCueCard both rely on this so the height budget always matches the
// drawn card and no text line is ever dropped.
func (m CutsModel) bottomCardContent(cue model.SubtitleCue) []string {
	innerWidth := m.bottomCardInnerWidth()
	header := strings.Split(
		lipgloss.NewStyle().Width(innerWidth).Render(m.bottomCardHeader(cue)),
		"\n",
	)
	content := make([]string, 0, len(header)+4)
	content = append(content, header...)
	content = append(content, m.wrapCueText(cue)...)
	return content
}

// bottomCardHeader builds the metadata line for the bottom cue card. The cut
// reason is truncated so the header stays on a single line and never steals a
// line from the cue text.
func (m CutsModel) bottomCardHeader(cue model.SubtitleCue) string {
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
	if ok, ch := m.isChapterStart(cue); ok {
		statusBadge += " " + m.theme.TitleStyle.Render(" 🔖 "+ch.Title+" ")
	}

	header := fmt.Sprintf(
		"Cue #%d of %d  [%s -> %s] (%s)  %s",
		m.cursor+1,
		len(m.cues),
		vtt.FormatTimestampShort(cue.Start),
		vtt.FormatTimestampShort(cue.End),
		durSec,
		statusBadge,
	)

	if cue.CutReason != "" {
		label := m.theme.HelpDesc.Render("  Reason: ")
		avail := m.bottomCardInnerWidth() - lipgloss.Width(header) - lipgloss.Width(label)
		if avail > 10 {
			reason := cue.CutReason
			if lipgloss.Width(reason) > avail {
				reason = ellipsize(reason, avail)
			}
			header += label + m.theme.HelpDesc.Render(reason)
		}
	}
	return header
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

// renderBottomCueCard renders the active cue card spanning full width.
func (m CutsModel) renderBottomCueCard(width, innerLines int) string {
	if len(m.cues) == 0 || m.cursor < 0 || m.cursor >= len(m.cues) {
		return ""
	}
	if innerLines < 1 {
		innerLines = 1
	}

	innerWidth := m.bottomCardInnerWidth()
	contentLines := m.bottomCardContent(m.cues[m.cursor])
	if len(contentLines) > innerLines {
		contentLines = contentLines[:innerLines]
		last := len(contentLines) - 1
		if last >= 0 {
			contentLines[last] = ellipsize(contentLines[last], innerWidth)
		}
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

// overlayFloatingHelp renders a centered floating modal overlay over the screen.
func (m CutsModel) overlayFloatingHelp(lines []string) []string {
	if len(lines) < 8 || m.width < 30 {
		return lines
	}

	dialog := m.renderFloatingHelpDialog(m.width)
	dialogLines := strings.Split(dialog, "\n")
	dHeight := len(dialogLines)
	if dHeight >= len(lines)-2 {
		dHeight = len(lines) - 2
		dialogLines = dialogLines[:dHeight]
	}

	dWidth := lipgloss.Width(dialogLines[0])
	startX := (m.width - dWidth) / 2
	if startX < 0 {
		startX = 0
	}

	startY := (len(lines) - dHeight) / 2
	if startY < 1 {
		startY = 1
	}

	res := make([]string, len(lines))
	copy(res, lines)

	for i := 0; i < dHeight; i++ {
		targetY := startY + i
		if targetY >= len(res)-1 {
			break
		}
		padLeft := strings.Repeat(" ", startX)
		padRight := ""
		if m.width > startX+dWidth {
			padRight = strings.Repeat(" ", m.width-startX-dWidth)
		}
		res[targetY] = padLeft + dialogLines[i] + padRight
	}

	return res
}

// renderFloatingHelpDialog renders a centered floating modal overlay with keyboard shortcuts.
func (m CutsModel) renderFloatingHelpDialog(width int) string {
	title := m.theme.TitleStyle.Render(" KEYBOARD SHORTCUTS ") + "  " + m.theme.HelpDesc.Render("(F1, ?, or Esc to close)")
	rows := []string{
		title,
		"",
		"  ← / →       Switch tabs (1..4)         Tab         Metadata screen",
		"  j / k       Navigate cues              Space / x   Toggle Cut / Keep",
		"  [ / ]       Jump prev / next chapter   m           Mark chapter start",
		"  g / G       Jump top / bottom          s / Ctrl+S  Save cuts to disk",
		"  pgdn / pgup Page down / up             p           Preview cue (ffplay)",
		"  n / N       Jump next / prev cut       c / Ctrl+R  Commit cuts & render",
		"  t (Render)  Clean transcript (AI)     r / Ctrl+A  Redo chapters with AI",
		"  q / Ctrl+C  Quit talk_cut              F1 / ?      Toggle help modal",
	}

	dialogWidth := 78
	if dialogWidth > width-4 {
		dialogWidth = width - 4
	}
	if dialogWidth < 40 {
		dialogWidth = 40
	}

	content := strings.Join(rows, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Primary).
		Background(lipgloss.Color("#0F172A")).
		Foreground(lipgloss.Color("#F8FAFC")).
		Padding(0, 1).
		Width(dialogWidth).
		Render(content)
}

// renderChapterBannerRow formats a dedicated separator line for a chapter header.
func (m CutsModel) renderChapterBannerRow(ch model.ChapterMarker, width int) string {
	timeStr := vtt.FormatTimestampShort(ch.OriginalTime)
	title := fmt.Sprintf("── 🔖 %s (%s) ", ch.Title, timeStr)
	tWidth := lipgloss.Width(title)
	if tWidth > width {
		runes := []rune(title)
		for len(runes) > 0 && lipgloss.Width(string(runes)+"...") > width {
			runes = runes[:len(runes)-1]
		}
		title = string(runes) + "..."
		tWidth = lipgloss.Width(title)
	}
	fill := ""
	if width > tWidth {
		fill = strings.Repeat("─", width-tWidth)
	}
	content := title + fill
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#818CF8")).
		Background(lipgloss.Color("#0F172A")).
		Width(width).
		Render(content)
}

// transcriptItem represents a single row in the scrolling transcript panel.
type transcriptItem struct {
	isChapter bool
	cueIdx    int
	chMarker  model.ChapterMarker
}

// buildTranscriptItems generates the unified row sequence with injected chapter headers.
func (m CutsModel) buildTranscriptItems() []transcriptItem {
	items := make([]transcriptItem, 0, len(m.cues)+len(m.chapters))
	for i, cue := range m.cues {
		if ok, ch := m.isChapterStart(cue); ok {
			items = append(items, transcriptItem{
				isChapter: true,
				chMarker:  ch,
			})
		}
		items = append(items, transcriptItem{
			isChapter: false,
			cueIdx:    i,
		})
	}
	return items
}

// cursorItemIndex locates the row index corresponding to the currently selected cue.
func (m CutsModel) cursorItemIndex(items []transcriptItem) int {
	for idx, item := range items {
		if !item.isChapter && item.cueIdx == m.cursor {
			return idx
		}
	}
	return 0
}
