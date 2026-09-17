package tui

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// explainState is the explain view's state: the page it was opened on, and
// which field of its header is selected.
type explainState struct {
	block   pgpage.BlockNumber
	summary pgpage.PageSummary
	field   int // index into explanations
	scroll  int // lines of the explanation scrolled past
}

// explanation is what explain mode says about one field of PageHeaderData.
//
// The words are data, not code: a field is explained by filling in a value
// of this type, and the view that draws them is the same for every field.
// Only what depends on the page, the numbers, is computed.
type explanation struct {
	field  string // as PostgreSQL names it in bufpage.h
	offset int    // where it is stored in the page
	size   int    // in bytes
	about  string // what the field is, in one sentence

	// purpose says why the field exists, the problem it solves, and how
	// says how PostgreSQL keeps and uses it. They are what the user came to
	// learn; the numbers below them only apply it to this page.
	purpose string
	how     string

	note string // a fact worth knowing to read the numbers, if any

	// value is the field as the list shows it, and steps what follows from
	// it on this page: the rule, then the rule with the page's numbers.
	value func(h pgpage.PageHeader) string
	steps func(s pgpage.PageSummary) []step

	// mark returns the byte of the page the field points at, for the fields
	// that are offsets into the page.
	mark func(h pgpage.PageHeader) (int, bool)
}

// step is one line of working: a rule written with the names of the fields,
// the same rule with the numbers of this page, its result, and what the
// result means. Any part can be empty; a step with only a meaning is a
// remark.
//
//	(pd_lower - 24) ÷ 4 = (764 - 24) ÷ 4 = 185   line pointers, 4 B each
type step struct {
	rule, values, result string
	meaning              string
}

