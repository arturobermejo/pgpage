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

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

	// summaries is what has been read from disk so far. A map is a
	// reference, so every copy of the Model shares this one; that is what
	// makes it a cache and not a snapshot, and why only Update writes to it.
	summaries summaryCache

	// help draws the key bindings, and showHelp says whether the full list
	// has taken over the screen.
	help     help.Model
	showHelp bool

	// prompt is the "go to block" input. While it is open it takes every
	// key, so the shortcuts below are not reachable.
	prompt prompt

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
	return Model{rel: rel, summaries: summaryCache{}, help: help.New(), prompt: newPrompt()}
}

// Init returns the command to run before the first View: reading the pages
// the navigator starts on. The screen is drawn before it finishes.
func (m Model) Init() tea.Cmd {
	return m.refresh()
}

// refresh returns the command that reads the pages the navigator shows and
// the cache does not have yet, or nil when there is nothing to read.
func (m Model) refresh() tea.Cmd {
	pages := m.rel.PageCount()
	if pages == 0 {
		return nil
	}

	last := lastVisible(pages, m.top, m.visibleRows())

	lo, hi, missing := m.summaries.missingRange(m.top, last)
	if !missing {
		return nil
	}

	return loadSummaries(m.rel, lo, hi)
}

// Update returns the state that msg leads to, and a command to run next.
// Bubble Tea calls it once per message, never concurrently.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// The size is state like any other: it arrives as a message and the
		// views read it from the model when they are drawn.
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width

		// A window of a different height holds a different number of rows,
		// so the navigator may have to scroll to keep showing the selection,
		// and it may now show pages nobody has read yet.
		m.top = scrollTo(m.rel.PageCount(), m.top, m.block, m.visibleRows())

		return m, m.refresh()

	case summariesMsg:
		// The command read these pages while the UI went on. Writing them
		// here, and only here, keeps the cache to a single goroutine.
		for block, summary := range msg.summaries {
			m.summaries[block] = summary
		}

	case tea.KeyMsg:
		// An open prompt owns the keyboard: it must come before every
		// shortcut, or typing "q" in it would quit the program.
		if m.prompt.active {
			return m.updatePrompt(msg)
		}

		switch {
		case key.Matches(msg, keys.Quit):
			// tea.Quit is a command, not an action: returning it asks the
			// runtime to stop after this update.
			return m, tea.Quit

		case key.Matches(msg, keys.GoTo):
			var cmd tea.Cmd

			m.showHelp = false
			m.prompt, cmd = m.prompt.open()

			return m, cmd

		case key.Matches(msg, keys.Help):
			m.showHelp = !m.showHelp

		case key.Matches(msg, keys.Back):
			// Back closes the help; at the top level there is nothing else
			// to go back to, so it quits.
			if !m.showHelp {
				return m, tea.Quit
			}

			m.showHelp = false

		case key.Matches(msg, keys.Up):
			m = m.move(-1)
		case key.Matches(msg, keys.Down):
			m = m.move(1)
		case key.Matches(msg, keys.PageUp):
			m = m.move(-m.visibleRows())
		case key.Matches(msg, keys.PageDown):
			m = m.move(m.visibleRows())
		case key.Matches(msg, keys.Home):
			m = m.selectBlock(0)
		case key.Matches(msg, keys.End):
			m = m.selectBlock(int64(m.rel.PageCount()) - 1)
		default:
			return m, nil
		}

		// Moving may have scrolled the window onto pages nobody has read.
		return m, m.refresh()
	}

	return m, nil
}

