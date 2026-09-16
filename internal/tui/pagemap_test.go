package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// regions returns the five regions of a header with the given boundaries.
func regions(lower, upper, special int) []pgpage.Region {
	return pgpage.PageRegions(pgpage.PageHeader{
		Lower:         uint16(lower),
		Upper:         uint16(upper),
		Special:       uint16(special),
		PageSize:      pgpage.PageSize,
		LayoutVersion: pgpage.PageLayoutVersion,
	})
}

// The bar is exactly as wide as asked, and no region with bytes is left out
// of it however small it is.
func TestMapCells(t *testing.T) {
	tests := []struct {
		name                  string
		lower, upper, special int
		width                 int
		want                  []int // header, line pointers, free, tuples, special
	}{
		{
			name: "the 24-byte header keeps a cell of its own",
			// A heap page: 740 B of line pointers, 1708 free, 5720 of tuples.
			lower: 764, upper: 2472, special: 8192, width: 64,
			want: []int{1, 6, 14, 43, 0},
		},
		{
			name:  "an empty page is almost all free space",
			lower: 24, upper: 8192, special: 8192, width: 10,
			want: []int{1, 0, 9, 0, 0},
		},
		{
			name:  "a btree page has a special area",
			lower: 100, upper: 4000, special: 8176, width: 20,
			want: []int{1, 1, 8, 9, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cells := mapCells(regions(tt.lower, tt.upper, tt.special), tt.width)

			if len(cells) != len(tt.want) {
				t.Fatalf("%d regions, want %d", len(cells), len(tt.want))
			}

			var total int

			for i, got := range cells {
				total += got

				if got != tt.want[i] {
					t.Errorf("region %d has %d cells, want %d (all: %v)", i, got, tt.want[i], cells)
				}
			}

			if total != tt.width {
				t.Errorf("the bar is %d cells wide, want %d", total, tt.width)
			}
		})
	}
}

// Whatever the page and whatever the terminal, the bar fills the width and
// shows every region that has bytes.
func TestMapCellsInvariants(t *testing.T) {
	pages := [][3]int{
		{764, 2472, 8192},  // the fixture
		{24, 8192, 8192},   // empty heap page
		{100, 4000, 8176},  // btree page with a special area
		{8000, 8100, 8192}, // almost no free space
		{28, 8188, 8192},   // one line pointer, four bytes of tuples
	}

	for _, page := range pages {
		rs := regions(page[0], page[1], page[2])

		for width := 1; width <= 200; width++ {
			cells := mapCells(rs, width)

			var (
				total    int
				nonEmpty int
			)

			for i, region := range rs {
				if region.Len() > 0 {
					nonEmpty++
				}

				if cells == nil {
					continue
				}

				total += cells[i]

				if region.Len() > 0 && cells[i] < 1 {
					t.Fatalf("page %v, width %d: region %v got no cell", page, width, region.Kind)
				}

				if region.Len() == 0 && cells[i] != 0 {
					t.Fatalf("page %v, width %d: empty region %v got %d cells", page, width, region.Kind, cells[i])
				}
			}

			switch {
			case cells == nil && width >= nonEmpty:
				t.Fatalf("page %v: no map at width %d, which fits %d regions", page, width, nonEmpty)
			case cells != nil && total != width:
				t.Fatalf("page %v, width %d: the bar is %d cells wide", page, width, total)
			}
		}
	}
}

// The drawn bar takes as many cells on screen as it was given.
func TestPageMapBarWidth(t *testing.T) {
	summary := pgpage.SummarizePage(fixturePage(t, 0))

	for _, width := range []int{28, 40, 64, 120} {
		lines := strings.Split(pageMap(0, summary, true, width), "\n")

		if len(lines) < 3 {
			t.Fatalf("width %d: the map has %d lines:\n%s", width, len(lines), strings.Join(lines, "\n"))
		}

		if got := lipgloss.Width(lines[2]); got != width {
			t.Errorf("width %d: the bar is %d cells wide:\n%s", width, got, lines[2])
		}

		if got := lipgloss.Width(lines[1]); got != width {
			t.Errorf("width %d: the scale is %d cells wide:\n%s", width, got, lines[1])
		}
	}
}

// The legend names every region that has bytes, with its range, and leaves
// out the ones that have none.
func TestPageMapLegend(t *testing.T) {
	m := pageMap(0, pgpage.SummarizePage(fixturePage(t, 0)), true, 64)

	for _, want := range []string{
		"PAGE 0 — 8192 BYTES",
		"Header 0-23 · 24 B · 1 cell",
		"Line pointers 24-763 · 740 B",
		"Free space 764-2471 · 1708 B",
		"Tuples 2472-8191 · 5720 B",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("the map does not contain %q:\n%s", want, m)
		}
	}

	// A heap page has no special area, so it has no legend line either.
	if strings.Contains(m, "Special") {
		t.Errorf("the map names the empty special area:\n%s", m)
	}
}

// Each region is drawn with its own glyph, so the map survives a terminal
// without color.
func TestPageMapGlyphs(t *testing.T) {
	seen := map[string]bool{}

	for kind, glyph := range regionGlyph {
		if seen[glyph] {
			t.Errorf("region %v repeats the glyph %q of another region", kind, glyph)
		}

		seen[glyph] = true

		if got := lipgloss.Width(glyph); got != 1 {
			t.Errorf("the glyph %q of %v is %d cells wide, want 1", glyph, kind, got)
		}
	}
}

// Pages without a layout say so instead of drawing an empty bar.
func TestPageMapNoLayout(t *testing.T) {
	tests := []struct {
		name    string
		summary pgpage.PageSummary
		cached  bool
		width   int
		want    string
	}{
		{name: "not read yet", width: 64, want: "reading…"},
		{
			name:    "new page",
			summary: pgpage.SummarizePage(make([]byte, pgpage.PageSize)),
			cached:  true, width: 64,
			want: "no layout: NEW",
		},
		{
			name:    "invalid page",
			summary: pgpage.PageSummary{Status: pgpage.StatusInvalid, Err: errors.New("boom")},
			cached:  true, width: 64,
			want: "no layout: INVALID",
		},
		{
			name:    "too narrow",
			summary: pgpage.SummarizePage(fixturePage(t, 0)),
			cached:  true, width: 2,
			want: "too narrow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pageMap(0, tt.summary, tt.cached, tt.width)

			if !strings.Contains(got, tt.want) {
				t.Errorf("map = %q, want it to contain %q", got, tt.want)
			}

			for _, glyph := range regionGlyph {
				if strings.Contains(got, glyph) {
					t.Errorf("a page with no map drew %q:\n%s", glyph, got)
				}
			}
		})
	}
}
