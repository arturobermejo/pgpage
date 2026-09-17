package tui

import (
	"bytes"
	"fmt"
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

	// The bottom line is cut to the width of the terminal, so it may not
	// show every key.
	for _, want := range []string{fixtureHeap, "3 pages", "24.0 KB", "blk 0/2", "PAGES", "> 0", "next page"} {
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

	go func() { done <- Run(rel, 2, strings.NewReader("q"), &out) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(out.String(), fixtureHeap) {
			t.Errorf("output does not name the relation:\n%q", out.String())
		}

		if !strings.Contains(out.String(), "> 2") {
			t.Errorf("the explorer did not start on block 2:\n%q", out.String())
		}
	case <-time.After(10 * time.Second):
		// Do not read out here: the program still writes to it.
		t.Fatal("Run did not return after q")
	}
}

// keyMsg returns the message Bubble Tea sends for a key name, the same names
// the bindings are declared with.
func keyMsg(name string) tea.KeyMsg {
	types := map[string]tea.KeyType{
		"up":        tea.KeyUp,
		"down":      tea.KeyDown,
		"pgup":      tea.KeyPgUp,
		"pgdown":    tea.KeyPgDown,
		"home":      tea.KeyHome,
		"end":       tea.KeyEnd,
		"enter":     tea.KeyEnter,
		"backspace": tea.KeyBackspace,
		"esc":       tea.KeyEsc,
		"tab":       tea.KeyTab,
		"shift+tab": tea.KeyShiftTab,
	}

	if kind, ok := types[name]; ok {
		return tea.KeyMsg{Type: kind}
	}

	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

// press sends the keys to the model one at a time, as the runtime does.
func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()

	for _, name := range keys {
		next, _ := m.Update(keyMsg(name))

		m, _ = next.(Model)
	}

	return m
}

// The keys of the navigator move the selection and scroll the list. With a
// height of 17 the window holds 10 rows.
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
			m.width, m.height = 80, 17

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
	m.width, m.height = 80, 17

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
	m.width, m.height = 80, 17

	m = press(t, m, "down", "end", "pgdown")

	if m.block != 0 || m.top != 0 {
		t.Errorf("block = %d, top = %d; want 0 and 0", m.block, m.top)
	}
}

// A shorter terminal holds fewer rows, so the list scrolls to keep the
// selection on screen.
func TestModelResizeScrolls(t *testing.T) {
	m := New(openRelation(t, 20))
	m.width, m.height = 80, 17

	m = press(t, m, "end") // block 19, window 10..19

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 10}) // 3 rows
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
	m.width, m.height = 80, 17

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
	m.width, m.height = 80, 17

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

	return loadedOn(t, openFixture(t), width, height)
}

// loadedOn is loaded on any relation.
func loadedOn(t *testing.T, rel *pgpage.Relation, width, height int) Model {
	t.Helper()

	m := New(rel)
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
		{name: "no room for the map", width: 95, want: []string{"PAGES", "PAGE HEADER"}, gone: []string{"BYTES"}},
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

// The bottom line names the keys that move around, and ? opens the list of
// all of them in place of the panels.
func TestModelHelp(t *testing.T) {
	m := loaded(t, 110, 20)

	view := m.View()

	for _, want := range []string{"next page", "q quit", "PAGES"} {
		if !strings.Contains(view, want) {
			t.Errorf("view does not contain %q:\n%s", want, view)
		}
	}

	if strings.Contains(view, "KEYS") {
		t.Errorf("the help screen is open before pressing ?:\n%s", view)
	}

	open := press(t, m, "?")

	view = open.View()

	for _, want := range []string{"KEYS", "one screen up", "close the help", "read-only"} {
		if !strings.Contains(view, want) {
			t.Errorf("help screen does not contain %q:\n%s", want, view)
		}
	}

	for _, gone := range []string{"PAGES", "PAGE HEADER"} {
		if strings.Contains(view, gone) {
			t.Errorf("the help screen still shows %q:\n%s", gone, view)
		}
	}

	if !strings.Contains(press(t, open, "?").View(), "PAGES") {
		t.Error("? did not close the help screen")
	}

	if !strings.Contains(press(t, open, "esc").View(), "PAGES") {
		t.Error("esc did not close the help screen")
	}
}

// Esc closes the help when it is open, and quits when it is not.
func TestModelEscape(t *testing.T) {
	m := loaded(t, 110, 20)

	_, cmd := m.Update(keyMsg("esc"))
	if !isQuit(cmd) {
		t.Error("esc did not quit with the help closed")
	}

	next, cmd := press(t, m, "?").Update(keyMsg("esc"))
	if isQuit(cmd) {
		t.Error("esc quit with the help open, instead of closing it")
	}

	if open, _ := next.(Model); open.showHelp {
		t.Error("esc left the help open")
	}
}

// The help screen obeys the size of the terminal like everything else.
func TestModelHelpFitsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{110, 20}, {80, 12}, {60, 9}, {40, 8}} {
		view := press(t, loaded(t, size[0], size[1]), "?").View()

		lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")

		if len(lines) > size[1] {
			t.Errorf("%dx%d: the screen has %d lines:\n%s", size[0], size[1], len(lines), view)
		}

		for _, line := range lines {
			if got := lipgloss.Width(line); got > size[0] {
				t.Errorf("%dx%d: a line is %d cells wide:\n%q", size[0], size[1], got, line)
			}
		}
	}
}

