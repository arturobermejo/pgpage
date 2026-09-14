package pgpage

import (
	"fmt"
	"io"
	"os"
)

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

// Relation is an open relation segment file.
//
// Size, PageCount and TrailingBytes describe the file as it was when it was
// opened; ReadPage always reads the file as it is now.
type Relation struct {
	file     *os.File
	path     string
	size     int64
	pages    BlockNumber
	trailing int64
}

var _ io.Closer = (*Relation)(nil)

// OpenRelation opens the relation segment file at path for reading.
// The caller must call Close when done.
func OpenRelation(path string) (*Relation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("pgpage: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("pgpage: %w", err)
	}

	if !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("pgpage: %s is not a regular file", path)
	}

	pages, trailing, err := PageCount(info.Size())
	if err != nil {
		f.Close()
		return nil, err
	}

	return &Relation{
		file:     f,
		path:     path,
		size:     info.Size(),
		pages:    pages,
		trailing: trailing,
	}, nil
}

// Close closes the underlying file.
func (r *Relation) Close() error {
	return r.file.Close()
}

// Path returns the path the relation was opened with.
func (r *Relation) Path() string { return r.path }

// Size returns the file size in bytes.
func (r *Relation) Size() int64 { return r.size }

// PageCount returns the number of complete pages in the file.
func (r *Relation) PageCount() BlockNumber { return r.pages }

// TrailingBytes returns the number of bytes after the last complete page.
func (r *Relation) TrailingBytes() int64 { return r.trailing }

// ReadPage reads the page at block into a new slice. See the ReadPage
// function for the errors it returns.
func (r *Relation) ReadPage(block BlockNumber) ([]byte, error) {
	return ReadPage(r.file, block)
}
