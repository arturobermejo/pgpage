package pgpage

import (
	"fmt"
	"testing"
)

func TestItemPointerString(t *testing.T) {
	tests := []struct {
		p    ItemPointer
		want string
	}{
		{p: ItemPointer{}, want: "(0,0)"},
		{p: ItemPointer{Block: 0, Offset: 4}, want: "(0,4)"},
		{p: ItemPointer{Block: 2, Offset: 168}, want: "(2,168)"},
		{p: ItemPointer{Block: MaxBlockNumber, Offset: 0xFFFF}, want: "(4294967294,65535)"},
	}

	for _, tt := range tests {
		if got := fmt.Sprint(tt.p); got != tt.want {
			t.Errorf("fmt.Sprint(%#v) = %q, want %q", tt.p, got, tt.want)
		}
	}
}

func TestDecodeItemPointer(t *testing.T) {
	tests := []struct {
		name  string
		bytes []byte
		want  ItemPointer
	}{
		// Bytes 12-17 of the tuple at line pointer 1 of block 0 in the fixture.
		{name: "fixture (0,1)", bytes: []byte{0x00, 0x00, 0x00, 0x00, 0x01, 0x00}, want: ItemPointer{Block: 0, Offset: 1}},
		// Bytes 12-17 of the tuple at line pointer 168 of block 2 in the fixture.
		{name: "fixture (2,168)", bytes: []byte{0x00, 0x00, 0x02, 0x00, 0xa8, 0x00}, want: ItemPointer{Block: 2, Offset: 168}},
		// 70000 = 0x00011170: bi_hi = 0x0001, bi_lo = 0x1170.
		{name: "block above 16 bits", bytes: []byte{0x01, 0x00, 0x70, 0x11, 0x04, 0x00}, want: ItemPointer{Block: 70000, Offset: 4}},
		{name: "all bits", bytes: []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, want: ItemPointer{Block: 0xFFFFFFFF, Offset: 0xFFFF}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decodeItemPointer(tt.bytes); got != tt.want {
				t.Errorf("decodeItemPointer(% x) = %v, want %v", tt.bytes, got, tt.want)
			}
		})
	}
}

// The ctid of every tuple in the fixture must match heap_page_items().
func TestFixtureItemPointers(t *testing.T) {
	// t_ctid starts at byte 12 of a heap tuple header, after t_xmin, t_xmax
	// and t_cid.
	const ctidOffset = 12

	rel := openRelation(t, fixtureHeap)

	ctids := make(map[string]string)
	for _, row := range readFixtureCSV(t, fixtureHeap+".items.csv") {
		ctids[row["blkno"]+"/"+row["lp"]] = row["t_ctid"]
	}

	checked := 0

	for block := range rel.PageCount() {
		page, err := rel.ReadPage(block)
		if err != nil {
			t.Fatalf("ReadPage(%d) returned error: %v", block, err)
		}

		h, err := ParsePageHeader(page)
		if err != nil {
			t.Fatalf("block %d: ParsePageHeader returned error: %v", block, err)
		}

		items, err := ParseItemIDs(page, h, nil)
		if err != nil {
			t.Fatalf("block %d: ParseItemIDs returned error: %v", block, err)
		}

		for i, id := range items {
			if id.State() != ItemNormal {
				continue
			}

			n := OffsetNumber(i + 1)
			start := int(id.Offset()) + ctidOffset
			got := decodeItemPointer(page[start : start+itemPointerSize]).String()

			if want := ctids[fmt.Sprintf("%d/%d", block, n)]; got != want {
				t.Errorf("block %d item %d: ctid %s, heap_page_items() shows %s", block, n, got, want)
			}

			checked++
		}
	}

	if checked == 0 {
		t.Fatal("no tuples checked")
	}
}