// explanations are the fields of the page header, in the order they are
// stored, which is the order the list shows them in.
var explanations = []explanation{
	{
		field:  "pd_lsn",
		offset: 0,
		size:   8,
		about:  "Position in the WAL of the last change made to this page.",
		purpose: "It enforces write-ahead logging: a page may only be written to disk after the WAL " +
			"has been flushed up to this position, so after a crash the log always holds every " +
			"change the page on disk may have.",
		how: "Every change to the page is logged first, and the LSN of that WAL record is stored " +
			"here. During recovery, a record is replayed on the page only if its LSN is newer than " +
			"pd_lsn: older ones are already in it. An LSN is a 64-bit byte position in the WAL, " +
			"written as its two 32-bit halves in hex.",
		value: func(h pgpage.PageHeader) string { return h.LSN.String() },
		steps: func(s pgpage.PageSummary) []step {
			raw := headerBytes(s.Header)
			lsn := uint64(s.Header.LSN)

			return []step{
				{"xlogid", byteList(raw[0:4]), fmt.Sprintf("0x%08x", lsn>>32), "high half"},
				{"xrecoff", byteList(raw[4:8]), fmt.Sprintf("0x%08x", uint32(lsn)), "low half"},
				{"xlogid × 2³² + xrecoff", "", fmt.Sprint(lsn), "bytes into the WAL"},
				{"", "", "", fmt.Sprintf("each half is read little-endian, and the LSN is written as xlogid/xrecoff in hex: %s",
					s.Header.LSN)},
			}
		},
	},
	{
		field:  "pd_checksum",
		offset: 8,
		size:   2,
		about:  "Checksum of the page, to detect corruption on disk.",
		purpose: "Disks and controllers can silently return damaged data. With data checksums on, " +
			"PostgreSQL reports a broken page when it reads it, instead of using wrong rows.",
		how: "The checksum is computed when the page is written out, over the whole page and its " +
			"block number, with this field taken as 0; on read it is computed again and compared. " +
			"The block number makes a correct page written in the wrong place fail too. Checksums " +
			"are chosen at initdb, on by default since PostgreSQL 18, and can be switched offline with " +
			"pg_checksums; with them off, the field is unused.",
		value: func(h pgpage.PageHeader) string { return fmt.Sprint(h.Checksum) },
		steps: func(s pgpage.PageSummary) []step {
			steps := []step{{"stored", "", fmt.Sprintf("%d = %#04x", s.Header.Checksum, s.Header.Checksum), ""}}

			switch s.Checksum {
			case pgpage.ChecksumOK, pgpage.ChecksumMismatch:
				steps = append(steps, step{
					"computed", "", fmt.Sprintf("%d = %#04x", s.ComputedChecksum, s.ComputedChecksum),
					checksumMeaning(s.Checksum),
				})
			case pgpage.ChecksumDisabled:
				steps = append(steps, step{"", "", "", "0 is never stored with checksums on: this page was written with them off"})
			}

			return append(steps, step{"", "", "", "the value is a 32-bit hash folded into 1-65535, so it is never 0"})
		},
	},
	{
		field:  "pd_flags",
		offset: 10,
		size:   2,
		about:  "Bits that summarize the state of the whole page.",
		purpose: "They let PostgreSQL skip work: a flag answers in one bit what would otherwise take " +
			"looking at every line pointer or tuple of the page.",
		how: "HAS_FREE_LINES says some line pointers are unused, so a new tuple can reuse one instead " +
			"of growing the array. FULL is set when an UPDATE found no room, a hint that pruning " +
			"may pay off. ALL_VISIBLE says every tuple is visible to every transaction, so a scan need " +
			"not check them one by one; the visibility map keeps a matching bit for VACUUM and " +
			"index-only scans.",
		value: func(h pgpage.PageHeader) string { return fmt.Sprintf("%#04x", uint16(h.Flags)) },
		steps: func(s pgpage.PageSummary) []step {
			flags := uint16(s.Header.Flags)
			steps := make([]step, 0, 3)

			for _, f := range []struct {
				bit  pgpage.PageFlags
				name string
			}{
				{pgpage.PageHasFreeLines, "HAS_FREE_LINES"},
				{pgpage.PageFull, "FULL"},
				{pgpage.PageAllVisible, "ALL_VISIBLE"},
			} {
				meaning := f.name + " not set"
				if s.Header.Flags&f.bit != 0 {
					meaning = f.name + " set"
				}

				steps = append(steps, step{
					fmt.Sprintf("pd_flags & %#04x", uint16(f.bit)),
					fmt.Sprintf("%#04x & %#04x", flags, uint16(f.bit)),
					fmt.Sprintf("%#04x", flags&uint16(f.bit)),
					meaning,
				})
			}

			return steps
		},
	},
	{
		field:  "pd_lower",
		offset: 12,
		size:   2,
		about:  "Offset to the end of the line pointer array.",
		purpose: "With pd_upper it bounds the free space in the middle of the page, so knowing whether " +
			"a tuple fits is a subtraction, not a search.",
		how: "The line pointer array starts right after the 24-byte header and grows towards higher " +
			"offsets, towards pd_upper: each new line pointer takes 4 bytes and moves pd_lower up by 4. " +
			"What a line pointer points to PostgreSQL calls an item; on a heap page, a tuple. Line " +
			"pointers are not removed when their tuple dies, because indexes find a tuple by its block " +
			"and line pointer number: VACUUM marks them unused so they can be reused, and only trims " +
			"the unused ones at the end of the array.",
		value: func(h pgpage.PageHeader) string { return fmt.Sprint(h.Lower) },
		steps: func(s pgpage.PageSummary) []step {
			h := s.Header
			used := int(h.Lower) - pgpage.PageHeaderSize

			return []step{
				{"pd_lower - 24", fmt.Sprintf("%d - 24", h.Lower), fmt.Sprint(used), "bytes of line pointers"},
				{"(pd_lower - 24) ÷ 4", fmt.Sprintf("(%d - 24) ÷ 4", h.Lower), fmt.Sprint(h.ItemCount()), "line pointers, 4 B each"},
				{"", "", "", fmt.Sprintf("24 is the size of the header, and the array runs from byte 24 to byte %d", h.Lower-1)},
			}
		},
		mark: func(h pgpage.PageHeader) (int, bool) { return int(h.Lower), true },
	},
	{
		field:  "pd_upper",
		offset: 14,
		size:   2,
		about:  "Offset to the beginning of tuple data.",
		purpose: "It says where the next tuple goes. Tuples are added from pd_special towards lower " +
			"offsets while the line pointer array grows towards higher ones, so both grow into the free " +
			"space between them without knowing in advance how many of each the page will hold.",
		how: "A new tuple is written just below pd_upper, which moves down by the tuple's size " +
			"rounded up to the platform's alignment, 8 bytes on 64-bit systems. When the free space " +
			"cannot hold the tuple, plus a new line pointer if no unused one can be reused, the tuple " +
			"goes to another page. Pruning and VACUUM pack the remaining tuples back towards " +
			"pd_special, and pd_upper moves up again.",
		note:  "Free space is the region between pd_lower and pd_upper.",
		value: func(h pgpage.PageHeader) string { return fmt.Sprint(h.Upper) },
		steps: func(s pgpage.PageSummary) []step {
			h := s.Header

			return []step{
				{"pd_upper - pd_lower", fmt.Sprintf("%d - %d", h.Upper, h.Lower), fmt.Sprint(h.FreeSpace()), "bytes of free space"},
				{"free ÷ 8192", fmt.Sprintf("%d ÷ 8192", h.FreeSpace()), fmt.Sprintf("%.0f %%", freePercent(h)), "of the page"},
				{"pd_special - pd_upper", fmt.Sprintf("%d - %d", h.Special, h.Upper), fmt.Sprint(int(h.Special) - int(h.Upper)), "bytes of tuples"},
			}
		},
		mark: func(h pgpage.PageHeader) (int, bool) { return int(h.Upper), true },
	},
	{
		field:  "pd_special",
		offset: 16,
		size:   2,
		about:  "Offset to the special space, at the end of the page.",
		purpose: "Every kind of relation shares this page layout, but some need a few bytes of their own " +
			"on each page: a B-tree page keeps the links to its siblings and its level in the tree there.",
		how: "The bytes from pd_special to the end of the page are reserved when the page is " +
			"initialized, and only the access method that owns the page reads them. Heap pages need " +
			"none, so pd_special is 8192 and the space is empty; on an index page it is smaller.",
		value: func(h pgpage.PageHeader) string { return fmt.Sprint(h.Special) },
		steps: func(s pgpage.PageSummary) []step {
			h := s.Header

			return []step{
				{"8192 - pd_special", fmt.Sprintf("8192 - %d", h.Special), fmt.Sprint(pgpage.PageSize - int(h.Special)), "bytes of special space"},
			}
		},
		mark: func(h pgpage.PageHeader) (int, bool) { return int(h.Special), true },
	},
	{
		field:  "pd_pagesize_version",
		offset: 18,
		size:   2,
		about:  "Size of the page and version of its layout, packed in one field.",
		purpose: "A page has to say how to read it: the page size is chosen when PostgreSQL is " +
			"compiled, and the layout of the header has changed across releases.",
		how: "Page sizes are multiples of 256, so the low byte of the size is always zero, and the " +
			"version is stored in it. Version 4 is the layout of PostgreSQL 8.3 and later, the only " +
			"one pgpage reads. The size is 8192 unless PostgreSQL was built with another block size.",
		value: func(h pgpage.PageHeader) string { return fmt.Sprint(int(h.PageSize) | int(h.LayoutVersion)) },
		steps: func(s pgpage.PageSummary) []step {
			h := s.Header
			v := uint16(h.PageSize) | uint16(h.LayoutVersion)

			return []step{
				{"value & 0xff00", fmt.Sprintf("%#04x & 0xff00", v), fmt.Sprintf("%#04x = %d", v&0xff00, v&0xff00), "page size"},
				{"value & 0x00ff", fmt.Sprintf("%#04x & 0x00ff", v), fmt.Sprintf("%#02x = %d", v&0x00ff, v&0x00ff), "layout version"},
			}
		},
	},
	{
		field:  "pd_prune_xid",
		offset: 20,
		size:   4,
		about:  "Oldest transaction that may have left something to prune on this page.",
		purpose: "Pruning reclaims the space of tuple versions nobody can see anymore. This field says " +
			"whether trying is worth it, without looking at a single tuple.",
		how: "A DELETE or UPDATE leaves the old version of the row in place and stores its transaction " +
			"id here, unless an older one is already stored. When a query later reads the page and it " +
			"is running short of free space, and that transaction is older than every snapshot still " +
			"running, the page is pruned: dead versions are removed, HOT chains shortened, and the " +
			"field updated. 0 means there is no hint.",
		value: func(h pgpage.PageHeader) string { return fmt.Sprint(uint32(h.PruneXID)) },
		steps: func(s pgpage.PageSummary) []step {
			if s.Header.PruneXID == 0 {
				return []step{{"pd_prune_xid", "", "0", "no hint: nothing on the page is known to be prunable"}}
			}

			return []step{{"pd_prune_xid", "", fmt.Sprint(uint32(s.Header.PruneXID)), "may have left dead versions behind"}}
		},
	},
}

