package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// fixtureHeap is the relation file shared with the pgpage package tests.
const fixtureHeap = "../../testdata/heap_small"

// openRelation opens a relation of pages zeroed pages, for the tests that
// need more blocks than the fixture has. Only its size matters here: the
// navigator lists block numbers, it does not read pages yet.
func openRelation(t *testing.T, pages int) *pgpage.Relation {
	t.Helper()

	path := filepath.Join(t.TempDir(), "relation")
	if err := os.WriteFile(path, make([]byte, pages*pgpage.PageSize), 0o600); err != nil {
		t.Fatal(err)
	}

	rel, err := pgpage.OpenRelation(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { rel.Close() })

	return rel
}

// openFixture opens the fixture relation and closes it when the test ends.
func openFixture(t *testing.T) *pgpage.Relation {
	t.Helper()

	rel, err := pgpage.OpenRelation(fixtureHeap)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { rel.Close() })

	return rel
}

// fixturePage returns one page of the fixture relation.
func fixturePage(t *testing.T, block pgpage.BlockNumber) []byte {
	t.Helper()

	page, err := openFixture(t).ReadPage(block)
	if err != nil {
		t.Fatal(err)
	}

	return page
}

// The first screen names the relation and its size, so the user knows what
// they opened.
func TestModelView(t *testing.T) {
	m, _ := New(openFixture(t)).Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	view := m.View()

	for _, want := range []string{fixtureHeap, "3 pages", "24.0 KB", "blk 0/2", "PAGES", "> 0", "q quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("view does not contain %q:\n%s", want, view)
		}
	}
}

// Before the first tea.WindowSizeMsg the size is unknown, and the view must
// still draw something.
func TestModelViewWithoutSize(t *testing.T) {
	view := New(openFixture(t)).View()

	if !strings.Contains(view, fixtureHeap) || strings.Contains(view, "blk") {
		t.Errorf("view without a size:\n%s", view)
	}
}

// The first thing the program does is ask for the pages it is about to
// list, in a command: the screen is drawn before the disk answers.
func TestModelInit(t *testing.T) {
	m := New(openFixture(t))

	msg, ok := run(t, m.Init()).(summariesMsg)
	if !ok {
		t.Fatalf("Init did not return a command that loads summaries")
	}

	if len(msg.summaries) != 3 {
		t.Errorf("%d summaries, want the 3 pages of the fixture", len(msg.summaries))
	}

	if len(m.summaries) != 0 {
		t.Errorf("the command wrote %d summaries into the model, want 0", len(m.summaries))
	}
}

func TestModelUpdate(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.Msg
		quit bool
	}{
		{name: "q quits", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}, quit: true},
		{name: "esc quits", msg: tea.KeyMsg{Type: tea.KeyEsc}, quit: true},
		{name: "ctrl+c quits", msg: tea.KeyMsg{Type: tea.KeyCtrlC}, quit: true},
		{name: "an unbound key does nothing", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")}},
		{name: "an unknown message does nothing", msg: struct{}{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(openFixture(t))

			next, cmd := m.Update(tt.msg)

			if !sameState(next, m) {
				t.Errorf("Update changed the model: %+v, want %+v", next, m)
			}

			if quit := isQuit(cmd); quit != tt.quit {
				t.Errorf("quit = %v, want %v", quit, tt.quit)
			}
		})
	}
}

// sameState reports whether a model went through Update unchanged. Model
// cannot be compared with ==, because the cache it carries is a map.
func sameState(next tea.Model, m Model) bool {
	got, ok := next.(Model)

	return ok && got.rel == m.rel && got.block == m.block && got.top == m.top &&
		got.width == m.width && got.height == m.height && len(got.summaries) == len(m.summaries)
}

// A resize is remembered, because the views are drawn to that width. A taller
// window also shows more pages, so it asks for the ones it does not have.
func TestModelResize(t *testing.T) {
	next, cmd := New(openFixture(t)).Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	m, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}

	if m.width != 100 || m.height != 40 {
		t.Errorf("size = %dx%d, want 100x40", m.width, m.height)
	}

	if _, ok := run(t, cmd).(summariesMsg); !ok {
		t.Error("a resize did not ask for the pages it now shows")
	}
}

