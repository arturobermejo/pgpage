package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/arturobermejo/pgpage"
)

// summaryCache holds what the navigator knows about each page. It is a map,
// so every copy of the Model shares it; only Update may write to it.
type summaryCache map[pgpage.BlockNumber]pgpage.PageSummary

// summariesMsg carries the pages a command read from disk. It is the only
// way the summaries reach the model: the command itself never touches it.
type summariesMsg struct {
	summaries summaryCache
}

// loadSummaries returns a command that reads the pages from first to last
// and summarizes them.
//
// The command runs in its own goroutine, so it must not read or write the
// model: it captures the relation, whose reads are positional (pread) and
// safe to run while the rest of the program goes on, and returns everything
// it found in a message.
func loadSummaries(rel *pgpage.Relation, first, last pgpage.BlockNumber) tea.Cmd {
	return func() tea.Msg {
		summaries := make(summaryCache, last-first+1)

		// One buffer for the whole scan: SummarizePage copies what it needs
		// into a PageSummary, which holds no slice of the page.
		buf := make([]byte, pgpage.PageSize)

		for block := first; block <= last; block++ {
			if err := rel.ReadPageInto(block, buf); err != nil {
				// A page that cannot be read is not a page that is corrupt:
				// StatusUnknown says the header was never examined.
				summaries[block] = pgpage.PageSummary{Status: pgpage.StatusUnknown, Err: err}
				continue
			}

			summaries[block] = pgpage.SummarizePage(buf)
		}

		return summariesMsg{summaries: summaries}
	}
}

// missingRange returns the smallest range inside [first, last] that covers
// every page the cache does not have yet. ok is false when they are all
// cached and there is nothing to read.
func (c summaryCache) missingRange(first, last pgpage.BlockNumber) (lo, hi pgpage.BlockNumber, ok bool) {
	for block := first; block <= last; block++ {
		if _, cached := c[block]; cached {
			continue
		}

		if !ok {
			lo, ok = block, true
		}

		hi = block
	}

	return lo, hi, ok
}