// g opens the prompt, and what is typed there moves the selection.
func TestModelGoToBlock(t *testing.T) {
	m := loaded(t, 110, 20)

	if strings.Contains(m.View(), "GO TO BLOCK") {
		t.Fatalf("the prompt is open before pressing g:\n%s", m.View())
	}

	m = press(t, m, "g")

	if !m.prompt.active || !strings.Contains(m.View(), "GO TO BLOCK") {
		t.Fatalf("g did not open the prompt:\n%s", m.View())
	}

	m = press(t, m, "2", "enter")

	if m.block != 2 {
		t.Errorf("block = %d after typing 2, want 2", m.block)
	}

	if m.prompt.active {
		t.Error("the prompt stayed open after a jump")
	}
}

// An open prompt owns the keyboard: keys that are shortcuts outside it are
// just text inside it.
func TestModelPromptCapturesKeys(t *testing.T) {
	m := press(t, loaded(t, 110, 20), "g")

	next, cmd := m.Update(keyMsg("q"))
	if isQuit(cmd) {
		t.Fatal("q quit the program while the prompt was open")
	}

	m, _ = next.(Model)

	m = press(t, m, "j", "k")

	if m.block != 0 {
		t.Errorf("block = %d, want 0: the navigation keys moved the selection", m.block)
	}

	if got := m.prompt.input.Value(); got != "qjk" {
		t.Errorf("the prompt holds %q, want %q", got, "qjk")
	}
}

// A block that does not exist leaves the prompt open, with the text and the
// reason, so a typo can be fixed instead of typed again.
func TestModelPromptRejectsBadInput(t *testing.T) {
	m := press(t, loaded(t, 110, 20), "g", "9", "9", "enter")

	if m.block != 0 {
		t.Errorf("block = %d, want 0: the jump should not have happened", m.block)
	}

	if !m.prompt.active {
		t.Fatal("the prompt closed after a rejected block")
	}

	if got := m.prompt.input.Value(); got != "99" {
		t.Errorf("the prompt holds %q, want the text to stay as %q", got, "99")
	}

	if !strings.Contains(m.View(), "past the last one") {
		t.Errorf("the view does not explain the rejection:\n%s", m.View())
	}

	// Typing again clears the complaint, and a valid block jumps.
	m = press(t, m, "backspace", "backspace", "1", "enter")

	if m.block != 1 || m.prompt.active {
		t.Errorf("block = %d, prompt open = %v; want 1 and closed", m.block, m.prompt.active)
	}
}

// Esc gives up on the jump and leaves the selection where it was.
func TestModelPromptCancel(t *testing.T) {
	m := press(t, loaded(t, 110, 20), "end", "g", "0", "esc")

	if m.block != 2 {
		t.Errorf("block = %d, want 2: Esc must not jump", m.block)
	}

	if m.prompt.active {
		t.Error("Esc did not close the prompt")
	}

	if !strings.Contains(m.View(), "next page") {
		t.Errorf("the help line did not come back:\n%s", m.View())
	}
}

// Panels are drawn side by side, so every framed line of the body has to be
// the same width: one cell of drift and the borders no longer line up.
func TestModelPanelsAlign(t *testing.T) {
	sizes := [][2]int{{150, 28}, {120, 24}, {100, 18}, {80, 14}, {70, 16}}

	for _, size := range sizes {
		views := map[string]string{
			"pages": loaded(t, size[0], size[1]).View(),
			"help":  press(t, loaded(t, size[0], size[1]), "?").View(),
		}

		for name, view := range views {
			var width int

			for _, line := range strings.Split(view, "\n") {
				if !strings.ContainsAny(line, borderTopLeft+borderVertical+borderBottomLeft) {
					continue
				}

				got := lipgloss.Width(line)
				if width == 0 {
					width = got
					continue
				}

				if got != width {
					t.Errorf("%s at %dx%d: a framed line is %d cells wide, another %d:\n%s",
						name, size[0], size[1], got, width, view)

					break
				}
			}
		}
	}
}

// The screen keeps the same margin from both edges of the terminal: every
// line starts after it, and none runs into the last columns.
func TestModelScreenMargin(t *testing.T) {
	margin := lipgloss.Width(screenMargin)

	for _, width := range []int{60, 100, 150} {
		views := map[string]string{
			"pages":  loaded(t, width, 24).View(),
			"help":   press(t, loaded(t, width, 24), "?").View(),
			"prompt": press(t, loaded(t, width, 24), "g").View(),
		}

		for name, view := range views {
			for i, line := range strings.Split(strings.TrimSuffix(view, "\n"), "\n") {
				if !strings.HasPrefix(line, screenMargin) {
					t.Errorf("%s at width %d: line %d does not start with the margin:\n%q", name, width, i, line)
				}

				if got := lipgloss.Width(strings.TrimRight(line, " ")); got > width-margin {
					t.Errorf("%s at width %d: line %d reaches column %d, past the right margin",
						name, width, i, got)
				}
			}
		}
	}
}

