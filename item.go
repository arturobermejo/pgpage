package pgpage

import "fmt"

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
