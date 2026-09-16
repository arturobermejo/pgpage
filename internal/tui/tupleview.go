package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// tupleValueWidth is the widest value of the tuple panel, the largest ctid,
// so that the panel keeps its width while moving between tuples.
var tupleValueWidth = lipgloss.Width(pgpage.ItemPointer{Block: 1<<32 - 1, Offset: 1<<16 - 1}.String())

// minFlagsWidth is the narrowest the flags panel can be and still show a
// flag name next to its value.
const minFlagsWidth = 44

// hasTuple reports whether a line pointer points to tuple bytes. A redirect
// points to another line pointer, and unused and most dead line pointers to
// nothing; a dead one that still keeps its storage does have a tuple, the
// version that pruning has not removed yet.
func hasTuple(id pgpage.ItemID) bool {
	return id.HasStorage() && id.State() != pgpage.ItemRedirect
}

// selectedTuple decodes the tuple of the selected line pointer from the page
// the line pointer view read.
func selectedTuple(it items) (pgpage.HeapTuple, error) {
	return pgpage.HeapTupleAt(it.page, it.header, number(it.selected))
}

// tupleTitle names the tuple panel.
func tupleTitle(it items) string {
	return fmt.Sprintf("TUPLE #%d", number(it.selected))
}

// tuplePanel renders the header of the selected tuple, grouped by what each
// field is about: where the tuple is, who can see it, and how it is laid out.
//
//	PHYSICAL
//	offset          8032
//	length          40
//
//	MVCC
//	t_xmin          742
//	t_xmax          0
//	t_cid           0
//	t_ctid          (0,4)
//
//	HEADER
//	t_infomask      0x0902
//	t_infomask2     0x0002
//	t_hoff          24
//	natts           2
func tuplePanel(it items) string {
	id := it.ids[it.selected]

	t, err := selectedTuple(it)
	if err != nil {
		wrap := lipgloss.NewStyle().Width(fieldColumn + tupleValueWidth)
		return wrap.Render(invalidStyle.Render("! " + err.Error()))
	}

	h := t.Header
	self := pgpage.ItemPointer{Block: it.block, Offset: number(it.selected)}

	// t_field3 is t_cid, except on tuples that a pre-9.0 VACUUM FULL moved,
	// where it holds the transaction that moved them.
	field3 := "t_cid"
	if h.Infomask.Has(pgpage.HeapMovedOff) || h.Infomask.Has(pgpage.HeapMovedIn) {
		field3 = "t_xvac"
	}

	mvcc := []headerRow{
		{"t_xmin", fmt.Sprint(uint32(h.Xmin)), xminStyle(h.Infomask)},
		{"t_xmax", fmt.Sprint(uint32(h.Xmax)), valueStyle},
		{field3, fmt.Sprint(h.Field3), valueStyle},
		{"t_ctid", h.Ctid.String(), valueStyle},
	}

	// A ctid that is not the tuple's own position points to the newer
	// version an UPDATE wrote.
	if h.Ctid != self {
		mvcc[3].style = redirectStyle
		mvcc = append(mvcc, headerRow{"", "↳ newer version", moreStyle})
	}

	layout := []headerRow{
		{"t_infomask", fmt.Sprintf("%#04x", uint16(h.Infomask)), valueStyle},
		{"t_infomask2", fmt.Sprintf("%#04x", uint16(h.Infomask2)), valueStyle},
		{"t_hoff", fmt.Sprint(h.Hoff), valueStyle},
		{"natts", fmt.Sprint(h.Natts()), valueStyle},
	}

	if t.Bits != nil {
		layout = append(layout, headerRow{"t_bits", nullBits(t), valueStyle})
	}

	sections := []string{
		section("PHYSICAL", []headerRow{
			{"offset", fmt.Sprint(id.Offset()), valueStyle},
			{"length", fmt.Sprint(id.Length()), valueStyle},
		}),
		section("MVCC", mvcc),
		section("HEADER", layout),
	}

	return strings.Join(sections, "\n\n")
}

// section renders a titled group of fields.
func section(title string, rows []headerRow) string {
	return moreStyle.Render(title) + "\n" + strings.Join(renderRows(rows), "\n")
}