// The navigator's last line, the count of pages below the window, fits in
// its panel: the rows are counted after every other line on screen.
func TestModelPageListMoreLineFits(t *testing.T) {
	m := New(openRelation(t, 20))
	m.width, m.height = 80, 17

	if view := m.View(); !strings.Contains(view, "↓ 10 more") {
		t.Errorf("the count of hidden pages is cut off:\n%s", view)
	}
}

// openItemsView opens the line pointer view on block of the fixture, with
// the line pointers already read, as the program does after pressing Enter.
func openItemsView(t *testing.T, block pgpage.BlockNumber, width, height int) Model {
	t.Helper()

	m := press(t, loaded(t, width, height), "g", fmt.Sprint(block), "enter")

	next, cmd := m.Update(keyMsg("enter"))
	m, _ = next.(Model)

	if m.current() != viewItems {
		t.Fatalf("enter on block %d did not open the line pointer view", block)
	}

	next, _ = m.Update(run(t, cmd))
	m, _ = next.(Model)

	return m
}

// Enter on a valid page opens its line pointers, read by a command.
func TestModelOpenItems(t *testing.T) {
	m := loaded(t, 150, 30)

	next, cmd := m.Update(keyMsg("enter"))
	m, _ = next.(Model)

	if m.current() != viewItems || len(m.views) != 2 {
		t.Fatalf("views = %v, want the line pointer view on top of the pages", m.views)
	}

	if !strings.Contains(m.View(), "reading…") {
		t.Errorf("the view does not say the line pointers are being read:\n%s", m.View())
	}

	msg, ok := run(t, cmd).(itemsMsg)
	if !ok || msg.block != 0 {
		t.Fatalf("enter did not ask for the line pointers of block 0: %#v", msg)
	}

	next, _ = m.Update(msg)
	m, _ = next.(Model)

	view := m.View()

	for _, want := range []string{"LINE POINTERS · 185", "LINE POINTER #1", "PAGE 0 — 8192 BYTES", "ITEMID WORD", "previous item"} {
		if !strings.Contains(view, want) {
			t.Errorf("view does not contain %q:\n%s", want, view)
		}
	}
}

// A page with no valid header has no line pointers to open.
func TestModelOpenItemsOnBrokenPage(t *testing.T) {
	m := New(openMixed(t))
	m.width, m.height = 150, 30

	next, _ := m.Update(run(t, m.Init()))
	m, _ = next.(Model)

	for _, keys := range [][]string{{"down"}, {"end"}} { // the new page, the invalid one
		page := press(t, m, keys...)

		next, cmd := page.Update(keyMsg("enter"))
		after, _ := next.(Model)

		if after.current() != viewPages || cmd != nil {
			t.Errorf("enter on block %d opened a view or read the page", page.block)
		}
	}
}

// In the line pointer view the keys move the selected line pointer, not the
// selected page, and Esc goes back to the pages where they were.
func TestModelItemsNavigation(t *testing.T) {
	m := openItemsView(t, 2, 150, 30)

	m = press(t, m, "down", "down", "down")

	if m.items.selected != 3 || m.block != 2 {
		t.Errorf("selected item index %d on block %d, want 3 on block 2", m.items.selected, m.block)
	}

	if m = press(t, m, "end"); m.items.selected != 179 {
		t.Errorf("end selected index %d, want 179, the last of 180", m.items.selected)
	}

	if m = press(t, m, "home", "up"); m.items.selected != 0 {
		t.Errorf("up from the first line pointer selected index %d, want 0", m.items.selected)
	}

	back := press(t, m, "esc")

	if back.current() != viewPages || back.block != 2 {
		t.Errorf("esc left view %v on block %d, want the pages on block 2", back.current(), back.block)
	}

	if _, cmd := back.Update(keyMsg("esc")); !isQuit(cmd) {
		t.Error("esc on the pages did not quit")
	}
}

// Going back and then opening another view must not change the stack an
// older copy of the model holds: models are values, and the copies share the
// stack's array.
func TestModelViewStackIsNotShared(t *testing.T) {
	opened := openItemsView(t, 0, 150, 30)

	const other view = 99 // a view pushed where the line pointers were

	_ = opened.pop().push(other)

	if opened.current() != viewItems {
		t.Errorf("the original model is now on view %v, want the line pointer view", opened.current())
	}
}

// Line pointers read for a page the user already left are dropped.
func TestModelStaleItemsMsg(t *testing.T) {
	m := openItemsView(t, 0, 150, 30)

	next, _ := m.Update(itemsMsg{block: 2, ids: make([]pgpage.ItemID, 7)})
	m, _ = next.(Model)

	if m.items.block != 0 || len(m.items.ids) != 185 {
		t.Errorf("items of block %d with %d line pointers, want block 0 with 185", m.items.block, len(m.items.ids))
	}
}

