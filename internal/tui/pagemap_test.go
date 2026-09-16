package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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

// mapLines returns the rows of the grid, without the scale and the legend.
func mapLines(t *testing.T, m string) []string {
	t.Helper()

	lines := strings.Split(m, "\n")
	if len(lines) < mapRows+1 {
		t.Fatalf("the map has %d lines, want at least %d:\n%s", len(lines), mapRows+1, m)
	}

	return lines[1 : mapRows+1]
}

// The grid has one row per 512 bytes, labelled with the offset that row
// starts at, and every row is as wide as the map.
func TestPageMapGrid(t *testing.T) {
	const width = 70

	m := pageMap(pgpage.SummarizePage(fixturePage(t, 0)), true, width)

	for i, line := range mapLines(t, m) {
		label := fmt.Sprintf("0x%04X", i*bytesPerRow)

		if !strings.HasPrefix(line, label) {
			t.Errorf("row %d starts with %q, want the offset %q", i, line[:min(len(line), 8)], label)
		}

		if got := lipgloss.Width(line); got != width {
			t.Errorf("row %d is %d cells wide, want %d", i, got, width)
		}
	}

	if got := lipgloss.Width(strings.Split(m, "\n")[0]); got > width {
		t.Errorf("the scale is %d cells wide, want at most %d", got, width)
	}
}

// Every region of the page is drawn, including the 24-byte header: a grid
// cell covers a handful of bytes, so nothing is rounded away.
func TestPageMapShowsEveryRegion(t *testing.T) {
	m := pageMap(pgpage.SummarizePage(fixturePage(t, 0)), true, 70)

	grid := strings.Join(mapLines(t, m), "\n")

	for _, kind := range []pgpage.RegionKind{
		pgpage.RegionHeader,
		pgpage.RegionLinePointers,
		pgpage.RegionFree,
		pgpage.RegionTuples,
	} {
		if !strings.Contains(grid, regionGlyph[kind]) {
			t.Errorf("the grid has no cell of %v:\n%s", kind, grid)
		}
	}

	// The page has no special area, so its glyph must not appear.
	if strings.Contains(grid, regionGlyph[pgpage.RegionSpecial]) {
		t.Errorf("the grid draws a special area the page does not have:\n%s", grid)
	}
}

// A cell that straddles a boundary belongs to the region that fills most of
// it, so no cell is claimed twice.
func TestCellRegion(t *testing.T) {
	rs := regions(764, 2472, 8192)

	tests := []struct {
		name       string
		start, end int
		want       pgpage.RegionKind
	}{
		{name: "inside the header", start: 0, end: 8, want: pgpage.RegionHeader},
		{name: "inside the line pointers", start: 100, end: 108, want: pgpage.RegionLinePointers},
		{name: "inside the free space", start: 1000, end: 1008, want: pgpage.RegionFree},
		{name: "inside the tuples", start: 8000, end: 8008, want: pgpage.RegionTuples},
		{name: "mostly header", start: 16, end: 32, want: pgpage.RegionHeader},
		{name: "mostly line pointers", start: 20, end: 40, want: pgpage.RegionLinePointers},
		{name: "on the free space boundary", start: 760, end: 768, want: pgpage.RegionLinePointers},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cellRegion(rs, tt.start, tt.end); got != tt.want {
				t.Errorf("cellRegion(%d, %d) = %v, want %v", tt.start, tt.end, got, tt.want)
			}
		})
	}
}

// The legend names every region that has bytes, with its range, and leaves
// out the ones that have none.
func TestPageMapLegend(t *testing.T) {
	m := pageMap(pgpage.SummarizePage(fixturePage(t, 0)), true, 70)

	for _, want := range []string{
		"Header 0-23 · 24 B",
		"Line ptrs 24-763 · 740 B",
		"Free 764-2471 · 1708 B",
		"Tuples 2472-8191 · 5720 B",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("the map does not contain %q:\n%s", want, m)
		}
	}

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

