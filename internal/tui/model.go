// Package tui is the interactive page explorer, built with Bubble Tea.
//
// It follows the Elm architecture: Model holds every piece of state the
// screen is drawn from, Update folds one message into a new state, and View
// turns that state into the text of the whole screen.
package tui

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
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

	// views is the stack of views the user went through: the pages first,
	// then whatever Enter opened on top. Esc goes back down it.
	views []view

	// items is the line pointer view's state: the line pointers of the page
	// it was opened on, and which one is selected.
	items items

	// hex is the hex view's state: the page it dumps and what is selected.
	hex hexState

	// explain is explain mode's state: the header it explains and the field
	// in focus.
	explain explainState

	// Size of the terminal, in cells. Both are zero until the first
	// tea.WindowSizeMsg arrives, which Bubble Tea sends before anything
	// else, so View must cope with not knowing the size yet.
	width  int
	height int
}

// Lines the screen spends on everything but the rows of the navigator: the
// top bar and the rule below it, the top and bottom borders of the panels,
// the "N more" line, and the rule and the help line at the bottom.
const listChrome = 7

// Rows of the navigator when the terminal size is unknown, and the fewest it
// may shrink to.
const (
	defaultListRows = 10
	minListRows     = 1
)

// view is one of the screens of the explorer.
type view int

const (
	viewPages   view = iota // the relation: pages, page map and page header
	viewItems               // one page: its line pointers
	viewTuple               // one line pointer: its tuple
	viewHex                 // one page, byte by byte
	viewExplain             // the fields of one page header, explained
)

var _ tea.Model = Model{}

// New returns the model of an explorer on rel, which the caller must keep
// open until Run returns.
func New(rel *pgpage.Relation) Model {
	return Model{
		rel:       rel,
		summaries: summaryCache{},
		help:      help.New(),
		prompt:    newPrompt(),
		views:     []view{viewPages},
	}
}

// current returns the view on top of the stack, the one on screen.
func (m Model) current() view {
	return m.views[len(m.views)-1]
}

// push puts v on top of the view stack.
func (m Model) push(v view) Model {
	m.views = append(m.views, v)

	return m
}

// pop takes the view on top off the stack, back to the one below.
//
// The full slice expression caps the stack at its new length. Without it the
// popped stack would keep the old array's spare room, and the next push would
// write into the array that older copies of the model still read, changing
// the view those copies are on.
func (m Model) pop() Model {
	n := len(m.views) - 1
	m.views = m.views[:n:n]

	return m
}

