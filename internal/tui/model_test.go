package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