// xminStyle colors t_xmin by what the hint bits say about its transaction:
// committed (or frozen), aborted, or not known yet.
func xminStyle(mask pgpage.InfoMask) lipgloss.Style {
	switch {
	case mask.Has(pgpage.HeapXminCommitted):
		return okStyle // committed, or frozen when invalid is set too
	case mask.Has(pgpage.HeapXminInvalid):
		return deadStyle
	default:
		return valueStyle
	}
}

// nullBits returns the null bitmap one attribute at a time, first attribute
// first: 1 for a stored value, 0 for a null, as the bits themselves are.
func nullBits(t pgpage.HeapTuple) string {
	var b strings.Builder

	for attnum := 1; attnum <= t.Header.Natts(); attnum++ {
		if t.IsNull(attnum) {
			b.WriteByte('0')
		} else {
			b.WriteByte('1')
		}
	}

	return b.String()
}

// flagsPanel renders what the infomask bits of the selected tuple mean, and
// the bytes of the tuple after its header, wrapped to width.
//
//	HEAP_HASVARWIDTH           0x0002
//	HEAP_XMIN_COMMITTED        0x0100
//	HEAP_XMAX_INVALID          0x0800
//
//	not set: HEAP_HASNULL · HEAP_HASEXTERNAL · …
//
//	DATA · 11 BYTES RAW
//	1fe8  02 00 00 00 0b 4c 75 69  73                       |.....Luis|
func flagsPanel(it items, width int) string {
	t, err := selectedTuple(it)
	if err != nil {
		return moreStyle.Render("no tuple to decode")
	}

	h := t.Header
	wrap := lipgloss.NewStyle().Width(width)

	var set, unset []string

	for bit := range 16 {
		flag := pgpage.InfoMask(1) << bit
		name := infoMaskName(flag)

		if h.Infomask.Has(flag) {
			set = append(set, flagRow(name, uint16(flag)))
			continue
		}

		unset = append(unset, name)
	}

	for _, flag := range []pgpage.InfoMask2{pgpage.HeapKeysUpdated, pgpage.HeapHotUpdated, pgpage.HeapOnlyTuple} {
		name := infoMask2Name(flag)

		if h.Infomask2.Has(flag) {
			set = append(set, flagRow(name, uint16(flag)))
			continue
		}

		unset = append(unset, name)
	}

	// Two bits set together mean something neither means alone.
	if h.Infomask.Has(pgpage.HeapXminFrozen) {
		set = append(set, moreStyle.Render("COMMITTED + INVALID = xmin frozen"))
	}

	parts := []string{
		strings.Join(set, "\n"),
		wrap.Render(moreStyle.Render("not set: " + strings.Join(unset, " · "))),
		dataSection(it, t, width),
	}

	return strings.Join(parts, "\n\n")
}

// flagRow renders a set flag and its bit.
func flagRow(name string, bit uint16) string {
	return okStyle.Render(padRight(name, flagColumn)) + moreStyle.Render(fmt.Sprintf("%#04x", bit))
}

// flagColumn is the width of the flag names, the longest one plus a space.
var flagColumn = func() int {
	width := 0

	for bit := range 16 {
		width = max(width, lipgloss.Width(infoMaskName(pgpage.InfoMask(1)<<bit)))
	}

	return width + 2
}()

// infoMaskName returns the PostgreSQL name of one t_infomask bit.
func infoMaskName(flag pgpage.InfoMask) string {
	return flag.Names()[0]
}

// infoMask2Name returns the PostgreSQL name of one t_infomask2 flag.
func infoMask2Name(flag pgpage.InfoMask2) string {
	return flag.Names()[0]
}

// dataSection renders the attribute bytes of the tuple, which stay raw: the
// page does not say what the columns are, only the table's catalog does.
func dataSection(it items, t pgpage.HeapTuple, width int) string {
	id := it.ids[it.selected]
	start := int(id.Offset()) + int(t.Header.Hoff) // page offset of the data

	lines := []string{moreStyle.Render(fmt.Sprintf("DATA · %d BYTES RAW", len(t.Data)))}

	for _, line := range pgpage.HexLines(t.Data, start) {
		lines = append(lines, valueStyle.Render(line.String()))
	}

	note := noteStyle.Render("no schema attached") + moreStyle.Render(fmt.Sprintf(
		" — the page stores column values without their types, so they stay raw. "+
			"t_hoff %d marks where the data begins, at byte %d of the page.", t.Header.Hoff, start))

	return strings.Join(lines, "\n") + "\n\n" + lipgloss.NewStyle().Width(width).Render(note)
}
