package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// The dump has one line per 16 bytes of the page, each with the same bytes
// hexdump -C shows, and the regions of the page the line falls in.
func TestHexLines(t *testing.T) {
	page := fixturePage(t, 0)
	lines := hexLines(page, nil)

	if len(lines) != pgpage.PageSize/pgpage.HexBytesPerLine {
		t.Fatalf("%d lines, want %d", len(lines), pgpage.PageSize/pgpage.HexBytesPerLine)
	}

	for i, dump := range pgpage.HexLines(page, 0) {
		// HexLine.String is the hexdump -C format: offset, bytes and text.
		// The view adds region names after it, and nothing else.
		if !strings.HasPrefix(flat(lines[i]), flat(dump.String())) {
			t.Fatalf("line %d:\n got %q\nwant it to start with %q", i, flat(lines[i]), flat(dump.String()))
		}
	}

	labels := map[int]string{
		0x00: "Header",
		0x01: "Header · Line ptrs", // 0x10-0x1f: the header ends at byte 24
		0x02: "Line ptrs",
		0x40: "Free",
		511:  "Tuples",
	}

	for line, want := range labels {
		if !strings.HasSuffix(lines[line], want) {
			t.Errorf("line %04x ends with %q, want %q", line*16, lines[line][len(lines[line])-20:], want)
		}
	}
}

// A page without a valid header gets no region names: there are no regions
// to name, only bytes.
func TestHexLinesNewPage(t *testing.T) {
	for i, line := range hexLines(make([]byte, pgpage.PageSize), nil) {
		if strings.Contains(line, "Header") || strings.Contains(line, "Free") {
			t.Fatalf("line %d of a new page names a region:\n%s", i, line)
		}
	}
}

// Bytes of the selection are drawn as one box, space included, and zero
// bytes apart from the rest.
func TestHexBytesClasses(t *testing.T) {
	withColor(t)

	line := pgpage.HexLines(fixturePage(t, 2), 0)[1] // 0x0010: #2's entry at 0x1c-0x1f
	hl := highlights{{start: 0x1c, end: 0x20, label: "line ptr #2"}}

	got := hexBytes(line, hl)

	if box := selectedByteStyle.Render("d8 9f 4a 00"); !strings.Contains(got, box) {
		t.Errorf("the selection is not drawn as one box %q:\n%q", box, got)
	}

	if zeros := zeroByteStyle.Render("00 00 00 00"); !strings.Contains(got, zeros) {
		t.Errorf("the zero bytes are not dimmed together as %q:\n%q", zeros, got)
	}

	// The gap in the middle of the line is never part of a box.
	if strings.Contains(got, selectedByteStyle.Render("00  00")) {
		t.Errorf("a box spans the middle gap:\n%q", got)
	}
}

// The line where the selection starts is marked, and only that one.
func TestLineLabelSelection(t *testing.T) {
	hl := highlights{{start: 0x1c, end: 0x40, label: "tuple #9"}} // spans three lines

	for _, offset := range []int{0x00, 0x10, 0x20, 0x30} {
		marked := strings.Contains(lineLabel(offset, nil, hl), "← tuple #9")

		if want := offset == 0x10; marked != want {
			t.Errorf("line %04x marked = %v, want %v", offset, marked, want)
		}
	}
}

func TestHexDetail(t *testing.T) {
	page := fixturePage(t, 2)

	tests := []struct {
		name string
		hl   highlights
		want []string
		gone []string
	}{
		{
			name: "short selection, spelled out whole",
			hl:   highlights{{start: 0x1c, end: 0x20, label: "line ptr #2"}},
			want: []string{"0x001C", "d8 9f 4a 00", "line ptr #2 · bytes 28-31 · 4 B", "highlighted bytes belong"},
			gone: []string{"…"},
		},
		{
			name: "long selection, first bytes only",
			hl:   highlights{{start: 8152, end: 8189, label: "tuple (2,2)"}},
			want: []string{"0x1FD8", "…", "tuple (2,2) · bytes 8152-8188 · 37 B"},
		},
		{
			name: "a line pointer and its tuple, one line each",
			hl: highlights{
				{start: 0x1c, end: 0x20, label: "line ptr #2"},
				{start: 8152, end: 8189, label: "tuple (2,2)", pointed: true},
			},
			want: []string{"line ptr #2 · bytes 28-31 · 4 B", "tuple (2,2) · bytes 8152-8188 · 37 B"},
			gone: []string{"highlighted bytes belong"},
		},
		{
			name: "nothing selected",
			want: []string{"nothing selected"},
			gone: []string{"0x", "→"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flat(hexDetail(page, tt.hl))

			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("detail does not contain %q:\n%s", want, got)
				}
			}

			for _, gone := range tt.gone {
				if strings.Contains(got, gone) {
					t.Errorf("detail contains %q:\n%s", gone, got)
				}
			}
		})
	}
}