// The help of the line pointer view describes its keys.
func TestModelItemsHelp(t *testing.T) {
	view := press(t, openItemsView(t, 0, 150, 30), "?").View()

	for _, want := range []string{"previous item", "last item", "page view"} {
		if !strings.Contains(view, want) {
			t.Errorf("help does not contain %q:\n%s", want, view)
		}
	}

	if strings.Contains(view, "go to block") || !strings.Contains(view, "go to line pointer") {
		t.Errorf("the help does not say that g goes to a line pointer here:\n%s", view)
	}
}

// The line pointer view fits the terminal like the page view does.
func TestModelItemsViewFits(t *testing.T) {
	for _, size := range [][2]int{{150, 30}, {100, 20}, {60, 12}} {
		view := press(t, openItemsView(t, 2, size[0], size[1]), "end").View()
		lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")

		if len(lines) > size[1] {
			t.Errorf("%dx%d: %d lines", size[0], size[1], len(lines))
		}

		for i, line := range lines {
			if got := lipgloss.Width(line); got > size[0] {
				t.Errorf("%dx%d: line %d is %d cells wide", size[0], size[1], i, got)
			}
		}
	}
}

// In the line pointer view, g asks for a line pointer and selects it.
func TestModelGoToItem(t *testing.T) {
	m := press(t, openItemsView(t, 2, 150, 30), "g")

	if !m.prompt.active || !strings.Contains(m.View(), "GO TO LINE POINTER") {
		t.Fatalf("g did not open the line pointer prompt:\n%s", m.View())
	}

	m = press(t, m, "1", "0", "enter")

	if m.items.selected != 9 || m.block != 2 || m.current() != viewItems {
		t.Errorf("item index %d on block %d, want #10 (index 9) on block 2", m.items.selected, m.block)
	}

	if !strings.Contains(m.View(), "LINE POINTER #10") {
		t.Errorf("the panel does not show #10:\n%s", m.View())
	}
}

// A 0 or a number past the last line pointer keeps the prompt open and says
// why; Esc gives up and leaves the selection where it was.
func TestModelGoToItemRejects(t *testing.T) {
	m := press(t, openItemsView(t, 2, 150, 30), "down", "g", "0", "enter")

	if !m.prompt.active || !strings.Contains(m.View(), "numbers start at 1") {
		t.Errorf("0 was not rejected with the reason:\n%s", m.View())
	}

	m = press(t, m, "esc")

	if m.prompt.active || m.items.selected != 1 || m.current() != viewItems {
		t.Errorf("esc: prompt open %v, item index %d, view %v; want closed, 1, line pointers",
			m.prompt.active, m.items.selected, m.current())
	}
}

// openTupleView opens the tuple of line pointer n of block of the fixture.
func openTupleView(t *testing.T, block pgpage.BlockNumber, n, width, height int) Model {
	t.Helper()

	m := press(t, openItemsView(t, block, width, height), "g", fmt.Sprint(n), "enter", "enter")

	if m.current() != viewTuple {
		t.Fatalf("enter on #%d of block %d did not open the tuple view", n, block)
	}

	return m
}

// Enter on a line pointer with a tuple opens it, decoded from the page the
// line pointer view already read.
func TestModelOpenTuple(t *testing.T) {
	m := press(t, openItemsView(t, 2, 150, 32), "g", "2", "enter")

	next, cmd := m.Update(keyMsg("enter"))
	m, _ = next.(Model)

	if m.current() != viewTuple || len(m.views) != 3 {
		t.Fatalf("views = %v, want the tuple view on top", m.views)
	}

	if cmd != nil {
		t.Error("opening a tuple ran a command; the page is already in memory")
	}

	view := m.View()

	for _, want := range []string{"TUPLE #2", "DECODED FLAGS", "t_xmin", "HEAP_XMIN_COMMITTED", "next tuple", "Esc line pointers"} {
		if !strings.Contains(view, want) {
			t.Errorf("view does not contain %q:\n%s", want, view)
		}
	}
}

// Line pointers without a tuple have nothing to open.
func TestModelOpenTupleWithoutOne(t *testing.T) {
	for _, n := range []string{"1", "10"} { // #1 is dead, #10 a redirect
		m := press(t, openItemsView(t, 2, 150, 32), "g", n, "enter", "enter")

		if m.current() != viewItems {
			t.Errorf("enter on #%s opened view %v, want to stay on the line pointers", n, m.current())
		}
	}
}

// Moving in the tuple view goes from tuple to tuple, skipping the line
// pointers that have none, and Esc lands on the last tuple shown.
func TestModelTupleNavigation(t *testing.T) {
	m := openTupleView(t, 2, 9, 150, 32) // #9; #10 is a redirect

	if m = press(t, m, "down"); number(m.items.selected) != 11 {
		t.Errorf("down from #9 went to #%d, want #11, past the redirect", number(m.items.selected))
	}

	if m = press(t, m, "home"); number(m.items.selected) != 2 {
		t.Errorf("home went to #%d, want #2, the first tuple after the dead #1", number(m.items.selected))
	}

	if m = press(t, m, "up"); number(m.items.selected) != 2 {
		t.Errorf("up from the first tuple went to #%d, want to stay on #2", number(m.items.selected))
	}

	back := press(t, m, "esc")

	if back.current() != viewItems || number(back.items.selected) != 2 {
		t.Errorf("esc left view %v on #%d, want the line pointers on #2", back.current(), number(back.items.selected))
	}

	if pages := press(t, back, "esc"); pages.current() != viewPages {
		t.Errorf("esc from the line pointers left view %v, want the pages", pages.current())
	}
}

