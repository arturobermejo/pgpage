package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
)

// bindings returns every binding of the key map, by field name.
func bindings() map[string]key.Binding {
	return map[string]key.Binding{
		"Up":       keys.Up,
		"Down":     keys.Down,
		"PageUp":   keys.PageUp,
		"PageDown": keys.PageDown,
		"Home":     keys.Home,
		"End":      keys.End,
		"GoTo":     keys.GoTo,
		"Open":     keys.Open,
		"Help":     keys.Help,
		"Quit":     keys.Quit,
		"Back":     keys.Back,
	}
}

// No key may trigger two actions: Update tries the bindings in order, so a
// repeated key would make the second one unreachable.
func TestKeysAreUnique(t *testing.T) {
	owner := map[string]string{}

	for name, binding := range bindings() {
		for _, k := range binding.Keys() {
			if other, taken := owner[k]; taken {
				t.Errorf("key %q is bound to both %s and %s", k, other, name)
			}

			owner[k] = name
		}
	}
}

// Every binding is described, because the help is generated from them.
func TestKeysAreDescribed(t *testing.T) {
	for name, binding := range bindings() {
		h := binding.Help()

		if h.Key == "" || h.Desc == "" {
			t.Errorf("%s has no help: key %q, description %q", name, h.Key, h.Desc)
		}

		if strings.ToLower(h.Desc) != h.Desc {
			t.Errorf("%s describes itself as %q; the help reads better in lower case", name, h.Desc)
		}
	}
}

// The short help is a subset of the full one: the bottom line may leave
// bindings out, but it cannot invent any.
func TestShortHelpIsPartOfFullHelp(t *testing.T) {
	full := map[string]bool{}

	for _, column := range keys.FullHelp() {
		for _, binding := range column {
			full[binding.Help().Key] = true
		}
	}

	for _, binding := range keys.ShortHelp() {
		if !full[binding.Help().Key] {
			t.Errorf("%q is in the short help but not in the full one", binding.Help().Key)
		}
	}
}

// Every binding of the key map appears in the full help, so that no key is
// left undocumented.
func TestFullHelpCoversEveryKey(t *testing.T) {
	shown := map[string]bool{}

	for _, column := range keys.FullHelp() {
		for _, binding := range column {
			shown[binding.Help().Key] = true
		}
	}

	for name, binding := range bindings() {
		if !shown[binding.Help().Key] {
			t.Errorf("%s is bound but missing from the help screen", name)
		}
	}
}

// The line pointer view moves with the same keys as the page view; only the
// descriptions change. A key that differs would be a key to learn twice.
func TestItemKeysMatchPageKeys(t *testing.T) {
	pairs := map[string][2]key.Binding{
		"Up":       {keys.Up, itemKeys.Up},
		"Down":     {keys.Down, itemKeys.Down},
		"PageUp":   {keys.PageUp, itemKeys.PageUp},
		"PageDown": {keys.PageDown, itemKeys.PageDown},
		"Home":     {keys.Home, itemKeys.Home},
		"End":      {keys.End, itemKeys.End},
		"GoTo":     {keys.GoTo, itemKeys.GoTo},
		"Help":     {keys.Help, itemKeys.Help},
		"Back":     {keys.Back, itemKeys.Back},
		"Quit":     {keys.Quit, itemKeys.Quit},
	}

	for name, pair := range pairs {
		if strings.Join(pair[0].Keys(), ",") != strings.Join(pair[1].Keys(), ",") {
			t.Errorf("%s: page view keys %v, line pointer view keys %v", name, pair[0].Keys(), pair[1].Keys())
		}
	}

	for name, binding := range map[string]key.Binding{"Open": itemKeys.Open} {
		if binding.Enabled() {
			t.Errorf("%s is enabled in the line pointer view, where it does nothing", name)
		}
	}
}