// keyMap returns the bindings of the view on screen.
func (m Model) keyMap() keyMap {
	switch m.current() {
	case viewItems:
		return itemKeys
	case viewTuple:
		return tupleKeys
	case viewHex:
		return hexKeys
	case viewExplain:
		return explainKeys
	default:
		return keys
	}
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
		m.help.Width = m.innerWidth()
		m.help.Styles.ShortKey, m.help.Styles.ShortDesc, m.help.Styles.ShortSeparator = helpStyles()
		m.help.Styles.FullKey, m.help.Styles.FullDesc, m.help.Styles.FullSeparator = helpStyles()

		// A window of a different height holds a different number of rows,
		// so the navigator may have to scroll to keep showing the selection,
		// and it may now show pages nobody has read yet.
		m.top = scrollTo(m.rel.PageCount(), m.top, m.block, m.visibleRows())
		m.items.top = windowTop(len(m.items.ids), m.items.top, m.items.selected, m.itemRows())
		m.hex.vp.Width, m.hex.vp.Height = m.hexSize()

		return m, m.refresh()

	case hexMsg:
		// Same guard as the line pointers: a page the user already left.
		if msg.block != m.hex.block || m.current() != viewHex {
			return m, nil
		}

		m = m.showHex(msg.page, msg.err)

	case itemsMsg:
		// Line pointers of a page the user already left are of no use: by
		// the time they arrive, Esc and Enter may have opened another page.
		if msg.block != m.items.block {
			return m, nil
		}

		m.items.loaded = true
		m.items.page, m.items.header, m.items.ids, m.items.err = msg.page, msg.header, msg.ids, msg.err
		m.items.top = windowTop(len(m.items.ids), m.items.top, m.items.selected, m.itemRows())

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

		switch m.current() {
		case viewItems:
			return m.updateItems(msg)
		case viewTuple:
			return m.updateTuple(msg)
		case viewHex:
			return m.updateHex(msg)
		case viewExplain:
			return m.updateExplain(msg)
		}

		switch {
		case key.Matches(msg, keys.Quit):
			// tea.Quit is a command, not an action: returning it asks the
			// runtime to stop after this update.
			return m, tea.Quit

		case key.Matches(msg, keys.GoTo):
			if m.rel.PageCount() == 0 {
				return m, nil // no block to go to
			}

			var cmd tea.Cmd

			m.showHelp = false
			m.prompt, cmd = m.prompt.open(blockTarget(m.rel.PageCount()))

			return m, cmd

		case key.Matches(msg, keys.Open):
			return m.openItems()

		case key.Matches(msg, keys.Hex):
			// Every page has bytes, a new or invalid one included: that is
			// where looking at them helps most. Only this view has not read
			// the page yet.
			if m.rel.PageCount() == 0 {
				return m, nil
			}

			m = m.openHex(m.block, nil, headerHighlight(m.summaries[m.block]))

			return m, loadHex(m.rel, m.block)

		case key.Matches(msg, keys.Explain):
			return m.openExplain(), nil

		case key.Matches(msg, keys.Help):
			m.showHelp = !m.showHelp

		case key.Matches(msg, keys.Back):
			// Back closes the help; at the bottom of the stack there is
			// nothing else to go back to, so it quits.
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

// openItems opens the line pointer view on the selected page. Only a page
// whose header is valid has line pointers to show; on any other, Enter does
// nothing, and the page view keeps explaining why.
func (m Model) openItems() (tea.Model, tea.Cmd) {
	summary, cached := m.summaries[m.block]
	if !cached || summary.Status != pgpage.StatusOK {
		return m, nil
	}

	m.showHelp = false
	m = m.push(viewItems)
	m.items = items{block: m.block}

	return m, loadItems(m.rel, m.block)
}

// updateItems handles a key in the line pointer view. The keys that move
// are the ones of the page view, and here they move the selected line
// pointer instead of the selected page.
func (m Model) updateItems(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, itemKeys.Quit):
		return m, tea.Quit

	case key.Matches(msg, itemKeys.Help):
		m.showHelp = !m.showHelp

	case key.Matches(msg, itemKeys.Open):
		// Only a line pointer that points to tuple bytes has a tuple to
		// open; the bytes are already here, in the page this view read.
		if m.items.selected < len(m.items.ids) && hasTuple(m.items.ids[m.items.selected]) {
			m.showHelp = false
			m = m.push(viewTuple)
		}

	case key.Matches(msg, itemKeys.Hex):
		if m.items.loaded && m.items.page != nil {
			m = m.openHex(m.items.block, m.items.page, entryHighlight(m.items))
		}

	case key.Matches(msg, itemKeys.GoTo):
		if len(m.items.ids) == 0 {
			return m, nil // not read yet, or a page without line pointers
		}

		var cmd tea.Cmd

		m.showHelp = false
		m.prompt, cmd = m.prompt.open(itemTarget(len(m.items.ids)))

		return m, cmd

	case key.Matches(msg, itemKeys.Back):
		if m.showHelp {
			m.showHelp = false
			break
		}

		m = m.pop()

	case key.Matches(msg, itemKeys.Up):
		m = m.selectItem(m.items.selected - 1)
	case key.Matches(msg, itemKeys.Down):
		m = m.selectItem(m.items.selected + 1)
	case key.Matches(msg, itemKeys.PageUp):
		m = m.selectItem(m.items.selected - m.itemRows())
	case key.Matches(msg, itemKeys.PageDown):
		m = m.selectItem(m.items.selected + m.itemRows())
	case key.Matches(msg, itemKeys.Home):
		m = m.selectItem(0)
	case key.Matches(msg, itemKeys.End):
		m = m.selectItem(len(m.items.ids) - 1)
	}

	return m, nil
}

