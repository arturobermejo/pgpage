package pgpage

import (
	"encoding/binary"
	"fmt"
	"testing"
)

// makeItemID packs the fields of a line pointer, the inverse of the
// ItemID accessors.
func makeItemID(offset uint16, flags uint8, length uint16) ItemID {
	return ItemID(uint32(length)<<(itemOffsetBits+itemFlagsBits) |
		uint32(flags)<<itemOffsetBits |
		uint32(offset))
}

func TestItemID(t *testing.T) {
	tests := []struct {
		name   string
		raw    ItemID
		offset uint16
		flags  uint8
		length uint16
	}{
		// Bytes d8 9f 46 00 at offset 24 of testdata/heap_small.
		{name: "fixture item 1", raw: 0x00469FD8, offset: 8152, flags: 1, length: 35},
		{name: "zero", raw: 0, offset: 0, flags: 0, length: 0},
		{name: "offset bits only", raw: 0x00007FFF, offset: 0x7FFF, flags: 0, length: 0},
		{name: "flags bits only", raw: 0x00018000, offset: 0, flags: 3, length: 0},
		{name: "length bits only", raw: 0xFFFE0000, offset: 0, flags: 0, length: 0x7FFF},
		{name: "all bits", raw: 0xFFFFFFFF, offset: 0x7FFF, flags: 3, length: 0x7FFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := tt.raw

			if got := id.Offset(); got != tt.offset {
				t.Errorf("ItemID(%#08x).Offset() = %d, want %d", uint32(id), got, tt.offset)
			}

			if got := id.Flags(); got != tt.flags {
				t.Errorf("ItemID(%#08x).Flags() = %d, want %d", uint32(id), got, tt.flags)
			}

			if got := id.Length(); got != tt.length {
				t.Errorf("ItemID(%#08x).Length() = %d, want %d", uint32(id), got, tt.length)
			}

			if got := makeItemID(tt.offset, tt.flags, tt.length); got != id {
				t.Errorf("makeItemID(%d, %d, %d) = %#08x, want %#08x", tt.offset, tt.flags, tt.length, uint32(got), uint32(id))
			}
		})
	}
}

// Every line pointer in the fixture must decode to what heap_page_items()
// reports.
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

		got := fmt.Sprintf("off=%d flags=%d len=%d", id.Offset(), id.Flags(), id.Length())
		want := fmt.Sprintf("off=%s flags=%s len=%s", row["lp_off"], row["lp_flags"], row["lp_len"])

		if got != want {
			t.Errorf("block %d item %d: %s, want %s", block, lp, got, want)
		}
	}
}
