package pgpage

import (
	"encoding/binary"
	"fmt"
)

// ChecksumStatus says whether the checksum stored in a page header matches
// the bytes of the page.
type ChecksumStatus uint8

const (
	// ChecksumUnknown means the checksum was not verified: the page has no
	// valid header to compare with. It is the zero value.
	ChecksumUnknown ChecksumStatus = iota
	// ChecksumOK means pd_checksum matches the page.
	ChecksumOK
	// ChecksumMismatch means pd_checksum does not match the page: it was
	// damaged after PostgreSQL wrote it, or it was written as another block.
	ChecksumMismatch
	// ChecksumDisabled means pd_checksum is 0, which PostgreSQL never stores
	// when data checksums are on: the page was written with them off.
	ChecksumDisabled
)

// String returns the status as shown to users, for example "OK".
func (s ChecksumStatus) String() string {
	switch s {
	case ChecksumUnknown:
		return "UNKNOWN"
	case ChecksumOK:
		return "OK"
	case ChecksumMismatch:
		return "MISMATCH"
	case ChecksumDisabled:
		return "DISABLED"
	default:
		return fmt.Sprintf("ChecksumStatus(%d)", uint8(s))
	}
}

// The page checksum is a variant of FNV-1a, computed as 32 partial sums in
// parallel so that a compiler can vectorize it (checksum_impl.h).
const (
	checksumSums = 32
	fnvPrime     = 16777619
)

// checksumBase holds the values the 32 partial sums start from, one per
// column of 4-byte words.
var checksumBase = [checksumSums]uint32{
	0x5B1F36E9, 0xB8525960, 0x02AB50AA, 0x1DE66D2A,
	0x79FF467A, 0x9BB9F8A3, 0x217E7CD2, 0x83E13D2C,
	0xF8D4474F, 0xE39EB970, 0x42C6AE16, 0x993216FA,
	0x7B093B5D, 0x98DAFF3C, 0xF718902A, 0x0B1C9CDB,
	0xE58F764B, 0x187636BC, 0x5D7B3BB1, 0xE73DE7DE,
	0x92BEC979, 0xCCA6C0B2, 0x304A0979, 0x85AA43D4,
	0x783125BB, 0x6CA8EAA2, 0xE407EAC6, 0x4B5CFC3E,
	0x9FBF8C76, 0x15CA20BE, 0xF2CA9FD3, 0x959BD756,
}

// checksumStep mixes one word into a partial sum (CHECKSUM_COMP).
func checksumStep(sum, word uint32) uint32 {
	tmp := sum ^ word

	return (tmp * fnvPrime) ^ (tmp >> 17)
}

// PageChecksum returns the checksum PostgreSQL stores in pd_checksum when it
// writes page as block (pg_checksum_page). The stored pd_checksum takes no
// part in it, so the result can be compared with it. The block number does:
// a page written in the wrong place fails the check too.
//
// It reads the first PageSize bytes of page, and panics if there are fewer.
func PageChecksum(page []byte, block BlockNumber) uint16 {
	page = page[:PageSize]
	sums := checksumBase
	le := binary.LittleEndian

	for offset := 0; offset < PageSize; offset += 4 {
		word := le.Uint32(page[offset:])

		// pd_checksum is the low half of the word at byte 8, and is taken
		// as 0: the checksum cannot depend on itself.
		if offset == 8 {
			word &^= 0xFFFF
		}

		j := (offset / 4) % checksumSums
		sums[j] = checksumStep(sums[j], word)
	}

	// Two rounds of zeroes, so that the last words of the page get mixed as
	// well as the first ones.
	for range 2 {
		for j := range sums {
			sums[j] = checksumStep(sums[j], 0)
		}
	}

	var result uint32
	for _, sum := range sums {
		result ^= sum
	}

	result ^= uint32(block)

	// Fold into 16 bits, leaving 0 out: a stored 0 then means "no checksum".
	return uint16(result%65535 + 1)
}

// VerifyChecksum compares the pd_checksum stored in page with the checksum
// of its bytes, read as block. It also returns the computed checksum, which
// says what the header should hold when the two differ.
//
// It is only meaningful on a page with a valid header. PostgreSQL does not
// checksum new pages, and on a page that is not a page at all the result
// says nothing more than "these bytes are not that page".
func VerifyChecksum(page []byte, block BlockNumber) (status ChecksumStatus, computed uint16) {
	computed = PageChecksum(page, block)
	stored := binary.LittleEndian.Uint16(page[8:10])

	switch stored {
	case 0:
		return ChecksumDisabled, computed
	case computed:
		return ChecksumOK, computed
	default:
		return ChecksumMismatch, computed
	}
}
