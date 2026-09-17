package tui

import (
	"strings"
	"testing"

	"github.com/arturobermejo/pgpage"
)

// rows returns the lines of a rendered navigator.
func rows(list string) []string {
	return strings.Split(list, "\n")
}

// The navigator lists the window of blocks that starts at top, marks the
// selected one and says how many are left below.
func TestPageList(t *testing.T) {
	tests := []struct {
		name                 string
		pages, selected, top pgpage.BlockNumber
		rows                 int
		want                 []string
	}{
		{
			name:  "all the pages fit",
			pages: 3, selected: 0, top: 0, rows: 10,
			want: []string{"# LP FREE STATUS", "> 0 …", "  1 …", "  2 …"},
		},
		{
			name:  "the window stops at the last block",
			pages: 3, selected: 2, top: 1, rows: 10,
			want: []string{"# LP FREE STATUS", "  1 …", "> 2 …"},
		},
		{
			name:  "a window in the middle counts what is left below",
			pages: 128, selected: 5, top: 4, rows: 3,
			want: []string{"# LP FREE STATUS", "4 …", "> 5 …", "6 …", "↓ 121 more"},
		},
		{
			name:  "numbers line up on the width of the largest block",
			pages: 128, selected: 127, top: 125, rows: 3,
			want: []string{"# LP FREE STATUS", "  125 …", "  126 …", "> 127 …"},
		},
		{
			name:  "an empty relation has no rows",
			pages: 0, rows: 10,
			want: []string{"no complete pages"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := pageList(tt.pages, tt.selected, tt.top, tt.rows, summaryCache{})

			got := rows(list)
			if len(got) != len(tt.want) {
				t.Fatalf("%d rows, want %d:\n%s", len(got), len(tt.want), list)
			}

			for i, want := range tt.want {
				if strings.Join(strings.Fields(got[i]), " ") != strings.Join(strings.Fields(want), " ") {
					t.Errorf("row %d = %q, want %q", i, got[i], want)
				}
			}
		})
	}
}

// The selected block is the only one with the marker, wherever it is.
func TestPageListMarksOnlyTheSelection(t *testing.T) {
	list := pageList(10, 7, 5, 5, summaryCache{})

	if n := strings.Count(list, ">"); n != 1 {
		t.Errorf("%d markers in the list, want 1:\n%s", n, list)
	}

	for _, row := range rows(list) {
		marked := strings.HasPrefix(strings.TrimSpace(row), ">")
		if marked != strings.Contains(row, "7") {
			t.Errorf("row %q: marked = %v", row, marked)
		}
	}
}

// The window follows the selection by the least it can, and never leaves
// empty rows at the bottom while blocks above could fill them.
func TestScrollTo(t *testing.T) {
	tests := []struct {
		name                 string
		pages, top, selected pgpage.BlockNumber
		rows                 int
		want                 pgpage.BlockNumber
	}{
		{name: "the selection is visible", pages: 100, top: 10, selected: 12, rows: 5, want: 10},
		{name: "first row of the window", pages: 100, top: 10, selected: 10, rows: 5, want: 10},
		{name: "last row of the window", pages: 100, top: 10, selected: 14, rows: 5, want: 10},
		{name: "one past the bottom scrolls one", pages: 100, top: 10, selected: 15, rows: 5, want: 11},
		{name: "one before the top scrolls one", pages: 100, top: 10, selected: 9, rows: 5, want: 9},
		{name: "a jump down lands at the bottom of the window", pages: 100, top: 0, selected: 40, rows: 5, want: 36},
		{name: "a jump up lands at the top of the window", pages: 100, top: 40, selected: 3, rows: 5, want: 3},
		{name: "the end of the relation fills the window", pages: 100, top: 97, selected: 99, rows: 5, want: 95},
		{name: "everything fits", pages: 3, top: 2, selected: 2, rows: 10, want: 0},
		{name: "rows below one is one row", pages: 100, top: 10, selected: 40, rows: 0, want: 40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scrollTo(tt.pages, tt.top, tt.selected, tt.rows); got != tt.want {
				t.Errorf("scrollTo(%d, %d, %d, %d) = %d, want %d",
					tt.pages, tt.top, tt.selected, tt.rows, got, tt.want)
			}
		})
	}
}

// Whatever the window, the selection is always drawn: that is the invariant
// the navigator depends on.
func TestScrollToKeepsTheSelectionVisible(t *testing.T) {
	const pages = 50

	for _, rows := range []int{1, 3, 7, 50, 80} {
		var top pgpage.BlockNumber

		// Walk the whole relation down and then up again, scrolling as the
		// model does, one step at a time.
		for _, block := range walk(pages) {
			top = scrollTo(pages, top, block, rows)

			if block < top || uint64(block) >= uint64(top)+uint64(rows) {
				t.Fatalf("rows %d: block %d is outside the window [%d, %d)", rows, block, top, int(top)+rows)
			}

			if last := lastVisible(pages, top, rows); uint64(last) >= pages {
				t.Fatalf("rows %d: window [%d, %d] runs past the relation", rows, top, last)
			}
		}
	}
}

// walk returns every block down and then back up.
func walk(pages pgpage.BlockNumber) []pgpage.BlockNumber {
	blocks := make([]pgpage.BlockNumber, 0, 2*pages)

	for block := range pages {
		blocks = append(blocks, block)
	}

	for block := pages; block > 0; block-- {
		blocks = append(blocks, block-1)
	}

	return blocks
}

// The column titles sit over the columns they name: each title ends where
// the numbers under it end, since numbers are right aligned.
func TestPageListColumnTitles(t *testing.T) {
	summaries := summaryCache{0: pgpage.SummarizePage(fixturePage(t, 0), 0)}
	lines := strings.Split(pageList(3, 0, 0, 3, summaries), "\n")

	title, row := lines[0], lines[1] // "> 0  185   21%  OK"

	for _, pair := range [][2]string{{"LP", "185"}, {"FREE", "21%"}} {
		if end := column(title, pair[0]) + len(pair[0]); end != column(row, pair[1])+len(pair[1]) {
			t.Errorf("%q ends at %d, but %q at %d:\n%s\n%s",
				pair[0], end, pair[1], column(row, pair[1])+len(pair[1]), title, row)
		}
	}

	if column(title, "STATUS") != column(row, "OK") {
		t.Errorf("STATUS starts at %d, OK at %d:\n%s\n%s", column(title, "STATUS"), column(row, "OK"), title, row)
	}
}
