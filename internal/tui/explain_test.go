package tui

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// fixtureHeader returns the page header of a block of the fixture.
func fixtureHeader(t *testing.T, block pgpage.BlockNumber) pgpage.PageHeader {
	t.Helper()

	h, err := pgpage.ParsePageHeader(fixturePage(t, block))
	if err != nil {
		t.Fatal(err)
	}

	return h
}

// explanationOf returns the explanation of a field by its name.
func explanationOf(t *testing.T, field string) explanation {
	t.Helper()

	for _, e := range explanations {
		if e.field == field {
			return e
		}
	}

	t.Fatalf("no explanation of %s", field)

	return explanation{}
}

// flat collapses the runs of spaces, so that a test can look for a line
// without counting the padding that aligns it.
func flatText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// The explanations cover PageHeaderData byte by byte, in the order the
// fields are stored: each one starts where the one before ends, and the
// last one ends at the size of the header.
func TestExplanationsCoverTheHeader(t *testing.T) {
	next := 0

	for _, e := range explanations {
		if e.offset != next {
			t.Errorf("%s starts at byte %d, want %d", e.field, e.offset, next)
		}

		if e.about == "" || e.purpose == "" || e.how == "" || e.value == nil {
			t.Errorf("%s does not say what it is, why it exists and how it works", e.field)
		}

		next = e.offset + e.size
	}

	if next != pgpage.PageHeaderSize {
		t.Errorf("the fields end at byte %d, want %d", next, pgpage.PageHeaderSize)
	}
}

// The bytes each explanation points at hold the value it shows: reading
// them again from the page gives the same number the header parser gave.
func TestExplanationsReadTheirBytes(t *testing.T) {
	page := fixturePage(t, 2)
	h := fixtureHeader(t, 2)

	le := binary.LittleEndian

	for _, e := range explanations {
		raw := page[e.offset : e.offset+e.size]

		var want string

		switch e.field {
		case "pd_lsn":
			want = pgpage.LSN(uint64(le.Uint32(raw[:4]))<<32 | uint64(le.Uint32(raw[4:]))).String()
		case "pd_flags":
			want = fmt.Sprintf("%#04x", le.Uint16(raw))
		case "pd_prune_xid":
			want = fmt.Sprint(le.Uint32(raw))
		default:
			want = fmt.Sprint(le.Uint16(raw))
		}

		if got := e.value(h); got != want {
			t.Errorf("%s = %s, but its bytes %x say %s", e.field, got, raw, want)
		}
	}
}

// The working of pd_lower and pd_upper arrives at what heap_page_items and
// page_header() report for block 0: 185 line pointers and 1708 bytes free.
func TestExplainSteps(t *testing.T) {
	h := fixtureHeader(t, 0)

	tests := map[string][]string{
		"pd_lower": {
			"PageHeaderData 24 bytes",
			"pd_lower 764 bytes",
			"764 - 24 = 740 bytes used by line pointers",
			"740 ÷ 4 = 185 line pointers (ItemIdData is 4 B)",
		},
		"pd_upper": {
			"2472 - 764 = 1708 bytes free",
			"1708 ÷ 8192 = 21 % of the page",
		},
		"pd_special":          {"8192 - 8192 = 0 bytes of special space"},
		"pd_pagesize_version": {"8192 | 4 = 8196 as stored"},
		"pd_flags":            {"0x0000 no flag set", "HAS_FREE_LINES not set"},
		"pd_prune_xid":        {"0 nothing on the page is known to be prunable"},
	}

	for field, lines := range tests {
		box := flatText(explainBox(explanationOf(t, field), h, 60))

		for _, want := range lines {
			if !strings.Contains(box, want) {
				t.Errorf("%s does not say %q:\n%s", field, want, box)
			}
		}
	}

	// Block 2 has a free line pointer, and its flags say so.
	if box := flatText(explainBox(explanationOf(t, "pd_flags"), fixtureHeader(t, 2), 60)); !strings.Contains(box, "0x0001 HAS_FREE_LINES") ||
		!strings.Contains(box, "HAS_FREE_LINES set") {
		t.Errorf("pd_flags of block 2:\n%s", box)
	}
}