// updateTuple handles a key in the tuple view. Moving goes to the previous
// or next line pointer that has a tuple, and changes the selection of the
// line pointer view too: both views look at the same selected line pointer,
// so Esc lands on the tuple that was last shown.
func (m Model) updateTuple(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, tupleKeys.Quit):
		return m, tea.Quit

	case key.Matches(msg, tupleKeys.Help):
		m.showHelp = !m.showHelp

	case key.Matches(msg, tupleKeys.Back):
		if m.showHelp {
			m.showHelp = false
			break
		}

		m = m.pop()

	case key.Matches(msg, tupleKeys.Hex):
		m = m.openHex(m.items.block, m.items.page, tupleHighlight(m.items))

	case key.Matches(msg, tupleKeys.Up):
		m = m.selectTuple(m.items.selected-1, -1)
	case key.Matches(msg, tupleKeys.Down):
		m = m.selectTuple(m.items.selected+1, 1)
	case key.Matches(msg, tupleKeys.Home):
		m = m.selectTuple(0, 1)
	case key.Matches(msg, tupleKeys.End):
		m = m.selectTuple(len(m.items.ids)-1, -1)
	}

	return m, nil
}

// openExplain opens explain mode on the header of the selected page, with
// pd_lower selected: the field whose numbers say the most about the page.
// Like the line pointers, it needs a header that parsed.
func (m Model) openExplain() Model {
	summary, cached := m.summaries[m.block]
	if !cached || summary.Status != pgpage.StatusOK {
		return m
	}

	m.showHelp = false
	m = m.push(viewExplain)
	m.explain = explainState{block: m.block, summary: summary, field: explainLower}

	return m
}

// explainLower is the index of pd_lower in explanations.
var explainLower = slices.IndexFunc(explanations, func(e explanation) bool { return e.field == "pd_lower" })

// updateExplain handles a key in explain mode: the moving keys select a
// field of the header, x shows its bytes, and e or Esc close the mode.
func (m Model) updateExplain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, explainKeys.Quit):
		return m, tea.Quit

	case key.Matches(msg, explainKeys.Help):
		m.showHelp = !m.showHelp

	case key.Matches(msg, explainKeys.Back):
		if m.showHelp {
			m.showHelp = false
			break
		}

		m = m.pop()

	case key.Matches(msg, explainKeys.Hex):
		// Esc in the hex view comes back here, to the same field.
		m = m.openHex(m.explain.block, nil, explanations[m.explain.field].highlight())

		return m, loadHex(m.rel, m.explain.block)

	case key.Matches(msg, explainKeys.Up):
		m = m.selectField(m.explain.field - 1)
	case key.Matches(msg, explainKeys.Down):
		m = m.selectField(m.explain.field + 1)
	case key.Matches(msg, explainKeys.Home):
		m = m.selectField(0)
	case key.Matches(msg, explainKeys.End):
		m = m.selectField(len(explanations) - 1)

	case key.Matches(msg, explainKeys.PageUp):
		_, _, rows := m.explainLayout()
		m.explain.scroll = max(m.explain.scroll-(rows-2), 0)
	case key.Matches(msg, explainKeys.PageDown):
		lines, _, rows := m.explainLayout()
		m.explain.scroll = min(m.explain.scroll+(rows-2), maxScroll(len(lines), rows))
	}

	return m, nil
}

