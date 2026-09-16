package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// Offsets of the fields of HeapTupleHeaderData inside a tuple.
const (
	ctidPosid      = 16 // ip_posid, the low byte
	infomaskHigh   = 21 // t_infomask, high byte
	selectedTuple2 = 1  // index of #2 on block 2: NORMAL at 8152, 37 bytes
)

// withTupleBytes returns block 2 of the fixture with #2 selected, after
// change edits a copy of its page. The page is shared with nothing else, so
// a test can break the tuple without breaking the fixture.
func withTupleBytes(t *testing.T, change func(tuple []byte)) items {
	t.Helper()

	it := fixtureItems(t, 2)
	it.page = bytes.Clone(it.page)
	it.selected = selectedTuple2

	id := it.ids[it.selected]
	change(it.page[id.Offset() : id.Offset()+id.Length()])

	return it
}

// flat collapses the runs of spaces of a rendered panel, so that tests can
// look for "name value" without depending on the column widths.
func flat(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func TestHasTuple(t *testing.T) {
	item := func(off, state, length uint32) pgpage.ItemID {
		return pgpage.ItemID(off | state<<15 | length<<17)
	}

	tests := []struct {
		name string
		id   pgpage.ItemID
		want bool
	}{
		{name: "normal", id: item(8152, 1, 37), want: true},
		{name: "dead that keeps its storage", id: item(8152, 3, 37), want: true},
		{name: "dead without storage", id: item(0, 3, 0)},
		{name: "redirect", id: item(168, 2, 0)},
		{name: "unused", id: item(0, 0, 0)},
	}

	for _, tt := range tests {
		if got := hasTuple(tt.id); got != tt.want {
			t.Errorf("%s: hasTuple = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// The tuple panel shows the header fields of the tuple, grouped, with the
// values heap_page_items() reports for #2 of block 2.
func TestTuplePanel(t *testing.T) {
	it := fixtureItems(t, 2)
	it.selected = selectedTuple2

	panel := flat(tuplePanel(it))

	for _, want := range []string{
		"PHYSICAL offset 8152 length 37",
		"MVCC t_xmin 775 t_xmax 0 t_cid 0 t_ctid (2,2)",
		"HEADER t_infomask 0x0902 t_infomask2 0x0002 t_hoff 24 natts 2",
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("panel does not contain %q:\n%s", want, panel)
		}
	}

	if strings.Contains(panel, "newer version") || strings.Contains(panel, "t_bits") {
		t.Errorf("panel describes an update or nulls the tuple does not have:\n%s", panel)
	}

	if tupleTitle(it) != "TUPLE #2" {
		t.Errorf("title = %q, want TUPLE #2", tupleTitle(it))
	}
}

// A ctid that is not the tuple's own position points to the newer version
// an UPDATE wrote, and the panel says so.
func TestTuplePanelNewerVersion(t *testing.T) {
	it := withTupleBytes(t, func(tuple []byte) { tuple[ctidPosid] = 99 })

	panel := flat(tuplePanel(it))

	if !strings.Contains(panel, "t_ctid (2,99) ↳ newer version") {
		t.Errorf("panel does not point to the newer version:\n%s", panel)
	}
}

// The null bitmap is shown one attribute at a time, 1 for a value and 0 for
// a null, first attribute first.
func TestNullBits(t *testing.T) {
	tuple := pgpage.HeapTuple{
		Header: pgpage.HeapTupleHeader{Infomask: pgpage.HeapHasNull, Infomask2: 10},
		Bits:   []byte{0b1111_0101, 0b0000_0010}, // attributes 2, 4 and 9 are null
	}

	if got := nullBits(tuple); got != "1010111101" {
		t.Errorf("nullBits = %q, want %q", got, "1010111101")
	}
}

// t_xmin takes the color of what its hint bits say about the transaction.
func TestXminStyle(t *testing.T) {
	withColor(t)

	tests := []struct {
		name string
		mask pgpage.InfoMask
		want lipgloss.Style
	}{
		{name: "committed", mask: pgpage.HeapXminCommitted, want: okStyle},
		{name: "frozen", mask: pgpage.HeapXminFrozen, want: okStyle},
		{name: "aborted", mask: pgpage.HeapXminInvalid, want: deadStyle},
		{name: "not known yet", mask: 0, want: valueStyle},
	}

	for _, tt := range tests {
		if got, want := xminStyle(tt.mask).Render("x"), tt.want.Render("x"); got != want {
			t.Errorf("%s: %q, want %q", tt.name, got, want)
		}
	}
}

// The flags panel lists the bits that are set with their values, names the
// ones that are not, and shows the tuple's data at its page offset.
func TestFlagsPanel(t *testing.T) {
	it := fixtureItems(t, 2)
	it.selected = selectedTuple2

	panel := flat(flagsPanel(it, 80))

	for _, want := range []string{
		"HEAP_HASVARWIDTH 0x0002 HEAP_XMIN_COMMITTED 0x0100 HEAP_XMAX_INVALID 0x0800",
		"not set: HEAP_HASNULL · HEAP_HASEXTERNAL",
		"HEAP_ONLY_TUPLE",
		"DATA · 13 BYTES RAW",
		"1ff0 74 01 00 00 13 75 73 65 72 20 33 37 32", // 8152 + t_hoff 24 = 0x1ff0
		"no schema attached",
		"at byte 8176 of the page",
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("panel does not contain %q:\n%s", want, panel)
		}
	}

	if strings.Contains(panel, "xmin frozen") {
		t.Errorf("panel calls a merely committed xmin frozen:\n%s", panel)
	}
}

// Committed and invalid set together mean frozen, which neither bit says
// alone.
func TestFlagsPanelFrozen(t *testing.T) {
	it := withTupleBytes(t, func(tuple []byte) { tuple[infomaskHigh] |= 0x02 }) // HEAP_XMIN_INVALID

	if panel := flat(flagsPanel(it, 80)); !strings.Contains(panel, "COMMITTED + INVALID = xmin frozen") {
		t.Errorf("panel does not explain the frozen xmin:\n%s", panel)
	}
}

// Every line of the flags panel fits the width it was given, the long list
// of flags that are not set included.
func TestFlagsPanelWraps(t *testing.T) {
	it := fixtureItems(t, 2)
	it.selected = selectedTuple2

	for _, width := range []int{minFlagsWidth, 60, 80} {
		for _, line := range strings.Split(flagsPanel(it, width), "\n") {
			// The hex dump has a fixed width and the panel frame cuts it; the
			// prose around it has to wrap.
			if strings.Contains(line, "  |") {
				continue
			}

			if got := lipgloss.Width(line); got > width {
				t.Errorf("width %d: a line is %d cells wide:\n%q", width, got, line)
			}
		}
	}
}