// The tuple view fits the terminal, and keeps the fields panel when there
// is no room for the flags.
func TestModelTupleViewFits(t *testing.T) {
	for _, size := range [][2]int{{150, 32}, {100, 24}, {60, 16}} {
		view := openTupleView(t, 2, 168, size[0], size[1]).View()
		lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")

		if len(lines) > size[1] {
			t.Errorf("%dx%d: %d lines", size[0], size[1], len(lines))
		}

		for i, line := range lines {
			if got := lipgloss.Width(line); got > size[0] {
				t.Errorf("%dx%d: line %d is %d cells wide", size[0], size[1], i, got)
			}
		}

		if !strings.Contains(view, "TUPLE #168") {
			t.Errorf("%dx%d: the tuple panel is missing:\n%s", size[0], size[1], view)
		}
	}
}

// From the pages, x reads the selected page and opens its dump with the
// header selected; x again goes back.
func TestModelHexFromPages(t *testing.T) {
	m := loaded(t, 130, 26)

	next, cmd := m.Update(keyMsg("x"))
	m, _ = next.(Model)

	if m.current() != viewHex || !strings.Contains(m.View(), "reading…") {
		t.Fatalf("x did not open the hex view reading the page:\n%s", m.View())
	}

	msg, ok := run(t, cmd).(hexMsg)
	if !ok || msg.block != 0 {
		t.Fatalf("x did not read block 0: %#v", msg)
	}

	next, _ = m.Update(msg)
	m, _ = next.(Model)

	for _, want := range []string{"BYTES · BLOCK 0", "0000", "← page header", "page header · bytes 0-23"} {
		if !strings.Contains(m.View(), want) {
			t.Errorf("view does not contain %q:\n%s", want, m.View())
		}
	}

	if back := press(t, m, "x"); back.current() != viewPages {
		t.Errorf("x left view %v, want the pages", back.current())
	}
}

// From the line pointers and from a tuple, the page is already in memory:
// the dump opens at once, scrolled to the selection.
func TestModelHexFromItemsAndTuple(t *testing.T) {
	items := press(t, openItemsView(t, 2, 130, 26), "g", "1", "6", "8", "enter")

	next, cmd := items.Update(keyMsg("x"))
	fromItems, _ := next.(Model)

	if cmd != nil {
		t.Error("x from the line pointers read the page again")
	}

	if view := fromItems.View(); !strings.Contains(view, "← line pointer #168") {
		t.Errorf("the entry of #168 is not on screen:\n%s", view)
	}

	fromTuple := press(t, items, "enter", "x")

	if view := fromTuple.View(); !strings.Contains(view, "← tuple #168") || !strings.Contains(view, "0x0B38") {
		t.Errorf("the tuple #168 is not on screen:\n%s", view)
	}

	if back := press(t, fromTuple, "esc"); back.current() != viewTuple {
		t.Errorf("esc from the hex view left view %v, want the tuple", back.current())
	}
}

// The moving keys scroll the dump a line, a screen, or to either end.
func TestModelHexScroll(t *testing.T) {
	m := press(t, openItemsView(t, 0, 130, 26), "x") // #1's entry: already at the top

	if m.hex.vp.YOffset != 0 {
		t.Fatalf("the dump starts at line %d, want 0", m.hex.vp.YOffset)
	}

	if m = press(t, m, "down"); m.hex.vp.YOffset != 1 {
		t.Errorf("down scrolled to line %d, want 1", m.hex.vp.YOffset)
	}

	if m = press(t, m, "pgdown"); m.hex.vp.YOffset != 1+m.hex.vp.Height {
		t.Errorf("pgdown scrolled to line %d, want %d", m.hex.vp.YOffset, 1+m.hex.vp.Height)
	}

	if m = press(t, m, "end"); !m.hex.vp.AtBottom() || !strings.Contains(m.View(), "1ff0") {
		t.Errorf("end did not show the last line of the page:\n%s", m.View())
	}

	if m = press(t, m, "home"); m.hex.vp.YOffset != 0 {
		t.Errorf("home scrolled to line %d, want 0", m.hex.vp.YOffset)
	}
}

// A page without a valid header still has bytes to look at.
func TestModelHexOnBrokenPage(t *testing.T) {
	m := New(openMixed(t))
	m.width, m.height = 130, 26

	next, _ := m.Update(run(t, m.Init()))
	m, _ = next.(Model)

	next, cmd := press(t, m, "end").Update(keyMsg("x"))
	m, _ = next.(Model)

	next, _ = m.Update(run(t, cmd))
	m, _ = next.(Model)

	view := m.View()

	for _, want := range []string{"BYTES · BLOCK 2", "0000", "nothing selected"} {
		if !strings.Contains(view, want) {
			t.Errorf("view does not contain %q:\n%s", want, view)
		}
	}
}

