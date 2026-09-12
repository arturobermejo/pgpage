package pgpage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
)

// fakeFile returns an in-memory segment where every byte of page i is i+1.
func fakeFile(pages int) []byte {
	buf := make([]byte, pages*PageSize)

	for i := range pages {
		page := buf[i*PageSize : (i+1)*PageSize]
		for j := range page {
			page[j] = byte(i + 1)
		}
	}

	return buf
}

// wrappingReader returns a wrapped io.EOF instead of a bare one.
type wrappingReader struct{ data []byte }

func (w wrappingReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(w.data)) {
		return 0, fmt.Errorf("segment read at %d: %w", off, io.EOF)
	}

	n := copy(p, w.data[off:])

	if n < len(p) {
		return n, fmt.Errorf("segment read at %d: %w", off, io.EOF)
	}

	return n, nil
}

type failingReader struct{ err error }

func (f failingReader) ReadAt(p []byte, off int64) (int, error) { return 0, f.err }

func TestReadPage(t *testing.T) {
	data := fakeFile(3)

	tests := []struct {
		name  string
		block BlockNumber
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

// A full last page returned together with io.EOF is still a success.
func TestReadPageLastPageWithEOF(t *testing.T) {
	data := fakeFile(1)

	for _, r := range []io.ReaderAt{bytes.NewReader(data), wrappingReader{data: data}} {
		page, err := ReadPage(r, 0)
		if err != nil {
			t.Fatalf("%T: ReadPage(0) returned error: %v", r, err)
		}

		if len(page) != PageSize {
			t.Fatalf("%T: len(page) = %d, want %d", r, len(page), PageSize)
		}
	}
}

// Past end of file must be io.EOF, whether or not the reader wraps its EOF.
func TestReadPagePastEndIsEOF(t *testing.T) {
	data := fakeFile(2)

	readers := map[string]io.ReaderAt{
		"bare EOF":    bytes.NewReader(data),
		"wrapped EOF": wrappingReader{data: data},
	}

	for name, r := range readers {
		t.Run(name, func(t *testing.T) {
			page, err := ReadPage(r, 2)
			if page != nil {
				t.Errorf("page = %v, want nil", page)
			}

			if !errors.Is(err, io.EOF) {
				t.Fatalf("errors.Is(err, io.EOF) = false, want true (err = %v)", err)
			}

			if errors.Is(err, ErrPartialPage) {
				t.Errorf("past end of file must not report ErrPartialPage (err = %v)", err)
			}
		})
	}
}

// An incomplete page must be ErrPartialPage, not io.EOF.
func TestReadPagePartialPage(t *testing.T) {
	// One full page plus half of a second one.
	data := fakeFile(2)[:PageSize+PageSize/2]

	readers := map[string]io.ReaderAt{
		"bare EOF":    bytes.NewReader(data),
		"wrapped EOF": wrappingReader{data: data},
	}

	for name, r := range readers {
		t.Run(name, func(t *testing.T) {
			page, err := ReadPage(r, 1)
			if page != nil {
				t.Errorf("page = %v, want nil", page)
			}

			if !errors.Is(err, ErrPartialPage) {
				t.Fatalf("errors.Is(err, ErrPartialPage) = false, want true (err = %v)", err)
			}

			if errors.Is(err, io.EOF) {
				t.Errorf("a partial page must not report io.EOF (err = %v)", err)
			}
		})
	}
}

func TestReadPageReaderError(t *testing.T) {
	want := errors.New("input/output error")

	page, err := ReadPage(failingReader{err: want}, 7)
	if page != nil {
		t.Errorf("page = %v, want nil", page)
	}

	if !errors.Is(err, want) {
		t.Fatalf("errors.Is(err, want) = false, want true (err = %v)", err)
	}

	if errors.Is(err, io.EOF) || errors.Is(err, ErrPartialPage) {
		t.Errorf("an I/O failure must not be classified as EOF or partial page (err = %v)", err)
	}
}

// Errors should name the block that failed.
func TestReadPageErrorMentionsBlock(t *testing.T) {
	_, err := ReadPage(bytes.NewReader(fakeFile(1)), 4)
	if err == nil {
		t.Fatal("expected an error")
	}

	if got := err.Error(); !bytes.Contains([]byte(got), []byte("block 4")) {
		t.Errorf("error %q does not mention the failing block", got)
	}
}

func TestReadPageIntoReusesBuffer(t *testing.T) {
	data := fakeFile(3)
	r := bytes.NewReader(data)
	buf := make([]byte, PageSize)

	for block := range BlockNumber(3) {
		if err := ReadPageInto(r, block, buf); err != nil {
			t.Fatalf("ReadPageInto(%d) returned error: %v", block, err)
		}

		want := byte(block + 1)
		for i, b := range buf {
			if b != want {
				t.Fatalf("block %d: buf[%d] = %#x, want %#x", block, i, b, want)
			}
		}
	}
}

func TestReadPageIntoBufferSize(t *testing.T) {
	r := bytes.NewReader(fakeFile(1))

	for _, size := range []int{0, PageSize - 1, PageSize + 1} {
		err := ReadPageInto(r, 0, make([]byte, size))
		if err == nil {
			t.Fatalf("buffer of %d bytes: expected error, got nil", size)
		}

		if errors.Is(err, io.EOF) || errors.Is(err, ErrPartialPage) {
			t.Errorf("buffer of %d bytes: must not be classified as EOF or partial page (err = %v)", size, err)
		}
	}
}

// ReadPageInto must not allocate.
func TestReadPageIntoDoesNotAllocate(t *testing.T) {
	r := bytes.NewReader(fakeFile(1))
	buf := make([]byte, PageSize)

	allocs := testing.AllocsPerRun(100, func() {
		if err := ReadPageInto(r, 0, buf); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Errorf("ReadPageInto allocated %v times per call, want 0", allocs)
	}
}

func BenchmarkReadPage(b *testing.B) {
	r := bytes.NewReader(fakeFile(1))

	b.ReportAllocs()

	for b.Loop() {
		if _, err := ReadPage(r, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadPageInto(b *testing.B) {
	r := bytes.NewReader(fakeFile(1))
	buf := make([]byte, PageSize)

	b.ReportAllocs()

	for b.Loop() {
		if err := ReadPageInto(r, 0, buf); err != nil {
			b.Fatal(err)
		}
	}
}