// headerBytes encodes a header back into the 24 bytes it was read from. The
// header of a valid page is decoded without losing anything, so these are
// the bytes on disk, and the explanations can show them without reading the
// page again.
func headerBytes(h pgpage.PageHeader) [pgpage.PageHeaderSize]byte {
	var b [pgpage.PageHeaderSize]byte

	le := binary.LittleEndian
	le.PutUint32(b[0:4], uint32(uint64(h.LSN)>>32))
	le.PutUint32(b[4:8], uint32(h.LSN))
	le.PutUint16(b[8:10], h.Checksum)
	le.PutUint16(b[10:12], uint16(h.Flags))
	le.PutUint16(b[12:14], h.Lower)
	le.PutUint16(b[14:16], h.Upper)
	le.PutUint16(b[16:18], h.Special)
	le.PutUint16(b[18:20], h.PageSize|uint16(h.LayoutVersion))
	le.PutUint32(b[20:24], uint32(h.PruneXID))

	return b
}

// byteList writes bytes as a hex dump does: "fc 02".
func byteList(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("%02x", v)
	}

	return strings.Join(parts, " ")
}

// checksumMeaning says whether the computed checksum matches the stored one.
func checksumMeaning(status pgpage.ChecksumStatus) string {
	if status == pgpage.ChecksumOK {
		return "matches: the page is as PostgreSQL wrote it"
	}

	return "does not match: the page changed after it was written"
}