// selectField selects a field, clamped to the fields of the header as the
// lists clamp their selection, and shows its explanation from the top.
func (m Model) selectField(index int) Model {
	m.explain.field = min(max(index, 0), len(explanations)-1)
	m.explain.scroll = 0

	return m
}

// openHex opens the hex view on block with hl selected. page is nil when the
// view that opens it has not read the page, and a hexMsg brings it later.
func (m Model) openHex(block pgpage.BlockNumber, page []byte, hl highlight) Model {
	m.showHelp = false
	m = m.push(viewHex)
	m.hex = hexState{block: block, hl: hl}

	if page != nil {
		m = m.showHex(page, nil)
	}

	return m
}

// showHex fills the hex view with a page. The whole dump goes into the
// viewport once; scrolling only moves which lines of it are drawn.
func (m Model) showHex(page []byte, err error) Model {
	m.hex.loaded, m.hex.page, m.hex.err = true, page, err
	if err != nil {
		return m
	}

	width, height := m.hexSize()

	m.hex.vp = viewport.New(width, height)
	m.hex.vp.SetContent(strings.Join(hexLines(page, m.hex.hl), "\n"))

	// Scroll so the selection starts a couple of lines below the top, with
	// the bytes before it in view as context.
	if m.hex.hl.start < m.hex.hl.end {
		m.hex.vp.SetYOffset(m.hex.hl.start/pgpage.HexBytesPerLine - 2)
	}

	return m
}

// hexSize returns the size of the hex viewport: the panel less its frame
// and the lines around the dump. The frame takes panelFrame columns but only
// two lines, its top and bottom borders.
func (m Model) hexSize() (width, height int) {
	width = max(m.innerWidth()-panelFrame, 1)
	height = max(m.bodyHeight("")-2-hexChrome, 1)

	if m.height <= 0 {
		height = defaultListRows
	}

	return width, height
}

// updateHex handles a key in the hex view: the moving keys scroll the dump,
// and x or Esc close it.
func (m Model) updateHex(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, hexKeys.Quit):
		return m, tea.Quit

	case key.Matches(msg, hexKeys.Help):
		m.showHelp = !m.showHelp

	case key.Matches(msg, hexKeys.Back):
		if m.showHelp {
			m.showHelp = false
			break
		}

		m = m.pop()

	case key.Matches(msg, hexKeys.Up):
		m.hex.vp.ScrollUp(1)
	case key.Matches(msg, hexKeys.Down):
		m.hex.vp.ScrollDown(1)
	case key.Matches(msg, hexKeys.PageUp):
		m.hex.vp.PageUp()
	case key.Matches(msg, hexKeys.PageDown):
		m.hex.vp.PageDown()
	case key.Matches(msg, hexKeys.Home):
		m.hex.vp.GotoTop()
	case key.Matches(msg, hexKeys.End):
		m.hex.vp.GotoBottom()
	}

	return m, nil
}

// selectTuple selects the first line pointer with a tuple found walking from
// index in step direction. When there is none that way, the selection stays.
func (m Model) selectTuple(index, step int) Model {
	for i := index; i >= 0 && i < len(m.items.ids); i += step {
		if hasTuple(m.items.ids[i]) {
			return m.selectItem(i)
		}
	}

	return m
}

// selectItem selects the line pointer at index, clamped to the array, and
// scrolls the list the least it takes to show it.
func (m Model) selectItem(index int) Model {
	if len(m.items.ids) == 0 {
		return m
	}

	m.items.selected = min(max(index, 0), len(m.items.ids)-1)
	m.items.top = windowTop(len(m.items.ids), m.items.top, m.items.selected, m.itemRows())

	return m
}

// itemRows returns how many line pointers the list can show at once: the
// rows of the page navigator less its column titles and the state counts.
func (m Model) itemRows() int {
	return max(m.visibleRows()-itemListChrome, minListRows)
}

// itemListChrome is the lines of the line pointer list that are not rows,
// besides the "N more" line listChrome already counts: the column titles,
// and the blank line and the counts below the rows.
const itemListChrome = 3

