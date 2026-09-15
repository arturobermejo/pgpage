package pgpage

import (
	"bytes"
	"encoding/binary"
	"errors"
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

// fixtureTuple is the header of the tuple at (2,168) in the fixture: a HOT
// update of row 380, followed by 16 bytes of row data.
var fixtureTuple = HeapTupleHeader{
	Xmin:      776,
	Ctid:      ItemPointer{Block: 2, Offset: 168},
	Infomask2: 0x8002,
	Infomask:  0x2902,
	Hoff:      24,
}

// makeTuple serializes h into the header of an otherwise zero tuple of size
// bytes, the inverse of ParseHeapTupleHeader.
func makeTuple(h HeapTupleHeader, size int) []byte {
	tuple := make([]byte, size)
	le := binary.LittleEndian

	le.PutUint32(tuple[0:4], uint32(h.Xmin))
	le.PutUint32(tuple[4:8], uint32(h.Xmax))
	le.PutUint32(tuple[8:12], h.Field3)
	le.PutUint16(tuple[12:14], uint16(h.Ctid.Block>>16))
	le.PutUint16(tuple[14:16], uint16(h.Ctid.Block))
	le.PutUint16(tuple[16:18], uint16(h.Ctid.Offset))
	le.PutUint16(tuple[18:20], uint16(h.Infomask2))
	le.PutUint16(tuple[20:22], uint16(h.Infomask))
	tuple[22] = h.Hoff

	return tuple
}

// The raw bytes of (2,168) must decode to what heap_page_items() reports.
func TestParseHeapTupleHeaderRawBytes(t *testing.T) {
	tuple := []byte{
		0x08, 0x03, 0x00, 0x00, // t_xmin
		0x00, 0x00, 0x00, 0x00, // t_xmax
		0x00, 0x00, 0x00, 0x00, // t_cid
		0x00, 0x00, 0x02, 0x00, 0xa8, 0x00, // t_ctid
		0x02, 0x80, // t_infomask2
		0x02, 0x29, // t_infomask
		0x18,                   // t_hoff
		0x00,                   // padding
		0x7c, 0x01, 0x00, 0x00, // id = 380
		0x19, 'u', 's', 'e', 'r', ' ', '3', '8', '0', ' ', 'v', '2', // name
	}

	got, err := ParseHeapTupleHeader(tuple)
	if err != nil {
		t.Fatalf("ParseHeapTupleHeader returned error: %v", err)
	}

	if got != fixtureTuple {
		t.Errorf("ParseHeapTupleHeader =\n  %+v\nwant\n  %+v", got, fixtureTuple)
	}

	if n := got.Natts(); n != 2 {
		t.Errorf("Natts() = %d, want 2", n)
	}
}

func TestParseHeapTupleHeaderRoundTrip(t *testing.T) {
	want := HeapTupleHeader{
		Xmin:      0xFFFFFFF0,
		Xmax:      0x12345678,
		Field3:    7,
		Ctid:      ItemPointer{Block: 70000, Offset: 4},
		Infomask2: 0xC7FF,
		Infomask:  0xFFFF &^ HeapHasNull, // 2047 attributes would need a longer null bitmap
		Hoff:      32,
	}

	got, err := ParseHeapTupleHeader(makeTuple(want, 64))
	if err != nil {
		t.Fatalf("ParseHeapTupleHeader returned error: %v", err)
	}

	if got != want {
		t.Errorf("ParseHeapTupleHeader =\n  %+v\nwant\n  %+v", got, want)
	}
}

func TestHeapTupleHeaderNatts(t *testing.T) {
	tests := []struct {
		infomask2 InfoMask2
		want      int
	}{
		{infomask2: 0x0000, want: 0},
		{infomask2: 0x0002, want: 2},
		{infomask2: 0x8002, want: 2},    // HOT flag above the count
		{infomask2: 0x07FF, want: 2047}, // largest count the 11-bit mask can hold
		{infomask2: 0xF800, want: 0},    // flags only
	}

	for _, tt := range tests {
		if got := (HeapTupleHeader{Infomask2: tt.infomask2}).Natts(); got != tt.want {
			t.Errorf("Natts() with t_infomask2 %#04x = %d, want %d", uint16(tt.infomask2), got, tt.want)
		}
	}
}

func TestParseHeapTupleHeaderInvalid(t *testing.T) {
	tests := []struct {
		name  string
		tuple []byte
	}{
		{name: "empty", tuple: nil},
		{name: "shorter than header", tuple: makeTuple(fixtureTuple, 40)[:HeapTupleHeaderSize-1]},
		{name: "hoff inside header", tuple: makeTuple(HeapTupleHeader{Hoff: 16}, 40)},
		{name: "hoff at header end, not aligned", tuple: makeTuple(HeapTupleHeader{Hoff: 23}, 40)},
		{name: "hoff not aligned", tuple: makeTuple(HeapTupleHeader{Hoff: 28}, 40)},
		{name: "hoff past tuple", tuple: makeTuple(HeapTupleHeader{Hoff: 48}, 40)},
		{name: "zero hoff", tuple: makeTuple(HeapTupleHeader{}, 40)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := ParseHeapTupleHeader(tt.tuple)
			if !errors.Is(err, ErrInvalidTupleHeader) {
				t.Fatalf("errors.Is(err, ErrInvalidTupleHeader) = false, want true (err = %v)", err)
			}

			if h != (HeapTupleHeader{}) {
				t.Errorf("header = %+v with error, want zero value", h)
			}
		})
	}
}

