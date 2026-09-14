package pgpage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"
)

// makeItemID packs the fields of a line pointer, the inverse of the
// ItemID accessors.
func makeItemID(offset uint16, state ItemState, length uint16) ItemID {
	return ItemID(uint32(length)<<(itemOffsetBits+itemFlagsBits) |
		uint32(state)<<itemOffsetBits |
		uint32(offset))
}

func TestItemID(t *testing.T) {
	tests := []struct {
		name   string
		raw    ItemID
		offset uint16
		state  ItemState
		length uint16
	}{
		// Bytes d8 9f 46 00 at offset 24 of testdata/heap_small.
		{name: "fixture item 1", raw: 0x00469FD8, offset: 8152, state: ItemNormal, length: 35},
		{name: "zero", raw: 0, offset: 0, state: ItemUnused, length: 0},
		{name: "offset bits only", raw: 0x00007FFF, offset: 0x7FFF, state: ItemUnused, length: 0},
		{name: "flags bits only", raw: 0x00018000, offset: 0, state: ItemDead, length: 0},
		{name: "length bits only", raw: 0xFFFE0000, offset: 0, state: ItemUnused, length: 0x7FFF},
		{name: "all bits", raw: 0xFFFFFFFF, offset: 0x7FFF, state: ItemDead, length: 0x7FFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := tt.raw

			if got := id.Offset(); got != tt.offset {
				t.Errorf("ItemID(%#08x).Offset() = %d, want %d", uint32(id), got, tt.offset)
			}

			if got := id.State(); got != tt.state {
				t.Errorf("ItemID(%#08x).State() = %v, want %v", uint32(id), got, tt.state)
			}

			if got := id.Length(); got != tt.length {
				t.Errorf("ItemID(%#08x).Length() = %d, want %d", uint32(id), got, tt.length)
			}

			if got := makeItemID(tt.offset, tt.state, tt.length); got != id {
				t.Errorf("makeItemID(%d, %v, %d) = %#08x, want %#08x", tt.offset, tt.state, tt.length, uint32(got), uint32(id))
			}
		})
	}
}

func TestItemStateString(t *testing.T) {
	tests := []struct {
		state ItemState
		want  string
	}{
		{state: ItemUnused, want: "UNUSED"},
		{state: ItemNormal, want: "NORMAL"},
		{state: ItemRedirect, want: "REDIRECT"},
		{state: ItemDead, want: "DEAD"},
		{state: ItemDead + 1, want: "ItemState(4)"},
	}

	for _, tt := range tests {
		if got := fmt.Sprint(tt.state); got != tt.want {
			t.Errorf("fmt.Sprint(ItemState(%d)) = %q, want %q", uint8(tt.state), got, tt.want)
		}
	}
}

// The constants are part of the on-disk format and must match itemid.h.
func TestItemStateValues(t *testing.T) {
	for state, want := range map[ItemState]uint8{ItemUnused: 0, ItemNormal: 1, ItemRedirect: 2, ItemDead: 3} {
		if uint8(state) != want {
			t.Errorf("%v = %d, want %d", state, uint8(state), want)
		}
	}
}