// updatePrompt handles a key while the "go to block" prompt is open: Enter
// jumps if what was typed is a block of this relation, Esc gives up, and
// everything else is line editing.
func (m Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.prompt = m.prompt.close()

		return m, nil

	case tea.KeyEnter:
		n, err := m.prompt.target.parse(m.prompt.input.Value())
		if err != nil {
			// The prompt stays open with the text and the reason, so the
			// user can fix a typo instead of typing it all again.
			m.prompt.err = err.Error()

			return m, nil
		}

		// The prompt moves the list of the view it was opened in.
		target := m.prompt.target
		m.prompt = m.prompt.close()

		if target.view == viewItems {
			return m.selectItem(int(n) - int(pgpage.FirstOffsetNumber)), nil
		}

		m = m.selectBlock(int64(n))

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
	km := m.keyMap()
	short := km.ShortHelp()

	if m.current() != viewPages {
		// Esc goes back from any view but the first, where it quits and q
		// already says so.
		short = append(short[:len(short)-2:len(short)-2], km.Back, km.Help, km.Quit)
	}

	body, footer := m.body(), m.help.ShortHelpView(short)

	switch m.current() {
	case viewItems:
		body = m.itemsBody()
	case viewTuple:
		body = m.tupleBody()
	case viewHex:
		body = m.hexBody()
	case viewExplain:
		body = m.explainBody()
	}

	if m.prompt.active {
		footer = m.prompt.view()
	}

	if m.showHelp {
		content := helpScreen(m.help, km)
		width := m.innerWidth()

		if width <= 0 {
			width = boxWidth("KEYS", content)
		}

		body = panel("KEYS", content, width, m.bodyHeight(content), borderStyle)
		footer = m.help.ShortHelpView([]key.Binding{km.Help, describe(km.Back, "close the help")})
	}

	width := m.innerWidth()

	screen := strings.Join([]string{
		topBar(m.rel, m.block, width),
		rule(width),
		body,
		rule(width),
		footer,
	}, "\n")

	if width > 0 {
		// The last word on the size of the screen: a line wider than the
		// terminal would wrap and push everything below it down. Panels are
		// laid out to fit, but fixed text such as the help line is not.
		screen = lipgloss.NewStyle().MaxWidth(width).Render(screen)
	}

	// The same margin on both sides: every line starts after it, and none
	// reaches the last columns, since everything was laid out to width.
	lines := strings.Split(screen, "\n")
	for i, line := range lines {
		lines[i] = screenMargin + line
	}

	screen = strings.Join(lines, "\n")

	return screen + "\n"
}

// panelGap separates the panels of the body. One column looks as wide as
// the one row between the panels and the rules above and below them, since a
// terminal cell is about twice as tall as it is wide.
const panelGap = " "

// screenMargin separates the screen from the left and right edges of the
// terminal. It is the gap between panels, so every separation on screen
// looks the same.
const screenMargin = panelGap

// innerWidth returns the columns the screen is laid out in: the terminal
// minus the margin on each side. It is zero while the size is unknown.
func (m Model) innerWidth() int {
	if m.width <= 0 {
		return 0
	}

	return max(m.width-2*lipgloss.Width(screenMargin), 0)
}

// Widths a panel needs around its content: two borders and the padding.
const panelFrame = 2 + 2*panelPadding

// The narrowest page map worth drawing, and how wide the panels beside the
// navigator may be while the terminal size is unknown.
const (
	minMapWidth        = minMapCells + offsetLabel + panelFrame
	defaultDetailWidth = 64
)

