package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// The status panel wraps sentences, so it has a narrowest width at which it
// is still readable and a widest past which lines become hard to follow.
const (
	minStatusWidth = 34
	maxStatusWidth = 72
)

// statusPanel renders a page that has no layout to show: one that was never
// initialized, one whose header does not parse, or one that could not be
// read. Each says what it is, why, and what can still be done with it.
func statusPanel(summary pgpage.PageSummary, width int) string {
	if summary.Status == pgpage.StatusNew {
		return newPagePanel(width)
	}

	return brokenPagePanel(summary, width)
}

// newPagePanel describes a page of 8192 zero bytes: not a problem, just a
// page PostgreSQL has not used yet.
func newPagePanel(width int) string {
	center := lipgloss.NewStyle().Width(width).Align(lipgloss.Center)

	return strings.Join([]string{
		"",
		center.Render(newStyle.Bold(true).Render("NEW PAGE")),
		"",
		center.Render(valueStyle.Render("This page is all zeroes. PostgreSQL extended the relation " +
			"but has not initialized this page yet.")),
		"",
		center.Render(fieldStyle.Render("pd_lower 0 · pd_upper 0 · pd_special 0 · line pointers 0")),
	}, "\n")
}

// brokenPagePanel describes a page whose header could not be used, and shows
// the parser's own words plus the rules the header has to meet.
func brokenPagePanel(summary pgpage.PageSummary, width int) string {
	wrap := lipgloss.NewStyle().Width(width)

	headline := "Invalid page header"
	if summary.Status != pgpage.StatusInvalid {
		headline = "This page could not be read"
	}

	rows := []string{
		invalidStyle.Render("⚠ " + headline),
		"",
	}

	if summary.Err != nil {
		// The parser already says which field is wrong and with what value:
		// its message is the most specific thing anyone can show here.
		rows = append(rows, wrap.Render(invalidStyle.Render(summary.Err.Error())), "")
	}

	if summary.Status == pgpage.StatusInvalid {
		rows = append(rows,
			fieldStyle.Render("expected"),
			wrap.Render(valueStyle.Render("page size 8192 and layout version 4")),
			wrap.Render(valueStyle.Render(fmt.Sprintf("%d ≤ pd_lower ≤ pd_upper ≤ pd_special ≤ %d",
				pgpage.PageHeaderSize, pgpage.PageSize))),
			wrap.Render(valueStyle.Render("pd_special aligned to 8, no unknown flag bits")),
			"",
		)
	}

	return strings.Join(append(rows,
		wrap.Render(moreStyle.Render("The page is left as it is: parsing stops here, navigation does not.")),
	), "\n")
}
