package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/arturobermejo/pgpage"
)

// fixtureHeap is the relation file shared with the pgpage package tests.
const fixtureHeap = "../../testdata/heap_small"

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

// The first screen names the relation and its size, so the user knows what
// they opened.
func TestModelView(t *testing.T) {
	view := New(openFixture(t)).View()

	for _, want := range []string{fixtureHeap, "3 pages", "24576 bytes", "q: quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("view does not contain %q:\n%s", want, view)
		}
	}
}

// Nothing is loaded before the first view, so there is no initial command.
func TestModelInit(t *testing.T) {
	if cmd := New(openFixture(t)).Init(); cmd != nil {
		t.Errorf("Init returned a command, want none")
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
		{name: "another key does nothing", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}},
		{name: "another message does nothing", msg: tea.WindowSizeMsg{Width: 80, Height: 24}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(openFixture(t))

			next, cmd := m.Update(tt.msg)

			if next != tea.Model(m) {
				t.Errorf("Update changed the model: %+v, want %+v", next, m)
			}

			if quit := isQuit(cmd); quit != tt.quit {
				t.Errorf("quit = %v, want %v", quit, tt.quit)
			}
		})
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