// freePercent returns the free space of the page as a percentage, rounded as
// the header panel rounds it.
func freePercent(h pgpage.PageHeader) float64 {
	return float64(h.FreeSpace()) * 100 / pgpage.PageSize
}

// title returns the title of the field's panel: "pd_lower = 764".
func (e explanation) title(h pgpage.PageHeader) string {
	return e.field + " = " + e.value(h)
}

// highlight returns the bytes the field is stored in, what x shows.
func (e explanation) highlight() highlights {
	return highlights{{start: e.offset, end: e.offset + e.size, label: e.field}}
}

// explainPanel renders the explanation of a field, width cells wide. The
// numbers of this page come first, right under what the field is, so that
// the text below them is read with something concrete in mind:
//
//	Offset to the end of the line pointer array.
//
//	ON THIS PAGE
//	┌──────────────────────────────────────────────────────────────┐
//	│ bytes 12-13    fc 02  →  0x02fc = 764, read little-endian    │
//	│ ──────────────────────────────────────────────────────────── │
//	│ pd_lower - 24       = 764 - 24       = 740  bytes of line... │
//	│ (pd_lower - 24) ÷ 4 = (764 - 24) ÷ 4 = 185  line pointers... │
//	└──────────────────────────────────────────────────────────────┘
//	▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏▏
//	    ↑ 764
//
//	PURPOSE
//	...
//
//	HOW IT WORKS
//	...
func explainPanel(e explanation, s pgpage.PageSummary, width int) string {
	wrap := lipgloss.NewStyle().Width(width)

	page := moreStyle.Render("ON THIS PAGE") + "\n" + panel("", explainBox(e, s, width-panelFrame), width, 0, ruleStyle)

	if e.mark != nil {
		if at, ok := e.mark(s.Header); ok {
			page += "\n" + pageStrip(s.Header, width, at)
		}
	}

	return strings.Join([]string{
		wrap.Render(valueStyle.Bold(true).Render(e.about)),
		page,
		moreStyle.Render("PURPOSE") + "\n" + wrap.Render(valueStyle.Render(e.purpose)),
		moreStyle.Render("HOW IT WORKS") + "\n" + wrap.Render(valueStyle.Render(e.how)),
	}, "\n\n")
}

