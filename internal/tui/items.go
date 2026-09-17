package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// items is what the line pointer view shows: the line pointers of one page,
// which of them is selected, and which the list starts at.
//
// It keeps the page too. The tuple view decodes the selected tuple from
// these bytes, so opening a tuple, or moving between tuples, reads nothing.
type items struct {
	block  pgpage.BlockNumber
	loaded bool
	page   []byte
	header pgpage.PageHeader
	ids    []pgpage.ItemID
	err    error

	selected int // index into ids: line pointer selected+1
	top      int // index of the first row the list shows
}

// itemsMsg carries the line pointers a command read from one page, and the
// page they were read from.
type itemsMsg struct {
	block  pgpage.BlockNumber
	page   []byte
	header pgpage.PageHeader
	ids    []pgpage.ItemID
	err    error
}

// loadItems returns a command that reads block and decodes its line pointer
// array. Like loadSummaries, it only reads: the result reaches the model as
// a message.
func loadItems(rel *pgpage.Relation, block pgpage.BlockNumber) tea.Cmd {
	return func() tea.Msg {
		page, err := rel.ReadPage(block)
		if err != nil {
			return itemsMsg{block: block, err: err}
		}

		h, err := pgpage.ParsePageHeader(page)
		if err != nil {
			return itemsMsg{block: block, page: page, err: err}
		}

		ids, err := pgpage.ParseItemIDs(page, h, nil)

		return itemsMsg{block: block, page: page, header: h, ids: ids, err: err}
	}
}

// number returns the line pointer number of an index into the array.
// PostgreSQL numbers line pointers from 1.
func number(index int) pgpage.OffsetNumber {
	return pgpage.OffsetNumber(index) + pgpage.FirstOffsetNumber
}

// itemList renders the line pointers of a page, the window of rows starting
// at top, with selected marked, and a count of each state below.
//
//	   # STATE     OFFSET  LEN
//	  #1 NORMAL      8152   35
//	> #2 DEAD           —    —
//	 #10 REDIRECT     →168   —
//
//	183 NORMAL · 1 REDIRECT
//	1 DEAD · 2 UNUSED
//
// The counts wrap to the width of the columns. The list is as wide as its
// widest line, and the counts of a page with all four states are wider than
// any row: left on one line they would widen the panel by half and squeeze
// the page map beside it down to its next size.
func itemList(it items, rows int) string {
	if !it.loaded {
		return moreStyle.Render("reading…")
	}

	if it.err != nil && len(it.ids) == 0 {
		return invalidStyle.Render(it.err.Error())
	}

	if len(it.ids) == 0 {
		return moreStyle.Render("no line pointers")
	}

	digits := len(fmt.Sprint(len(it.ids)))

	var b strings.Builder

	columns := itemColumns(digits)
	b.WriteString(fieldStyle.Render(columns))

	last := min(it.top+max(rows, 1), len(it.ids))

	for i := it.top; i < last; i++ {
		row := itemRow(i, it.ids[i], it.header, digits)

		if i == it.selected {
			b.WriteString("\n" + selectedStyle.Render("> ") + row)
			continue
		}

		b.WriteString("\n  " + row)
	}

	if hidden := len(it.ids) - last; hidden > 0 {
		b.WriteString("\n" + moreStyle.Render(fmt.Sprintf("  ↓ %d more", hidden)))
	}

	b.WriteString("\n")

	for _, line := range itemCountLines(it) {
		b.WriteString("\n" + moreStyle.Render(line))
	}

	return b.String()
}

// itemColumns returns the column titles of the list, for line pointer
// numbers of digits digits. Its width is the width of the rows.
func itemColumns(digits int) string {
	return fmt.Sprintf("  %-*s %-8s %6s %4s", digits+1, "#", "STATE", "OFFSET", "LEN")
}

// itemCountLines returns the state counts below the rows, wrapped to the
// width of the columns. How many lines they take depends on the page, and
// the model needs the number to know how many rows are left for the list.
func itemCountLines(it items) []string {
	digits := len(fmt.Sprint(len(it.ids)))

	return wrapParts(stateParts(it.ids), " · ", lipgloss.Width(itemColumns(digits)))
}

// wrapParts joins parts with sep into lines no wider than width, as many
// parts per line as fit, so a part is never cut in two. A part wider than
// width takes a line of its own.
func wrapParts(parts []string, sep string, width int) []string {
	lines := []string{""}

	for _, part := range parts {
		last := &lines[len(lines)-1]

		switch {
		case *last == "":
			*last = part
		case lipgloss.Width(*last)+lipgloss.Width(sep)+lipgloss.Width(part) <= width:
			*last += sep + part
		default:
			lines = append(lines, part)
		}
	}

	return lines
}

// itemRow renders one line pointer: its number, state, offset and length.
// A line pointer that points nowhere shows dashes, a redirect shows the line
// pointer it leads to, and one that breaks the page's rules is flagged.
func itemRow(index int, id pgpage.ItemID, h pgpage.PageHeader, digits int) string {
	n := number(index)
	offset, length := fmt.Sprint(id.Offset()), fmt.Sprint(id.Length())

	switch {
	case id.State() == pgpage.ItemRedirect:
		offset, length = fmt.Sprintf("→%d", id.Offset()), "—"
	case !id.HasStorage():
		offset, length = "—", "—"
	}

	row := rowStyle.Render(fmt.Sprintf("#%-*d ", digits, n)) +
		itemStateStyle(id.State()).Render(fmt.Sprintf("%-8s", id.State())) +
		rowStyle.Render(" "+padLeft(offset, 6)+" "+padLeft(length, 4))

	if h.CheckItemID(n, id) != nil {
		row += invalidStyle.Render(" !")
	}

	return row
}