// isQuit reports whether cmd is tea.Quit. Commands are functions, which are
// not comparable, so the only way to tell them apart is to run it and look at
// the message it produces.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}

	_, ok := cmd().(tea.QuitMsg)

	return ok
}

// Run draws the relation and returns once the user presses q.
//
// It runs in another goroutine with a deadline: a program that does not quit
// would otherwise block the test until the whole package times out.
func TestRun(t *testing.T) {
	var out bytes.Buffer

	rel := openFixture(t)
	done := make(chan error, 1)

	go func() { done <- Run(rel, strings.NewReader("q"), &out) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(out.String(), fixtureHeap) {
			t.Errorf("output does not name the relation:\n%q", out.String())
		}
	case <-time.After(10 * time.Second):
		// Do not read out here: the program still writes to it.
		t.Fatal("Run did not return after q")
	}
}

// key returns the message Bubble Tea sends for a key name, the same names
// Update matches on.
func key(name string) tea.KeyMsg {
	types := map[string]tea.KeyType{
		"up":     tea.KeyUp,
		"down":   tea.KeyDown,
		"pgup":   tea.KeyPgUp,
		"pgdown": tea.KeyPgDown,
		"home":   tea.KeyHome,
		"end":    tea.KeyEnd,
	}

	if t, ok := types[name]; ok {
		return tea.KeyMsg{Type: t}
	}

	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

// press sends the keys to the model one at a time, as the runtime does.
func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()

	for _, name := range keys {
		next, _ := m.Update(key(name))

		m, _ = next.(Model)
	}

	return m
}

// The keys of the navigator move the selection and scroll the list. With a
// height of 16 the window holds 10 rows.
func TestModelNavigation(t *testing.T) {
	tests := []struct {
		name  string
		keys  []string
		block pgpage.BlockNumber
		top   pgpage.BlockNumber
	}{
		{name: "no keys", block: 0, top: 0},
		{name: "down twice", keys: []string{"down", "down"}, block: 2, top: 0},
		{name: "j and k are the same keys", keys: []string{"j", "j", "k"}, block: 1, top: 0},
		{name: "up stops at the first block", keys: []string{"up", "up"}, block: 0, top: 0},
		{name: "the list scrolls when the selection reaches the bottom", keys: repeat("down", 11), block: 11, top: 2},
		{name: "end goes to the last block", keys: []string{"end"}, block: 19, top: 10},
		{name: "home comes back", keys: []string{"end", "home"}, block: 0, top: 0},
		{name: "pgdown moves a windowful", keys: []string{"pgdown"}, block: 10, top: 1},
		{name: "pgdown stops at the last block", keys: []string{"pgdown", "pgdown", "pgdown"}, block: 19, top: 10},
		{name: "pgup from the end", keys: []string{"end", "pgup"}, block: 9, top: 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(openRelation(t, 20))
			m.width, m.height = 80, 16

			m = press(t, m, tt.keys...)

			if m.block != tt.block || m.top != tt.top {
				t.Errorf("block = %d, top = %d; want %d and %d", m.block, m.top, tt.block, tt.top)
			}

			if m.block < m.top || uint64(m.block) >= uint64(m.top)+uint64(m.visibleRows()) {
				t.Errorf("block %d is outside the window [%d, %d)", m.block, m.top, int(m.top)+m.visibleRows())
			}
		})
	}
}

// repeat returns the key n times.
func repeat(name string, n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = name
	}

	return keys
}

// The screen shows the selection moving, not just the model.
func TestModelNavigationView(t *testing.T) {
	m := New(openRelation(t, 20))
	m.width, m.height = 80, 16

	m.width = 200 // wide enough for the long path of a temporary directory

	view := press(t, m, "end").View()

	if !strings.Contains(view, "> 19") || !strings.Contains(view, "blk 19/19") {
		t.Errorf("view after end:\n%s", view)
	}

	if strings.Contains(view, "more") {
		t.Errorf("the last window has nothing below it:\n%s", view)
	}
}

// A relation with no complete page has nothing to select, and the keys must
// not take the selection anywhere.
func TestModelNavigationEmptyRelation(t *testing.T) {
	m := New(openRelation(t, 0))
	m.width, m.height = 80, 16

	m = press(t, m, "down", "end", "pgdown")

	if m.block != 0 || m.top != 0 {
		t.Errorf("block = %d, top = %d; want 0 and 0", m.block, m.top)
	}
}