// body renders the panels side by side: the navigator, the page map and the
// page header, each in its own frame. Panels are dropped when the terminal
// cannot hold them; the navigator is the one the keys act on, so it stays.
func (m Model) body() string {
	summary, cached := m.summaries[m.block]

	list := pageList(m.rel.PageCount(), m.block, m.top, m.visibleRows(), m.summaries)
	height := m.bodyHeight(list)

	listBox := panel("PAGES", list, boxWidth("PAGES", list), height, borderStyle)
	if m.rel.PageCount() == 0 {
		return listBox
	}

	left := m.detailWidth(listBox)

	// A page with no layout gets a panel of its own instead of a map and a
	// header full of dashes.
	if cached && summary.Status != pgpage.StatusOK {
		if left < minStatusWidth+panelFrame {
			return listBox
		}

		// Sentences are read line by line: past some length the eye loses
		// the start of the next one, so the panel does not take the whole
		// terminal however wide it is.
		title := statusTitle(m.block, summary)
		width := min(left, maxStatusWidth+panelFrame)
		box := panel(title, statusPanel(summary, width-panelFrame), width, height, borderStyle)

		return lipgloss.JoinHorizontal(lipgloss.Top, listBox, panelGap, box)
	}

	header := headerPanel(summary, cached)
	headerWidth := headerBoxWidth(header)

	if headerWidth > left {
		return listBox
	}

	panels := []string{listBox}

	// The map takes what the other two panels leave: it is the one that can
	// be drawn at any width.
	if mapWidth := left - headerWidth - lipgloss.Width(panelGap); mapWidth >= minMapWidth {
		title := fmt.Sprintf("PAGE %d — %d BYTES", m.block, pgpage.PageSize)
		content := pageMap(summary, cached, mapWidth-panelFrame)

		panels = append(panels, panel(title, content, mapWidth, height, borderStyle))
	}

	panels = append(panels, panel(headerTitle, header, headerWidth, height, borderStyle))

	return lipgloss.JoinHorizontal(lipgloss.Top, join(panels, panelGap)...)
}

// itemsBody renders the line pointer view: the list of line pointers, the
// page map with the selected one's tuple highlighted, and the selected line
// pointer in detail. Like the page view, it drops the map first and keeps
// the list, which is what the keys act on.
func (m Model) itemsBody() string {
	title := "LINE POINTERS"
	if m.items.loaded {
		title = fmt.Sprintf("LINE POINTERS · %d", len(m.items.ids))
	}

	list := itemList(m.items, m.itemRows())
	height := m.bodyHeight(list)

	listBox := panel(title, list, boxWidth(title, list), height, borderStyle)
	left := m.detailWidth(listBox)

	detailTitle, detail := itemTitle(m.items), itemPanel(m.items)
	detailWidth := max(boxWidth(detailTitle, detail), fieldColumn+itemPanelWidth+panelFrame)

	if detailWidth > left {
		return listBox
	}

	panels := []string{listBox}

	if mapWidth := left - detailWidth - lipgloss.Width(panelGap); mapWidth >= minMapWidth {
		summary, cached := m.summaries[m.items.block]
		mapTitle := fmt.Sprintf("PAGE %d — %d BYTES", m.items.block, pgpage.PageSize)
		content := pageMapWith(summary, cached, mapWidth-panelFrame, itemHighlight(m.items))

		panels = append(panels, panel(mapTitle, content, mapWidth, height, borderStyle))
	}

	panels = append(panels, panel(detailTitle, detail, detailWidth, height, borderStyle))

	return lipgloss.JoinHorizontal(lipgloss.Top, join(panels, panelGap)...)
}

// tupleBody renders the tuple view: the fields of the tuple header on the
// left, and on the right what its flags mean and the bytes of its data.
func (m Model) tupleBody() string {
	title, fields := tupleTitle(m.items), tuplePanel(m.items)
	height := m.bodyHeight(fields)

	width := max(boxWidth(title, fields), fieldColumn+tupleValueWidth+panelFrame)
	box := panel(title, fields, width, height, borderStyle)

	left := m.detailWidth(box)
	if left < minFlagsWidth+panelFrame {
		return box
	}

	flags := panel("DECODED FLAGS", flagsPanel(m.items, left-panelFrame), left, height, borderStyle)

	return lipgloss.JoinHorizontal(lipgloss.Top, box, panelGap, flags)
}

