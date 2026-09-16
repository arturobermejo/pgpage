package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/arturobermejo/pgpage"
)

// The map draws the page as a grid: one row per bytesPerRow bytes, the same
// shape a hex dump has, so an offset is always in the same place on screen.
const (
	mapRows     = 16
	bytesPerRow = pgpage.PageSize / mapRows // 512
	offsetLabel = 7                         // "0x0000" and a space
	minMapCells = 16
)

// cellGlyph is drawn in every cell of the map when the terminal shows color:
// a thin line along the left edge of the cell, in a darker shade of the
// region, over the region's background. A cell of the grid holds about 8
// bytes, so the lines mark the page every 8 bytes.
//
// The line along the bottom, which keeps the rows apart, is not part of the
// glyph: it is the terminal's underline, which a terminal draws in the text
// color, so it takes the same darker shade (see swatch). No widely supported character has both lines. "🭼" does, but
// it belongs to Symbols for Legacy Computing (Unicode 13), and fonts without
// it showed question marks; "▏" comes from Block Elements, in Unicode since
// 1.1, and underline is one of the oldest terminal attributes.
const cellGlyph = "▏"

// regionGlyph is what each region is drawn with when there is no color, as
// on a monochrome terminal or in a plain text capture. There every cell would
// be the same cellGlyph, so the regions need shapes of their own.
var regionGlyph = map[pgpage.RegionKind]string{
	pgpage.RegionHeader:       "█",
	pgpage.RegionLinePointers: "╱",
	pgpage.RegionFree:         "╎",
	pgpage.RegionTuples:       "╲",
	pgpage.RegionSpecial:      "╳",
}

// glyph returns what the cells of a region are drawn with on this terminal.
func glyph(kind pgpage.RegionKind) string {
	if lipgloss.ColorProfile() == termenv.Ascii {
		return regionGlyph[kind]
	}

	return cellGlyph
}

// highlightGlyph draws the highlighted cells when there is no color: a shade
// no region uses.
const highlightGlyph = "▒"

// highlight is a range of bytes the map paints over its regions, such as the
// tuple of the selected line pointer. The zero value highlights nothing.
type highlight struct {
	start, end int    // bytes, end excluded
	label      string // what the legend calls the range
}

// covers reports whether the highlight shares any byte with [start, end).
func (h highlight) covers(start, end int) bool {
	return h.start < h.end && start < h.end && h.start < end
}

// paint is what one cell of the map is drawn as: its region, and whether it
// is highlighted.
type paint struct {
	kind        pgpage.RegionKind
	highlighted bool
}

// swatch returns n cells painted the same way, as the map draws them.
func swatch(p paint, n int) string {
	if lipgloss.ColorProfile() == termenv.Ascii {
		// Without color the glyphs alone tell the regions apart, and an
		// underline would only add noise to a plain text capture.
		if p.highlighted {
			return strings.Repeat(highlightGlyph, n)
		}

		return strings.Repeat(regionGlyph[p.kind], n)
	}

	colors := regionPalette[p.kind]
	if p.highlighted {
		colors = highlightColors
	}

	// termenv, the library under Lip Gloss, styles the whole run with a
	// single escape sequence. Lip Gloss would underline it one character at
	// a time, which is 64 sequences for a row of a single region.
	profile := lipgloss.ColorProfile()

	return profile.String(strings.Repeat(cellGlyph, n)).
		Foreground(profile.Color(string(colors.line))).
		Background(profile.Color(string(colors.background))).
		Underline().
		String()
}

// pageMap renders the page as a grid of width cells, with the byte offset of
// each row on the left and a legend below:
//
//	       +0        +128      +256      +384
//	0x0000 ███╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱
//	0x0200 ╱╱╱╱╱╱╱╱╱╱╎╎╎╎╎╎╎╎╎╎╎╎╎╎╎╎╎╎╎╎╎╎
//	...
//	██ Header 0-23 · 24 B     ╱╱ Line ptrs 24-763 · 740 B
//
// That is how it looks without color; with color every cell is a cellGlyph
// over the background of its region.
func pageMap(summary pgpage.PageSummary, cached bool, width int) string {
	return pageMapWith(summary, cached, width, highlight{})
}

