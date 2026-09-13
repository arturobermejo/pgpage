package pgpage

import (
	"encoding/binary"
	"errors"
	"testing"
)

// devPage is the header of the heap page inspected during development.
var devPage = PageHeader{
	LSN:           0x019A5810,
	Checksum:      0xB073,
	Flags:         0,
	Lower:         72,
	Upper:         8176,
	Special:       8176,
	PageSize:      PageSize,
	LayoutVersion: PageLayoutVersion,
	PruneXID:      731,
}

// makePage serializes h into the header of an otherwise zero page.
func makePage(h PageHeader) []byte {
	page := make([]byte, PageSize)
	le := binary.LittleEndian

	le.PutUint32(page[0:4], uint32(h.LSN>>32))
	le.PutUint32(page[4:8], uint32(h.LSN))
	le.PutUint16(page[8:10], h.Checksum)
	le.PutUint16(page[10:12], uint16(h.Flags))
	le.PutUint16(page[12:14], h.Lower)
	le.PutUint16(page[14:16], h.Upper)
	le.PutUint16(page[16:18], h.Special)
	le.PutUint16(page[18:20], h.PageSize|uint16(h.LayoutVersion))
	le.PutUint32(page[20:24], uint32(h.PruneXID))

	return page
}

func TestParsePageHeader(t *testing.T) {
	got, err := ParsePageHeader(makePage(devPage))
	if err != nil {
		t.Fatalf("ParsePageHeader returned error: %v", err)
	}

	if got != devPage {
		t.Errorf("ParsePageHeader = %+v, want %+v", got, devPage)
	}

	if got.IsNew() {
		t.Error("IsNew() = true, want false")
	}
}

// The raw bytes from the PRD must decode to the LSN pageinspect reports.
func TestParsePageHeaderRawBytes(t *testing.T) {
	page := make([]byte, PageSize)
	copy(page, []byte{
		0x00, 0x00, 0x00, 0x00, 0x10, 0x58, 0x9a, 0x01, // pd_lsn
		0x73, 0xb0, // pd_checksum
		0x00, 0x00, // pd_flags
		0x48, 0x00, // pd_lower
		0xf0, 0x1f, // pd_upper
		0xf0, 0x1f, // pd_special
		0x04, 0x20, // pd_pagesize_version
		0x00, 0x00, 0x00, 0x00, // pd_prune_xid
	})

	h, err := ParsePageHeader(page)
	if err != nil {
		t.Fatalf("ParsePageHeader returned error: %v", err)
	}

	if got, want := h.LSN.String(), "0/19A5810"; got != want {
		t.Errorf("LSN = %s, want %s", got, want)
	}

	if h.Lower != 72 || h.Upper != 8176 || h.Special != 8176 {
		t.Errorf("lower=%d upper=%d special=%d, want 72 8176 8176", h.Lower, h.Upper, h.Special)
	}

	if h.PageSize != 8192 || h.LayoutVersion != 4 {
		t.Errorf("pagesize=%d version=%d, want 8192 4", h.PageSize, h.LayoutVersion)
	}
}

func TestLSNString(t *testing.T) {
	tests := []struct {
		lsn  LSN
		want string
	}{
		{lsn: 0, want: "0/0"},
		{lsn: 0x019A5810, want: "0/19A5810"},
		{lsn: 0x00000016_B374D848, want: "16/B374D848"},
		{lsn: 0xFFFFFFFF_FFFFFFFF, want: "FFFFFFFF/FFFFFFFF"},
	}

	for _, tt := range tests {
		if got := tt.lsn.String(); got != tt.want {
			t.Errorf("LSN(%#x).String() = %q, want %q", uint64(tt.lsn), got, tt.want)
		}
	}
}

func TestParsePageHeaderNewPage(t *testing.T) {
	h, err := ParsePageHeader(make([]byte, PageSize))
	if err != nil {
		t.Fatalf("ParsePageHeader returned error: %v", err)
	}

	if !h.IsNew() {
		t.Error("IsNew() = false, want true")
	}

	if h != (PageHeader{}) {
		t.Errorf("header = %+v, want zero value", h)
	}

	if n := h.ItemCount(); n != 0 {
		t.Errorf("ItemCount() = %d, want 0", n)
	}
}

// pd_upper of 0 is only a new page if the whole page is zero.
func TestParsePageHeaderZeroUpperWithData(t *testing.T) {
	tests := map[string]func([]byte){
		"garbage in header": func(p []byte) { p[12] = 72 },
		"garbage in body":   func(p []byte) { p[PageSize-1] = 1 },
	}

	for name, corrupt := range tests {
		t.Run(name, func(t *testing.T) {
			page := make([]byte, PageSize)
			corrupt(page)

			_, err := ParsePageHeader(page)
			if !errors.Is(err, ErrInvalidPageHeader) {
				t.Fatalf("errors.Is(err, ErrInvalidPageHeader) = false, want true (err = %v)", err)
			}
		})
	}
}

func TestParsePageHeaderLength(t *testing.T) {
	for _, size := range []int{0, PageHeaderSize - 1, PageHeaderSize, PageSize - 1, PageSize + 1} {
		page := make([]byte, size)
		copy(page, makePage(devPage))

		h, err := ParsePageHeader(page)
		if err == nil {
			t.Fatalf("page of %d bytes: expected error, got nil", size)
		}

		if h != (PageHeader{}) {
			t.Errorf("page of %d bytes: header = %+v, want zero value", size, h)
		}
	}
}