// The page view opens the hex view on the header, and a page without one on
// nothing.
func TestHeaderHighlight(t *testing.T) {
	if got := headerHighlight(pgpage.SummarizePage(fixturePage(t, 0), 0)); len(got) != 1 ||
		got[0] != (highlight{end: 24, label: "page header"}) {
		t.Errorf("header highlight = %+v", got)
	}

	if got := headerHighlight(pgpage.SummarizePage(make([]byte, pgpage.PageSize), 0)); got != nil {
		t.Errorf("a new page highlights a header it does not have: %+v", got)
	}
}

// Both highlights are drawn in the dump, each in its shade, and each marks
// the line where it starts.
func TestHexLinesTwoHighlights(t *testing.T) {
	withColor(t)

	it := fixtureItems(t, 2)
	it.selected = selectedTuple2

	lines := hexLines(it.page, itemHighlights(it))

	if entry := lines[0x10/16]; !strings.Contains(entry, selectedByteStyle.Render("d8 9f 4a 00")) ||
		!strings.Contains(entry, "← line ptr #2") {
		t.Errorf("the entry is not drawn in the selected shade:\n%q", entry)
	}

	if tuple := lines[8152/16]; !strings.Contains(tuple, "48;5;133m") || !strings.Contains(tuple, "← tuple (2,2)") {
		t.Errorf("the tuple is not drawn in the pointed shade:\n%q", tuple)
	}
}

// The hex view scrolls with the keys of every other view, and closes with x
// as well as Esc; the keys that mean nothing on a dump are disabled.
func TestHexKeys(t *testing.T) {
	same := map[string][2]key.Binding{
		"Up":       {keys.Up, hexKeys.Up},
		"Down":     {keys.Down, hexKeys.Down},
		"PageUp":   {keys.PageUp, hexKeys.PageUp},
		"PageDown": {keys.PageDown, hexKeys.PageDown},
		"Home":     {keys.Home, hexKeys.Home},
		"End":      {keys.End, hexKeys.End},
		"Help":     {keys.Help, hexKeys.Help},
		"Quit":     {keys.Quit, hexKeys.Quit},
	}

	for name, pair := range same {
		if fmt.Sprint(pair[0].Keys()) != fmt.Sprint(pair[1].Keys()) {
			t.Errorf("%s: page view keys %v, hex view keys %v", name, pair[0].Keys(), pair[1].Keys())
		}
	}

	for _, k := range []string{"esc", "x"} {
		if !key.Matches(keyMsg(k), hexKeys.Back) {
			t.Errorf("%s does not close the hex view", k)
		}
	}

	for name, binding := range map[string]key.Binding{"Hex": hexKeys.Hex, "GoTo": hexKeys.GoTo, "Open": hexKeys.Open} {
		if binding.Enabled() {
			t.Errorf("%s is enabled in the hex view", name)
		}
	}
}

// Every line of the hex view fits the terminal.
func TestHexViewFits(t *testing.T) {
	for _, size := range [][2]int{{130, 26}, {90, 20}} {
		m := press(t, openItemsView(t, 2, size[0], size[1]), "x")

		for i, line := range strings.Split(strings.TrimSuffix(m.View(), "\n"), "\n") {
			if got := lipgloss.Width(line); got > size[0] {
				t.Errorf("%dx%d: line %d is %d cells wide", size[0], size[1], i, got)
			}
		}
	}
}
