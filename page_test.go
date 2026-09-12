package pgpage

import (
	"bytes"
	"errors"
	"testing"
)

// fakeFile builds an in-memory "relation file" with the given number of
// pages, where every byte of page i is set to byte(i+1) so pages are
// distinguishable from each other.
func fakeFile(pages int) []byte {
	buf := make([]byte, pages*PageSize)

	for i := range pages {
		start := i * PageSize
		for j := start; j < start+PageSize; j++ {
			buf[j] = byte(i + 1)
		}
	}

	return buf
}

func TestReadPage(t *testing.T) {
	data := fakeFile(3)

	tests := []struct {
		name  string
		block uint32
		want  byte
	}{
		{name: "first page", block: 0, want: 1},
		{name: "middle page", block: 1, want: 2},
		{name: "last page", block: 2, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := ReadPage(bytes.NewReader(data), tt.block)

			if err != nil {
				t.Fatalf("ReadPage(%d) returned error: %v", tt.block, err)
			}

			if len(page) != PageSize {
				t.Fatalf("len(page) = %d, want %d", len(page), PageSize)
			}

			for i, b := range page {
				if b != tt.want {
					t.Fatalf("page[%d] = %#x, want %#x", i, b, tt.want)
				}
			}
		})
	}
}

func TestReadPageOutOfRange(t *testing.T) {
	data := fakeFile(2)

	_, err := ReadPage(bytes.NewReader(data), 2)
	if err == nil {
		t.Fatal("ReadPage past end of file: expected error, got nil")
	}
}

func TestReadPageTruncated(t *testing.T) {
	// One full page plus half of a second one.
	data := fakeFile(2)[:PageSize+PageSize/2]

	_, err := ReadPage(bytes.NewReader(data), 1)
	if err == nil {
		t.Fatal("ReadPage on truncated page: expected error, got nil")
	}
}

type failingReader struct{ err error }

func (f failingReader) ReadAt(p []byte, off int64) (int, error) { return 0, f.err }

func TestReadPageReaderError(t *testing.T) {
	want := errors.New("disk on fire")

	_, err := ReadPage(failingReader{err: want}, 0)
	if !errors.Is(err, want) {
		t.Fatalf("ReadPage returned %v, want %v", err, want)
	}
}
