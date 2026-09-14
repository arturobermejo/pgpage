package pgpage

import "testing"

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