// The list shows the fields of the header with their values, the selected
// one marked, and below a rule what the header panel derives from them.
func TestExplainList(t *testing.T) {
	summary := pgpage.SummarizePage(fixturePage(t, 2), 2)
	list := explainList(summary, explainLower)
	lines := strings.Split(list, "\n")

	for i, e := range explanations {
		marker := "  "
		if i == explainLower {
			marker = "> "
		}

		if !strings.HasPrefix(lines[i], marker) || flatText(lines[i]) != flatText(marker+e.field+" "+e.value(summary.Header)) {
			t.Errorf("line %d = %q, want %s marked %q", i, lines[i], e.field, marker)
		}
	}

	for _, want := range []string{"line pointers 180", "free space 1728 B", "flags decoded HAS_FREE_LINES", "status OK"} {
		if !strings.Contains(flatText(list), want) {
			t.Errorf("the list does not show %q:\n%s", want, list)
		}
	}

	// The values start in one column, however long the name.
	for _, line := range lines[:len(explanations)] {
		if got := column(line, strings.Fields(line)[len(strings.Fields(line))-1]); got != 2+explainColumn {
			t.Errorf("the value of %q starts at %d, want %d", line, got, 2+explainColumn)
		}
	}

	if got := maxWidth(lines); got > explainListWidth {
		t.Errorf("the list is %d cells wide, want at most %d", got, explainListWidth)
	}
}

// The equals signs of the sums line up, and so do the meanings after them.
func TestRenderStepsAlign(t *testing.T) {
	lines := renderSteps([]step{
		{"764 - 24 = 740", "bytes used by line pointers"},
		{"740 ÷ 4 = 185", "line pointers"},
		{"", "a remark"},
	}, 60)

	if column(lines[0], "=") != column(lines[1], "=") {
		t.Errorf("the equals signs do not line up:\n%s\n%s", lines[0], lines[1])
	}

	if column(lines[0], "bytes") != column(lines[1], "line") {
		t.Errorf("the meanings do not line up:\n%s\n%s", lines[0], lines[1])
	}

	if strings.TrimRight(lines[2], " ") != "a remark" {
		t.Errorf("a remark is drawn as %q", lines[2])
	}
}

// column returns the cell where sub starts in line. strings.Index counts
// bytes, and "÷" takes two.
func column(line, sub string) int {
	return lipgloss.Width(line[:strings.Index(line, sub)])
}

// The strip is exactly as wide as asked, and the arrow under it points at
// the cell that holds the offset.
func TestPageStrip(t *testing.T) {
	h := fixtureHeader(t, 0)

	const width = 50

	for _, at := range []int{int(h.Lower), int(h.Upper), int(h.Special)} {
		lines := strings.Split(pageStrip(h, width, at), "\n")

		if got := lipgloss.Width(lines[0]); got != width {
			t.Errorf("offset %d: the strip is %d cells, want %d", at, got, width)
		}

		if got := lipgloss.Width(lines[1]); got > width {
			t.Errorf("offset %d: the label is %d cells, past the strip", at, got)
		}

		cell := min(at, pgpage.PageSize-1) * width / pgpage.PageSize
		strip := []rune(lines[0])

		if strip[cell] != []rune(highlightGlyph)[0] {
			t.Errorf("offset %d: cell %d is %q, want the highlight:\n%s", at, cell, strip[cell], lines[0])
		}

		if arrow := column(lines[1], "↑"); arrow != cell {
			t.Errorf("offset %d: the arrow is under cell %d, want %d:\n%s\n%s", at, arrow, cell, lines[0], lines[1])
		}

		if !strings.Contains(lines[1], fmt.Sprint(at)) {
			t.Errorf("offset %d is not written under the strip: %q", at, lines[1])
		}
	}
}

// Every line of an explanation fits the panel it is drawn in.
func TestExplainPanelFits(t *testing.T) {
	h := fixtureHeader(t, 2)

	for _, width := range []int{minExplainWidth - panelFrame, 60, 120} {
		for _, e := range explanations {
			for _, line := range strings.Split(explainPanel(e, h, width), "\n") {
				if got := lipgloss.Width(line); got > width {
					t.Errorf("%s at width %d: a line is %d cells:\n%s", e.field, width, got, line)
				}
			}
		}
	}
}

