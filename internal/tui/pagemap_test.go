package tui

import (
	"errors"
	"fmt"
	"slices"
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
	const width = offsetLabel + 64

	m := pageMap(pgpage.SummarizePage(fixturePage(t, 0), 0), true, width)

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
	m := pageMap(pgpage.SummarizePage(fixturePage(t, 0), 0), true, 70)

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
	m := pageMap(pgpage.SummarizePage(fixturePage(t, 0), 0), true, 70)

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
			summary: pgpage.SummarizePage(make([]byte, pgpage.PageSize), 0),
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
			summary: pgpage.SummarizePage(fixturePage(t, 0), 0),
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
	summary := pgpage.SummarizePage(fixturePage(t, 0), 0)

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

	const width = offsetLabel + 64

	rows := mapLines(t, pageMap(pgpage.SummarizePage(fixturePage(t, 0), 0), true, width))

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

	rows := mapLines(t, pageMap(pgpage.SummarizePage(fixturePage(t, 0), 0), true, 70))

	// The last row is all tuples: a single run, a single background.
	if n := strings.Count(rows[mapRows-1], "48;5;"); n != 1 {
		t.Errorf("a row of one region sets the background %d times, want 1:\n%q", n, rows[mapRows-1])
	}
}

// The legend shows the color of each region with a single plain cell: the
// grid lines of the map are not repeated there.
func TestPageMapLegendSwatch(t *testing.T) {
	withColor(t)

	m := pageMap(pgpage.SummarizePage(fixturePage(t, 0), 0), true, 140)

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

	rows := mapLines(t, pageMap(pgpage.SummarizePage(fixturePage(t, 0), 0), true, 70))

	// The last row is all tuples: sand gray background, darker sand lines,
	// and 4, the SGR code for underline.
	const want = "38;5;101;48;5;144;4m"

	if !strings.Contains(rows[mapRows-1], want) {
		t.Errorf("the tuples row is not drawn as %q:\n%q", want, rows[mapRows-1])
	}
}

// A highlight paints the cells its bytes fall in, and nothing else, and the
// legend names it.
func TestPageMapHighlight(t *testing.T) {
	summary := pgpage.SummarizePage(fixturePage(t, 0), 0)
	hl := highlight{start: 8152, end: 8187, label: "item #1"}

	m := pageMapWith(summary, true, 71, hl) // 64 cells: 8 bytes each
	rows := mapLines(t, m)

	for i, row := range rows {
		has := strings.Contains(row, highlightGlyph)

		// 8152 falls in the last row, 7680-8191.
		if want := i == mapRows-1; has != want {
			t.Errorf("row %d highlighted = %v, want %v:\n%s", i, has, want, row)
		}
	}

	// 35 bytes starting mid-cell touch 5 cells of 8 bytes.
	if n := strings.Count(rows[mapRows-1], highlightGlyph); n != 5 {
		t.Errorf("%d highlighted cells, want 5:\n%s", n, rows[mapRows-1])
	}

	if !strings.Contains(m, "item #1 bytes 8152-8186 · 35 B") {
		t.Errorf("the legend does not name the highlight:\n%s", m)
	}

	if plain := pageMap(summary, true, 71); strings.Contains(plain, highlightGlyph) {
		t.Errorf("the map highlights bytes nobody asked for:\n%s", plain)
	}
}

// The grid is drawn at a few fixed widths only, each a whole number of bytes
// per cell, so resizing the terminal does not change the scale one column at
// a time: across a hundred widths there are only as many scales as choices.
func TestPageMapScaleIsStable(t *testing.T) {
	summary := pgpage.SummarizePage(fixturePage(t, 0), 0)
	scales := map[int]bool{}

	for width := offsetLabel + minMapCells; width <= 200; width++ {
		cells := mapCells(width)

		if cells == 0 || bytesPerRow%cells != 0 {
			t.Fatalf("width %d: %d cells, which do not divide a row of %d bytes", width, cells, bytesPerRow)
		}

		// The widest choice that fits: the next wider one would not.
		if i := slices.Index(mapCellChoices, cells); i > 0 && offsetLabel+mapCellChoices[i-1] <= width {
			t.Errorf("width %d draws %d cells, but %d fit", width, cells, mapCellChoices[i-1])
		}

		m := pageMap(summary, true, width)

		for _, row := range mapLines(t, m) {
			if got := lipgloss.Width(row); got != offsetLabel+cells {
				t.Fatalf("width %d: a row is %d cells, want %d", width, got, offsetLabel+cells)
			}
		}

		if want := fmt.Sprintf("1 cell = %d B", bytesPerRow/cells); !strings.Contains(m, want) {
			t.Errorf("width %d: the map does not say %q", width, want)
		}

		scales[cells] = true
	}

	if len(scales) != len(mapCellChoices) {
		t.Errorf("%d scales between widths %d and 200, want %d", len(scales), offsetLabel+minMapCells, len(mapCellChoices))
	}

	if got := mapCells(offsetLabel + minMapCells - 1); got != 0 {
		t.Errorf("a map narrower than the narrowest grid draws %d cells", got)
	}
}