// A page read for a hex view the user already closed is dropped.
func TestModelStaleHexMsg(t *testing.T) {
	m := loaded(t, 130, 26)

	next, cmd := m.Update(keyMsg("x"))
	m, _ = next.(Model)
	msg := run(t, cmd)

	m = press(t, m, "esc", "down")

	next, _ = m.Update(msg)
	m, _ = next.(Model)

	if m.current() != viewPages || m.hex.loaded {
		t.Errorf("a late page reopened or filled the hex view: view %v, loaded %v", m.current(), m.hex.loaded)
	}
}

// The dump fills the panel to its last line: no blank rows below it.
func TestModelHexFillsThePanel(t *testing.T) {
	for _, height := range []int{20, 26, 40} {
		m := press(t, openItemsView(t, 2, 130, height), "x")
		lines := strings.Split(strings.TrimSuffix(m.View(), "\n"), "\n")

		if len(lines) != height {
			t.Errorf("height %d: %d lines", height, len(lines))
		}

		// The line above the bottom border is the second line of detail.
		if above := lines[len(lines)-4]; !strings.Contains(above, "highlighted bytes") {
			t.Errorf("height %d: the panel ends in blank lines:\n%s", height, strings.Join(lines[len(lines)-6:], "\n"))
		}
	}
}

// e opens explain mode on the header of the selected page: the fields on the
// left with pd_lower selected, and its explanation on the right. Tab selects
// the next field, and e closes the mode.
func TestModelExplain(t *testing.T) {
	m := press(t, loaded(t, 130, 26), "e")

	if m.current() != viewExplain {
		t.Fatalf("e left view %v, want explain mode", m.current())
	}

	view := m.View()

	for _, want := range []string{"PAGE HEADER", "> pd_lower", "pd_lower = 764", "PURPOSE", "HOW IT WORKS", "close explain"} {
		if !strings.Contains(view, want) {
			t.Errorf("view does not contain %q:\n%s", want, view)
		}
	}

	// The list is on the left, and the explanation beside it.
	if line := lineWith(t, view, "pd_lower = 764"); strings.Index(line, "PAGE HEADER") > strings.Index(line, "pd_lower") {
		t.Errorf("the explanation is not to the right of the list:\n%s", line)
	}

	// The working is further down the explanation.
	if down := press(t, m, "pgdown").View(); !strings.Contains(down, "740 ÷ 4  = 185") {
		t.Errorf("the working is not below the explanation:\n%s", down)
	}

	next := press(t, m, "tab").View()
	if !strings.Contains(next, "> pd_upper") || !strings.Contains(next, "pd_upper = 2472") {
		t.Errorf("tab did not select pd_upper:\n%s", next)
	}

	if back := press(t, m, "e"); back.current() != viewPages {
		t.Errorf("e left view %v, want the pages", back.current())
	}
}

// The moving keys walk down and up the fields, and stop at both ends.
func TestModelExplainNavigation(t *testing.T) {
	m := press(t, loaded(t, 130, 26), "e")
	last := len(explanations) - 1

	tests := []struct {
		keys []string
		want int
	}{
		{keys: []string{"down"}, want: explainLower + 1},
		{keys: []string{"shift+tab", "k"}, want: explainLower - 2},
		{keys: []string{"home", "up"}, want: 0},
		{keys: []string{"end", "tab"}, want: last},
		{keys: repeat("j", len(explanations)), want: last},
	}

	for _, tt := range tests {
		if got := press(t, m, tt.keys...).explain.field; got != tt.want {
			t.Errorf("%v: field %d, want %d", tt.keys, got, tt.want)
		}
	}

	if explanations[explainLower].field != "pd_lower" {
		t.Errorf("explain mode opens on %s", explanations[explainLower].field)
	}
}

// A page without a valid header has no fields to explain.
func TestModelExplainOnBrokenPage(t *testing.T) {
	m := New(openMixed(t))
	m.width, m.height = 130, 26

	next, _ := m.Update(run(t, m.Init()))
	m, _ = next.(Model)

	if m = press(t, m, "end", "e"); m.current() != viewPages {
		t.Errorf("e opened view %v on an invalid page", m.current())
	}
}

// x shows the bytes of the selected field, and Esc comes back to it.
func TestModelExplainHex(t *testing.T) {
	m := press(t, loaded(t, 130, 26), "e", "tab")

	next, cmd := m.Update(keyMsg("x"))
	hex, _ := next.(Model)

	next, _ = hex.Update(run(t, cmd))
	hex, _ = next.(Model)

	if view := hex.View(); !strings.Contains(view, "← pd_upper") || !strings.Contains(view, "pd_upper · bytes 14-15 · 2 B") {
		t.Errorf("the hex view does not select pd_upper:\n%s", view)
	}

	if back := press(t, hex, "esc"); back.current() != viewExplain || back.explain.field != explainLower+1 {
		t.Errorf("esc from the hex view left view %v on field %d", back.current(), back.explain.field)
	}
}

