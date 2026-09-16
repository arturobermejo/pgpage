package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// Glyphs of the regions. They differ in shade and in pattern, so the map can
// be read on a monochrome terminal, in a screenshot or by someone who does
// not see the colors apart.
var regionGlyph = map[pgpage.RegionKind]string{
	pgpage.RegionHeader:       "█",
	pgpage.RegionLinePointers: "▞",
	pgpage.RegionFree:         "░",
	pgpage.RegionTuples:       "▓",
	pgpage.RegionSpecial:      "▚",
}

// pageMap renders the page as a bar of width cells, where every region takes
// a share of the bar proportional to its bytes, and a legend below it.
//
//	PAGE 0 — 8192 BYTES
//	0                                                              8192
//	█▞▞▞▞▞░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓
//	█ Header 0-23 · 24 B
//	▞ Line pointers 24-763 · 740 B
func pageMap(block pgpage.BlockNumber, summary pgpage.PageSummary, cached bool, width int) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("PAGE %d — %d BYTES", block, pgpage.PageSize)))

	if !cached {
		b.WriteString("\n" + moreStyle.Render("reading…"))
		return b.String()
	}

	regions := pgpage.PageRegions(summary.Header)
	if regions == nil {
		// A new or invalid page has no layout to draw.
		b.WriteString("\n" + moreStyle.Render("no layout: "+summary.Status.String()))
		return b.String()
	}

	cells := mapCells(regions, width)
	if cells == nil {
		b.WriteString("\n" + moreStyle.Render("too narrow"))
		return b.String()
	}

	b.WriteString("\n" + scale(width))
	b.WriteString("\n" + bar(regions, cells))

	for i, region := range regions {
		if region.Len() == 0 {
			continue
		}

		b.WriteString("\n" + legend(region, cells[i]))
	}

	return b.String()
}

// mapCells returns how many cells of a bar of width cells each region takes.
//
// Every region that has bytes gets one cell first: the 24-byte header is a
// fifth of a cell in a 70-cell bar, and a map that rounded it away would
// claim the page has no header. The rest of the bar is shared out in
// proportion to the bytes, and the cells that rounding leaves over go to the
// regions with the largest remainders, so the bar is exactly width cells.
//
// It returns nil when the bar cannot hold one cell per region with bytes.
func mapCells(regions []pgpage.Region, width int) []int {
	type share struct {
		index     int
		remainder float64
	}

	var (
		cells    = make([]int, len(regions))
		shares   []share
		nonEmpty int
	)

	for _, region := range regions {
		if region.Len() > 0 {
			nonEmpty++
		}
	}

	if nonEmpty == 0 || width < nonEmpty {
		return nil
	}

	// One cell per region is spoken for; the rest is what gets shared out.
	used, rest := nonEmpty, width-nonEmpty

	for i, region := range regions {
		if region.Len() == 0 {
			continue
		}

		exact := float64(region.Len()) * float64(rest) / pgpage.PageSize
		whole := int(exact)

		cells[i] = 1 + whole
		used += whole

		shares = append(shares, share{index: i, remainder: exact - float64(whole)})
	}

	// Largest remainder first, keeping the order of the page when two tie,
	// so that the same page always yields the same bar.
	sort.SliceStable(shares, func(a, b int) bool {
		return shares[a].remainder > shares[b].remainder
	})

	for i := 0; used < width; i++ {
		cells[shares[i%len(shares)].index]++
		used++
	}

	return cells
}

// bar returns the row of glyphs, cells[i] of them for each region.
func bar(regions []pgpage.Region, cells []int) string {
	var b strings.Builder

	for i, region := range regions {
		if cells[i] == 0 {
			continue
		}

		b.WriteString(regionStyle(region.Kind).Render(strings.Repeat(regionGlyph[region.Kind], cells[i])))
	}

	return b.String()
}

// scale returns the ruler above the bar: the first and last byte offsets of
// the page, at the ends of the bar.
func scale(width int) string {
	const (
		first = "0"
		last  = "8192"
	)

	if width < len(first)+len(last)+1 {
		return ""
	}

	return moreStyle.Render(first + strings.Repeat(" ", width-len(first)-len(last)) + last)
}

// legend returns the line that names one region, with its glyph, its byte
// range and its size.
func legend(region pgpage.Region, cells int) string {
	style := regionStyle(region.Kind)

	unit := "cells"
	if cells == 1 {
		unit = "cell"
	}

	return style.Render(regionGlyph[region.Kind]) + " " +
		valueStyle.Render(region.Kind.String()) + " " +
		fieldStyle.Render(fmt.Sprintf("%d-%d · %d B · %d %s", region.Start, region.End-1, region.Len(), cells, unit))
}

// regionStyle returns the color of a region, which repeats the difference
// its glyph already makes.
func regionStyle(kind pgpage.RegionKind) lipgloss.Style {
	switch kind {
	case pgpage.RegionHeader:
		return headerRegionStyle
	case pgpage.RegionLinePointers:
		return itemsRegionStyle
	case pgpage.RegionFree:
		return freeRegionStyle
	case pgpage.RegionTuples:
		return tuplesRegionStyle
	case pgpage.RegionSpecial:
		return specialRegionStyle
	default:
		return moreStyle
	}
}
