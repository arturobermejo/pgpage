package pgpage

import (
	"encoding/binary"
	"fmt"
)

// itemPointerSize is the size of ItemPointerData on disk.
const itemPointerSize = 6

// ItemPointer is a tuple identifier (ItemPointerData, or TID): the physical
// address of a tuple as a block number and a line pointer number. PostgreSQL
// exposes it as the ctid system column.
type ItemPointer struct {
	Block  BlockNumber
	Offset OffsetNumber
}

// String formats the pointer as PostgreSQL shows a ctid, for example "(0,4)".
func (p ItemPointer) String() string {
	return fmt.Sprintf("(%d,%d)", p.Block, p.Offset)
}

// decodeItemPointer decodes the ItemPointerData in the first itemPointerSize
// bytes of b.
//
// The block number is stored as two 16-bit halves (BlockIdData), high half
// first, so that the struct is 6 bytes with no padding:
//
//	0       2       4       6
//	+-------+-------+-------+
//	| bi_hi | bi_lo | posid |
//	+-------+-------+-------+
func decodeItemPointer(b []byte) ItemPointer {
	le := binary.LittleEndian
	hi, lo := le.Uint16(b[0:2]), le.Uint16(b[2:4])

	return ItemPointer{
		Block:  BlockNumber(hi)<<16 | BlockNumber(lo),
		Offset: OffsetNumber(le.Uint16(b[4:itemPointerSize])),
	}
}