// A shorter terminal holds fewer rows, so the list scrolls to keep the
// selection on screen.
func TestModelResizeScrolls(t *testing.T) {
	m := New(openRelation(t, 20))
	m.width, m.height = 80, 16

	m = press(t, m, "end") // block 19, window 10..19

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 9}) // 3 rows
	m, _ = next.(Model)

	if m.visibleRows() != 3 {
		t.Fatalf("visibleRows = %d, want 3", m.visibleRows())
	}

	if m.block != 19 || m.top != 17 {
		t.Errorf("block = %d, top = %d; want 19 and 17", m.block, m.top)
	}
}

// Summaries reach the model only through a message, and the navigator then
// shows them.
func TestModelSummariesMsg(t *testing.T) {
	m := New(openFixture(t))
	m.width, m.height = 80, 16

	if strings.Contains(m.View(), "items") {
		t.Fatalf("the list shows summaries before reading any page:\n%s", m.View())
	}

	next, _ := m.Update(run(t, m.Init()))

	m, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}

	if len(m.summaries) != 3 {
		t.Fatalf("%d summaries cached, want 3", len(m.summaries))
	}

	view := m.View()

	for _, want := range []string{"185 items", "21% free", "OK", "180 items"} {
		if !strings.Contains(view, want) {
			t.Errorf("view does not contain %q:\n%s", want, view)
		}
	}
}

// A page is read once. Moving over pages the cache already holds asks for
// nothing, which is what keeps navigation instant.
func TestModelRefreshUsesTheCache(t *testing.T) {
	m := New(openFixture(t))
	m.width, m.height = 80, 16

	next, _ := m.Update(run(t, m.Init()))
	m, _ = next.(Model)

	for _, keys := range [][]string{{"down"}, {"end"}, {"home"}, {"pgdown"}} {
		m = press(t, m, keys...)

		if cmd := m.refresh(); cmd != nil {
			t.Errorf("after %v the model reads pages it already has", keys)
		}
	}
}

// loaded returns a model of the fixture with every page already read.
func loaded(t *testing.T, width, height int) Model {
	t.Helper()

	m := New(openFixture(t))
	m.width, m.height = width, height

	next, _ := m.Update(run(t, m.Init()))
	m, _ = next.(Model)

	return m
}

// The header panel sits next to the navigator and describes the selected
// page, not the first one.
func TestModelBodyPanel(t *testing.T) {
	m := loaded(t, 120, 24)

	if !strings.Contains(m.View(), "PAGE HEADER") {
		t.Fatalf("the body has no header panel:\n%s", m.View())
	}

	first := lineWith(t, m.View(), "pd_lsn")

	view := press(t, m, "end").View()

	if second := lineWith(t, view, "pd_lsn"); second == first {
		t.Errorf("the panel still shows the LSN of the first page: %q", second)
	}

	// Both panels are drawn on the same lines, side by side.
	if !strings.Contains(view, "PAGES") || !strings.Contains(view, "PAGE HEADER") {
		t.Errorf("view after end:\n%s", view)
	}
}

// lineWith returns the first line of the view that contains want.
func lineWith(t *testing.T, view, want string) string {
	t.Helper()

	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, want) {
			return line
		}
	}

	t.Fatalf("no line contains %q:\n%s", want, view)

	return ""
}

// A terminal too narrow for both panels keeps the navigator, which is the
// one the keys act on.
func TestModelBodyNarrow(t *testing.T) {
	m := loaded(t, 40, 24)

	view := m.View()

	if strings.Contains(view, "PAGE HEADER") {
		t.Errorf("the panel does not fit but was drawn:\n%s", view)
	}

	if !strings.Contains(view, "PAGES") {
		t.Errorf("the navigator is missing:\n%s", view)
	}
}

// Whatever the terminal, the screen never has more lines than it, which
// would scroll it and break the drawing.
func TestModelViewFitsTheTerminal(t *testing.T) {
	for _, height := range []int{8, 12, 20, 24, 40} {
		m := loaded(t, 120, height)

		view := press(t, m, "end").View()

		if lines := strings.Count(strings.TrimSuffix(view, "\n"), "\n") + 1; lines > height {
			t.Errorf("height %d: the screen has %d lines:\n%s", height, lines, view)
		}
	}
}

