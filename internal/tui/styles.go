package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// The palette of the interface, in one place so that the panels cannot drift
// apart. Colors are ANSI 256 numbers, the closest ones to the mockups, which
// Lip Gloss degrades on terminals that show fewer colors, down to no color
// at all.
//
// Meaning is never carried by color alone: everything colored here is also
// spelled out or marked with a glyph.
var (
	// Chrome: the name, the top bar, panel borders and titles.
	nameStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("177"))
	pathStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	factStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	blockStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("80"))
	borderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	ruleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	// Content: rows, field names and values.
	rowStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("177"))
	fieldStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	valueStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	numberStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("180"))
	moreStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	offsetStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	// Page status.
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	newStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("80"))
	invalidStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))

	// Line pointer states. A dead line pointer is part of a page's normal
	// life, not damage, so it is a dusty red and not the red of INVALID.
	deadStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("174"))
	redirectStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("146"))

	// Bytes of the hex view: zeroes dimmed, and the selection in the colors
	// the page map highlights it with.
	zeroByteStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedByteStyle = lipgloss.NewStyle().Background(lipgloss.Color("177")).Foreground(lipgloss.Color("234"))
	pointedByteStyle  = lipgloss.NewStyle().Background(lipgloss.Color("133")).Foreground(lipgloss.Color("234"))

	// A caveat the reader should keep in mind, such as the tuple bytes that
	// cannot be decoded without the table's schema.
	noteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))

	// Key bindings in the help line.
	helpKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	helpDescStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// regionColors are the two colors of a region of the page map: the
// background of its cells, and a darker shade of the same hue for the lines
// drawn over it.
type regionColors struct {
	background lipgloss.Color
	line       lipgloss.Color
}

// regionPalette colors the regions of the page map.
//
// They are muted grays with a tint each, on purpose: green, amber, red and
// cyan belong to the page states (OK, WARN, INVALID, NEW), and a region
// painted in one of them would read as a state.
var regionPalette = map[pgpage.RegionKind]regionColors{
	pgpage.RegionHeader:       {background: "138", line: "95"},  // ash rose
	pgpage.RegionLinePointers: {background: "109", line: "66"},  // blue gray
	pgpage.RegionFree:         {background: "236", line: "234"}, // near black
	pgpage.RegionTuples:       {background: "144", line: "101"}, // sand gray
	pgpage.RegionSpecial:      {background: "244", line: "240"}, // gray
}

// selectedColors paint what was selected on the map, such as the entry of a
// line pointer, in the color of the selection everywhere else, so the eye
// links the two; pointedColors paint what it points to, its tuple, in a
// darker shade of the same color.
var (
	selectedColors = regionColors{background: "177", line: "133"}
	pointedColors  = regionColors{background: "133", line: "96"}
)

// helpStyles returns the styles of the help component, which comes with a
// palette of its own that has nothing to do with ours.
func helpStyles() (key, desc, sep lipgloss.Style) {
	return helpKeyStyle, helpDescStyle, moreStyle
}