func TestParsePageHeaderInvalid(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*PageHeader)
	}{
		{name: "lower inside header", modify: func(h *PageHeader) { h.Lower = PageHeaderSize - 4 }},
		{name: "lower past upper", modify: func(h *PageHeader) { h.Lower = h.Upper + 4 }},
		{name: "upper past special", modify: func(h *PageHeader) { h.Upper = h.Special + 8 }},
		{name: "special past page", modify: func(h *PageHeader) { h.Upper, h.Special = PageSize, PageSize+8 }},
		{name: "special not aligned", modify: func(h *PageHeader) { h.Upper, h.Special = 8170, 8171 }},
		{name: "page size 4096", modify: func(h *PageHeader) { h.PageSize = 4096 }},
		{name: "page size 16384", modify: func(h *PageHeader) { h.PageSize = 16384 }},
		{name: "layout version 3", modify: func(h *PageHeader) { h.LayoutVersion = 3 }},
		{name: "unknown flag", modify: func(h *PageHeader) { h.Flags = 0x0008 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hdr := devPage
			tt.modify(&hdr)

			h, err := ParsePageHeader(makePage(hdr))
			if !errors.Is(err, ErrInvalidPageHeader) {
				t.Fatalf("errors.Is(err, ErrInvalidPageHeader) = false, want true (err = %v)", err)
			}

			if h != (PageHeader{}) {
				t.Errorf("header = %+v, want zero value", h)
			}
		})
	}
}

// Boundary values that PostgreSQL accepts must parse.
func TestParsePageHeaderValidBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*PageHeader)
	}{
		{name: "no items", modify: func(h *PageHeader) { h.Lower = PageHeaderSize }},
		{name: "no free space", modify: func(h *PageHeader) { h.Lower = h.Upper }},
		{name: "no special space", modify: func(h *PageHeader) { h.Upper, h.Special = PageSize, PageSize }},
		{name: "all flags", modify: func(h *PageHeader) { h.Flags = PageHasFreeLines | PageFull | PageAllVisible }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hdr := devPage
			tt.modify(&hdr)

			got, err := ParsePageHeader(makePage(hdr))
			if err != nil {
				t.Fatalf("ParsePageHeader returned error: %v", err)
			}

			if got != hdr {
				t.Errorf("ParsePageHeader = %+v, want %+v", got, hdr)
			}
		})
	}
}

func TestPageFlags(t *testing.T) {
	tests := []struct {
		flags                       PageFlags
		freeLines, full, allVisible bool
	}{
		{flags: 0},
		{flags: PageHasFreeLines, freeLines: true},
		{flags: PageFull, full: true},
		{flags: PageAllVisible, allVisible: true},
		{flags: PageHasFreeLines | PageFull | PageAllVisible, freeLines: true, full: true, allVisible: true},
	}

	for _, tt := range tests {
		if got := tt.flags.HasFreeLines(); got != tt.freeLines {
			t.Errorf("PageFlags(%#x).HasFreeLines() = %v, want %v", uint16(tt.flags), got, tt.freeLines)
		}

		if got := tt.flags.IsFull(); got != tt.full {
			t.Errorf("PageFlags(%#x).IsFull() = %v, want %v", uint16(tt.flags), got, tt.full)
		}

		if got := tt.flags.IsAllVisible(); got != tt.allVisible {
			t.Errorf("PageFlags(%#x).IsAllVisible() = %v, want %v", uint16(tt.flags), got, tt.allVisible)
		}
	}
}

func TestPageHeaderHelpers(t *testing.T) {
	if got, want := devPage.FreeSpace(), 8176-72; got != want {
		t.Errorf("FreeSpace() = %d, want %d", got, want)
	}

	if got, want := devPage.ItemCount(), 12; got != want {
		t.Errorf("ItemCount() = %d, want %d", got, want)
	}
}

// ParsePageHeader must not allocate on success.
func TestParsePageHeaderDoesNotAllocate(t *testing.T) {
	page := makePage(devPage)

	allocs := testing.AllocsPerRun(100, func() {
		if _, err := ParsePageHeader(page); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Errorf("ParsePageHeader allocated %v times per call, want 0", allocs)
	}
}

func FuzzParsePageHeader(f *testing.F) {
	f.Add(makePage(devPage)[:PageHeaderSize])
	f.Add(make([]byte, PageHeaderSize))

	// The fuzzer mutates the header; the rest of the page stays zero.
	f.Fuzz(func(t *testing.T, header []byte) {
		page := make([]byte, PageSize)
		copy(page, header)

		h, err := ParsePageHeader(page)
		if err != nil {
			return
		}

		if h.IsNew() {
			if h != (PageHeader{}) {
				t.Fatalf("new page returned non-zero header %+v", h)
			}

			return
		}

		if h.Lower < PageHeaderSize || h.Lower > h.Upper || h.Upper > h.Special || h.Special > PageSize {
			t.Fatalf("accepted invalid boundaries: %+v", h)
		}

		if h.PageSize != PageSize || h.LayoutVersion != PageLayoutVersion {
			t.Fatalf("accepted unsupported size or version: %+v", h)
		}
	})
}
