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

	GoTo    key.Binding
	Open    key.Binding
	Hex     key.Binding
	Explain key.Binding
	Reload  key.Binding
	Help    key.Binding
	Quit    key.Binding
	Back    key.Binding
}

// ShortHelp returns the bindings of the one-line help at the bottom of the
// screen: the few that a user needs to get anywhere.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.GoTo, k.Hex, k.Explain, k.Help, k.Quit}
}

// FullHelp returns the bindings of the help screen, in columns.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Home, k.End, k.GoTo, k.Open, k.Hex, k.Explain},
		{k.Reload, k.Help, k.Back, k.Quit},
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
	Hex: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "hex"),
	),
	Explain: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "explain"),
	),
	Reload: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "read the page again"),
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
	Up:       describe(keys.Up, "previous line pointer"),
	Down:     describe(keys.Down, "next line pointer"),
	PageUp:   keys.PageUp,
	PageDown: keys.PageDown,
	Home:     describe(keys.Home, "first line pointer"),
	End:      describe(keys.End, "last line pointer"),
	GoTo:     describe(keys.GoTo, "go to line pointer"),
	Open:     describe(keys.Open, "tuple"),
	Hex:      describe(keys.Hex, "hex at line pointer"),
	Explain:  key.NewBinding(key.WithDisabled()),
	Reload:   keys.Reload,
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
	Hex:      describe(keys.Hex, "hex at tuple"),
	Explain:  key.NewBinding(key.WithDisabled()),
	Reload:   keys.Reload,
	Help:     keys.Help,
	Back:     describe(keys.Back, "line pointers"),
	Quit:     keys.Quit,
}

// hexKeys are the bindings of the hex view, where the keys scroll the dump.
// x closes the view it opened, so Back takes both x and Esc, and the help
// lists them once.
var hexKeys = keyMap{
	Up:       describe(keys.Up, "16 bytes up"),
	Down:     describe(keys.Down, "16 bytes down"),
	PageUp:   describe(keys.PageUp, "one screen up"),
	PageDown: describe(keys.PageDown, "one screen down"),
	Home:     describe(keys.Home, "start of the page"),
	End:      describe(keys.End, "end of the page"),
	GoTo:     key.NewBinding(key.WithDisabled()),
	Open:     key.NewBinding(key.WithDisabled()),
	Hex:      key.NewBinding(key.WithDisabled()),
	Explain:  key.NewBinding(key.WithDisabled()),
	Reload:   keys.Reload,
	Help:     keys.Help,
	Back: key.NewBinding(
		key.WithKeys(append(keys.Back.Keys(), keys.Hex.Keys()...)...),
		key.WithHelp("x/Esc", "close hex"),
	),
	Quit: keys.Quit,
}

// explainKeys are the bindings of explain mode, where the keys move the
// selection down the fields of the page header. Tab does too, as it does
// between the fields of a form. e closes the mode it opened, as x closes the
// hex view.
var explainKeys = keyMap{
	Up: key.NewBinding(
		key.WithKeys(append(keys.Up.Keys(), "shift+tab")...),
		key.WithHelp("↑/S-Tab", "previous field"),
	),
	Down: key.NewBinding(
		key.WithKeys(append(keys.Down.Keys(), "tab")...),
		key.WithHelp("↓/Tab", "next field"),
	),
	PageUp:   describe(keys.PageUp, "scroll explanation up"),
	PageDown: describe(keys.PageDown, "scroll explanation down"),
	Home:     describe(keys.Home, "first field"),
	End:      describe(keys.End, "last field"),
	GoTo:     key.NewBinding(key.WithDisabled()),
	Open:     key.NewBinding(key.WithDisabled()),
	Hex:      describe(keys.Hex, "show bytes"),
	Explain:  key.NewBinding(key.WithDisabled()),
	Reload:   keys.Reload,
	Help:     keys.Help,
	Back: key.NewBinding(
		key.WithKeys(append(keys.Explain.Keys(), keys.Back.Keys()...)...),
		key.WithHelp("e/Esc", "close explain"),
	),
	Quit: keys.Quit,
}
