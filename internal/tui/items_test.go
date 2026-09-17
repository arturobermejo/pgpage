package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// fixtureItems returns the line pointers of a block of the fixture, loaded
// as the command loads them.
func fixtureItems(t *testing.T, block pgpage.BlockNumber) items {
	t.Helper()

	msg, ok := run(t, loadItems(openFixture(t), block)).(itemsMsg)
	if !ok {
		t.Fatal("loadItems did not return an itemsMsg")
	}

	return items{block: block, loaded: true, page: msg.page, header: msg.header, ids: msg.ids, err: msg.err}
}

// The command decodes the whole line pointer array of the page: the same
// counts heap_page_items() reports for the fixture.
func TestLoadItems(t *testing.T) {
	for block, want := range map[pgpage.BlockNumber]int{0: 185, 2: 180} {
		it := fixtureItems(t, block)

		if it.err != nil {
			t.Fatalf("block %d: %v", block, it.err)
		}

		if len(it.ids) != want {
			t.Errorf("block %d has %d line pointers, want %d", block, len(it.ids), want)
		}
	}
}

// A page that cannot be read comes back as an error, not as an empty page.
func TestLoadItemsReadError(t *testing.T) {
	msg, ok := run(t, loadItems(openFixture(t), 3)).(itemsMsg)
	if !ok || msg.err == nil || msg.block != 3 {
		t.Errorf("reading past the end returned %#v, want an error for block 3", msg)
	}
}

// The list shows each line pointer with its state and where it points, and
// what it cannot point to as dashes. Block 2 of the fixture has the four
// states of a heap page: normal, dead, redirected and, at 172, unused.
func TestItemList(t *testing.T) {
	it := fixtureItems(t, 2)
	it.selected = 9 // #10, the redirect

	list := itemList(it, 12)
	lines := strings.Split(list, "\n")

	want := map[int]string{
		1:  "#1 DEAD — —",
		2:  "#2 NORMAL 8152 37",
		10: "> #10 REDIRECT →168 —",
	}

	for line, text := range want {
		if got := strings.Join(strings.Fields(lines[line]), " "); got != text {
			t.Errorf("line %d = %q, want %q", line, got, text)
		}
	}

	for _, want := range []string{"# STATE OFFSET LEN", "↓ 168 more", stateCounts(it.ids)} {
		if !strings.Contains(strings.Join(strings.Fields(list), " "), want) {
			t.Errorf("list does not contain %q:\n%s", want, list)
		}
	}
}

// The column titles sit over the columns they name.
func TestItemListColumnsAlign(t *testing.T) {
	it := fixtureItems(t, 2)
	lines := strings.Split(itemList(it, 12), "\n")

	title, row := lines[0], lines[2] // "#2 NORMAL 8152 37"

	for _, pair := range [][2]string{{"#", "#2"}, {"STATE", "NORMAL"}} {
		if strings.Index(title, pair[0]) != strings.Index(row, pair[1]) {
			t.Errorf("%q starts at %d but %q at %d:\n%s\n%s",
				pair[0], strings.Index(title, pair[0]), pair[1], strings.Index(row, pair[1]), title, row)
		}
	}
}

func TestStateCounts(t *testing.T) {
	ids := func(states ...pgpage.ItemState) []pgpage.ItemID {
		out := make([]pgpage.ItemID, len(states))
		for i, s := range states {
			out[i] = pgpage.ItemID(uint32(s) << 15)
		}

		return out
	}

	tests := []struct {
		ids  []pgpage.ItemID
		want string
	}{
		{ids: nil, want: ""},
		{ids: ids(pgpage.ItemNormal, pgpage.ItemNormal), want: "2 NORMAL"},
		{
			ids:  ids(pgpage.ItemUnused, pgpage.ItemDead, pgpage.ItemNormal, pgpage.ItemRedirect, pgpage.ItemDead),
			want: "1 NORMAL · 1 REDIRECT · 2 DEAD · 1 UNUSED",
		},
	}

	for _, tt := range tests {
		if got := stateCounts(tt.ids); got != tt.want {
			t.Errorf("stateCounts = %q, want %q", got, tt.want)
		}
	}
}

// The panel shows the fields of the line pointer and how its 32-bit word
// splits into them: the three fields rebuild the raw word.
func TestItemPanel(t *testing.T) {
	it := fixtureItems(t, 2)
	it.selected = 1 // #2, NORMAL at 8152, 37 bytes

	panel := itemPanel(it)
	id := it.ids[1]

	raw := uint32(id.Offset()) | uint32(id.State())<<15 | uint32(id.Length())<<17

	for _, want := range []string{
		"lp_off 8152",
		"lp_len 37",
		"lp_flags LP_NORMAL",
		"entry at 0x001C", // 24 + (2-1)*4
		fmt.Sprintf("raw 0x%08X", raw),
		"off bits 0-14 = 8152",
		"flags bits 15-16 = 1",
		"len bits 17-31 = 37",
	} {
		if !strings.Contains(strings.Join(strings.Fields(panel), " "), want) {
			t.Errorf("panel does not contain %q:\n%s", want, panel)
		}
	}

	if itemTitle(it) != "LINE POINTER #2" {
		t.Errorf("title = %q, want LINE POINTER #2", itemTitle(it))
	}
}

// A redirect names the line pointer it leads to.
func TestItemPanelRedirect(t *testing.T) {
	it := fixtureItems(t, 2)
	it.selected = 9 // #10 → #168

	if panel := itemPanel(it); !strings.Contains(strings.Join(strings.Fields(panel), " "), "redirect to #168") {
		t.Errorf("panel does not name the target:\n%s", panel)
	}
}

// A line pointer that breaks the page's rules is flagged in the list and
// explained in the panel, wrapped to the panel's width.
func TestItemPanelInvalid(t *testing.T) {
	it := fixtureItems(t, 0)
	it.ids[0] |= 1 // #1: offset 8152 → 8153, no longer aligned

	if row := strings.Split(itemList(it, 5), "\n")[1]; !strings.HasSuffix(strings.TrimSpace(row), "!") {
		t.Errorf("the list does not flag the broken line pointer:\n%q", row)
	}

	panel := itemPanel(it)

	if !strings.Contains(strings.Join(strings.Fields(panel), " "), "not aligned to 8") {
		t.Errorf("panel does not explain the problem:\n%s", panel)
	}

	for _, line := range strings.Split(panel, "\n") {
		if got := lipgloss.Width(line); got > fieldColumn+itemPanelWidth {
			t.Errorf("a line of the panel is %d cells wide:\n%q", got, line)
		}
	}
}

// Only a line pointer with a tuple highlights bytes on the map.
func TestItemHighlight(t *testing.T) {
	it := fixtureItems(t, 2)

	tests := []struct {
		name     string
		selected int
		want     highlight
	}{
		{name: "normal", selected: 1, want: highlight{start: 8152, end: 8189, label: "tuple (2,2)"}},
		{name: "dead", selected: 0},
		{name: "redirect", selected: 9},
	}

	for _, tt := range tests {
		it.selected = tt.selected

		if got := itemHighlight(it); got != tt.want {
			t.Errorf("%s: highlight = %+v, want %+v", tt.name, got, tt.want)
		}
	}

	broken := fixtureItems(t, 0)
	broken.ids[0] |= 1

	if got := itemHighlight(broken); got != (highlight{}) {
		t.Errorf("a line pointer that fails its checks highlights %+v", got)
	}
}
