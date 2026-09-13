package pgpage

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// PageHeaderSize is the size of PageHeaderData at the start of every page.
const PageHeaderSize = 24

// PageLayoutVersion is the page layout version this package understands
// (PG_PAGE_LAYOUT_VERSION), used by PostgreSQL 8.3 and later.
const PageLayoutVersion = 4

// itemIDSize is the size of one ItemIdData entry in the line pointer array.
const itemIDSize = 4

// maxAlign is MAXIMUM_ALIGNOF on the 64-bit platforms this package targets.
const maxAlign = 8

// ErrInvalidPageHeader means the page header is inconsistent, which
// indicates a corrupt page or one that is not a PostgreSQL page at all.
var ErrInvalidPageHeader = errors.New("invalid page header")

// LSN is a write-ahead log position (XLogRecPtr).
type LSN uint64

// String formats the LSN as PostgreSQL does, for example "0/19A5810".
func (l LSN) String() string {
	return fmt.Sprintf("%X/%X", uint32(l>>32), uint32(l))
}

// TransactionID is a 32-bit PostgreSQL transaction ID.
type TransactionID uint32

// PageFlags holds the pd_flags bits of a page header.
type PageFlags uint16

const (
	// PageHasFreeLines means there are unused line pointers before pd_lower.
	PageHasFreeLines PageFlags = 0x0001
	// PageFull means there is not enough free space for a new tuple.
	PageFull PageFlags = 0x0002
	// PageAllVisible means all tuples on the page are visible to everyone.
	PageAllVisible PageFlags = 0x0004

	validPageFlags = PageHasFreeLines | PageFull | PageAllVisible
)

// HasFreeLines reports whether PageHasFreeLines is set.
func (f PageFlags) HasFreeLines() bool { return f&PageHasFreeLines != 0 }

// IsFull reports whether PageFull is set.
func (f PageFlags) IsFull() bool { return f&PageFull != 0 }

// IsAllVisible reports whether PageAllVisible is set.
func (f PageFlags) IsAllVisible() bool { return f&PageAllVisible != 0 }

// PageHeader is the decoded PageHeaderData of a page.
type PageHeader struct {
	LSN           LSN
	Checksum      uint16
	Flags         PageFlags
	Lower         uint16 // end of the line pointer array
	Upper         uint16 // start of tuple data
	Special       uint16 // start of the special space
	PageSize      uint16
	LayoutVersion uint8
	PruneXID      TransactionID
}

// IsNew reports whether the page has never been initialized. PostgreSQL
// leaves such all-zero pages behind when it extends a relation.
func (h PageHeader) IsNew() bool { return h.Upper == 0 }

// FreeSpace returns the number of bytes between the line pointer array and
// the tuple data.
func (h PageHeader) FreeSpace() int { return int(h.Upper) - int(h.Lower) }

// ItemCount returns the number of line pointers on the page.
func (h PageHeader) ItemCount() int {
	if h.IsNew() {
		return 0
	}

	return (int(h.Lower) - PageHeaderSize) / itemIDSize
}

// ParsePageHeader decodes the header of page, which must be PageSize bytes
// long, as returned by ReadPage. Fields are decoded as little-endian.
//
// An all-zero page is valid: it yields a zero PageHeader for which IsNew
// reports true. Errors other than a wrong page length wrap
// ErrInvalidPageHeader.
func ParsePageHeader(page []byte) (PageHeader, error) {
	if len(page) != PageSize {
		return PageHeader{}, fmt.Errorf("pgpage: page is %d bytes, want %d", len(page), PageSize)
	}

	le := binary.LittleEndian
	sizeVersion := le.Uint16(page[18:20])

	h := PageHeader{
		// pd_lsn is stored as two uint32s: xlogid (high) then xrecoff (low).
		LSN:           LSN(uint64(le.Uint32(page[0:4]))<<32 | uint64(le.Uint32(page[4:8]))),
		Checksum:      le.Uint16(page[8:10]),
		Flags:         PageFlags(le.Uint16(page[10:12])),
		Lower:         le.Uint16(page[12:14]),
		Upper:         le.Uint16(page[14:16]),
		Special:       le.Uint16(page[16:18]),
		PageSize:      sizeVersion &^ 0x00FF,
		LayoutVersion: uint8(sizeVersion),
		PruneXID:      TransactionID(le.Uint32(page[20:24])),
	}

	if h.IsNew() {
		for i, b := range page {
			if b != 0 {
				return PageHeader{}, fmt.Errorf("pgpage: pd_upper is 0 but byte %d is %#x: %w", i, b, ErrInvalidPageHeader)
			}
		}

		return PageHeader{}, nil
	}

	if err := h.validate(); err != nil {
		return PageHeader{}, err
	}

	return h, nil
}

func (h PageHeader) validate() error {
	if h.PageSize != PageSize {
		return fmt.Errorf("pgpage: unsupported page size %d, want %d: %w", h.PageSize, PageSize, ErrInvalidPageHeader)
	}

	if h.LayoutVersion != PageLayoutVersion {
		return fmt.Errorf("pgpage: unsupported layout version %d, want %d: %w", h.LayoutVersion, PageLayoutVersion, ErrInvalidPageHeader)
	}

	if h.Flags&^validPageFlags != 0 {
		return fmt.Errorf("pgpage: invalid flags %#04x: %w", uint16(h.Flags), ErrInvalidPageHeader)
	}

	if h.Lower < PageHeaderSize || h.Lower > h.Upper || h.Upper > h.Special || h.Special > PageSize {
		return fmt.Errorf("pgpage: invalid page boundaries: lower=%d upper=%d special=%d: %w",
			h.Lower, h.Upper, h.Special, ErrInvalidPageHeader)
	}

	if h.Special%maxAlign != 0 {
		return fmt.Errorf("pgpage: special offset %d is not aligned to %d: %w", h.Special, maxAlign, ErrInvalidPageHeader)
	}

	return nil
}