// explainBox renders the working inside the explanation: where the field is
// on disk and what its bytes read as, the note, a rule, and the steps.
func explainBox(e explanation, s pgpage.PageSummary, width int) string {
	raw := headerBytes(s.Header)
	field := raw[e.offset : e.offset+e.size]

	top := []string{fieldStyle.Render(padRight(fmt.Sprintf("bytes %d-%d", e.offset, e.offset+e.size-1), fieldColumn)) +
		valueStyle.Render(byteList(field)) + littleEndian(field)}

	if e.note != "" {
		top = append(top, lipgloss.NewStyle().Width(width).Render(fieldStyle.Render(e.note)))
	}

	lines := append(top, ruleStyle.Render(strings.Repeat(borderHorizontal, width)))

	return strings.Join(append(lines, renderSteps(e.steps(s), width)...), "\n")
}

// littleEndian returns how the bytes of a 2 or 4-byte field read as one
// number, "  →  0x02fc = 764", or nothing for the 8-byte LSN, which is two
// numbers and has steps of its own.
func littleEndian(b []byte) string {
	var v uint32

	switch len(b) {
	case 2:
		v = uint32(binary.LittleEndian.Uint16(b))
	case 4:
		v = binary.LittleEndian.Uint32(b)
	default:
		return ""
	}

	return fieldStyle.Render("  →  ") + valueStyle.Render(fmt.Sprintf("0x%0*x = %d", 2*len(b), v, v)) +
		fieldStyle.Render(", read little-endian")
}

// renderSteps lines the steps up in columns: the rules, the rules with the
// numbers, the results and the meanings, each starting at the same cell in
// every step. A meaning longer than the room left wraps under itself, and a
// remark, a step with only a meaning, wraps to the whole width.
//
//	pd_lower - 24       = 764 - 24       = 740  bytes of line pointers
//	(pd_lower - 24) ÷ 4 = (764 - 24) ÷ 4 = 185  line pointers, 4 B each
func renderSteps(steps []step, width int) []string {
	// A step without numbers, such as "pd_prune_xid = 0", has its result
	// right after the rule, in the column where the others have numbers.
	columns := func(s step) (values, result string) {
		if s.values == "" {
			return s.result, ""
		}

		return s.values, s.result
	}

	var ruleW, valuesW, resultW int

	for _, s := range steps {
		values, result := columns(s)
		ruleW = max(ruleW, lipgloss.Width(s.rule))
		valuesW = max(valuesW, lipgloss.Width(values))
		resultW = max(resultW, lipgloss.Width(result))
	}

	var lines []string

	for _, s := range steps {
		if s.rule == "" && s.values == "" && s.result == "" {
			lines = append(lines, lipgloss.NewStyle().Width(width).Render(fieldStyle.Render(s.meaning)))
			continue
		}

		values, result := columns(s)

		// Each column is its text after "= ", padded to the widest, and the
		// last one is followed by two spaces before the meaning.
		var sum strings.Builder

		sum.WriteString(fieldStyle.Render(padRight(s.rule, ruleW+1)))
		sum.WriteString(valueStyle.Render(padRight("= "+values, valuesW+3)))

		if resultW > 0 {
			text := ""
			if result != "" {
				text = "= " + result
			}

			sum.WriteString(valueStyle.Render(padRight(text, resultW+3)))
		}

		sum.WriteString(" ")

		// The meaning wraps in the room right of the sum, indented under
		// itself, and takes the whole width when that room is too small.
		indent := lipgloss.Width(sum.String())
		room := width - indent

		if room < 16 {
			lines = append(lines, sum.String(), lipgloss.NewStyle().Width(width).Render(fieldStyle.Render(s.meaning)))
			continue
		}

		meaning := strings.Split(lipgloss.NewStyle().Width(room).Render(s.meaning), "\n")
		lines = append(lines, sum.String()+fieldStyle.Render(strings.TrimRight(meaning[0], " ")))

		for _, more := range meaning[1:] {
			lines = append(lines, strings.Repeat(" ", indent)+fieldStyle.Render(strings.TrimRight(more, " ")))
		}
	}

	return lines
}

