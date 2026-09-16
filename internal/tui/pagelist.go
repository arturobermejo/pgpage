package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// Styles of the page navigator.
var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
	rowStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	moreStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	newStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	invalidStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
)

// pageList renders the page navigator: the block numbers of the relation,
// the window of rows starting at top, with selected marked and what the
// cache knows about each page.
//
//	PAGES
//	    0   12 items   93% free   OK
//	  > 1   41 items   73% free   OK
//	    2    0 items          —   NEW
//	    ↓ 125 more
func pageList(pages, selected, top pgpage.BlockNumber, rows int, summaries summaryCache) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("PAGES"))

	if pages == 0 {
		b.WriteString("\n" + moreStyle.Render("no complete pages"))
		return b.String()
	}

	last := lastVisible(pages, top, rows)

	// Every number is right aligned to the width of the largest one, so the
	// digits line up however many pages the relation has.
	digits := len(fmt.Sprint(pages - 1))

	for block := top; block <= last; block++ {
		summary, cached := summaries[block]
		row := fmt.Sprintf("%*d %s", digits, block, summaryColumns(summary, cached))

		if block == selected {
			b.WriteString("\n" + selectedStyle.Render("> ") + rowStyle.Render(row))
			continue
		}

		b.WriteString("\n  " + rowStyle.Render(row))
	}

	if hidden := pages - 1 - last; hidden > 0 {
		b.WriteString("\n" + moreStyle.Render(fmt.Sprintf("  ↓ %d more", hidden)))
	}

	return b.String()
}

// summaryColumns returns the columns the navigator shows about a page: how
// many line pointers it has, how much of it is free and its status. A page
// the cache does not hold yet shows an ellipsis, and a page whose header
// says nothing shows dashes.
func summaryColumns(s pgpage.PageSummary, cached bool) string {
	if !cached {
		return moreStyle.Render("…")
	}

	items, free := "—", "—"

	if s.Status == pgpage.StatusOK || s.Status == pgpage.StatusNew {
		items = fmt.Sprintf("%d items", s.Header.ItemCount())
	}

	if percent, ok := s.FreeSpacePercent(); ok {
		free = fmt.Sprintf("%.0f%% free", percent)
	}

	return fmt.Sprintf("%s %s  %s",
		padLeft(items, 10), padLeft(free, 9), statusStyle(s.Status).Render(s.Status.String()))
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
// is inside a window of rows blocks. The list stays still while the
// selection is visible, and follows it by the least amount when it is not.
func scrollTo(pages, top, selected pgpage.BlockNumber, rows int) pgpage.BlockNumber {
	if rows < 1 {
		rows = 1
	}

	switch {
	case selected < top:
		top = selected // moved up, past the top of the window

	case uint64(selected) >= uint64(top)+uint64(rows):
		top = selected - pgpage.BlockNumber(rows) + 1 // moved down, past the bottom
	}

	// Leave no empty rows at the bottom while there are blocks above the
	// window that could fill them, which is what a window made taller does.
	if uint64(top)+uint64(rows) > uint64(pages) {
		if uint64(rows) >= uint64(pages) {
			return 0
		}

		return pages - pgpage.BlockNumber(rows)
	}

	return top
}
