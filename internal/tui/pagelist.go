package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// pageList renders the page navigator: the block numbers of the relation,
// the window of rows starting at top, with selected marked and what the
// cache knows about each page.
//
//	  #   LP  FREE  STATUS
//	  0   12   93%  OK
//	> 1   41   73%  OK
//	  2    0     —  NEW
//	  ↓ 125 more
//
// LP is the number of line pointers, not of tuples: the array keeps unused
// and dead line pointers too.
func pageList(pages, selected, top pgpage.BlockNumber, rows int, summaries summaryCache) string {
	if pages == 0 {
		return moreStyle.Render("no complete pages")
	}

	var b strings.Builder

	last := lastVisible(pages, top, rows)

	// Every number is right aligned to the width of the largest one, so the
	// digits line up however many pages the relation has.
	digits := len(fmt.Sprint(pages - 1))

	// The columns are named once, above them, instead of in every row.
	b.WriteString(fieldStyle.Render(fmt.Sprintf("  %*s %s  %s  %s",
		digits, "#", padLeft("LP", lpColumn), padLeft("FREE", freeColumn), "STATUS")) + "\n")

	for block := top; block <= last; block++ {
		summary, cached := summaries[block]
		row := fmt.Sprintf("%*d %s", digits, block, summaryColumns(summary, cached))

		if block == selected {
			b.WriteString(prefix(block, top) + selectedStyle.Render("> "+row))
			continue
		}

		b.WriteString(prefix(block, top) + "  " + rowStyle.Render(row))
	}

	if hidden := pages - 1 - last; hidden > 0 {
		b.WriteString("\n" + moreStyle.Render(fmt.Sprintf("  ↓ %d more", hidden)))
	}

	return b.String()
}

// prefix returns the line break before a row, except before the first one.
func prefix(block, top pgpage.BlockNumber) string {
	if block == top {
		return ""
	}

	return "\n"
}

// Widths of the columns of the navigator: the most line pointers a page can
// hold, (8192 - 24) / 4 = 2042, and "100%".
const (
	lpColumn   = 4
	freeColumn = 4
)

// summaryColumns returns the columns the navigator shows about a page: how
// many line pointers it has, how much of it is free and its status. A page
// the cache does not hold yet shows an ellipsis, and a page whose header
// says nothing shows dashes.
func summaryColumns(s pgpage.PageSummary, cached bool) string {
	if !cached {
		return moreStyle.Render("…")
	}

	pointers, free := "—", "—"

	if s.Status == pgpage.StatusOK || s.Status == pgpage.StatusNew {
		pointers = fmt.Sprint(s.Header.ItemCount())
	}

	if percent, ok := s.FreeSpacePercent(); ok {
		free = fmt.Sprintf("%.0f%%", percent)
	}

	return fmt.Sprintf("%s  %s  %s",
		padLeft(pointers, lpColumn), padLeft(free, freeColumn), statusStyle(s.Status).Render(s.Status.String()))
}

// padLeft right aligns s in a field of width cells. It counts cells, not
// bytes: the dash "—" is one cell wide and three bytes long.
func padLeft(s string, width int) string {
	if pad := width - lipgloss.Width(s); pad > 0 {
		return strings.Repeat(" ", pad) + s
	}

	return s
}

// statusStyle returns the color a page status is shown in. Color is never
// the only difference: the status is spelled out next to it.
func statusStyle(status pgpage.PageStatus) lipgloss.Style {
	switch status {
	case pgpage.StatusOK:
		return okStyle
	case pgpage.StatusNew:
		return newStyle
	case pgpage.StatusInvalid:
		return invalidStyle
	default:
		return moreStyle
	}
}

// lastVisible returns the last block the window shows: rows blocks starting
// at top, without running past the end of the relation.
func lastVisible(pages, top pgpage.BlockNumber, rows int) pgpage.BlockNumber {
	if rows < 1 {
		rows = 1
	}

	// uint64 because top+rows can be past the largest block number.
	last := uint64(top) + uint64(rows) - 1
	if last > uint64(pages-1) {
		last = uint64(pages - 1)
	}

	return pgpage.BlockNumber(last)
}

// scrollTo returns the first block the navigator must show so that selected
// is inside a window of rows blocks.
func scrollTo(pages, top, selected pgpage.BlockNumber, rows int) pgpage.BlockNumber {
	return windowTop(pages, top, selected, rows)
}

// windowTop returns the first row a window of rows rows must start at so
// that selected is inside it, in a list of count rows. The same rule serves
// the pages of a relation and the line pointers of a page.
//
// The window stays still while the selection is visible, and follows it by
// the least amount when it is not. The arithmetic is done in uint64: block
// numbers reach 2^32, and top+rows must not wrap around.
func windowTop[T ~int | ~uint32](count, top, selected T, rows int) T {
	c, t, s, r := uint64(count), uint64(top), uint64(selected), uint64(max(rows, 1))

	switch {
	case s < t:
		t = s // moved up, past the top of the window

	case s >= t+r:
		t = s - r + 1 // moved down, past the bottom
	}

	// Leave no empty rows at the bottom while there are rows above the window
	// that could fill them, which is what a window made taller does.
	if t+r > c {
		if r >= c {
			return 0
		}

		t = c - r
	}

	return T(t)
}
