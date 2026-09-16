// Package tui is the interactive page explorer, built with Bubble Tea.
//
// It follows the Elm architecture: Model holds every piece of state the
// screen is drawn from, Update folds one message into a new state, and View
// turns that state into the text of the whole screen.
package tui

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/arturobermejo/pgpage"
)

// Model is the state of the explorer.
//
// It is a value, not a pointer: Update receives a copy and returns the next
// state, so a view can never be drawn from a model somebody else is changing.
type Model struct {
	rel   *pgpage.Relation
	block pgpage.BlockNumber // the selected page
	top   pgpage.BlockNumber // the first page the navigator shows

	// Size of the terminal, in cells. Both are zero until the first
	// tea.WindowSizeMsg arrives, which Bubble Tea sends before anything
	// else, so View must cope with not knowing the size yet.
	width  int
	height int
}

// Lines the screen spends on everything but the rows of the navigator: the
// top bar and its blank line, the list title, the "N more" line, another
// blank line and the help line.
const listChrome = 6

// Rows of the navigator when the terminal size is unknown, and the fewest it
// may shrink to.
const (
	defaultListRows = 10
	minListRows     = 1
)

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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// The size is state like any other: it arrives as a message and the
		// views read it from the model when they are drawn.
		m.width, m.height = msg.Width, msg.Height

		// A window of a different height holds a different number of rows,
		// so the navigator may have to scroll to keep showing the selection.
		m.top = scrollTo(m.rel.PageCount(), m.top, m.block, m.visibleRows())

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			// tea.Quit is a command, not an action: returning it asks the
			// runtime to stop after this update.
			return m, tea.Quit

		case "up", "k":
			return m.move(-1), nil
		case "down", "j":
			return m.move(1), nil
		case "pgup":
			return m.move(-m.visibleRows()), nil
		case "pgdown":
			return m.move(m.visibleRows()), nil
		case "home":
			return m.selectBlock(0), nil
		case "end":
			return m.selectBlock(int64(m.rel.PageCount()) - 1), nil
		}
	}

	return m, nil
}

// move changes the selection by delta pages.
func (m Model) move(delta int) Model {
	// int64 all the way: adding a negative delta to block 0 must not wrap
	// around an unsigned block number.
	return m.selectBlock(int64(m.block) + int64(delta))
}

// selectBlock selects block, clamped to the pages the relation has, and
// scrolls the navigator the least it takes to show it.
func (m Model) selectBlock(block int64) Model {
	pages := m.rel.PageCount()
	if pages == 0 {
		return m
	}

	switch last := int64(pages) - 1; {
	case block < 0:
		block = 0
	case block > last:
		block = last
	}

	m.block = pgpage.BlockNumber(block)
	m.top = scrollTo(pages, m.top, m.block, m.visibleRows())

	return m
}

// visibleRows returns how many pages the navigator can list at once.
func (m Model) visibleRows() int {
	if m.height <= 0 {
		return defaultListRows // the size is not known yet
	}

	if rows := m.height - listChrome; rows > minListRows {
		return rows
	}

	return minListRows
}

// View returns the whole screen as text. It must not change the model nor
// read anything but it: Bubble Tea may call it after any message.
func (m Model) View() string {
	return strings.Join([]string{
		topBar(m.rel, m.block, m.width),
		"",
		pageList(m.rel.PageCount(), m.block, m.top, m.visibleRows()),
		"",
		helpLine,
	}, "\n") + "\n"
}

// helpLine is the reminder at the bottom of the screen. Step 26 replaces it
// with a real help component.
const helpLine = "↑↓/jk page · PgUp/PgDn · Home/End · q quit"

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
