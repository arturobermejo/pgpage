package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// invalidSummary is the summary of the invalid page of the PRD: a header
// whose pd_lower is past its pd_upper.
func invalidSummary() pgpage.PageSummary {
	page := make([]byte, pgpage.PageSize)
	page[12], page[13] = 0xF4, 0x01 // pd_lower = 500
	page[14], page[15] = 0x64, 0x00 // pd_upper = 100
	page[16], page[17] = 0x00, 0x20 // pd_special = 8192
	page[18], page[19] = 0x04, 0x20 // page size 8192, layout version 4

	return pgpage.SummarizePage(page)
}

// A new page is not a problem, and its panel says what it is instead of
// showing an error.
func TestStatusPanelNew(t *testing.T) {
	panel := statusPanel(17, pgpage.SummarizePage(make([]byte, pgpage.PageSize)), 60)

	for _, want := range []string{"PAGE 17", "NEW PAGE", "all zeroes", "pd_lower 0"} {
		if !strings.Contains(panel, want) {
			t.Errorf("panel does not contain %q:\n%s", want, panel)
		}
	}

	for _, absent := range []string{"⚠", "expected", "INVALID"} {
		if strings.Contains(panel, absent) {
			t.Errorf("the panel of a new page contains %q:\n%s", absent, panel)
		}
	}
}

// An invalid page shows what the parser found, with the values it found,
// and the rules a header has to meet.
func TestStatusPanelInvalid(t *testing.T) {
	summary := invalidSummary()

	panel := statusPanel(382, summary, 72)

	for _, want := range []string{
		"BLOCK 382",
		"⚠ Invalid page header",
		"lower=500 upper=100 special=8192",
		"expected",
		"24 ≤ pd_lower ≤ pd_upper ≤ pd_special ≤ 8192",
		"navigation does not",
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("panel does not contain %q:\n%s", want, panel)
		}
	}
}

// A page that could not be read is not a corrupt page, and the panel does
// not claim it is.
func TestStatusPanelUnreadable(t *testing.T) {
	summary := pgpage.PageSummary{Status: pgpage.StatusUnknown, Err: errors.New("pgpage: input/output error")}

	panel := statusPanel(4, summary, 60)

	for _, want := range []string{"BLOCK 4", "could not be read", "input/output error"} {
		if !strings.Contains(panel, want) {
			t.Errorf("panel does not contain %q:\n%s", want, panel)
		}
	}

	for _, absent := range []string{"Invalid page header", "expected"} {
		if strings.Contains(panel, absent) {
			t.Errorf("the panel of an unreadable page contains %q:\n%s", absent, panel)
		}
	}
}

// Whatever the message, the panel wraps to the width it was given: a long
// error must not push the panels beside it off the screen.
func TestStatusPanelWraps(t *testing.T) {
	long := pgpage.PageSummary{
		Status: pgpage.StatusInvalid,
		Err:    errors.New("pgpage: " + strings.Repeat("a very long explanation ", 20)),
	}

	summaries := []pgpage.PageSummary{
		pgpage.SummarizePage(make([]byte, pgpage.PageSize)),
		invalidSummary(),
		long,
	}

	for _, summary := range summaries {
		for _, width := range []int{minStatusWidth, 50, maxStatusWidth} {
			panel := statusPanel(0, summary, width)

			for _, line := range strings.Split(panel, "\n") {
				if got := lipgloss.Width(line); got > width {
					t.Errorf("status %v at width %d: a line is %d cells wide:\n%q",
						summary.Status, width, got, line)
				}
			}
		}
	}
}
