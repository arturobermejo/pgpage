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
)

// mapCellChoices are the only widths the grid is drawn at, widest first. Each
// divides a row evenly, so every cell holds a whole number of bytes: 8, 16 or
// 32. The grid used to take whatever width the terminal left, and resizing
// the window changed how many bytes a cell stood for one column at a time;
// now it keeps its scale until the window crosses one of these steps.
//
// 32 cells is also one cell per line of the hex view, 16 bytes.
var mapCellChoices = []int{64, 32, minMapCells}

// minMapCells is the narrowest grid, the last of mapCellChoices.
const minMapCells = 16

// mapCells returns the widest grid that fits in width cells next to the
// offsets, or 0 when not even the narrowest one does.
func mapCells(width int) int {
	for _, cells := range mapCellChoices {
		if offsetLabel+cells <= width {
			return cells
		}
	}

	return 0
}

// cellGlyph is drawn in every cell of the map when the terminal shows color:
// a thin line along the left edge of the cell, in a darker shade of the
// region, over the region's background. A cell of the grid holds 8, 16 or 32
// bytes (see mapCellChoices), so the lines mark the page at that step.
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

	colors := colorsOf(p)

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

// pageMap renders the page as a grid, with the byte offset of each row on the
// left and a legend below, in width cells. The grid takes the widest of
// mapCellChoices that fits, and the legend the whole width:
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

	cells := mapCells(width)
	if cells == 0 {
		return moreStyle.Render("too narrow")
	}

	var b strings.Builder

	b.WriteString(columnScale(cells))

	for row := range mapRows {
		b.WriteString("\n" + offsetStyle.Render(fmt.Sprintf("0x%04X ", row*bytesPerRow)))
		b.WriteString(mapRow(regions, row, cells, hl))
	}

	b.WriteString("\n\n" + legend(regions, hl, width))
	b.WriteString("\n\n" + fieldStyle.Render(fmt.Sprintf("1 cell = %d B", bytesPerRow/cells)))

	return b.String()
}

// mapRow draws one row of the grid: cells cells covering the bytesPerRow
// bytes that start at row*bytesPerRow, with the cells hl covers highlighted.
func mapRow(regions []pgpage.Region, row, cells int, hl highlight) string {
	return stripRow(regions, row*bytesPerRow, bytesPerRow, cells, hl)
}

// stripRow draws cells cells covering the span bytes that start at base.
func stripRow(regions []pgpage.Region, base, span, cells int, hl highlight) string {
	var (
		b   strings.Builder
		cur paint
		run int
	)

	flush := func() {
		if run > 0 {
			b.WriteString(swatch(cur, run))
			run = 0
		}
	}

	for i := range cells {
		// Cell boundaries are computed from the cell index, not accumulated,
		// so rounding cannot drift along the row.
		start := base + i*span/cells
		end := max(base+(i+1)*span/cells, start+1)

		cell := splitCell(regions, hl, start, end)

		// A cell split between two paints is drawn on its own.
		if cell.eighths < eighths {
			flush()
			b.WriteString(splitSwatch(cell))

			continue
		}

		// Consecutive whole cells painted alike are styled together: one
		// escape sequence for the run instead of one per cell.
		if run > 0 && cell.left != cur {
			flush()
		}

		cur = cell.left
		run++
	}

	flush()

	return b.String()
}

// eighths is how finely a cell of the map is split: the block characters
// ▏▎▍▌▋▊▉█ fill a cell from the left in eighths.
const eighths = 8

// partialBlocks are the characters that fill the left 1 to 7 eighths of a
// cell, indexed by eighths.
var partialBlocks = [eighths]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// cell is how one cell of the map is painted: whole in one paint, or split
// where the bytes it covers change from one paint to the next.
type cell struct {
	left, right paint
	eighths     int // of the cell painted left; eighths means the whole cell
}

// splitCell returns how the bytes in [start, end) paint a cell.
//
// A cell holds several bytes, and a boundary between two regions rarely falls
// between two cells. Painting the cell whole in one of them would move the
// boundary by up to a cell: with 16 bytes a cell, the 24-byte header would
// look like 32 bytes. So a cell on a boundary is split: the left part in the
// paint of its first bytes, in eighths of the cell, and the rest in the paint
// of its last byte. With 8, 16 or 32 bytes a cell an eighth is 1, 2 or 4
// bytes, and the boundaries of a heap page, which fall on multiples of 4, are
// drawn exactly.
//
// Without color there is no way to draw half a glyph, and the cell takes the
// region that owns most of its bytes, as before.
func splitCell(regions []pgpage.Region, hl highlight, start, end int) cell {
	if lipgloss.ColorProfile() == termenv.Ascii {
		p := paint{kind: cellRegion(regions, start, end), highlighted: hl.covers(start, end)}

		return cell{left: p, right: p, eighths: eighths}
	}

	left := paintAt(regions, hl, start)

	same := 1
	for same < end-start && paintAt(regions, hl, start+same) == left {
		same++
	}

	if same == end-start {
		return cell{left: left, right: left, eighths: eighths}
	}

	// Rounded to the nearest eighth, but never to none or all of the cell:
	// both paints are in it, so both must show.
	n := end - start
	e := min(max((same*eighths+n/2)/n, 1), eighths-1)

	return cell{left: left, right: paintAt(regions, hl, end-1), eighths: e}
}