// pageStrip draws the whole page as one row of width cells, as the page map
// draws a row, with the cell that holds byte at marked and its offset written
// under it:
//
//	█╱╱╱╱╱╱╱╱╎╎╎╎╎╎╎╎╎╎╎╎╎╎▒╲╲╲╲╲╲╲╲╲╲╲╲╲╲╲╲╲╲╲╲╲╲
//	                       ↑ 2472
//
// An offset can be PageSize, one past the last byte, as pd_special is on a
// heap page: it marks the last cell.
func pageStrip(h pgpage.PageHeader, width, at int) string {
	byteAt := min(max(at, 0), pgpage.PageSize-1)
	row := stripRow(pgpage.PageRegions(h), 0, pgpage.PageSize, width, highlights{{start: byteAt, end: byteAt + 1}})

	cell := byteAt * width / pgpage.PageSize

	// The label starts at the arrow, unless it would run past the edge;
	// then it ends at it, with the arrow on the right.
	label := fmt.Sprintf("↑ %d", at)
	if cell+lipgloss.Width(label) > width {
		label = fmt.Sprintf("%d ↑", at)
		cell = max(cell-lipgloss.Width(label)+1, 0)
	}

	return row + "\n" + strings.Repeat(" ", cell) + selectedStyle.Render(label)
}

// minExplainWidth is the narrowest an explanation panel can be and still
// hold a step and its meaning.
const minExplainWidth = 48

// explainStyles color the values of the list as the header panel colors
// them: the two boundaries in the colors of the regions they end and start.
var explainStyles = map[string]lipgloss.Style{
	"pd_lower": regionText(pgpage.RegionLinePointers),
	"pd_upper": regionText(pgpage.RegionTuples),
}

// explainColumn is the width of the field names of the list: the longest
// name, pd_pagesize_version, and a space.
var explainColumn = func() int {
	width := 0
	for _, e := range explanations {
		width = max(width, lipgloss.Width(e.field))
	}

	return width + 2
}()

// explainListWidth is the width of the list's content: the marker, the
// names and the widest value a header can have.
var explainListWidth = 2 + explainColumn + headerValueWidth

// explainList renders the page header as the header panel does, one field
// per line with the selected one marked, and what is derived from them below
// a rule. Only the stored fields can be selected: they are the ones that have
// bytes, and an explanation.
//
//	  pd_checksum          6769
//	  pd_flags             0x0000
//	> pd_lower             764
//	  pd_upper             2472
//	  ...
//	  ──────────────────────────────
//	  items                185
func explainList(summary pgpage.PageSummary, selected int) string {
	h := summary.Header
	lines := make([]string, 0, len(explanations))

	for i, e := range explanations {
		style, ok := explainStyles[e.field]
		if !ok {
			style = valueStyle
		}

		row := "  " + fieldStyle.Render(padRight(e.field, explainColumn))
		if i == selected {
			row = selectedStyle.Render("> " + padRight(e.field, explainColumn))
		}

		lines = append(lines, row+style.Render(e.value(h)))
	}

	indent := "  "
	rule := indent + ruleStyle.Render(strings.Repeat(borderHorizontal, explainColumn+headerValueWidth))

	lines = append(lines, "", rule, "")

	for _, row := range derivedRows(summary) {
		lines = append(lines, indent+fieldStyle.Render(padRight(row.name, explainColumn))+row.style.Render(row.value))
	}

	return strings.Join(lines, "\n")
}

// scrollLines returns the lines of an explanation that fit in rows lines,
// from line scroll on. The lines left out above or below are counted in a
// hint that takes the first or last row, with the key that shows them.
func scrollLines(lines []string, scroll, rows int) []string {
	if len(lines) <= rows || rows < 3 {
		return lines
	}

	scroll = min(max(scroll, 0), maxScroll(len(lines), rows))

	var out []string

	if scroll > 0 {
		out = append(out, moreStyle.Render(fmt.Sprintf("↑ %d more · PgUp", scroll)))
	}

	end := len(lines)
	if room := rows - len(out); end-scroll > room {
		end = scroll + room - 1 // one row for the hint below
	}

	out = append(out, lines[scroll:end]...)

	if end < len(lines) {
		out = append(out, moreStyle.Render(fmt.Sprintf("↓ %d more · PgDn", len(lines)-end)))
	}

	return out
}

// maxScroll returns how far n lines can scroll in rows rows: until the last
// line is in view below the hint at the top.
func maxScroll(n, rows int) int {
	if n <= rows {
		return 0
	}

	return n - rows + 1
}