// Explain mode fits the terminal, and on a narrow one keeps the explanation
// alone, whose title names the selected field.
func TestModelExplainFits(t *testing.T) {
	for _, size := range [][2]int{{130, 26}, {100, 30}, {80, 24}, {60, 20}} {
		m := press(t, loaded(t, size[0], size[1]), "e")
		view := m.View()
		lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")

		if len(lines) != size[1] {
			t.Errorf("%dx%d: %d lines", size[0], size[1], len(lines))
		}

		for _, line := range lines {
			if got := lipgloss.Width(line); got > size[0] {
				t.Errorf("%dx%d: a line is %d cells:\n%s", size[0], size[1], got, line)
			}
		}

		if !strings.Contains(view, "pd_lower = 764") {
			t.Errorf("%dx%d: the explanation is missing:\n%s", size[0], size[1], view)
		}

		wide := size[0]-2 >= explainListWidth+panelFrame+1+minExplainWidth+panelFrame
		if strings.Contains(view, "PAGE HEADER") != wide {
			t.Errorf("%dx%d: the list shown = %v, want %v:\n%s", size[0], size[1], !wide, wide, view)
		}
	}
}

// PgDn scrolls an explanation taller than the panel down to its end, where
// the strip is; PgUp scrolls back, and selecting another field starts it from
// the top.
func TestModelExplainScroll(t *testing.T) {
	m := press(t, loaded(t, 130, 26), "e")

	if view := m.View(); strings.Contains(view, "↑ 764") || !strings.Contains(view, "more · PgDn") {
		t.Fatalf("the explanation fits at this height, the test needs a shorter terminal:\n%s", view)
	}

	end := press(t, m, "pgdown", "pgdown", "pgdown")

	if view := end.View(); !strings.Contains(view, "↑ 764") || !strings.Contains(view, "more · PgUp") ||
		strings.Contains(view, "PgDn") {
		t.Errorf("pgdown did not reach the end of the explanation:\n%s", view)
	}

	if top := press(t, end, "pgup", "pgup", "pgup"); top.explain.scroll != 0 || !strings.Contains(top.View(), "PURPOSE") {
		t.Errorf("pgup did not go back to the top: scroll %d", top.explain.scroll)
	}

	if other := press(t, end, "down"); other.explain.scroll != 0 {
		t.Errorf("another field starts scrolled %d lines in", other.explain.scroll)
	}

	if tall := press(t, loaded(t, 130, 60), "e", "pgdown"); tall.explain.scroll != 0 {
		t.Errorf("an explanation that fits scrolled %d lines", tall.explain.scroll)
	}
}

// openCopy copies the fixture to a temporary file and opens it, for the tests
// that change the relation on disk while the explorer looks at it.
func openCopy(t *testing.T) (*pgpage.Relation, string) {
	t.Helper()

	data, err := os.ReadFile(fixtureHeap)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "copy")
	if err = os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	rel, err := pgpage.OpenRelation(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { rel.Close() })

	return rel, path
}

// writeAt overwrites the file at path with data, starting at byte offset
// of page block, as PostgreSQL would while the explorer is open.
func writeAt(t *testing.T, path string, block pgpage.BlockNumber, offset int, data ...byte) {
	t.Helper()

	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if _, err := f.WriteAt(data, int64(block)*pgpage.PageSize+int64(offset)); err != nil {
		t.Fatal(err)
	}
}

// settle runs cmd and the commands of any batch it returns, and feeds every
// message to the model, as the runtime does.
func settle(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		return m
	}

	msg := cmd()

	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = settle(t, m, c)
		}

		return m
	}

	next, _ := m.Update(msg)
	m, _ = next.(Model)

	return m
}

// pressAndSettle sends a key and runs the commands it returns to the end.
func pressAndSettle(t *testing.T, m Model, name string) Model {
	t.Helper()

	next, cmd := m.Update(keyMsg(name))
	m, _ = next.(Model)

	return settle(t, m, cmd)
}

// r reads the selected page again: a change made on disk after it was read
// shows up, and until it arrives the screen keeps the old page.
func TestModelReloadPage(t *testing.T) {
	rel, path := openCopy(t)
	m := loadedOn(t, rel, 130, 26)

	writeAt(t, path, 0, 8, 0x34, 0x12) // pd_checksum = 0x1234 = 4660

	if strings.Contains(m.View(), "4660") {
		t.Fatal("the change is on screen before reading the page again")
	}

	next, cmd := m.Update(keyMsg("r"))
	m, _ = next.(Model)

	view := m.View()
	if !strings.Contains(view, "block 0 read again from disk") || strings.Contains(view, "reading…") {
		t.Errorf("r does not say what it did, or blanks the page while it reads:\n%s", view)
	}

	m = settle(t, m, cmd)

	if !strings.Contains(lineWith(t, m.View(), "pd_checksum"), "4660") {
		t.Errorf("the page read again does not show the new checksum:\n%s", m.View())
	}

	if strings.Contains(press(t, m, "down").View(), "read again") {
		t.Error("the notice outlived the next key")
	}
}

