package tui

import "github.com/charmbracelet/lipgloss"

// The palette of the interface, in one place so that the panels cannot drift
// apart. Colors are ANSI 256 numbers, which Lip Gloss degrades on terminals
// that show fewer colors, down to no color at all.
//
// Meaning is never carried by color alone: everything colored here is also
// spelled out or marked with a glyph.
var (
	// Chrome: titles and the top bar.
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	nameStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
	pathStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	factStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	blockStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	// Content: rows, field names and values.
	rowStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
	fieldStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	valueStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	numberStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("180"))
	moreStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	// Page status.
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	newStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	invalidStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
)
