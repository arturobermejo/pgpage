package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// hexState is what the hex view shows: the bytes of one page, the range of
// them that belongs to what was selected when it was opened, and how far the
// dump is scrolled.
type hexState struct {
	block  pgpage.BlockNumber
	loaded bool
	page   []byte
	err    error
	hl     highlight

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
// the dump, and the blank line and the two lines of detail below it.
const hexChrome = 4

// byteClass is how a byte of the dump is drawn.
type byteClass int

const (
	byteValue       byteClass = iota // any byte
	byteZero                         // 00, dimmed: most of a page is zeroes
	byteHighlighted                  // part of the selection
)

// hexStyle returns the style of a class of bytes.
func hexStyle(class byteClass) lipgloss.Style {
	switch class {
	case byteZero:
		return zeroByteStyle
	case byteHighlighted:
		return highlightByteStyle
	default:
		return valueStyle
	}
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
func hexLines(page []byte, hl highlight) []string {
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
func hexBytes(line pgpage.HexLine, hl highlight) string {
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

		switch {
		case hl.covers(offset, offset+1):
			c = byteHighlighted
		case value == 0:
			c = byteZero
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
func lineLabel(offset int, regions []pgpage.Region, hl highlight) string {
	var names []string

	for _, region := range regions {
		if region.Len() > 0 && region.Start < offset+pgpage.HexBytesPerLine && offset < region.End {
			names = append(names, regionText(region.Kind).Render(regionName(region.Kind)))
		}
	}

	label := strings.Join(names, moreStyle.Render(" · "))

	if hl.start < hl.end && offset <= hl.start && hl.start < offset+pgpage.HexBytesPerLine {
		label += selectedStyle.Render(" ← " + hl.label)
	}

	if label == "" {
		return ""
	}

	return "  " + label
}

// hexDetail describes the selection under the dump: where it starts, its
// first bytes, and what it is.
//
//	0x001C  d8 9f 4a 00  → line pointer #2 · bytes 28-31 · 4 B
func hexDetail(page []byte, hl highlight) string {
	if hl.start >= hl.end {
		return moreStyle.Render("nothing selected: zero bytes are dimmed")
	}

	const shown = 8 // bytes of the selection spelled out; longer ones get "…"

	end := min(hl.end, hl.start+shown)

	var raw []string
	for _, value := range page[hl.start:end] {
		raw = append(raw, fmt.Sprintf("%02x", value))
	}

	more := ""
	if hl.end > end {
		more = " …"
	}

	return offsetStyle.Render(fmt.Sprintf("0x%04X  ", hl.start)) +
		highlightByteStyle.Render(strings.Join(raw, " ")) + moreStyle.Render(more) +
		valueStyle.Render(fmt.Sprintf("  → %s · bytes %d-%d · %d B", hl.label, hl.start, hl.end-1, hl.end-hl.start)) +
		"\n" + moreStyle.Render("highlighted bytes belong to the selection; zero bytes are dimmed")
}

// headerHighlight selects the page header, what the page view opens the hex
// view on. A page whose header does not parse has no header to point at.
func headerHighlight(summary pgpage.PageSummary) highlight {
	if summary.Status != pgpage.StatusOK {
		return highlight{}
	}

	return highlight{start: 0, end: pgpage.PageHeaderSize, label: "page header"}
}

// entryHighlight selects the 4 bytes of the selected line pointer's entry in
// the line pointer array.
func entryHighlight(it items) highlight {
	if !it.loaded || it.selected >= len(it.ids) {
		return highlight{}
	}

	start := pgpage.PageHeaderSize + it.selected*4

	return highlight{start: start, end: start + 4, label: fmt.Sprintf("line pointer #%d", number(it.selected))}
}

// tupleHighlight selects the bytes of the selected tuple.
func tupleHighlight(it items) highlight {
	hl := itemHighlight(it)
	hl.label = fmt.Sprintf("tuple #%d", number(it.selected))

	return hl
}
