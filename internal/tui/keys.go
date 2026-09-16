package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap declares every shortcut of the explorer: which keys trigger it and
// how it is described in the help.
//
// Declaring them as data, instead of matching strings inside Update, means
// the help can be generated from the same source that handles the keys, so
// the two cannot disagree. It is also where an unbound key would show up: a
// binding nobody matches on is dead code the compiler points at.
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding

	GoTo key.Binding
	Open key.Binding
	Help key.Binding
	Quit key.Binding
	Back key.Binding
}

// ShortHelp returns the bindings of the one-line help at the bottom of the
// screen: the few that a user needs to get anywhere.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.GoTo, k.Help, k.Quit}
}

// FullHelp returns the bindings of the help screen, in columns.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Home, k.End, k.GoTo, k.Open},
		{k.Help, k.Back, k.Quit},
	}
}

// keys are the bindings of the explorer. The help text of each one is
// written for the user, not for the code: "next page", not "move down".
var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "previous page"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "next page"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("PgUp", "one screen up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("PgDn", "one screen down"),
	),
	Home: key.NewBinding(
		key.WithKeys("home"),
		key.WithHelp("Home", "first page"),
	),
	End: key.NewBinding(
		key.WithKeys("end"),
		key.WithHelp("End", "last page"),
	),
	GoTo: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "go to block"),
	),
	Open: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("↵", "line pointers"),
	),
	Help: key.NewBinding(
		key.WithKeys("?", "h"),
		key.WithHelp("?", "keys"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("Esc", "back"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

// itemKeys are the bindings of the line pointer view. The keys are the same
// as in the page view, so moving works the same everywhere; what changes is
// what they move, and so how the help describes them. Keys that do nothing
// in this view are disabled, which also hides them from the help.
var itemKeys = keyMap{
	Up:       describe(keys.Up, "previous item"),
	Down:     describe(keys.Down, "next item"),
	PageUp:   keys.PageUp,
	PageDown: keys.PageDown,
	Home:     describe(keys.Home, "first item"),
	End:      describe(keys.End, "last item"),
	GoTo:     describe(keys.GoTo, "go to line pointer"),
	Open:     describe(keys.Open, "tuple"),
	Help:     keys.Help,
	Back:     describe(keys.Back, "page view"),
	Quit:     keys.Quit,
}

// describe returns binding with the same keys and another description.
func describe(binding key.Binding, desc string) key.Binding {
	return key.NewBinding(
		key.WithKeys(binding.Keys()...),
		key.WithHelp(binding.Help().Key, desc),
	)
}

// tupleKeys are the bindings of the tuple view, where moving goes from one
// tuple of the page to the next. Line pointers without a tuple are skipped,
// so a screen at a time means nothing here and PgUp and PgDn are disabled.
var tupleKeys = keyMap{
	Up:       describe(keys.Up, "previous tuple"),
	Down:     describe(keys.Down, "next tuple"),
	PageUp:   key.NewBinding(key.WithDisabled()),
	PageDown: key.NewBinding(key.WithDisabled()),
	Home:     describe(keys.Home, "first tuple"),
	End:      describe(keys.End, "last tuple"),
	GoTo:     key.NewBinding(key.WithDisabled()),
	Open:     key.NewBinding(key.WithDisabled()),
	Help:     keys.Help,
	Back:     describe(keys.Back, "line pointers"),
	Quit:     keys.Quit,
}
