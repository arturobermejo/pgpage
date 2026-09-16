// Package tui is the interactive page explorer, built with Bubble Tea.
//
// It follows the Elm architecture: Model holds every piece of state the
// screen is drawn from, Update folds one message into a new state, and View
// turns that state into the text of the whole screen.
package tui

import (
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/arturobermejo/pgpage"
)

// Model is the state of the explorer.
//
// It is a value, not a pointer: Update receives a copy and returns the next
// state, so a view can never be drawn from a model somebody else is changing.
type Model struct {
	rel *pgpage.Relation
}

var _ tea.Model = Model{}

// New returns the model of an explorer on rel, which the caller must keep
// open until Run returns.
func New(rel *pgpage.Relation) Model {
	return Model{rel: rel}
}

// Init returns the command to run before the first View. There is nothing to
// load yet, so it returns none.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update returns the state that msg leads to, and a command to run next.
// Bubble Tea calls it once per message, never concurrently.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q", "esc", "ctrl+c":
			// tea.Quit is a command, not an action: returning it asks the
			// runtime to stop after this update.
			return m, tea.Quit
		}
	}

	return m, nil
}

// View returns the whole screen as text. It must not change the model nor
// read anything but it: Bubble Tea may call it after any message.
func (m Model) View() string {
	return fmt.Sprintf("pgpage\n\n%s\n%d pages, %d bytes\n\nq: quit\n",
		m.rel.Path(), m.rel.PageCount(), m.rel.Size())
}

// Run starts the explorer on rel and blocks until the user quits. It reads
// keys from in and draws on out, which is the terminal in practice.
func Run(rel *pgpage.Relation, in io.Reader, out io.Writer) error {
	p := tea.NewProgram(New(rel),
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("pgpage: %w", err)
	}

	return nil
}