// pageMapWith is pageMap with a range of bytes highlighted, and named in the
// legend.
func pageMapWith(summary pgpage.PageSummary, cached bool, width int, hl highlight) string {
	if !cached {
		return moreStyle.Render("reading…")
	}

	regions := pgpage.PageRegions(summary.Header)
	if regions == nil {
		// A new or invalid page has no layout to draw.
		return moreStyle.Render("no layout: " + summary.Status.String())
	}

	cells := width - offsetLabel
	if cells < minMapCells {
		return moreStyle.Render("too narrow")
	}

	var b strings.Builder

	b.WriteString(columnScale(cells))

	for row := range mapRows {
		b.WriteString("\n" + offsetStyle.Render(fmt.Sprintf("0x%04X ", row*bytesPerRow)))
		b.WriteString(mapRow(regions, row, cells, hl))
	}

	b.WriteString("\n\n" + legend(regions, width))

	if hl.start < hl.end {
		b.WriteString("\n" + swatch(paint{highlighted: true}, 1) + " " + valueStyle.Render(hl.label) + " " +
			fieldStyle.Render(fmt.Sprintf("bytes %d-%d · %d B", hl.start, hl.end-1, hl.end-hl.start)))
	}

	return b.String()
}

// mapRow draws one row of the grid: cells cells covering the bytesPerRow
// bytes that start at row*bytesPerRow, with the cells hl covers highlighted.
func mapRow(regions []pgpage.Region, row, cells int, hl highlight) string {
	var (
		b   strings.Builder
		cur paint
		run int
	)

	base := row * bytesPerRow

	for i := range cells {
		// Cell boundaries are computed from the cell index, not accumulated,
		// so rounding cannot drift along the row.
		start := base + i*bytesPerRow/cells
		end := max(base+(i+1)*bytesPerRow/cells, start+1)

		cell := paint{
			kind:        cellRegion(regions, start, end),
			highlighted: hl.covers(start, end),
		}

		// Consecutive cells painted alike are styled together: one escape
		// sequence for the run instead of one per cell.
		if run > 0 && cell != cur {
			b.WriteString(swatch(cur, run))

			run = 0
		}

		cur = cell
		run++
	}

	b.WriteString(swatch(cur, run))

	return b.String()
}

// cellRegion returns the region that owns most of the bytes in [start, end).
// A cell that straddles a boundary belongs to whichever side fills it more,
// so one cell is never claimed by two regions.
func cellRegion(regions []pgpage.Region, start, end int) pgpage.RegionKind {
	var (
		best  pgpage.RegionKind
		bytes int
	)

	for _, region := range regions {
		overlap := min(end, region.End) - max(start, region.Start)
		if overlap > bytes {
			best, bytes = region.Kind, overlap
		}
	}

	return best
}

// columnScale returns the ruler above the grid, marking byte offsets inside
// a row every scaleStep bytes.
func columnScale(cells int) string {
	const scaleStep = 128

	line := []rune(strings.Repeat(" ", offsetLabel+cells))

	for offset := 0; offset < bytesPerRow; offset += scaleStep {
		at := offsetLabel + offset*cells/bytesPerRow

		label := fmt.Sprintf("+%d", offset)
		if at+len(label) > len(line) {
			break
		}

		copy(line[at:], []rune(label))
	}

	return offsetStyle.Render(string(line))
}

// legend names every region that has bytes, with its range and its size,
// in as many columns as width allows.
func legend(regions []pgpage.Region, width int) string {
	const gap = 2

	var (
		entries []string
		longest int
	)

	for _, region := range regions {
		if region.Len() == 0 {
			continue
		}

		entry := legendEntry(region)
		longest = max(longest, lipgloss.Width(entry))

		entries = append(entries, entry)
	}

	// Columns are as wide as the longest entry, so they line up whatever the
	// page holds: at least one, even when nothing really fits.
	columns := max((width+gap)/(longest+gap), 1)

	var b strings.Builder

	for i, entry := range entries {
		switch {
		case i == 0:
		case i%columns == 0:
			b.WriteString("\n")
		default:
			b.WriteString(strings.Repeat(" ", gap+longest-lipgloss.Width(entries[i-1])))
		}

		b.WriteString(entry)
	}

	return b.String()
}

// legendEntry returns "█ Header 0-23 · 24 B".
func legendEntry(region pgpage.Region) string {
	return legendSwatch(region.Kind) + " " +
		valueStyle.Render(regionName(region.Kind)) + " " +
		fieldStyle.Render(fmt.Sprintf("%d-%d · %d B", region.Start, region.End-1, region.Len()))
}

// legendSwatch returns the one cell that shows the color of a region in the
// legend: its background alone, since the lines of the grid are there to keep
// cells apart and one cell has nothing to be kept apart from. Without color
// it is the region's glyph.
func legendSwatch(kind pgpage.RegionKind) string {
	if lipgloss.ColorProfile() == termenv.Ascii {
		return regionGlyph[kind]
	}

	return lipgloss.NewStyle().Background(regionPalette[kind].background).Render(" ")
}

// regionName returns the name the legend uses, which is shorter than the one
// the library spells out so that the entries fit in a couple of columns.
func regionName(kind pgpage.RegionKind) string {
	switch kind {
	case pgpage.RegionLinePointers:
		return "Line ptrs"
	case pgpage.RegionFree:
		return "Free"
	default:
		return kind.String()
	}
}
