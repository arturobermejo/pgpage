package pgpage

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// HeapTupleHeaderSize is the size of the fixed part of HeapTupleHeaderData
// (SizeofHeapTupleHeader). The null bitmap, if any, and padding follow it up
// to t_hoff.
const HeapTupleHeaderSize = 23

// ErrInvalidTupleHeader means a heap tuple header is inconsistent, which
// indicates a corrupt tuple or a line pointer to something else.
var ErrInvalidTupleHeader = errors.New("invalid heap tuple header")

// HeapTupleHeader is the decoded fixed part of a heap tuple header
// (HeapTupleHeaderData).
type HeapTupleHeader struct {
	Xmin TransactionID // inserting transaction
	Xmax TransactionID // deleting or locking transaction, 0 if none
	// Field3 is t_cid, the command ID within the inserting or deleting
	// transaction, or t_xvac on tuples moved by pre-9.0 VACUUM FULL.
	Field3    uint32
	Ctid      ItemPointer // this tuple, or its newer version after an update
	Infomask2 InfoMask2   // number of attributes and HOT flags
	Infomask  InfoMask    // visibility and layout flags
	Hoff      uint8       // offset to user data
}

// Natts returns the number of attributes stored in the tuple, which can be
// lower than the table's after ALTER TABLE ADD COLUMN.
func (h HeapTupleHeader) Natts() int {
	return h.Infomask2.Natts()
}

// ParseHeapTupleHeader decodes the header of tuple, the lp_len bytes a line
// pointer references. Fields are decoded as little-endian.
//
// Errors wrap ErrInvalidTupleHeader.
func ParseHeapTupleHeader(tuple []byte) (HeapTupleHeader, error) {
	if len(tuple) < HeapTupleHeaderSize {
		return HeapTupleHeader{}, fmt.Errorf("pgpage: tuple is %d bytes, shorter than its %d-byte header: %w",
			len(tuple), HeapTupleHeaderSize, ErrInvalidTupleHeader)
	}

	le := binary.LittleEndian
	h := HeapTupleHeader{
		Xmin:      TransactionID(le.Uint32(tuple[0:4])),
		Xmax:      TransactionID(le.Uint32(tuple[4:8])),
		Field3:    le.Uint32(tuple[8:12]),
		Ctid:      decodeItemPointer(tuple[12:18]),
		Infomask2: InfoMask2(le.Uint16(tuple[18:20])),
		Infomask:  InfoMask(le.Uint16(tuple[20:22])),
		Hoff:      tuple[22],
	}

	// PostgreSQL rounds t_hoff up to MAXALIGN, so user data starts aligned.
	if h.Hoff < HeapTupleHeaderSize || h.Hoff%maxAlign != 0 || int(h.Hoff) > len(tuple) {
		return HeapTupleHeader{}, fmt.Errorf("pgpage: t_hoff %d is not an aligned offset between %d and the tuple length %d: %w",
			h.Hoff, HeapTupleHeaderSize, len(tuple), ErrInvalidTupleHeader)
	}

	return h, nil
}

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
