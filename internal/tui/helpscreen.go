package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
)

// helpScreen renders the list of every key binding, the screen that `?`
// opens. It replaces the panels rather than floating over them: Lip Gloss
// draws blocks of text and has no layers, so an overlay would mean cutting
// the panels apart line by line and pasting the box into them.
func helpScreen(h help.Model, k keyMap) string {
	return strings.Join([]string{
		moreStyle.Render("pgpage · read-only"),
		"",
		h.FullHelpView(k.FullHelp()),
		"",
		moreStyle.Render("Nothing here writes to the relation file."),
	}, "\n")
}
