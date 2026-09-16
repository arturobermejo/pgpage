package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Border pieces. Lip Gloss can draw a border around a block, but not write a
// title into its top edge, so the frame is drawn here.
const (
	borderTopLeft     = "┌"
	borderTopRight    = "┐"
	borderBottomLeft  = "└"
	borderBottomRight = "┘"
	borderHorizontal  = "─"
	borderVertical    = "│"
)

// panelPadding is the space between the border and the content, on each side.
const panelPadding = 1

// panel frames content in a box of width cells, with title written into the
// top edge:
//
//	┌─ PAGES ──────────────┐
//	│  > 0  185 items  OK  │
//	└──────────────────────┘
//
// Lines longer than the box are cut, so a panel is never wider than asked.
// A height above zero pads or cuts the content to exactly that many lines,
// which is what makes panels side by side end at the same row.
func panel(title, content string, width, height int, border lipgloss.Style) string {
	inner := width - 2 - 2*panelPadding
	if inner < 1 {
		return ""
	}

	var b strings.Builder

	// The top edge is "┌─ TITLE ─────┐": five cells go to the border and
	// the spaces around a title, which is cut if it does not fit. Without a
	// title it is a plain edge, like the bottom one.
	if title == "" {
		b.WriteString(border.Render(borderTopLeft + strings.Repeat(borderHorizontal, width-2) + borderTopRight))
	} else {
		title = lipgloss.NewStyle().MaxWidth(width - 5).Render(title)

		b.WriteString(border.Render(borderTopLeft+borderHorizontal) + " " +
			titleStyle.Render(title) + " ")

		if rest := width - 5 - lipgloss.Width(title); rest > 0 {
			b.WriteString(border.Render(strings.Repeat(borderHorizontal, rest)))
		}

		b.WriteString(border.Render(borderTopRight))
	}

	lines := strings.Split(content, "\n")
	if height > 0 {
		lines = fitLines(lines, height-2)
	}

	pad := strings.Repeat(" ", panelPadding)

	for _, line := range lines {
		line = lipgloss.NewStyle().MaxWidth(inner).Render(line)

		b.WriteString("\n" + border.Render(borderVertical) + pad +
			line + strings.Repeat(" ", max(inner-lipgloss.Width(line), 0)) +
			pad + border.Render(borderVertical))
	}

	b.WriteString("\n" + border.Render(borderBottomLeft+
		strings.Repeat(borderHorizontal, inner+2*panelPadding)+borderBottomRight))

	return b.String()
}

// boxWidth returns how wide a panel must be so that neither its content nor
// its title is cut.
func boxWidth(title, content string) int {
	return max(lipgloss.Width(content), lipgloss.Width(title)+1) + panelFrame
}

// fitLines returns exactly n lines: the first n, or all of them followed by
// blanks.
func fitLines(lines []string, n int) []string {
	if n < 1 {
		return nil
	}

	if len(lines) >= n {
		return lines[:n]
	}

	return append(lines, make([]string, n-len(lines))...)
}

// rule returns the horizontal line that separates the top bar and the help
// line from the panels.
func rule(width int) string {
	if width < 1 {
		return ""
	}

	return ruleStyle.Render(strings.Repeat(borderHorizontal, width))
}
