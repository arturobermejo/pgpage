package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// topBarGap is the smallest number of spaces between the two sides of the
// top bar. With less room than that, the right side is dropped.
const topBarGap = 2

// topBar renders the relation summary, the first line of every screen:
//
//	pgpage  ./base/16384/24576  128 pages  8192 B/page  1.0 MB     blk 0/127
//
// The block counter sits at the right edge of width. A terminal too narrow
// for both sides gets only the left one, cut to width.
func topBar(rel *pgpage.Relation, block pgpage.BlockNumber, width int) string {
	left := strings.Join([]string{
		nameStyle.Render("pgpage"),
		pathStyle.Render(rel.Path()),
		factStyle.Render(fmt.Sprintf("%d pages", rel.PageCount())),
		factStyle.Render(fmt.Sprintf("%d B/page", pgpage.PageSize)),
		factStyle.Render(humanSize(rel.Size())),
	}, "  ")

	if width <= 0 {
		return left // no tea.WindowSizeMsg yet
	}

	right := blockStyle.Render(blockCounter(rel.PageCount(), block))

	// Width counts what the terminal shows: it skips the escape sequences
	// the styles added, and counts a wide rune as the two cells it takes.
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < topBarGap {
		return lipgloss.NewStyle().MaxWidth(width).Render(left)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)
}

// blockCounter returns the position of block among pages, as "blk 0/127".
func blockCounter(pages, block pgpage.BlockNumber) string {
	if pages == 0 {
		return "no pages"
	}

	return fmt.Sprintf("blk %d/%d", block, pages-1)
}

// humanSize formats a byte count the way the top bar shows it: 24576 becomes
// "24.0 KB" and 1048576 becomes "1.0 MB". The units are powers of 1024, the
// ones a page size of 8192 divides evenly. Sizes above a terabyte keep
// growing in GB, which no relation segment reaches.
func humanSize(n int64) string {
	const unit = 1024

	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	value, exp := float64(n)/unit, 0
	for value >= unit && exp < 2 {
		value /= unit
		exp++
	}

	return fmt.Sprintf("%.1f %s", value, [...]string{"KB", "MB", "GB"}[exp])
}
