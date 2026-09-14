package pgpage

import (
	"encoding/binary"
	"fmt"
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
// reports, and follow the lp_len conventions documented in itemid.h.
func TestFixtureItemIDs(t *testing.T) {
	rel := openRelation(t, fixtureHeap)
	rows := readFixtureCSV(t, fixtureHeap+".items.csv")

	pages := make(map[BlockNumber][]byte)

	for _, row := range rows {
		block := BlockNumber(parseInt(t, row["blkno"], 32))
		lp := parseInt(t, row["lp"], 16)

		page, ok := pages[block]
		if !ok {
			var err error
			if page, err = rel.ReadPage(block); err != nil {
				t.Fatalf("ReadPage(%d) returned error: %v", block, err)
			}

			pages[block] = page
		}

		// Line pointers are numbered from 1 and start right after the header.
		start := PageHeaderSize + (lp-1)*itemIDSize
		id := ItemID(binary.LittleEndian.Uint32(page[start : start+itemIDSize]))

		got := fmt.Sprintf("off=%d flags=%d len=%d", id.Offset(), uint8(id.State()), id.Length())
		want := fmt.Sprintf("off=%s flags=%s len=%s", row["lp_off"], row["lp_flags"], row["lp_len"])

		if got != want {
			t.Errorf("block %d item %d: %s, want %s", block, lp, got, want)
		}

		switch state := id.State(); {
		case state == ItemNormal && !id.HasStorage():
			t.Errorf("block %d item %d: NORMAL without storage", block, lp)
		case (state == ItemUnused || state == ItemRedirect) && id.HasStorage():
			t.Errorf("block %d item %d: %v with storage", block, lp, state)
		}
	}
}