// Boundary values that PostgreSQL can write must parse.
func TestParseHeapTupleHeaderValidBoundaries(t *testing.T) {
	tests := []struct {
		name string
		hoff uint8
		size int
	}{
		{name: "no user data", hoff: 24, size: 24},
		{name: "null bitmap up to 8 bytes", hoff: 32, size: 40},
		{name: "largest aligned hoff", hoff: 248, size: 248},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := HeapTupleHeader{Xmin: 1, Hoff: tt.hoff}

			got, err := ParseHeapTupleHeader(makeTuple(want, tt.size))
			if err != nil {
				t.Fatalf("ParseHeapTupleHeader returned error: %v", err)
			}

			if got != want {
				t.Errorf("ParseHeapTupleHeader = %+v, want %+v", got, want)
			}
		})
	}
}

// ParseHeapTupleHeader must not allocate on success.
func TestParseHeapTupleHeaderDoesNotAllocate(t *testing.T) {
	tuple := makeTuple(fixtureTuple, 40)

	allocs := testing.AllocsPerRun(100, func() {
		if _, err := ParseHeapTupleHeader(tuple); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Errorf("ParseHeapTupleHeader allocated %v times per call, want 0", allocs)
	}
}

// Every tuple header in the fixture must decode to what heap_page_items()
// reports.
func TestFixtureTupleHeaders(t *testing.T) {
	rel := openRelation(t, fixtureHeap)

	rows := make(map[string]map[string]string)
	for _, row := range readFixtureCSV(t, fixtureHeap+".items.csv") {
		rows[row["blkno"]+"/"+row["lp"]] = row
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
			row := rows[fmt.Sprintf("%d/%d", block, n)]
			tuple := page[id.Offset() : id.Offset()+id.Length()]

			got, err := ParseHeapTupleHeader(tuple)
			if err != nil {
				t.Errorf("block %d item %d: ParseHeapTupleHeader returned error: %v", block, n, err)
				continue
			}

			want := HeapTupleHeader{
				Xmin:      TransactionID(parseInt(t, row["t_xmin"], 64)),
				Xmax:      TransactionID(parseInt(t, row["t_xmax"], 64)),
				Field3:    uint32(parseInt(t, row["t_field3"], 64)),
				Ctid:      got.Ctid, // checked as text below
				Infomask2: InfoMask2(parseInt(t, row["t_infomask2"], 32)),
				Infomask:  InfoMask(parseInt(t, row["t_infomask"], 32)),
				Hoff:      uint8(parseInt(t, row["t_hoff"], 16)),
			}

			if got != want {
				t.Errorf("block %d item %d: ParseHeapTupleHeader =\n  %+v\nwant\n  %+v", block, n, got, want)
			}

			if s := got.Ctid.String(); s != row["t_ctid"] {
				t.Errorf("block %d item %d: ctid %s, heap_page_items() shows %s", block, n, s, row["t_ctid"])
			}

			checked++
		}
	}

	if checked == 0 {
		t.Fatal("no tuples checked")
	}

	t.Logf("checked %d tuple headers", checked)
}

// The null bitmap must fit between the fixed header and t_hoff when
// HEAP_HASNULL is set.
func TestParseHeapTupleHeaderNullBitmap(t *testing.T) {
	tests := []struct {
		name     string
		infomask InfoMask
		natts    InfoMask2
		hoff     uint8
		valid    bool
	}{
		{name: "no nulls, many attributes", infomask: 0, natts: 1600, hoff: 24, valid: true},
		{name: "8 attributes fit in byte 23", infomask: HeapHasNull, natts: 8, hoff: 24, valid: true},
		{name: "9 attributes need 2 bytes", infomask: HeapHasNull, natts: 9, hoff: 24, valid: false},
		{name: "9 attributes with hoff 32", infomask: HeapHasNull, natts: 9, hoff: 32, valid: true},
		{name: "72 attributes fit before 32", infomask: HeapHasNull, natts: 72, hoff: 32, valid: true},
		{name: "73 attributes do not", infomask: HeapHasNull, natts: 73, hoff: 32, valid: false},
		{name: "no attributes", infomask: HeapHasNull, natts: 0, hoff: 24, valid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := HeapTupleHeader{Infomask: tt.infomask, Infomask2: tt.natts, Hoff: tt.hoff}

			_, err := ParseHeapTupleHeader(makeTuple(h, 64))
			if tt.valid && err != nil {
				t.Errorf("ParseHeapTupleHeader returned error: %v", err)
			}

			if !tt.valid && !errors.Is(err, ErrInvalidTupleHeader) {
				t.Errorf("errors.Is(err, ErrInvalidTupleHeader) = false, want true (err = %v)", err)
			}
		})
	}
}