// Pages without a layout say so instead of drawing an empty grid.
func TestPageMapNoLayout(t *testing.T) {
	tests := []struct {
		name    string
		summary pgpage.PageSummary
		cached  bool
		width   int
		want    string
	}{
		{name: "not read yet", width: 70, want: "reading…"},
		{
			name:    "new page",
			summary: pgpage.SummarizePage(make([]byte, pgpage.PageSize)),
			cached:  true, width: 70,
			want: "no layout: NEW",
		},
		{
			name:    "invalid page",
			summary: pgpage.PageSummary{Status: pgpage.StatusInvalid, Err: errors.New("boom")},
			cached:  true, width: 70,
			want: "no layout: INVALID",
		},
		{
			name:    "too narrow",
			summary: pgpage.SummarizePage(fixturePage(t, 0)),
			cached:  true, width: 10,
			want: "too narrow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pageMap(tt.summary, tt.cached, tt.width)

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

// The legend is laid out in columns that line up, and fits the width it was
// given.
func TestPageMapLegendColumns(t *testing.T) {
	summary := pgpage.SummarizePage(fixturePage(t, 0))

	for _, width := range []int{40, 70, 100, 140} {
		m := pageMap(summary, true, width)

		lines := strings.Split(m, "\n")[mapRows+2:]

		var starts []int

		for _, line := range lines {
			if got := lipgloss.Width(line); got > width {
				t.Errorf("width %d: a legend line is %d cells wide:\n%q", width, got, line)
			}

			// Every entry starts with a glyph, so the columns show up as the
			// offsets at which the glyphs sit.
			for i, r := range []rune(line) {
				if strings.ContainsRune("█╱╎╲╳", r) {
					starts = append(starts, i)
				}
			}
		}

		if len(starts) < 4 {
			t.Errorf("width %d: %d legend entries, want the 4 regions of the page:\n%s", width, len(starts), m)
		}
	}
}

// withColor switches Lip Gloss to 256 colors until the test ends. TestMain
// turns color off for the package, so the tests that are about color have
// to ask for it.
func withColor(t *testing.T) {
	t.Helper()

	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
}

// With color, every cell is the corner glyph over the background of its
// region, and the rows still take exactly the width of the map.
func TestPageMapColor(t *testing.T) {
	withColor(t)

	const width = 70

	rows := mapLines(t, pageMap(pgpage.SummarizePage(fixturePage(t, 0)), true, width))

	for i, row := range rows {
		if got := lipgloss.Width(row); got != width {
			t.Errorf("row %d is %d cells wide, want %d", i, got, width)
		}

		if !strings.Contains(row, cellGlyph) {
			t.Errorf("row %d has no cell glyph:\n%q", i, row)
		}

		for kind, g := range regionGlyph {
			if strings.Contains(row, g) {
				t.Errorf("row %d draws the monochrome glyph of %v with color on:\n%q", i, kind, row)
			}
		}
	}

	// ANSI 256 backgrounds are written "48;5;N". The first row holds the
	// header and the line pointers, the last one only tuples.
	backgrounds := map[int][]string{
		0:           {"48;5;138", "48;5;109"},
		mapRows - 1: {"48;5;144"},
	}

	for i, want := range backgrounds {
		for _, bg := range want {
			if !strings.Contains(rows[i], bg) {
				t.Errorf("row %d has no background %q:\n%q", i, bg, rows[i])
			}
		}
	}
}

// Runs of cells of one region are styled once, so a row does not carry one
// escape sequence per cell.
func TestPageMapColorRuns(t *testing.T) {
	withColor(t)

	rows := mapLines(t, pageMap(pgpage.SummarizePage(fixturePage(t, 0)), true, 70))

	// The last row is all tuples: a single run, a single background.
	if n := strings.Count(rows[mapRows-1], "48;5;"); n != 1 {
		t.Errorf("a row of one region sets the background %d times, want 1:\n%q", n, rows[mapRows-1])
	}
}

// The legend shows the color of each region with a single plain cell: the
// grid lines of the map are not repeated there.
func TestPageMapLegendSwatch(t *testing.T) {
	withColor(t)

	m := pageMap(pgpage.SummarizePage(fixturePage(t, 0)), true, 140)

	legend := strings.Join(strings.Split(m, "\n")[mapRows+2:], "\n")

	if strings.Contains(legend, cellGlyph) {
		t.Errorf("the legend draws the grid glyph:\n%q", legend)
	}

	for _, bg := range []string{"48;5;138", "48;5;109", "48;5;236", "48;5;144"} {
		if !strings.Contains(legend, bg) {
			t.Errorf("the legend has no swatch with background %q:\n%q", bg, legend)
		}
	}
}

// The map is drawn only with Box Drawing and Block Elements, the two blocks
// every monospaced font has had since Unicode 1.1. A character from a newer
// block shows as a question mark on fonts without it, and its replacement
// can be wider than a cell and push the rows out of the panel.
func TestPageMapGlyphsAreWidelySupported(t *testing.T) {
	const (
		first = 0x2500 // Box Drawing starts here
		last  = 0x259F // Block Elements ends here
	)

	glyphs := map[string]string{"cell": cellGlyph}
	for kind, g := range regionGlyph {
		glyphs[kind.String()] = g
	}

	for name, g := range glyphs {
		for _, r := range g {
			if r < first || r > last {
				t.Errorf("%s glyph %q is U+%04X, outside Box Drawing and Block Elements", name, g, r)
			}
		}
	}
}

// The line that keeps rows apart is the terminal's underline, so every cell
// of the map is underlined, in the darker shade of its region.
func TestPageMapCellsAreUnderlined(t *testing.T) {
	withColor(t)

	rows := mapLines(t, pageMap(pgpage.SummarizePage(fixturePage(t, 0)), true, 70))

	// The last row is all tuples: sand gray background, darker sand lines,
	// and 4, the SGR code for underline.
	const want = "38;5;101;48;5;144;4m"

	if !strings.Contains(rows[mapRows-1], want) {
		t.Errorf("the tuples row is not drawn as %q:\n%q", want, rows[mapRows-1])
	}
}
