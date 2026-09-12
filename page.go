package pgpage

import (
	"fmt"
	"io"
)

const PageSize = 8192

func ReadPage(r io.ReaderAt, block uint32) ([]byte, error) {
	page := make([]byte, PageSize)

	offset := int64(block) * PageSize

	n, err := r.ReadAt(page, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}

	if n != PageSize {
		return nil, fmt.Errorf("invalid page size: got %d bytes", n)
	}

	return page, nil
}