func TestItemIDHasStorage(t *testing.T) {
	tests := []struct {
		name string
		id   ItemID
		want bool
	}{
		{name: "unused", id: makeItemID(0, ItemUnused, 0), want: false},
		{name: "normal", id: makeItemID(8152, ItemNormal, 35), want: true},
		{name: "redirect", id: makeItemID(168, ItemRedirect, 0), want: false},
		{name: "dead without storage", id: makeItemID(0, ItemDead, 0), want: false},
		{name: "dead with storage", id: makeItemID(8112, ItemDead, 35), want: true},
	}

	for _, tt := range tests {
		if got := tt.id.HasStorage(); got != tt.want {
			t.Errorf("%s: HasStorage() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// Every line pointer in the fixture must decode to what heap_page_items()
// reports and be consistent with its page.
func TestFixtureItemIDs(t *testing.T) {
	rel := openRelation(t, fixtureHeap)

	rowsByBlock := make(map[BlockNumber][]map[string]string)

	for _, row := range readFixtureCSV(t, fixtureHeap+".items.csv") {
		block := BlockNumber(parseInt(t, row["blkno"], 32))
		rowsByBlock[block] = append(rowsByBlock[block], row)
	}

	var items []ItemID

	for block := range rel.PageCount() {
		page, err := rel.ReadPage(block)
		if err != nil {
			t.Fatalf("ReadPage(%d) returned error: %v", block, err)
		}

		h, err := ParsePageHeader(page)
		if err != nil {
			t.Fatalf("block %d: ParsePageHeader returned error: %v", block, err)
		}

		// Reusing items across pages is how callers avoid an allocation per page.
		if items, err = ParseItemIDs(page, h, items); err != nil {
			t.Fatalf("block %d: ParseItemIDs returned error: %v", block, err)
		}

		rows := rowsByBlock[block]
		if len(items) != len(rows) {
			t.Fatalf("block %d: ParseItemIDs returned %d line pointers, heap_page_items() has %d", block, len(items), len(rows))
		}

		for i, id := range items {
			n := OffsetNumber(i + 1)
			row := rows[i]

			if lp := OffsetNumber(parseInt(t, row["lp"], 16)); lp != n {
				t.Fatalf("block %d: row %d is line pointer %d, want %d", block, i, lp, n)
			}

			got := fmt.Sprintf("off=%d flags=%d len=%d", id.Offset(), uint8(id.State()), id.Length())
			want := fmt.Sprintf("off=%s flags=%s len=%s", row["lp_off"], row["lp_flags"], row["lp_len"])

			if got != want {
				t.Errorf("block %d item %d: %s, want %s", block, n, got, want)
			}

			if err := h.CheckItemID(n, id); err != nil {
				t.Errorf("block %d: CheckItemID returned error: %v", block, err)
			}
		}
	}
}

func TestItemIDAt(t *testing.T) {
	page := makeItemPage(t, testItemHeader, makeItemID(8152, ItemNormal, 35), makeItemID(0, ItemUnused, 0))

	tests := []struct {
		n    OffsetNumber
		want ItemID
	}{
		{n: 1, want: makeItemID(8152, ItemNormal, 35)},
		{n: 2, want: makeItemID(0, ItemUnused, 0)},
	}

	for _, tt := range tests {
		got, err := ItemIDAt(page, testItemHeader, tt.n)
		if err != nil {
			t.Fatalf("ItemIDAt(%d) returned error: %v", tt.n, err)
		}

		if got != tt.want {
			t.Errorf("ItemIDAt(%d) = %#08x, want %#08x", tt.n, uint32(got), uint32(tt.want))
		}
	}
}

func TestItemIDAtOutOfRange(t *testing.T) {
	page := makeItemPage(t, testItemHeader)

	// testItemHeader has 12 line pointers.
	for _, n := range []OffsetNumber{0, 13, 0xFFFF} {
		if _, err := ItemIDAt(page, testItemHeader, n); err == nil {
			t.Errorf("ItemIDAt(%d): expected error, got nil", n)
		}
	}

	if _, err := ItemIDAt(make([]byte, PageSize), PageHeader{}, FirstOffsetNumber); err == nil {
		t.Error("ItemIDAt on a new page: expected error, got nil")
	}
}

func TestParseItemIDs(t *testing.T) {
	want := []ItemID{
		makeItemID(8152, ItemNormal, 35),
		makeItemID(0, ItemDead, 0),
		makeItemID(1, ItemRedirect, 0),
	}

	h := testItemHeader
	h.Lower = PageHeaderSize + uint16(len(want))*itemIDSize
	page := makeItemPage(t, h, want...)

	got, err := ParseItemIDs(page, h, nil)
	if err != nil {
		t.Fatalf("ParseItemIDs returned error: %v", err)
	}

	if !slices.Equal(got, want) {
		t.Errorf("ParseItemIDs = %#08x, want %#08x", got, want)
	}
}

func TestParseItemIDsNewPage(t *testing.T) {
	got, err := ParseItemIDs(make([]byte, PageSize), PageHeader{}, nil)
	if err != nil {
		t.Fatalf("ParseItemIDs returned error: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("ParseItemIDs = %v, want no line pointers", got)
	}
}

// A header that does not match the page must be an error, not a panic.
func TestParseItemIDsBadInput(t *testing.T) {
	tests := []struct {
		name string
		page []byte
		h    PageHeader
	}{
		{name: "short page", page: make([]byte, PageSize-1), h: testItemHeader},
		{name: "lower past page", page: make([]byte, PageSize), h: PageHeader{Lower: 0xFFFF, Upper: 0xFFFF}},
		{name: "lower inside header", page: make([]byte, PageSize), h: PageHeader{Lower: 4, Upper: 8000}},
		// (22-24)/4 truncates to 0, so a negative item count alone misses it.
		{name: "lower just inside header", page: make([]byte, PageSize), h: PageHeader{Lower: 22, Upper: 8000}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseItemIDs(tt.page, tt.h, []ItemID{1, 2, 3})
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if len(got) != 0 {
				t.Errorf("ParseItemIDs = %v with error, want no line pointers", got)
			}
		})
	}
}

// Reusing the previous result must not allocate.
func TestParseItemIDsDoesNotAllocate(t *testing.T) {
	page := makeItemPage(t, testItemHeader)
	items := make([]ItemID, 0, testItemHeader.ItemCount())

	allocs := testing.AllocsPerRun(100, func() {
		var err error
		if items, err = ParseItemIDs(page, testItemHeader, items); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Errorf("ParseItemIDs allocated %v times per call, want 0", allocs)
	}
}

func TestCheckItemID(t *testing.T) {
	// 12 line pointers, tuple space 8000-8191.
	h := testItemHeader

	tests := []struct {
		name  string
		n     OffsetNumber
		id    ItemID
		valid bool
	}{
		{name: "unused", n: 1, id: makeItemID(0, ItemUnused, 0), valid: true},
		{name: "unused with garbage", n: 1, id: makeItemID(3, ItemUnused, 99), valid: true},
		{name: "normal", n: 1, id: makeItemID(8000, ItemNormal, 40), valid: true},
		{name: "normal ending at special", n: 1, id: makeItemID(8152, ItemNormal, 40), valid: true},
		{name: "dead without storage", n: 1, id: makeItemID(0, ItemDead, 0), valid: true},
		{name: "dead with storage", n: 1, id: makeItemID(8000, ItemDead, 40), valid: true},
		{name: "redirect", n: 1, id: makeItemID(12, ItemRedirect, 0), valid: true},

		{name: "normal without storage", n: 1, id: makeItemID(8000, ItemNormal, 0)},
		{name: "normal before upper", n: 1, id: makeItemID(7992, ItemNormal, 40)},
		{name: "normal past special", n: 1, id: makeItemID(8160, ItemNormal, 40)},
		{name: "normal not aligned", n: 1, id: makeItemID(8004, ItemNormal, 40)},
		{name: "dead past special", n: 1, id: makeItemID(8160, ItemDead, 40)},
		{name: "redirect with storage", n: 1, id: makeItemID(2, ItemRedirect, 40)},
		{name: "redirect to zero", n: 1, id: makeItemID(0, ItemRedirect, 0)},
		{name: "redirect past last item", n: 1, id: makeItemID(13, ItemRedirect, 0)},
		{name: "redirect to itself", n: 5, id: makeItemID(5, ItemRedirect, 0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.CheckItemID(tt.n, tt.id)

			if tt.valid && err != nil {
				t.Errorf("CheckItemID returned error: %v", err)
			}

			if !tt.valid && !errors.Is(err, ErrInvalidItemID) {
				t.Errorf("errors.Is(err, ErrInvalidItemID) = false, want true (err = %v)", err)
			}
		})
	}
}

// Asking about a line pointer the page does not have is not corruption.
func TestCheckItemIDOutOfRange(t *testing.T) {
	for _, n := range []OffsetNumber{0, 13} {
		err := testItemHeader.CheckItemID(n, makeItemID(8000, ItemNormal, 40))
		if err == nil {
			t.Errorf("CheckItemID(%d): expected error, got nil", n)
		}

		if errors.Is(err, ErrInvalidItemID) {
			t.Errorf("CheckItemID(%d): must not wrap ErrInvalidItemID (err = %v)", n, err)
		}
	}
}

// testItemHeader describes a page with 12 line pointers and tuple space
// from byte 8000 to the end of the page.
var testItemHeader = PageHeader{
	Lower:         PageHeaderSize + 12*itemIDSize,
	Upper:         8000,
	Special:       PageSize,
	PageSize:      PageSize,
	LayoutVersion: PageLayoutVersion,
}

// makeItemPage serializes h and ids into a page, starting at line pointer 1.
func makeItemPage(t testing.TB, h PageHeader, ids ...ItemID) []byte {
	t.Helper()

	page := makePage(h)

	for i, id := range ids {
		start := PageHeaderSize + i*itemIDSize
		binary.LittleEndian.PutUint32(page[start:start+itemIDSize], uint32(id))
	}

	if _, err := ParsePageHeader(page); err != nil {
		t.Fatalf("makeItemPage built an invalid page: %v", err)
	}

	return page
}

func FuzzItemIDs(f *testing.F) {
	data, err := os.ReadFile(fixtureHeap)
	if err != nil {
		f.Fatal(err)
	}

	// Block 2 of the fixture has NORMAL, DEAD, REDIRECT and UNUSED items.
	f.Add(data[2*PageSize:3*PageSize], uint16(744), uint16(2472), uint16(PageSize))
	f.Add(makeItemPage(f, testItemHeader, makeItemID(8000, ItemNormal, 40)), testItemHeader.Lower, testItemHeader.Upper, testItemHeader.Special)
	f.Add([]byte{}, uint16(0), uint16(0), uint16(0))

	// data becomes the start of the page, including its header. The header
	// is used twice: as fuzzed field values that need not match the page,
	// and as decoded by ParsePageHeader when the bytes happen to be valid.
	f.Fuzz(func(t *testing.T, data []byte, lower, upper, special uint16) {
		page := make([]byte, PageSize)
		copy(page, data)

		checkItemInvariants(t, page, PageHeader{Lower: lower, Upper: upper, Special: special})

		h, err := ParsePageHeader(page)
		if err != nil {
			return
		}

		items, err := ParseItemIDs(page, h, nil)
		if err != nil {
			t.Fatalf("ParseItemIDs failed on a valid header %+v: %v", h, err)
		}

		if len(items) != h.ItemCount() {
			t.Fatalf("ParseItemIDs returned %d line pointers, header says %d", len(items), h.ItemCount())
		}

		checkItemInvariants(t, page, h)
	})
}

// checkItemInvariants checks what callers rely on for any header h, valid or
// not: reading line pointers never panics, and a line pointer that
// CheckItemID accepts can be used without further checks.
func checkItemInvariants(t *testing.T, page []byte, h PageHeader) {
	t.Helper()

	items, err := ParseItemIDs(page, h, nil)
	if err != nil {
		return
	}

	for i, id := range items {
		n := OffsetNumber(i + 1)

		if got, err := ItemIDAt(page, h, n); err != nil || got != id {
			t.Fatalf("ItemIDAt(%d) = %#08x, %v; ParseItemIDs has %#08x", n, uint32(got), err, uint32(id))
		}

		if err := h.CheckItemID(n, id); err != nil {
			// n exists, so any complaint must be about the line pointer.
			if !errors.Is(err, ErrInvalidItemID) {
				t.Fatalf("CheckItemID(%d) error does not wrap ErrInvalidItemID: %v", n, err)
			}

			continue
		}

		switch id.State() {
		case ItemNormal:
			if !id.HasStorage() {
				t.Fatalf("CheckItemID accepted NORMAL line pointer %d without storage", n)
			}
		case ItemRedirect:
			if target := OffsetNumber(id.Offset()); target < FirstOffsetNumber || int(target) > len(items) || target == n {
				t.Fatalf("CheckItemID accepted line pointer %d redirecting to %d of %d", n, target, len(items))
			}
		case ItemUnused, ItemDead:
		}

		if id.State() != ItemUnused && id.HasStorage() {
			start, end := int(id.Offset()), int(id.Offset())+int(id.Length())
			if start%maxAlign != 0 || end > len(page) {
				t.Fatalf("CheckItemID accepted line pointer %d at bytes %d-%d of a %d-byte page", n, start, end-1, len(page))
			}
		}
	}
}