// nullTupleHeader is row 4 of the hdr_demo table in the step 11 notes: nine
// int columns where only the first is set, so t_bits is 10000000 00000000.
var nullTupleHeader = HeapTupleHeader{
	Xmin:      1,
	Infomask2: 9,
	Infomask:  HeapHasNull | HeapXmaxInvalid,
	Hoff:      32,
}

// makeTuplePage builds a valid page whose line pointer 1 references tuple,
// placed at the end of the page.
func makeTuplePage(t *testing.T, tuple []byte) []byte {
	t.Helper()

	offset := uint16(PageSize - (len(tuple)+maxAlign-1)/maxAlign*maxAlign)
	h := PageHeader{
		Lower:         PageHeaderSize + itemIDSize,
		Upper:         offset,
		Special:       PageSize,
		PageSize:      PageSize,
		LayoutVersion: PageLayoutVersion,
	}

	page := makeItemPage(t, h, makeItemID(offset, ItemNormal, uint16(len(tuple))))
	copy(page[offset:], tuple)

	return page
}

func TestHeapTupleAtWithNulls(t *testing.T) {
	tuple := makeTuple(nullTupleHeader, 36)
	tuple[23] = 0x01                                 // only attribute 1 is not null
	copy(tuple[32:], []byte{0x04, 0x00, 0x00, 0x00}) // a = 4

	page := makeTuplePage(t, tuple)

	h, err := ParsePageHeader(page)
	if err != nil {
		t.Fatal(err)
	}

	got, err := HeapTupleAt(page, h, 1)
	if err != nil {
		t.Fatalf("HeapTupleAt returned error: %v", err)
	}

	if got.Header != nullTupleHeader {
		t.Errorf("Header = %+v, want %+v", got.Header, nullTupleHeader)
	}

	if want := []byte{0x01, 0x00}; !bytes.Equal(got.Bits, want) {
		t.Errorf("Bits = % x, want % x", got.Bits, want)
	}

	if want := []byte{0x04, 0x00, 0x00, 0x00}; !bytes.Equal(got.Data, want) {
		t.Errorf("Data = % x, want % x", got.Data, want)
	}

	for attnum, want := range map[int]bool{0: true, 1: false, 2: true, 8: true, 9: true, 10: true} {
		if isNull := got.IsNull(attnum); isNull != want {
			t.Errorf("IsNull(%d) = %v, want %v", attnum, isNull, want)
		}
	}
}

// Tuples without HEAP_HASNULL have no bitmap and no nulls among their
// stored attributes.
func TestHeapTupleIsNullWithoutBitmap(t *testing.T) {
	tuple := HeapTuple{Header: HeapTupleHeader{Infomask2: 2, Hoff: 24}}

	for attnum, want := range map[int]bool{0: true, 1: false, 2: false, 3: true} {
		if got := tuple.IsNull(attnum); got != want {
			t.Errorf("IsNull(%d) = %v, want %v", attnum, got, want)
		}
	}
}

// Line pointers without storage have no tuple, which is not corruption.
func TestHeapTupleAtNoStorage(t *testing.T) {
	page := makeItemPage(t, testItemHeader,
		makeItemID(0, ItemUnused, 0),
		makeItemID(1, ItemRedirect, 0),
		makeItemID(0, ItemDead, 0),
	)

	for n := OffsetNumber(1); n <= 3; n++ {
		_, err := HeapTupleAt(page, testItemHeader, n)
		if err == nil {
			t.Errorf("HeapTupleAt(%d): expected error, got nil", n)
		}

		if errors.Is(err, ErrInvalidItemID) || errors.Is(err, ErrInvalidTupleHeader) {
			t.Errorf("HeapTupleAt(%d): must not report corruption (err = %v)", n, err)
		}
	}
}

