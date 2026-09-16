package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// run executes a command and returns the message it produced.
func run(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()

	if cmd == nil {
		t.Fatal("no command to run")
	}

	return cmd()
}

// The command reads the pages of the range and summarizes each one.
func TestLoadSummaries(t *testing.T) {
	msg, ok := run(t, loadSummaries(openFixture(t), 0, 2)).(summariesMsg)
	if !ok {
		t.Fatalf("the command did not return a summariesMsg")
	}

	if len(msg.summaries) != 3 {
		t.Fatalf("%d summaries, want 3", len(msg.summaries))
	}

	for block, summary := range msg.summaries {
		if summary.Status != pgpage.StatusOK {
			t.Errorf("block %d: status %v, want OK", block, summary.Status)
		}
	}

	// The fixture's first page: the same numbers the CLI prints.
	if items := msg.summaries[0].Header.ItemCount(); items != 185 {
		t.Errorf("block 0 has %d items, want 185", items)
	}
}

// A page that cannot be read is reported, not dropped: the navigator must
// have a row for every page it asked about.
func TestLoadSummariesReadError(t *testing.T) {
	msg, ok := run(t, loadSummaries(openFixture(t), 2, 3)).(summariesMsg)
	if !ok {
		t.Fatalf("the command did not return a summariesMsg")
	}

	if len(msg.summaries) != 2 {
		t.Fatalf("%d summaries, want 2", len(msg.summaries))
	}

	past := msg.summaries[3] // one page past the end of the fixture

	if past.Status != pgpage.StatusUnknown {
		t.Errorf("status %v, want UNKNOWN", past.Status)
	}

	if past.Err == nil {
		t.Error("a page that could not be read has no error")
	}
}

func TestMissingRange(t *testing.T) {
	cache := summaryCache{1: {}, 2: {}, 5: {}}

	tests := []struct {
		name        string
		first, last pgpage.BlockNumber
		lo, hi      pgpage.BlockNumber
		missing     bool
	}{
		{name: "all cached", first: 1, last: 2},
		{name: "one page", first: 3, last: 3, lo: 3, hi: 3, missing: true},
		{name: "a gap between cached pages", first: 1, last: 5, lo: 3, hi: 4, missing: true},
		{name: "nothing cached", first: 7, last: 9, lo: 7, hi: 9, missing: true},
		{name: "cached at the start", first: 2, last: 4, lo: 3, hi: 4, missing: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lo, hi, missing := cache.missingRange(tt.first, tt.last)

			if missing != tt.missing || (missing && (lo != tt.lo || hi != tt.hi)) {
				t.Errorf("missingRange(%d, %d) = %d, %d, %v; want %d, %d, %v",
					tt.first, tt.last, lo, hi, missing, tt.lo, tt.hi, tt.missing)
			}
		})
	}
}

// The columns say what each page holds, and dashes stand for what a page
// without a valid header cannot tell.
func TestSummaryColumns(t *testing.T) {
	okPage := pgpage.SummarizePage(fixturePage(t, 0), 0)

	tests := []struct {
		name    string
		summary pgpage.PageSummary
		cached  bool
		want    []string
	}{
		{name: "not read yet", want: []string{"…"}},
		{name: "valid page", summary: okPage, cached: true, want: []string{"185 items", "21% free", "OK"}},
		{
			name:    "new page",
			summary: pgpage.SummarizePage(make([]byte, pgpage.PageSize), 0),
			cached:  true,
			want:    []string{"0 items", "—", "NEW"},
		},
		{
			name:    "invalid page",
			summary: pgpage.PageSummary{Status: pgpage.StatusInvalid, Err: errors.New("boom")},
			cached:  true,
			want:    []string{"—", "INVALID"},
		},
		{
			name:    "unreadable page",
			summary: pgpage.PageSummary{Status: pgpage.StatusUnknown, Err: errors.New("boom")},
			cached:  true,
			want:    []string{"—", "UNKNOWN"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summaryColumns(tt.summary, tt.cached)

			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("columns %q do not contain %q", got, want)
				}
			}
		})
	}
}

// Columns line up whatever the dashes and percentages are: the dash is three
// bytes long and one cell wide, which byte padding gets wrong.
func TestSummaryColumnsAlign(t *testing.T) {
	summaries := []pgpage.PageSummary{
		pgpage.SummarizePage(fixturePage(t, 0), 0),
		pgpage.SummarizePage(make([]byte, pgpage.PageSize), 0),
		{Status: pgpage.StatusInvalid, Err: errors.New("boom")},
	}

	var width int

	for _, summary := range summaries {
		columns := summaryColumns(summary, true)

		// Compare the offset at which the status column starts.
		status := lipgloss.Width(columns) - lipgloss.Width(summary.Status.String())

		if width == 0 {
			width = status
			continue
		}

		if status != width {
			t.Errorf("status of %v starts at cell %d, want %d:\n%q", summary.Status, status, width, columns)
		}
	}
}