// hexBody renders the hex view: one panel with the dump of the page, as wide
// as the screen, and the selection described below it.
func (m Model) hexBody() string {
	title := fmt.Sprintf("BYTES · BLOCK %d", m.hex.block)

	var content string

	switch {
	case !m.hex.loaded:
		content = moreStyle.Render("reading…")
	case m.hex.err != nil:
		content = invalidStyle.Render(m.hex.err.Error())
	default:
		content = hexColumns() + "\n" + m.hex.vp.View() + "\n\n" + hexDetail(m.hex.page, m.hex.hl)
	}

	width := m.innerWidth()
	if width <= 0 {
		width = boxWidth(title, content)
	}

	return panel(title, content, width, m.bodyHeight(content), borderStyle)
}

// explainBody renders explain mode: the page header on the left, as the
// page view shows it, with the selected field marked, and its explanation on
// the right, scrolled if it is taller than the panel. A terminal too narrow
// for both keeps the explanation, whose title names the field.
func (m Model) explainBody() string {
	summary := m.explain.summary
	e := explanations[m.explain.field]

	list := explainList(summary, m.explain.field)
	lines, width, rows := m.explainLayout()
	height := rows + 2

	detail := panel(e.title(summary.Header), strings.Join(scrollLines(lines, m.explain.scroll, rows), "\n"),
		width, height, borderStyle)

	if listWidth := explainListWidth + panelFrame; width+lipgloss.Width(panelGap)+listWidth <= m.innerWidth() {
		listBox := panel(headerTitle, list, listWidth, height, borderStyle)

		return lipgloss.JoinHorizontal(lipgloss.Top, listBox, panelGap, detail)
	}

	return detail
}

// explainLayout returns the lines of the selected field's explanation, the
// width of its panel and how many lines of it the panel shows. The list
// takes a fixed width, and the explanation the rest, or the whole screen when
// the rest is too narrow to read.
func (m Model) explainLayout() (lines []string, width, rows int) {
	e := explanations[m.explain.field]

	width = m.innerWidth()
	if width <= 0 {
		width = 2 * defaultDetailWidth
	}

	if rest := width - explainListWidth - panelFrame - lipgloss.Width(panelGap); rest >= minExplainWidth+panelFrame {
		width = rest
	}

	lines = strings.Split(explainPanel(e, m.explain.summary.Header, width-panelFrame), "\n")
	rows = m.bodyHeight(explainList(m.explain.summary, m.explain.field)) - 2

	return lines, width, rows
}

// bodyHeight returns how many lines the framed panels take: everything the
// terminal leaves, or what the navigator needs while the size is unknown.
func (m Model) bodyHeight(list string) int {
	if m.height <= 0 {
		return lipgloss.Height(list) + panelFrame
	}

	return max(m.height-bodyChrome, 3)
}

// detailWidth returns the cells left for the panels beside the navigator.
func (m Model) detailWidth(listBox string) int {
	if m.width <= 0 {
		return defaultDetailWidth
	}

	return m.innerWidth() - lipgloss.Width(listBox) - lipgloss.Width(panelGap)
}

// statusTitle names the panel of a page that has no layout.
func statusTitle(block pgpage.BlockNumber, summary pgpage.PageSummary) string {
	if summary.Status == pgpage.StatusNew {
		return fmt.Sprintf("PAGE %d", block)
	}

	return fmt.Sprintf("BLOCK %d", block)
}

// clip cuts a block taller than the terminal, which would otherwise scroll
// the screen and break the drawing.
func (m Model) clip(block string) string {
	if m.height <= 0 {
		return block
	}

	return lipgloss.NewStyle().MaxHeight(m.height - bodyChrome).Render(block)
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