func TestHeapTupleAtCorrupt(t *testing.T) {
	tests := []struct {
		name string
		id   ItemID
		want error
	}{
		{name: "line pointer past the page", id: makeItemID(8160, ItemNormal, 40), want: ErrInvalidItemID},
		{name: "tuple shorter than its header", id: makeItemID(8184, ItemNormal, 8), want: ErrInvalidTupleHeader},
		{name: "tuple with a zero header", id: makeItemID(8000, ItemNormal, 40), want: ErrInvalidTupleHeader},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := makeItemPage(t, testItemHeader, tt.id)

			got, err := HeapTupleAt(page, testItemHeader, 1)
			if !errors.Is(err, tt.want) {
				t.Fatalf("errors.Is(err, %v) = false, want true (err = %v)", tt.want, err)
			}

			if got.Data != nil || got.Bits != nil || got.Header != (HeapTupleHeader{}) {
				t.Errorf("HeapTupleAt = %+v with error, want zero value", got)
			}
		})
	}
}

// Data shares memory with the page, but appending to it must not write into
// the bytes that follow the tuple.
func TestHeapTupleAtSharesPage(t *testing.T) {
	// A 36-byte tuple is aligned to 40, so it spans bytes 8152-8187 and 4
	// padding bytes follow it before the end of the page.
	page := makeTuplePage(t, makeTuple(HeapTupleHeader{Hoff: 24}, 36))

	h, err := ParsePageHeader(page)
	if err != nil {
		t.Fatal(err)
	}

	tuple, err := HeapTupleAt(page, h, 1)
	if err != nil {
		t.Fatalf("HeapTupleAt returned error: %v", err)
	}

	// Data is bytes 8176-8187 of the page, not a copy of them.
	page[8187] = 0xAB

	if tuple.Data[len(tuple.Data)-1] != 0xAB {
		t.Error("Data does not reflect a change to the page")
	}

	if got := cap(tuple.Data); got != len(tuple.Data) {
		t.Errorf("cap(Data) = %d, want %d so appends cannot reach past the tuple", got, len(tuple.Data))
	}

	_ = append(tuple.Data, 0xCD)

	if page[8188] != 0 {
		t.Errorf("appending to Data wrote %#x into the padding after the tuple", page[8188])
	}
}

func TestHeapTupleAtDoesNotAllocate(t *testing.T) {
	tuple := makeTuple(nullTupleHeader, 36)
	tuple[23] = 0x01
	page := makeTuplePage(t, tuple)

	h, err := ParsePageHeader(page)
	if err != nil {
		t.Fatal(err)
	}

	allocs := testing.AllocsPerRun(100, func() {
		if _, err := HeapTupleAt(page, h, 1); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Errorf("HeapTupleAt allocated %v times per call, want 0", allocs)
	}
}

// Every line pointer of the fixture either yields its tuple or reports that
// it has none, and row data matches the tuple lengths from heap_page_items().
func TestFixtureHeapTuples(t *testing.T) {
	rel := openRelation(t, fixtureHeap)

	page, err := rel.ReadPage(2)
	if err != nil {
		t.Fatal(err)
	}

	h, err := ParsePageHeader(page)
	if err != nil {
		t.Fatal(err)
	}

	// (2,168) is the HOT update of row 380: id int4 = 380, name text = 'user 380 v2'.
	got, err := HeapTupleAt(page, h, 168)
	if err != nil {
		t.Fatalf("HeapTupleAt(168) returned error: %v", err)
	}

	want := append([]byte{0x7c, 0x01, 0x00, 0x00, 0x19}, "user 380 v2"...)
	if !bytes.Equal(got.Data, want) {
		t.Errorf("Data = % x, want % x", got.Data, want)
	}

	if got.Bits != nil {
		t.Errorf("Bits = % x, want nil", got.Bits)
	}

	for block := range rel.PageCount() {
		if page, err = rel.ReadPage(block); err != nil {
			t.Fatal(err)
		}

		if h, err = ParsePageHeader(page); err != nil {
			t.Fatal(err)
		}

		for n := FirstOffsetNumber; int(n) <= h.ItemCount(); n++ {
			id, err := ItemIDAt(page, h, n)
			if err != nil {
				t.Fatal(err)
			}

			tuple, err := HeapTupleAt(page, h, n)

			switch {
			case !id.HasStorage() && err == nil:
				t.Errorf("block %d item %d: %v without storage returned a tuple", block, n, id.State())
			case id.HasStorage() && err != nil:
				t.Errorf("block %d item %d: HeapTupleAt returned error: %v", block, n, err)
			case id.HasStorage() && len(tuple.Data) != int(id.Length())-int(tuple.Header.Hoff):
				t.Errorf("block %d item %d: %d data bytes, want %d", block, n, len(tuple.Data), int(id.Length())-int(tuple.Header.Hoff))
			}
		}
	}
}
