package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// A panel is a rectangle: every line takes exactly the cells it was given,
// which is what lets panels sit side by side without drifting apart.
func TestPanelIsARectangle(t *testing.T) {
	contents := map[string]string{
		"empty":        "",
		"one line":     "hello",
		"several":      "one\ntwo\nthree",
		"styled":       titleStyle.Render("colored") + "\n" + moreStyle.Render("dim"),
		"wide runes":   "▓▓▓▓▓▓▓▓\n░░░░",
		"long line":    strings.Repeat("x", 200),
		"ragged lines": "short\n" + strings.Repeat("y", 60) + "\nmid",
	}

	for name, content := range contents {
		t.Run(name, func(t *testing.T) {
			for _, width := range []int{12, 20, 40, 80} {
				for _, height := range []int{0, 3, 6, 12} {
					box := panel("TITLE", content, width, height, borderStyle)

					lines := strings.Split(box, "\n")

					for i, line := range lines {
						if got := lipgloss.Width(line); got != width {
							t.Fatalf("width %d, height %d: line %d is %d cells wide:\n%s",
								width, height, i, got, box)
						}
					}

					if height > 0 && len(lines) != height {
						t.Errorf("width %d, height %d: the panel has %d lines:\n%s",
							width, height, len(lines), box)
					}
				}
			}
		})
	}
}

// The title is written into the top edge, and a title too long for the box
// does not push the border out of shape.
func TestPanelTitle(t *testing.T) {
	box := panel("PAGES", "content", 20, 4, borderStyle)

	first := strings.Split(box, "\n")[0]

	if !strings.Contains(first, "PAGES") {
		t.Errorf("the top edge has no title:\n%s", box)
	}

	if !strings.HasPrefix(first, borderTopLeft) || !strings.HasSuffix(first, borderTopRight) {
		t.Errorf("the top edge is not a border:\n%s", box)
	}

	long := panel(strings.Repeat("LONG", 10), "content", 20, 4, borderStyle)

	for i, line := range strings.Split(long, "\n") {
		if got := lipgloss.Width(line); got != 20 {
			t.Fatalf("line %d of a panel with a long title is %d cells wide:\n%s", i, got, long)
		}
	}
}

// A box too narrow to hold anything is not drawn at all.
func TestPanelTooNarrow(t *testing.T) {
	for _, width := range []int{0, 1, 3, 4} {
		if box := panel("T", "content", width, 3, borderStyle); box != "" {
			t.Errorf("width %d drew a panel:\n%q", width, box)
		}
	}
}