// The page map sits between the navigator and the header panel when there
// is room for the three, and is the first panel dropped when there is not.
func TestModelBodyMap(t *testing.T) {
	tests := []struct {
		name  string
		width int
		want  []string
		gone  []string
	}{
		{name: "three panels", width: 140, want: []string{"PAGES", "PAGE 0 — 8192 BYTES", "PAGE HEADER"}},
		{name: "no room for the map", width: 70, want: []string{"PAGES", "PAGE HEADER"}, gone: []string{"BYTES"}},
		{name: "only the navigator", width: 40, want: []string{"PAGES"}, gone: []string{"BYTES", "PAGE HEADER"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := loaded(t, tt.width, 24).View()

			for _, want := range tt.want {
				if !strings.Contains(view, want) {
					t.Errorf("view does not contain %q:\n%s", want, view)
				}
			}

			for _, gone := range tt.gone {
				if strings.Contains(view, gone) {
					t.Errorf("view contains %q, which does not fit:\n%s", gone, view)
				}
			}

			for _, line := range strings.Split(view, "\n") {
				if got := lipgloss.Width(line); got > tt.width {
					t.Fatalf("a line is %d cells wide, the terminal is %d:\n%q", got, tt.width, line)
				}
			}
		})
	}
}

// The map follows the selection, like the header panel.
func TestModelBodyMapFollowsSelection(t *testing.T) {
	view := press(t, loaded(t, 140, 24), "end").View()

	if !strings.Contains(view, "PAGE 2 — 8192 BYTES") {
		t.Errorf("the map still shows another page:\n%s", view)
	}
}

// openMixed opens a relation whose three pages are one valid page, one new
// page and one with a corrupt header.
func openMixed(t *testing.T) *pgpage.Relation {
	t.Helper()

	bad := make([]byte, pgpage.PageSize)
	bad[12], bad[13] = 0xF4, 0x01 // pd_lower = 500
	bad[14], bad[15] = 0x64, 0x00 // pd_upper = 100
	bad[16], bad[17] = 0x00, 0x20 // pd_special = 8192
	bad[18], bad[19] = 0x04, 0x20 // page size 8192, layout version 4

	data := fixturePage(t, 0)
	data = append(data, make([]byte, pgpage.PageSize)...)
	data = append(data, bad...)

	path := filepath.Join(t.TempDir(), "mixed")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	rel, err := pgpage.OpenRelation(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { rel.Close() })

	return rel
}

// A page without a layout replaces the map and the header panel with one
// panel that explains it.
func TestModelBodyStatusPanel(t *testing.T) {
	m := New(openMixed(t))
	m.width, m.height = 120, 22

	next, _ := m.Update(run(t, m.Init()))
	m, _ = next.(Model)

	tests := []struct {
		name string
		keys []string
		want []string
		gone []string
	}{
		{
			name: "valid page",
			want: []string{"PAGE 0 — 8192 BYTES", "PAGE HEADER"},
			gone: []string{"NEW PAGE", "⚠"},
		},
		{
			name: "new page",
			keys: []string{"down"},
			want: []string{"PAGE 1", "NEW PAGE", "all zeroes"},
			gone: []string{"PAGE HEADER", "BYTES", "⚠"},
		},
		{
			name: "invalid page",
			keys: []string{"end"},
			want: []string{"BLOCK 2", "⚠ Invalid page header", "lower=500 upper=100"},
			gone: []string{"PAGE HEADER", "BYTES", "NEW PAGE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := press(t, m, tt.keys...).View()

			for _, want := range tt.want {
				if !strings.Contains(view, want) {
					t.Errorf("view does not contain %q:\n%s", want, view)
				}
			}

			for _, gone := range tt.gone {
				if strings.Contains(view, gone) {
					t.Errorf("view contains %q:\n%s", gone, view)
				}
			}
		})
	}
}

// The navigator still works on pages that cannot be parsed: they are where
// the user most needs to move around.
func TestModelNavigatesBrokenPages(t *testing.T) {
	m := New(openMixed(t))
	m.width, m.height = 120, 22

	next, _ := m.Update(run(t, m.Init()))
	m, _ = next.(Model)

	m = press(t, m, "end", "up", "up")

	if m.block != 0 {
		t.Errorf("block = %d after walking back from a broken page, want 0", m.block)
	}
}