// Explain mode moves with the keys of the page view, and Tab too; e and Esc
// close it, and x shows the bytes as everywhere else.
func TestExplainKeys(t *testing.T) {
	for name, pair := range map[string][2]key.Binding{
		"Home": {keys.Home, explainKeys.Home},
		"End":  {keys.End, explainKeys.End},
		"Hex":  {keys.Hex, explainKeys.Hex},
		"Help": {keys.Help, explainKeys.Help},
		"Quit": {keys.Quit, explainKeys.Quit},
	} {
		if fmt.Sprint(pair[0].Keys()) != fmt.Sprint(pair[1].Keys()) {
			t.Errorf("%s: page view keys %v, explain keys %v", name, pair[0].Keys(), pair[1].Keys())
		}
	}

	matches := map[string]key.Binding{
		"up": explainKeys.Up, "k": explainKeys.Up, "shift+tab": explainKeys.Up,
		"down": explainKeys.Down, "j": explainKeys.Down, "tab": explainKeys.Down,
		"e": explainKeys.Back, "esc": explainKeys.Back,
		"pgup": explainKeys.PageUp, "pgdown": explainKeys.PageDown,
	}

	for k, binding := range matches {
		if !key.Matches(keyMsg(k), binding) {
			t.Errorf("%s does not trigger %q", k, binding.Help().Desc)
		}
	}

	for name, binding := range map[string]key.Binding{
		"GoTo": explainKeys.GoTo, "Open": explainKeys.Open, "Explain": explainKeys.Explain,
	} {
		if binding.Enabled() {
			t.Errorf("%s is enabled in explain mode", name)
		}
	}
}

// An explanation taller than its panel shows what fits, and says how many
// lines are left above and below, which the rows it says it on do not hide:
// scrolling to the end shows the last line.
func TestScrollLines(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}

	tests := []struct {
		scroll      int
		first, last string
	}{
		{scroll: 0, first: "line 0", last: "↓ 11 more · PgDn"},
		{scroll: 5, first: "↑ 5 more · PgUp", last: "↓ 7 more · PgDn"},
		{scroll: maxScroll(20, 10), first: "↑ 11 more · PgUp", last: "line 19"},
		{scroll: 99, first: "↑ 11 more · PgUp", last: "line 19"},
		{scroll: -3, first: "line 0", last: "↓ 11 more · PgDn"},
	}

	for _, tt := range tests {
		out := scrollLines(lines, tt.scroll, 10)

		if len(out) != 10 || out[0] != tt.first || out[9] != tt.last {
			t.Errorf("scroll %d: %d lines, %q … %q, want 10, %q … %q", tt.scroll, len(out), out[0], out[len(out)-1], tt.first, tt.last)
		}

		// The lines in view are consecutive: none skipped behind a hint.
		var numbers []int

		for _, line := range out {
			var n int
			if _, err := fmt.Sscanf(line, "line %d", &n); err == nil {
				numbers = append(numbers, n)
			}
		}

		for i := 1; i < len(numbers); i++ {
			if numbers[i] != numbers[i-1]+1 {
				t.Errorf("scroll %d skips from line %d to %d", tt.scroll, numbers[i-1], numbers[i])
			}
		}

		// What the hints count plus what is in view is every line.
		hidden := 0

		for _, line := range []string{out[0], out[9]} {
			var n int
			if _, err := fmt.Sscanf(strings.TrimLeft(line, "↑↓ "), "%d more", &n); err == nil {
				hidden += n
			}
		}

		if hidden+len(numbers) != len(lines) {
			t.Errorf("scroll %d: %d hidden and %d shown, want %d lines", tt.scroll, hidden, len(numbers), len(lines))
		}
	}

	if short := scrollLines(lines[:5], 3, 10); len(short) != 5 {
		t.Errorf("lines that fit are not scrolled: %q", short)
	}
}
