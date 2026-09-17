package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// hexState is what the hex view shows: the bytes of one page, the ranges of
// them that belong to what was selected when it was opened, and how far the
// dump is scrolled.
type hexState struct {
	block  pgpage.BlockNumber
	loaded bool
	page   []byte
	err    error
	hl     highlights

	// vp scrolls the dump. It holds the whole page, 512 lines, and draws
	// only the ones that fit.
	vp viewport.Model
}

// hexMsg carries a page read for the hex view.
type hexMsg struct {
	block pgpage.BlockNumber
	page  []byte
	err   error
}

// loadHex returns a command that reads block for the hex view. The views
// that already hold the page pass it instead, and nothing is read.
func loadHex(rel *pgpage.Relation, block pgpage.BlockNumber) tea.Cmd {
	return func() tea.Msg {
		page, err := rel.ReadPage(block)

		return hexMsg{block: block, page: page, err: err}
	}
}

// Lines of the hex panel that are not dump lines: the column titles above
// the dump, and the blank line and the two lines of detail below it, one per
// highlight, or one and the key to the colors.
const hexChrome = 4

// byteClass is how a byte of the dump is drawn.
type byteClass int

const (
	byteValue    byteClass = iota // any byte
	byteZero                      // 00, dimmed: most of a page is zeroes
	byteSelected                  // part of the selection
	bytePointed                   // part of what the selection points to
)

// hexStyle returns the style of a class of bytes.
func hexStyle(class byteClass) lipgloss.Style {
	switch class {
	case byteZero:
		return zeroByteStyle
	case byteSelected:
		return selectedByteStyle
	case bytePointed:
		return pointedByteStyle
	default:
		return valueStyle
	}
}

// markStyle returns the style of the bytes a highlight paints.
func markStyle(h highlight) lipgloss.Style {
	if h.pointed {
		return pointedByteStyle
	}

	return selectedByteStyle
}

// hexColumns is the line of column titles above the dump: the position of
// each byte inside its line.
func hexColumns() string {
	var b strings.Builder

	b.WriteString(strings.Repeat(" ", 6))

	for i := range pgpage.HexBytesPerLine {
		if i == pgpage.HexBytesPerLine/2 {
			b.WriteString(" ")
		}

		fmt.Fprintf(&b, "%02x ", i)
	}

	return moreStyle.Render(strings.TrimRight(b.String(), " "))
}

// hexLines renders the page as a dump, one line per 16 bytes: offset, bytes,
// the bytes as text, and the regions of the page the line falls in.
//
//	0000  00 00 00 00 10 58 9a 01  73 b0 00 00 48 00 20 1e  |.....X..s...H. .|  Header · Line ptrs
//
// Bytes in hl are highlighted and zero bytes dimmed, so the eye goes to what
// was selected and to the bytes that hold something.
func hexLines(page []byte, hl highlights) []string {
	h, _ := pgpage.ParsePageHeader(page)
	regions := pgpage.PageRegions(h) // nil for a new or invalid page: no labels

	lines := pgpage.HexLines(page, 0)
	out := make([]string, len(lines))

	for i, line := range lines {
		out[i] = offsetStyle.Render(fmt.Sprintf("%04x", line.Offset)) + "  " +
			hexBytes(line, hl) + "  " +
			moreStyle.Render("|"+line.ASCII()+"|") +
			lineLabel(line.Offset, regions, hl)
	}

	return out
}

// hexBytes renders the 16 bytes of a line. Consecutive bytes of the same
// class are styled together, and the space between two highlighted bytes is
// highlighted too, so a field reads as one box: "48 00", not "48" "00".
func hexBytes(line pgpage.HexLine, hl highlights) string {
	var (
		b     strings.Builder
		run   strings.Builder
		class byteClass
	)

	flush := func() {
		if run.Len() > 0 {
			b.WriteString(hexStyle(class).Render(run.String()))
			run.Reset()
		}
	}

	for i, value := range line.Bytes {
		offset := line.Offset + i

		c := byteValue

		switch hl.markIn(offset, offset+1) {
		case markSelected:
			c = byteSelected
		case markPointed:
			c = bytePointed
		default:
			if value == 0 {
				c = byteZero
			}
		}

		sep := " "
		if i == pgpage.HexBytesPerLine/2 {
			sep = "  " // the gap in the middle of the line, as hexdump -C
		}

		switch {
		case i == 0:
		case c == class && i != pgpage.HexBytesPerLine/2:
			run.WriteString(sep)
		default:
			flush()
			b.WriteString(sep)
		}

		class = c

		fmt.Fprintf(&run, "%02x", value)
	}

	flush()

	return b.String()
}

// lineLabel names the regions of the page that the 16 bytes at offset fall
// in, and marks the line where the selection starts.
func lineLabel(offset int, regions []pgpage.Region, hl highlights) string {
	var names []string

	for _, region := range regions {
		if region.Len() > 0 && region.Start < offset+pgpage.HexBytesPerLine && offset < region.End {
			names = append(names, regionText(region.Kind).Render(regionName(region.Kind)))
		}
	}

	label := strings.Join(names, moreStyle.Render(" · "))

	for _, h := range hl {
		if h.start < h.end && offset <= h.start && h.start < offset+pgpage.HexBytesPerLine {
			label += selectedStyle.Render(" ← " + h.label)
		}
	}

	if label == "" {
		return ""
	}

	return "  " + label
}

// hexDetail describes the selection under the dump, one line per highlight:
// where it starts, its first bytes in its color, and what it is. With a
// single highlight, the second line says what the colors mean.
//
//	0x001C  d8 9f 4a 00  → line ptr #2 · bytes 28-31 · 4 B
//	0x1FD8  02 00 00 00 00 00 00 00 …  → tuple (2,2) · bytes 8152-8188 · 37 B
func hexDetail(page []byte, hl highlights) string {
	var lines []string

	for _, h := range hl {
		if h.start < h.end {
			lines = append(lines, detailLine(page, h))
		}
	}

	switch len(lines) {
	case 0:
		return moreStyle.Render("nothing selected: zero bytes are dimmed")
	case 1:
		lines = append(lines, moreStyle.Render("highlighted bytes belong to the selection; zero bytes are dimmed"))
	}

	return strings.Join(lines, "\n")
}

// detailLine describes one highlight.
func detailLine(page []byte, h highlight) string {
	const shown = 8 // bytes spelled out; longer ranges get "…"

	end := min(h.end, h.start+shown)

	var raw []string
	for _, value := range page[h.start:end] {
		raw = append(raw, fmt.Sprintf("%02x", value))
	}

	more := ""
	if h.end > end {
		more = " …"
	}

	return offsetStyle.Render(fmt.Sprintf("0x%04X  ", h.start)) +
		markStyle(h).Render(strings.Join(raw, " ")) + moreStyle.Render(more) +
		valueStyle.Render(fmt.Sprintf("  → %s · bytes %d-%d · %d B", h.label, h.start, h.end-1, h.end-h.start))
}

// headerHighlight selects the page header, what the page view opens the hex
// view on. A page whose header does not parse has no header to point at.
func headerHighlight(summary pgpage.PageSummary) highlights {
	if summary.Status != pgpage.StatusOK {
		return nil
	}

	return highlights{{start: 0, end: pgpage.PageHeaderSize, label: "page header"}}
}