// In the line pointer view, r reads the line pointers again. When the page
// now has fewer, the selection moves to the last one.
func TestModelReloadItems(t *testing.T) {
	rel, path := openCopy(t)
	m := press(t, loadedOn(t, rel, 130, 26), "g", "2", "enter")
	m = pressAndSettle(t, m, "enter")
	m = press(t, m, "down") // #2, NORMAL

	writeAt(t, path, 2, 28, 0x00, 0x80, 0x01, 0x00) // #2 becomes DEAD without storage

	m = pressAndSettle(t, m, "r")

	if row := lineWith(t, m.View(), "> #2"); !strings.Contains(strings.Join(strings.Fields(row), " "), "#2 DEAD — —") {
		t.Errorf("#2 is not dead after reading the page again:\n%s", row)
	}

	m = press(t, m, "end") // #180

	writeAt(t, path, 2, 12, 64, 0) // pd_lower = 64: 10 line pointers

	if m = pressAndSettle(t, m, "r"); m.items.selected != 9 || len(m.items.ids) != 10 {
		t.Errorf("selected %d of %d line pointers, want the last of 10", m.items.selected, len(m.items.ids))
	}

	if !strings.Contains(m.View(), "LINE POINTERS · 10") {
		t.Errorf("the view still lists the old line pointers:\n%s", m.View())
	}
}

// In the hex view, r reads the bytes again and keeps the dump scrolled where
// it was.
func TestModelReloadHex(t *testing.T) {
	rel, path := openCopy(t)
	m := press(t, loadedOn(t, rel, 130, 26), "g", "2", "enter")
	m = pressAndSettle(t, m, "enter")
	m = press(t, m, "x", "down", "down", "down")

	offset := m.hex.vp.YOffset

	writeAt(t, path, 2, 0x40, 0xAB, 0xCD)

	m = pressAndSettle(t, m, "r")

	if m.hex.vp.YOffset != offset {
		t.Errorf("the dump scrolled to line %d, want %d", m.hex.vp.YOffset, offset)
	}

	if !strings.Contains(lineWith(t, m.View(), "0040  "), "ab cd") {
		t.Errorf("the dump does not show the new bytes:\n%s", m.View())
	}
}

// Explain mode explains the header read again, but keeps the old one when
// the new one no longer parses: there would be nothing true to say about it.
func TestModelReloadExplain(t *testing.T) {
	rel, path := openCopy(t)
	m := press(t, loadedOn(t, rel, 130, 26), "e")

	writeAt(t, path, 0, 8, 0x34, 0x12)

	if m = pressAndSettle(t, m, "r"); !strings.Contains(lineWith(t, m.View(), "pd_checksum"), "4660") {
		t.Errorf("explain mode shows the old checksum:\n%s", m.View())
	}

	writeAt(t, path, 0, 14, 100, 0) // pd_upper = 100, below pd_lower

	m = pressAndSettle(t, m, "r")

	if m.explain.summary.Status != pgpage.StatusOK || m.summaries[0].Status != pgpage.StatusInvalid {
		t.Errorf("explain status %v, cache status %v; want the old header kept and the page invalid",
			m.explain.summary.Status, m.summaries[0].Status)
	}

	if back := press(t, m, "esc"); !strings.Contains(back.View(), "INVALID") {
		t.Errorf("the page view does not show the page as invalid:\n%s", back.View())
	}
}

// Resizing the terminal one column at a time keeps the map at the same scale
// until the next grid fits, and the panels still fill the screen: the map
// panel keeps the width it had, and pads the grid.
func TestModelMapKeepsItsScale(t *testing.T) {
	changes, last := 0, 0

	for width := 110; width <= 170; width++ {
		for _, m := range []Model{loaded(t, width, 26), openItemsView(t, 2, width, 26)} {
			view := m.View()
			if !strings.Contains(view, "0x0200 ") {
				continue // too narrow for a map next to the other panels
			}

			top := strings.Split(view, "\n")[2]
			if lipgloss.Width(top) != width-1 || !strings.HasSuffix(top, borderTopRight) {
				t.Errorf("width %d, view %v: the panels do not reach the right margin:\n%s", width, m.current(), top)
			}

			if m.current() != viewPages {
				continue
			}

			// The grid is the row after its offset, up to the blank space
			// the panel pads it with.
			row := lineWith(t, view, "0x0200 ")
			row = row[strings.Index(row, "0x0200 ")+len("0x0200 "):]
			cells := lipgloss.Width(strings.TrimRight(row[:strings.Index(row, borderVertical)], " "))

			if cells != last {
				changes++
				last = cells
			}
		}
	}

	// From no grid to 16 cells, 32 and 64: three changes in sixty columns,
	// not one per column.
	if changes > len(mapCellChoices) {
		t.Errorf("the map changed its scale %d times between 110 and 170 columns, want at most %d", changes, len(mapCellChoices))
	}
}