// The labels of the ruler above the grid never run into each other: on the
// narrowest grid, where +128 fills the space up to +256, the second is left
// out.
func TestColumnScaleLabelsDoNotTouch(t *testing.T) {
	for _, cells := range mapCellChoices {
		scale := columnScale(cells)

		if strings.Contains(scale, "8+") || strings.Contains(scale, "6+") {
			t.Errorf("%d cells: labels run together: %q", cells, scale)
		}

		if !strings.Contains(scale, "+0") || !strings.Contains(scale, "+128") {
			t.Errorf("%d cells: the ruler lost its first labels: %q", cells, scale)
		}
	}
}

// A cell on a boundary is split in eighths, so a region that does not fill a
// whole number of cells still shows its size: with 16 bytes a cell, the
// 24-byte header is one cell and a half, not two.
func TestSplitCell(t *testing.T) {
	withColor(t)

	rs := regions(764, 2472, 8192)
	hl := highlight{start: 8152, end: 8189}

	header, pointers := paint{kind: pgpage.RegionHeader}, paint{kind: pgpage.RegionLinePointers}

	tests := []struct {
		name       string
		start, end int
		want       cell
	}{
		{name: "inside the header", start: 0, end: 16, want: cell{header, header, eighths}},
		{name: "header ends mid cell", start: 16, end: 32, want: cell{header, pointers, 4}},
		{name: "a quarter of header", start: 16, end: 48, want: cell{header, pointers, 2}},
		{name: "most of the cell header", start: 0, end: 32, want: cell{header, pointers, 6}},
		{
			name: "free space into tuples", start: 2464, end: 2480,
			want: cell{paint{kind: pgpage.RegionFree}, paint{kind: pgpage.RegionTuples}, 4},
		},
		{
			name: "the highlight starts", start: 8144, end: 8160,
			want: cell{paint{kind: pgpage.RegionTuples}, paint{kind: pgpage.RegionTuples, highlighted: true}, 4},
		},
		{
			// 13 of 16 bytes are 6.5 eighths, rounded up.
			name: "rounded to the nearest eighth", start: 8176, end: 8192,
			want: cell{paint{kind: pgpage.RegionTuples, highlighted: true}, paint{kind: pgpage.RegionTuples}, 7},
		},
		{
			name: "the highlight ends", start: 8160, end: 8192,
			want: cell{paint{kind: pgpage.RegionTuples, highlighted: true}, paint{kind: pgpage.RegionTuples}, 7},
		},
	}

	for _, tt := range tests {
		if got := splitCell(rs, hl, tt.start, tt.end); got != tt.want {
			t.Errorf("%s: splitCell(%d, %d) = %+v, want %+v", tt.name, tt.start, tt.end, got, tt.want)
		}
	}

	// A single byte is a quarter of an eighth of a 32-byte cell, which rounds
	// to nothing; it is drawn as one eighth, so that it does not vanish.
	tiny := highlight{start: 1000, end: 1001}
	if got := splitCell(rs, tiny, 1000, 1032); got.eighths != 1 {
		t.Errorf("one highlighted byte fills %d eighths of its cell, want 1", got.eighths)
	}
}

// With color, the map adds up: counting whole cells and the eighths of split
// ones gives back the size of every region, exactly, at every scale. The
// boundaries of a heap page fall on multiples of 4 bytes, and an eighth of a
// cell is at most 4 bytes.
func TestPageMapAddsUp(t *testing.T) {
	withColor(t)

	for _, block := range []pgpage.BlockNumber{0, 2} {
		summary := pgpage.SummarizePage(fixturePage(t, block), block)
		rs := pgpage.PageRegions(summary.Header)

		for _, cells := range mapCellChoices {
			perCell := bytesPerRow / cells
			drawn := map[pgpage.RegionKind]int{}

			for row := range mapRows {
				for i := range cells {
					start := row*bytesPerRow + i*perCell
					c := splitCell(rs, highlight{}, start, start+perCell)

					drawn[c.left.kind] += c.eighths * perCell / eighths
					drawn[c.right.kind] += (eighths - c.eighths) * perCell / eighths
				}
			}

			for _, region := range rs {
				if drawn[region.Kind] != region.Len() {
					t.Errorf("block %d, %d cells: %v is drawn as %d bytes, want %d",
						block, cells, region.Kind, drawn[region.Kind], region.Len())
				}
			}
		}
	}
}

// A split cell is drawn with the block character of its eighths, in the
// colors of the two regions, and the row keeps the width of the grid.
func TestPageMapSplitCellColors(t *testing.T) {
	withColor(t)

	rows := mapLines(t, pageMap(pgpage.SummarizePage(fixturePage(t, 0), 0), true, offsetLabel+32))

	// Row 0 at 16 bytes a cell: the header ends half way through cell 1, a
	// half block in the header's color over the line pointers' color.
	if !strings.Contains(rows[0], "38;5;138;48;5;109m▌") {
		t.Errorf("row 0 has no half cell from header to line pointers:\n%q", rows[0])
	}

	// A split cell is not underlined: the underline would take the left
	// color, and show as a bright dash under the row. One eighth is the same
	// character as a whole cell, which is underlined, so it is left out.
	for _, row := range rows {
		for _, block := range partialBlocks[1:] {
			if block != cellGlyph && strings.Contains(row, ";4m"+block) {
				t.Errorf("a split cell %q is underlined:\n%q", block, row)
			}
		}
	}

	for i, row := range rows {
		if got := lipgloss.Width(row); got != offsetLabel+32 {
			t.Errorf("row %d is %d cells wide, want %d", i, got, offsetLabel+32)
		}
	}
}
