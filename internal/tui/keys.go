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

	Help key.Binding
	Quit key.Binding
	Back key.Binding
}

// ShortHelp returns the bindings of the one-line help at the bottom of the
// screen: the few that a user needs to get anywhere.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Home, k.End, k.Help, k.Quit}
}

// FullHelp returns the bindings of the help screen, in columns.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Home, k.End},
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
	Help: key.NewBinding(
		key.WithKeys("?", "h"),
		key.WithHelp("?", "keys"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("Esc", "close the help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}
