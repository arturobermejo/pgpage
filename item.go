package pgpage

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

// Flags returns lp_flags, the 2-bit state of the line pointer.
func (id ItemID) Flags() uint8 {
	return uint8((id >> itemOffsetBits) & (1<<itemFlagsBits - 1))
}

// Length returns lp_len, the length of the tuple in bytes.
func (id ItemID) Length() uint16 {
	return uint16((id >> (itemOffsetBits + itemFlagsBits)) & (1<<itemLengthBits - 1))
}