// stateCounts returns how many line pointers are in each state, in the order
// of the states, leaving out the ones with none: "10 NORMAL · 1 DEAD".
func stateCounts(ids []pgpage.ItemID) string {
	return strings.Join(stateParts(ids), " · ")
}

// stateParts returns the counts of stateCounts one state per element.
func stateParts(ids []pgpage.ItemID) []string {
	var counts [4]int

	for _, id := range ids {
		counts[id.State()]++
	}

	var parts []string

	for _, state := range []pgpage.ItemState{pgpage.ItemNormal, pgpage.ItemRedirect, pgpage.ItemDead, pgpage.ItemUnused} {
		if counts[state] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[state], state))
		}
	}

	return parts
}

// itemStateStyle returns the color a line pointer state is shown in. Like the
// page states, the name is always written next to it.
func itemStateStyle(state pgpage.ItemState) lipgloss.Style {
	switch state {
	case pgpage.ItemNormal:
		return okStyle
	case pgpage.ItemDead:
		return deadStyle
	case pgpage.ItemRedirect:
		return redirectStyle
	default:
		return moreStyle
	}
}

// itemTitle names the panel of the selected line pointer.
func itemTitle(it items) string {
	if !it.loaded || it.selected >= len(it.ids) {
		return "LINE POINTER"
	}

	return fmt.Sprintf("LINE POINTER #%d", number(it.selected))
}

// itemPanel renders the selected line pointer: its fields, where its entry
// is in the page, and how the four bytes of the entry split into them.
//
//	lp_off      8152
//	lp_len      35
//	lp_flags    LP_NORMAL
//	entry at    0x0018
//
//	ITEMID WORD
//	raw         0x0047AFD8
//	off         bits 0-14  = 8152
//	flags       bits 15-16 = 1
//	len         bits 17-31 = 35
func itemPanel(it items) string {
	if !it.loaded || it.selected >= len(it.ids) {
		return moreStyle.Render("reading…")
	}

	n, id := number(it.selected), it.ids[it.selected]

	fields := renderRows([]headerRow{
		{"lp_off", fmt.Sprint(id.Offset()), valueStyle},
		{"lp_len", fmt.Sprint(id.Length()), valueStyle},
		{"lp_flags", "LP_" + id.State().String(), itemStateStyle(id.State())},
		{"entry at", fmt.Sprintf("0x%04X", pgpage.PageHeaderSize+int(n-1)*4), valueStyle},
	})

	if id.State() == pgpage.ItemRedirect {
		fields = append(fields, renderRows([]headerRow{
			{"redirect to", fmt.Sprintf("#%d", id.Offset()), redirectStyle},
		})...)
	}

	word := renderRows([]headerRow{
		{"raw", fmt.Sprintf("0x%08X", uint32(id)), valueStyle},
		{"off", fmt.Sprintf("bits 0-14  = %d", id.Offset()), valueStyle},
		{"flags", fmt.Sprintf("bits 15-16 = %d", id.State()), valueStyle},
		{"len", fmt.Sprintf("bits 17-31 = %d", id.Length()), valueStyle},
	})

	out := strings.Join(fields, "\n") + "\n\n" + titleStyle.Render("ITEMID WORD") + "\n" + strings.Join(word, "\n")

	if err := it.header.CheckItemID(n, id); err != nil {
		// The check says which rule the line pointer breaks. Its message is
		// shown whole, wrapped to the panel, as the page view does with the
		// errors of a header.
		wrap := lipgloss.NewStyle().Width(fieldColumn + itemPanelWidth)
		out += "\n\n" + wrap.Render(invalidStyle.Render("! "+err.Error()))
	}

	return out
}

// itemHighlights returns what the selected line pointer highlights: the 4
// bytes of its entry in the array, and the tuple it points to, if any. The
// entry comes first, as what the user selected.
//
// The tuple is named by its TID, (block,line pointer), as PostgreSQL does in
// t_ctid: a tuple has no number of its own, the number is the line pointer's.
// A redirect points to another line pointer instead, and says which.
func itemHighlights(it items) highlights {
	if !it.loaded || it.selected >= len(it.ids) {
		return nil
	}

	n, id := number(it.selected), it.ids[it.selected]
	start := pgpage.PageHeaderSize + it.selected*4

	entry := highlight{start: start, end: start + 4, label: fmt.Sprintf("line ptr #%d", n)}
	if id.State() == pgpage.ItemRedirect {
		entry.label += fmt.Sprintf(" → #%d", id.Offset())
	}

	// A line pointer that fails its checks may point anywhere, even past the
	// page: its bytes are not drawn as if they were a tuple.
	if !id.HasStorage() || id.State() == pgpage.ItemRedirect || it.header.CheckItemID(n, id) != nil {
		return highlights{entry}
	}

	return highlights{entry, {
		start:   int(id.Offset()),
		end:     int(id.Offset()) + int(id.Length()),
		label:   "tuple " + tid(it).String(),
		pointed: true,
	}}
}

// tupleFirst returns the highlights of the selected line pointer with its
// tuple first, for the hex view opened on the tuple to scroll to it. The
// colors do not change: they say which is the entry and which the tuple.
func tupleFirst(hl highlights) highlights {
	if len(hl) < 2 {
		return hl
	}

	return highlights{hl[1], hl[0]}
}

// itemPanelWidth is the widest value the line pointer panel shows, so that
// the panel keeps its width while the selection moves.
var itemPanelWidth = lipgloss.Width("bits 17-31 = 32767")

// tid returns the TID of the selected line pointer's tuple: its block, and
// the number of the line pointer that points to it.
func tid(it items) pgpage.ItemPointer {
	return pgpage.ItemPointer{Block: it.block, Offset: number(it.selected)}
}