// updatePrompt handles a key while the "go to block" prompt is open: Enter
// jumps if what was typed is a block of this relation, Esc gives up, and
// everything else is line editing.
func (m Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.prompt = m.prompt.close()

		return m, nil

	case tea.KeyEnter:
		block, err := parseBlock(m.prompt.input.Value(), m.rel.PageCount())
		if err != nil {
			// The prompt stays open with the text and the reason, so the
			// user can fix a typo instead of typing it all again.
			m.prompt.err = err.Error()

			return m, nil
		}

		m.prompt = m.prompt.close()
		m = m.selectBlock(int64(block))

		return m, m.refresh()

	default:
		var cmd tea.Cmd

		m.prompt.err = "" // typing again clears the last complaint
		m.prompt, cmd = m.prompt.update(msg)

		return m, cmd
	}
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
	body, footer := m.body(), m.help.ShortHelpView(keys.ShortHelp())

	if m.prompt.active {
		footer = m.prompt.view(m.rel.PageCount())
	}

	if m.showHelp {
		body = m.clip(helpScreen(m.help, keys))
		footer = m.help.ShortHelpView([]key.Binding{keys.Help, keys.Back})
	}

	screen := strings.Join([]string{
		topBar(m.rel, m.block, m.width),
		"",
		body,
		"",
		footer,
	}, "\n")

	if m.width > 0 {
		// The last word on the size of the screen: a line wider than the
		// terminal would wrap and push everything below it down. Panels are
		// laid out to fit, but fixed text such as the help line is not.
		screen = lipgloss.NewStyle().MaxWidth(m.width).Render(screen)
	}

	return screen + "\n"
}

// panelGap separates the panels of the body.
const panelGap = "   "

// minMapWidth is the narrowest page map worth drawing. Below it the bar
// cannot tell the regions apart and the legend does not fit.
const minMapWidth = 28

// body renders the panels side by side: the navigator, the page map and the
// page header. Panels are dropped, widest first, when the terminal cannot
// hold them; the navigator is the one the keys act on, so it always stays.
func (m Model) body() string {
	list := pageList(m.rel.PageCount(), m.block, m.top, m.visibleRows(), m.summaries)

	summary, cached := m.summaries[m.block]
	if m.rel.PageCount() == 0 {
		return list
	}

	panels := []string{list}
	left := m.width - lipgloss.Width(list) - lipgloss.Width(panelGap)

	if m.width <= 0 {
		left = defaultDetailWidth
	}

	// A page with no layout gets a panel of its own instead of a map and a
	// header full of dashes.
	if cached && summary.Status != pgpage.StatusOK {
		if left < minStatusWidth {
			return list
		}

		// Sentences are read line by line: past some length the eye loses
		// the start of the next one, so the panel does not take the whole
		// terminal however wide it is.
		panel := statusPanel(m.block, summary, min(left, maxStatusWidth))

		return m.clip(lipgloss.JoinHorizontal(lipgloss.Top, list, panelGap, panel))
	}

	if header := headerPanel(summary, cached); m.width <= 0 || lipgloss.Width(header) <= left {
		// The map takes what the other two panels leave: it is the one that
		// can be drawn at any width.
		if mapWidth := left - lipgloss.Width(header) - lipgloss.Width(panelGap); mapWidth >= minMapWidth {
			panels = append(panels, pageMap(m.block, summary, cached, mapWidth))
		}

		panels = append(panels, header)
	}

	return m.clip(lipgloss.JoinHorizontal(lipgloss.Top, join(panels, panelGap)...))
}

// defaultDetailWidth is how wide the panels beside the navigator may be
// while the terminal size is unknown.
const defaultDetailWidth = 60

// clip cuts a body taller than the terminal, which would otherwise scroll
// the screen and break the drawing.
func (m Model) clip(body string) string {
	if m.height <= 0 {
		return body
	}

	return lipgloss.NewStyle().MaxHeight(m.height - bodyChrome).Render(body)
}

// join returns the blocks with sep between each pair, ready for
// JoinHorizontal, which takes the blocks as separate arguments.
func join(blocks []string, sep string) []string {
	out := make([]string, 0, 2*len(blocks)-1)

	for i, block := range blocks {
		if i > 0 {
			out = append(out, sep)
		}

		out = append(out, block)
	}

	return out
}

// bodyChrome is the number of lines around the body: the top bar, the help
// line and the blank line before each.
const bodyChrome = 4

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
