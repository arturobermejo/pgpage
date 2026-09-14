package pgpage

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestPageCount(t *testing.T) {
	maxPages := int64(MaxBlockNumber) + 1

	tests := []struct {
		name     string
		size     int64
		pages    BlockNumber
		trailing int64
	}{
		{name: "empty file", size: 0, pages: 0, trailing: 0},
		{name: "less than a page", size: PageSize - 1, pages: 0, trailing: PageSize - 1},
		{name: "one page", size: PageSize, pages: 1, trailing: 0},
		{name: "one page and one byte", size: PageSize + 1, pages: 1, trailing: 1},
		{name: "three and a half pages", size: 3*PageSize + PageSize/2, pages: 3, trailing: PageSize / 2},
		{name: "1 GB segment", size: 1 << 30, pages: 131072, trailing: 0},
		{name: "maximum pages", size: maxPages * PageSize, pages: MaxBlockNumber + 1, trailing: 0},
		{name: "maximum pages with trailing bytes", size: maxPages*PageSize + 10, pages: MaxBlockNumber + 1, trailing: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pages, trailing, err := PageCount(tt.size)
			if err != nil {
				t.Fatalf("PageCount(%d) returned error: %v", tt.size, err)
			}

			if pages != tt.pages || trailing != tt.trailing {
				t.Errorf("PageCount(%d) = %d pages, %d trailing; want %d pages, %d trailing",
					tt.size, pages, trailing, tt.pages, tt.trailing)
			}
		})
	}
}

func TestPageCountInvalid(t *testing.T) {
	maxPages := int64(MaxBlockNumber) + 1

	tests := []struct {
		name string
		size int64
	}{
		{name: "negative", size: -1},
		{name: "one page too many", size: (maxPages + 1) * PageSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pages, trailing, err := PageCount(tt.size)
			if err == nil {
				t.Fatalf("PageCount(%d) = %d, %d; want an error", tt.size, pages, trailing)
			}

			if pages != 0 || trailing != 0 {
				t.Errorf("PageCount(%d) = %d, %d with error; want 0, 0", tt.size, pages, trailing)
			}
		})
	}
}

// openRelation opens path and closes it when the test ends.
func openRelation(t *testing.T, path string) *Relation {
	t.Helper()

	rel, err := OpenRelation(path)
	if err != nil {
		t.Fatalf("OpenRelation(%q) returned error: %v", path, err)
	}

	t.Cleanup(func() { rel.Close() })

	return rel
}

func TestOpenRelation(t *testing.T) {
	rel := openRelation(t, fixtureHeap)

	if got := rel.Path(); got != fixtureHeap {
		t.Errorf("Path() = %q, want %q", got, fixtureHeap)
	}

	if got, want := rel.Size(), int64(3*PageSize); got != want {
		t.Errorf("Size() = %d, want %d", got, want)
	}

	if got := rel.PageCount(); got != 3 {
		t.Errorf("PageCount() = %d, want 3", got)
	}

	if got := rel.TrailingBytes(); got != 0 {
		t.Errorf("TrailingBytes() = %d, want 0", got)
	}
}

// Pages read through a Relation must be the bytes of the file.
func TestRelationReadPage(t *testing.T) {
	data, err := os.ReadFile(fixtureHeap)
	if err != nil {
		t.Fatal(err)
	}

	rel := openRelation(t, fixtureHeap)

	for block := range rel.PageCount() {
		page, err := rel.ReadPage(block)
		if err != nil {
			t.Fatalf("ReadPage(%d) returned error: %v", block, err)
		}

		start := int64(block) * PageSize
		if !bytes.Equal(page, data[start:start+PageSize]) {
			t.Errorf("ReadPage(%d) does not match bytes %d-%d of the file", block, start, start+PageSize-1)
		}
	}

	if _, err := rel.ReadPage(rel.PageCount()); !errors.Is(err, io.EOF) {
		t.Errorf("ReadPage past the last page: errors.Is(err, io.EOF) = false, want true (err = %v)", err)
	}
}

func TestOpenRelationPartialPage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial")
	if err := os.WriteFile(path, make([]byte, 2*PageSize+PageSize/2), 0o600); err != nil {
		t.Fatal(err)
	}

	rel := openRelation(t, path)

	if got := rel.PageCount(); got != 2 {
		t.Errorf("PageCount() = %d, want 2", got)
	}

	if got, want := rel.TrailingBytes(), int64(PageSize/2); got != want {
		t.Errorf("TrailingBytes() = %d, want %d", got, want)
	}

	if _, err := rel.ReadPage(2); !errors.Is(err, ErrPartialPage) {
		t.Errorf("ReadPage(2): errors.Is(err, ErrPartialPage) = false, want true (err = %v)", err)
	}
}

func TestOpenRelationNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")

	rel, err := OpenRelation(path)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("errors.Is(err, fs.ErrNotExist) = false, want true (err = %v)", err)
	}

	if rel != nil {
		t.Errorf("OpenRelation returned %+v with error, want nil", rel)
	}
}

func TestOpenRelationDirectory(t *testing.T) {
	rel, err := OpenRelation(t.TempDir())
	if err == nil {
		rel.Close()
		t.Fatal("OpenRelation on a directory: expected error, got nil")
	}

	if rel != nil {
		t.Errorf("OpenRelation returned %+v with error, want nil", rel)
	}
}

func TestRelationReadPageAfterClose(t *testing.T) {
	rel, err := OpenRelation(fixtureHeap)
	if err != nil {
		t.Fatal(err)
	}

	if err := rel.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	if _, err := rel.ReadPage(0); !errors.Is(err, os.ErrClosed) {
		t.Errorf("errors.Is(err, os.ErrClosed) = false, want true (err = %v)", err)
	}
}
