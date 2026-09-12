// Package pgpage reads raw pages from PostgreSQL relation files.
package pgpage

import (
	"errors"
	"fmt"
	"io"
)

// PageSize is the PostgreSQL page size (BLCKSZ), 8 kB by default.
const PageSize = 8192

// BlockNumber is the zero-based position of a page within a file.
// It is a distinct type so it cannot be mixed up with other uint32 IDs.
type BlockNumber uint32

// ErrPartialPage means a block exists but is shorter than PageSize,
// which indicates a torn write or a truncated file. It is not an io.EOF.
var ErrPartialPage = errors.New("partial page")

// ReadPage reads the page at block into a new slice.
// For reading many pages, ReadPageInto avoids an allocation per call.
//
// block is relative to r, which must be a single relation segment.
//
// Errors wrap io.EOF if block is past the end of the file, ErrPartialPage
// if the page is incomplete, or the error returned by r otherwise.
func ReadPage(r io.ReaderAt, block BlockNumber) ([]byte, error) {
	page := make([]byte, PageSize)
	if err := ReadPageInto(r, block, page); err != nil {
		return nil, err
	}
	return page, nil
}

// ReadPageInto is like ReadPage but reads into buf, which must be PageSize
// bytes long. On error the contents of buf are undefined.
func ReadPageInto(r io.ReaderAt, block BlockNumber, buf []byte) error {
	if len(buf) != PageSize {
		return fmt.Errorf("pgpage: buffer is %d bytes, want %d", len(buf), PageSize)
	}

	offset := int64(block) * PageSize

	// ReadAt may return io.EOF with a full page, so check the length too.
	n, err := r.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("pgpage: read block %d at offset %d: %w", block, offset, err)
	}

	if n != PageSize {
		if n == 0 && errors.Is(err, io.EOF) {
			return fmt.Errorf("pgpage: block %d at offset %d is past end of file: %w", block, offset, err)
		}
		return fmt.Errorf("pgpage: block %d at offset %d: read %d of %d bytes: %w", block, offset, n, PageSize, ErrPartialPage)
	}

	return nil
}
