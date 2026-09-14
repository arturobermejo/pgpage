package pgpage

import "fmt"

// MaxBlockNumber is the highest valid block number. PostgreSQL reserves
// 0xFFFFFFFF as InvalidBlockNumber, so a relation holds at most
// MaxBlockNumber+1 pages.
const MaxBlockNumber BlockNumber = 0xFFFFFFFE

// PageCount returns the number of complete pages in a relation segment of
// size bytes, and the number of trailing bytes after the last complete page.
// Trailing bytes mean the last page is partial, as ReadPage would report.
//
// It returns an error if size is negative or holds more pages than a
// relation can address.
func PageCount(size int64) (pages BlockNumber, trailing int64, err error) {
	if size < 0 {
		return 0, 0, fmt.Errorf("pgpage: negative size %d", size)
	}

	n := size / PageSize
	if n > int64(MaxBlockNumber)+1 {
		return 0, 0, fmt.Errorf("pgpage: size %d holds %d pages, more than the maximum %d", size, n, int64(MaxBlockNumber)+1)
	}

	return BlockNumber(n), size % PageSize, nil
}
