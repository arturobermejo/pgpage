package pgpage

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// OffsetNumber is the 1-based number of a line pointer within a page, the
// second half of a tuple identifier such as (0,4).
type OffsetNumber uint16

// FirstOffsetNumber is the number of the first line pointer of a page.
// PostgreSQL reserves 0 as InvalidOffsetNumber.
const FirstOffsetNumber OffsetNumber = 1

// ErrInvalidItemID means a line pointer is inconsistent with its page, which
// indicates a corrupt page.
var ErrInvalidItemID = errors.New("invalid line pointer")

// ItemState is the state of a line pointer, stored in its 2-bit lp_flags.
// The values are those written on disk.
type ItemState uint8

const (
	// ItemUnused is a free line pointer, available for reuse (LP_UNUSED).
	ItemUnused ItemState = 0
	// ItemNormal points to a tuple (LP_NORMAL).
	ItemNormal ItemState = 1
	// ItemRedirect is the root of a HOT chain whose first tuple was pruned.
	// Its lp_off holds the item number it redirects to (LP_REDIRECT).
	ItemRedirect ItemState = 2
	// ItemDead points to a dead tuple that index entries may still
	// reference, so it cannot be reused until VACUUM removes them (LP_DEAD).
	ItemDead ItemState = 3
)

// String returns the state as shown to users, for example "NORMAL".
func (s ItemState) String() string {
	switch s {
	case ItemUnused:
		return "UNUSED"
	case ItemNormal:
		return "NORMAL"
	case ItemRedirect:
		return "REDIRECT"
	case ItemDead:
		return "DEAD"
	default:
		return fmt.Sprintf("ItemState(%d)", uint8(s))
	}
}

// Bit widths of the fields packed into an ItemIdData.
const (
	itemOffsetBits = 15
	itemFlagsBits  = 2
	itemLengthBits = 15
)

// ItemID is one line pointer (ItemIdData): a 32-bit word that packs where a
// tuple starts in the page, its state flags and its length.
//
// The layout matches the C bit-field
//
//	unsigned lp_off:15, lp_flags:2, lp_len:15;
//
// as laid out by GCC and Clang on little-endian machines, where the first
// field takes the least significant bits:
//
//	bit 31          17 16 15 14            0
//	    +-------------+-----+---------------+
//	    |   lp_len    |flags|    lp_off     |
//	    +-------------+-----+---------------+
type ItemID uint32

// Offset returns lp_off, the offset of the tuple from the start of the page.
func (id ItemID) Offset() uint16 {
	return uint16(id & (1<<itemOffsetBits - 1))
}

// State returns the state stored in lp_flags.
func (id ItemID) State() ItemState {
	return ItemState((id >> itemOffsetBits) & (1<<itemFlagsBits - 1))
}

// HasStorage reports whether the line pointer references tuple bytes on the
// page. By convention lp_len is 0 exactly when it does not: always for
// ItemUnused and ItemRedirect, never for ItemNormal, and either for ItemDead.
func (id ItemID) HasStorage() bool {
	return id.Length() != 0
}

// Length returns lp_len, the length of the tuple in bytes.
func (id ItemID) Length() uint16 {
	return uint16((id >> (itemOffsetBits + itemFlagsBits)) & (1<<itemLengthBits - 1))
}

// ItemIDAt returns line pointer n of page, whose header is h, without
// validating it. n must be between FirstOffsetNumber and h.ItemCount().
func ItemIDAt(page []byte, h PageHeader, n OffsetNumber) (ItemID, error) {
	count, err := itemCount(page, h)
	if err != nil {
		return 0, err
	}

	if n < FirstOffsetNumber || int(n) > count {
		return 0, fmt.Errorf("pgpage: no line pointer %d, page has %d", n, count)
	}

	return itemIDAt(page, n), nil
}

// ParseItemIDs returns all line pointers of page, whose header is h, in
// order, without validating them: the first element is line pointer 1.
// It appends to dst[:0], so passing the previous result reuses its memory.
func ParseItemIDs(page []byte, h PageHeader, dst []ItemID) ([]ItemID, error) {
	count, err := itemCount(page, h)
	if err != nil {
		return dst[:0], err
	}

	dst = dst[:0]
	for n := FirstOffsetNumber; int(n) <= count; n++ {
		dst = append(dst, itemIDAt(page, n))
	}

	return dst, nil
}

// itemCount returns the number of line pointers of page after checking that
// h can describe it.
func itemCount(page []byte, h PageHeader) (int, error) {
	if len(page) != PageSize {
		return 0, fmt.Errorf("pgpage: page is %d bytes, want %d", len(page), PageSize)
	}

	if !h.IsNew() && (h.Lower < PageHeaderSize || int(h.Lower) > len(page)) {
		return 0, fmt.Errorf("pgpage: pd_lower %d does not fit the page", h.Lower)
	}

	return h.ItemCount(), nil
}

// itemIDAt decodes line pointer n, which the caller has checked exists.
func itemIDAt(page []byte, n OffsetNumber) ItemID {
	start := PageHeaderSize + (int(n)-1)*itemIDSize

	return ItemID(binary.LittleEndian.Uint32(page[start : start+itemIDSize]))
}

// CheckItemID reports whether line pointer n, with value id, is consistent
// with the page header h. Errors about the line pointer wrap
// ErrInvalidItemID.
//
// Unused line pointers are not checked, as their bits carry no meaning.
func (h PageHeader) CheckItemID(n OffsetNumber, id ItemID) error {
	count := h.ItemCount()
	if n < FirstOffsetNumber || int(n) > count {
		return fmt.Errorf("pgpage: no line pointer %d, page has %d", n, count)
	}

	switch id.State() {
	case ItemUnused:
		return nil
	case ItemNormal:
		if !id.HasStorage() {
			return fmt.Errorf("pgpage: line pointer %d is NORMAL but has no storage: %w", n, ErrInvalidItemID)
		}
	case ItemRedirect:
		if id.HasStorage() {
			return fmt.Errorf("pgpage: line pointer %d is REDIRECT but has storage: %w", n, ErrInvalidItemID)
		}

		target := OffsetNumber(id.Offset())
		if target < FirstOffsetNumber || int(target) > count || target == n {
			return fmt.Errorf("pgpage: line pointer %d redirects to %d, want another of 1-%d: %w", n, target, count, ErrInvalidItemID)
		}
	case ItemDead:
		// A dead line pointer may or may not keep its storage.
	}

	if !id.HasStorage() {
		return nil
	}

	// Checking PageSize too keeps accepted line pointers inside the page
	// even when h was not decoded from it.
	start, end := int(id.Offset()), int(id.Offset())+int(id.Length())
	if start < int(h.Upper) || end > int(h.Special) || end > PageSize {
		return fmt.Errorf("pgpage: line pointer %d spans bytes %d-%d, outside tuple space %d-%d: %w",
			n, start, end-1, h.Upper, h.Special-1, ErrInvalidItemID)
	}

	if start%maxAlign != 0 {
		return fmt.Errorf("pgpage: line pointer %d offset %d is not aligned to %d: %w", n, start, maxAlign, ErrInvalidItemID)
	}

	return nil
}