// paintAt returns how byte b of the page is painted.
func paintAt(regions []pgpage.Region, hl highlight, b int) paint {
	p := paint{highlighted: hl.covers(b, b+1)}

	for _, region := range regions {
		if region.Start <= b && b < region.End {
			p.kind = region.Kind
			break
		}
	}

	return p
}

// colorsOf returns the colors a paint is drawn with.
func colorsOf(p paint) regionColors {
	if p.highlighted {
		return highlightColors
	}

	return regionPalette[p.kind]
}

// splitSwatch draws a split cell: the left eighths are a block character in
// the background color of the left paint, over the background of the right
// one.
//
// It has no underline. A terminal draws the underline in the text color,
// which in a split cell is the left paint's background, not a darker line:
// under a cell half line pointers and half free space it showed as a bright
// dash on the dark row. The escape sequence that gives the underline a color
// of its own (SGR 58) is not understood by every terminal, so the one cell
// goes without the line between rows instead.
func splitSwatch(c cell) string {
	profile := lipgloss.ColorProfile()

	return profile.String(partialBlocks[c.eighths]).
		Foreground(profile.Color(string(colorsOf(c.left).background))).
		Background(profile.Color(string(colorsOf(c.right).background))).
		String()
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

		// On a narrow grid the labels would run into each other, as in
		// "+128+256": a label that does not leave a space is skipped.
		if at > offsetLabel && line[at-1] != ' ' {
			continue
		}

		copy(line[at:], []rune(label))
	}

	return offsetStyle.Render(string(line))
}

// legendRow is one line of the legend: a swatch, a name and a byte range.
type legendRow struct {
	swatch     string
	name       string
	start, end int // bytes, end excluded
}

// legend names every region that has bytes, and the highlight if there is
// one, as a table: the names, the ranges and the sizes each in a column of
// their own, the same width in every entry, so that the entries line up
// however many columns of them fit in width.
//
//	█ Header        0-23      24 B    ╱ Line ptrs    24-763    740 B
//	╎ Free        764-2471  1708 B    ╲ Tuples     2472-8191  5720 B
//
// The regions come in page order, row by row. The highlight gets a line of
// its own below them: it is a part of one region, not a region.
func legend(regions []pgpage.Region, hl highlight, width int) string {
	const gap = 4 // between columns of entries

	var rows []legendRow

	for _, region := range regions {
		if region.Len() > 0 {
			rows = append(rows, legendRow{legendSwatch(region.Kind), regionName(region.Kind), region.Start, region.End})
		}
	}

	if hl.start < hl.end {
		rows = append(rows, legendRow{swatch(paint{highlighted: true}, 1), hl.label, hl.start, hl.end})
	}

	entries := renderLegend(rows)
	if hl.start < hl.end {
		entries = entries[:len(entries)-1]
	}

	entryWidth := maxWidth(entries)
	columns := max((width+gap)/(entryWidth+gap), 1)

	var b strings.Builder

	for i, entry := range entries {
		switch {
		case i == 0:
		case i%columns == 0:
			b.WriteString("\n")
		default:
			b.WriteString(strings.Repeat(" ", gap))
		}

		b.WriteString(entry)
	}

	if hl.start < hl.end {
		b.WriteString("\n" + renderLegend(rows)[len(rows)-1])
	}

	return b.String()
}

// renderLegend renders the rows with their columns aligned across all of
// them: names left aligned, the range around its dash, and sizes right
// aligned, and every entry padded to the same width.
func renderLegend(rows []legendRow) []string {
	var nameW, startW, endW, sizeW int

	for _, r := range rows {
		nameW = max(nameW, lipgloss.Width(r.name))
		startW = max(startW, len(fmt.Sprint(r.start)))
		endW = max(endW, len(fmt.Sprint(r.end-1)))
		sizeW = max(sizeW, len(fmt.Sprint(r.end-r.start)))
	}

	out := make([]string, len(rows))

	for i, r := range rows {
		out[i] = r.swatch + " " +
			valueStyle.Render(padRight(r.name, nameW+1)) + " " +
			fieldStyle.Render(fmt.Sprintf("%*d-%-*d  %*d B", startW, r.start, endW, r.end-1, sizeW, r.end-r.start))
	}

	return out
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
