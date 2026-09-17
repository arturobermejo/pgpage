package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// fieldColumn is the width of the field names of a panel, the longest one
// plus a space.
const fieldColumn = 16

// headerTitle names the page header panel.
const headerTitle = "PAGE HEADER"

// headerValueWidth is the widest value the header panel can show: the
// largest LSN, or the longest pd_flags name, since each flag goes on a line
// of its own. The panel is always that wide, so it does not grow and push
// the page map around when the selection moves onto a page with flags.
var headerValueWidth = func() int {
	width := lipgloss.Width(pgpage.LSN(math.MaxUint64).String())

	for _, name := range pageFlagNames(pgpage.PageHasFreeLines | pgpage.PageFull | pgpage.PageAllVisible) {
		width = max(width, lipgloss.Width(name))
	}

	return width
}()

// headerBoxWidth returns the width of the frame around the header panel:
// the same for every page, unless content is wider than any page could make
// it, as an error message can be.
func headerBoxWidth(content string) int {
	return max(boxWidth(headerTitle, content), fieldColumn+headerValueWidth+panelFrame)
}

// headerPanel renders what the page header of the selected page says: first
// the fields as they are stored, then the values derived from them.
//
//	PAGE HEADER
//	pd_lsn          0/21A8A18
//	pd_checksum     45171
//	...
//	items           185
//	free space      1708 B
//	status          OK
func headerPanel(summary pgpage.PageSummary, cached bool) string {
	var b strings.Builder

	if !cached {
		return moreStyle.Render("reading…")
	}

	if summary.Status != pgpage.StatusOK {
		// A header that did not parse describes nothing. Step 25 gives these
		// pages a view of their own.
		b.WriteString(field("status", statusStyle(summary.Status).Render(summary.Status.String())))

		if summary.Err != nil {
			b.WriteString("\n" + field("error", invalidStyle.Render(summary.Err.Error())))
		}

		return b.String()
	}

	h := summary.Header

	// Values are plain text, except the two boundaries that the page map
	// draws: pd_lower ends the line pointers and pd_upper starts the tuples,
	// so they take the colors of those regions and the eye can match the
	// number with the place on the map where one color turns into the next.
	stored := []headerRow{
		{"pd_lsn", h.LSN.String(), valueStyle},
		{"pd_checksum", fmt.Sprint(h.Checksum), valueStyle},
		{"pd_flags", fmt.Sprintf("%#04x", uint16(h.Flags)), valueStyle},
		{"pd_lower", fmt.Sprint(h.Lower), regionText(pgpage.RegionLinePointers)},
		{"pd_upper", fmt.Sprint(h.Upper), regionText(pgpage.RegionTuples)},
		{"pd_special", fmt.Sprint(h.Special), valueStyle},
		{"page size", fmt.Sprint(h.PageSize), valueStyle},
		{"layout version", fmt.Sprint(h.LayoutVersion), valueStyle},
		{"pd_prune_xid", fmt.Sprint(uint32(h.PruneXID)), valueStyle},
	}

	top, bottom := renderRows(stored), renderRows(derivedRows(summary))

	// The rule between what is stored on disk and what is computed from it
	// spans the panel, which is as wide as its widest possible line.
	width := max(maxWidth(top), maxWidth(bottom), fieldColumn+headerValueWidth)

	return strings.Join(top, "\n") + "\n\n" + ruleStyle.Render(strings.Repeat(borderHorizontal, width)) +
		"\n\n" + strings.Join(bottom, "\n")
}

// derivedRows returns what the header panel computes from the fields of a
// valid header, below the fields themselves.
func derivedRows(summary pgpage.PageSummary) []headerRow {
	h := summary.Header

	free := "—"
	if percent, ok := summary.FreeSpacePercent(); ok {
		free = fmt.Sprintf("%.0f %%", percent)
	}

	rows := []headerRow{
		{"line pointers", fmt.Sprint(h.ItemCount()), valueStyle},
		{"free space", fmt.Sprintf("%d B", h.FreeSpace()), valueStyle},
		{"free", free, valueStyle},
	}

	rows = append(rows, checksumRows(summary)...)

	// One flag per line, the name of the field on the first one only: the
	// names are long, and listed side by side they would make the panel as
	// wide as all of them together.
	for i, name := range pageFlagNames(h.Flags) {
		field := ""
		if i == 0 {
			field = "flags decoded"
		}

		rows = append(rows, headerRow{field, name, valueStyle})
	}

	return append(rows, headerRow{"status", summary.Status.String(), statusStyle(summary.Status)})
}

// checksumRows returns what the panel says about the checksum of a page,
// in the color of what it says: a mismatch is drawn like an invalid page,
// with the checksum the bytes give on a line of its own, as the flags are
// listed, so that the panel stays as wide as on any other page. A page
// written with checksums off is dimmed: there is nothing to check.
func checksumRows(summary pgpage.PageSummary) []headerRow {
	switch summary.Checksum {
	case pgpage.ChecksumOK:
		return []headerRow{{"checksum", "OK", okStyle}}
	case pgpage.ChecksumMismatch:
		return []headerRow{
			{"checksum", "MISMATCH", invalidStyle},
			{"", fmt.Sprintf("computed %d", summary.ComputedChecksum), invalidStyle},
		}
	case pgpage.ChecksumDisabled:
		return []headerRow{{"checksum", "DISABLED", moreStyle}}
	default:
		return []headerRow{{"checksum", "—", moreStyle}}
	}
}

// headerRow is one field of the header panel and the style of its value.
type headerRow struct {
	name  string
	value string
	style lipgloss.Style
}

// renderRows returns one "name  value" line per row.
func renderRows(rows []headerRow) []string {
	lines := make([]string, len(rows))

	for i, row := range rows {
		lines[i] = fieldStyle.Render(padRight(row.name, fieldColumn)) + row.style.Render(row.value)
	}

	return lines
}

// maxWidth returns the width of the widest line.
func maxWidth(lines []string) int {
	var width int

	for _, line := range lines {
		width = max(width, lipgloss.Width(line))
	}

	return width
}

// regionText returns a text style in the color of a region of the page map.
func regionText(kind pgpage.RegionKind) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(regionPalette[kind].background)
}

// field renders one "name  value" line of a panel.
func field(name, value string) string {
	return fieldStyle.Render(padRight(name, fieldColumn)) + valueStyle.Render(value)
}

// padRight left aligns s in a field of width cells, counting cells and not
// bytes, as padLeft does.
func padRight(s string, width int) string {
	if pad := width - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}

	return s + " "
}

// pageFlagNames returns the names of the pd_flags bits that are set, as
// PostgreSQL spells them, or a dash when none is.
func pageFlagNames(flags pgpage.PageFlags) []string {
	var names []string

	if flags.HasFreeLines() {
		names = append(names, "HAS_FREE_LINES")
	}

	if flags.IsFull() {
		names = append(names, "FULL")
	}

	if flags.IsAllVisible() {
		names = append(names, "ALL_VISIBLE")
	}

	if len(names) == 0 {
		return []string{"—"}
	}

	return names
}
