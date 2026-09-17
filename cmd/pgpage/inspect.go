package main

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/arturobermejo/pgpage"
)

// runInspect implements "pgpage inspect".
func runInspect(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("inspect", "usage: pgpage inspect <relation-file> [--block N]", stderr)

	block, page, code := readBlockArg(fs, args, stderr)
	if page == nil {
		return code
	}

	printInspect(stdout, block, pgpage.SummarizePage(page, block))

	return exitOK
}

// printInspect writes the summary of a page as aligned "Label: value" lines.
func printInspect(w io.Writer, block pgpage.BlockNumber, s pgpage.PageSummary) {
	tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)

	fmt.Fprintf(tw, "Block:\t%d\n", block)

	if s.Status == pgpage.StatusOK {
		fmt.Fprintf(tw, "LSN:\t%v\n", s.Header.LSN)
		fmt.Fprintf(tw, "Checksum:\t%s\n", checksumText(s))
		fmt.Fprintf(tw, "Line pointers:\t%d\n", s.Header.ItemCount())
		fmt.Fprintf(tw, "Free space:\t%d\n", s.Header.FreeSpace())
		fmt.Fprintf(tw, "Layout version:\t%d\n", s.Header.LayoutVersion)
	}

	fmt.Fprintf(tw, "Status:\t%v\n", s.Status)

	if s.Err != nil {
		fmt.Fprintf(tw, "Error:\t%v\n", s.Err)
	}

	tw.Flush()
}

// checksumText describes the checksum of a valid page: the stored value and
// whether it matches, for example "6769 (OK)" or "6769 (MISMATCH, computed
// 6770)".
func checksumText(s pgpage.PageSummary) string {
	if s.Checksum == pgpage.ChecksumMismatch {
		return fmt.Sprintf("%d (%v, computed %d)", s.Header.Checksum, s.Checksum, s.ComputedChecksum)
	}

	return fmt.Sprintf("%d (%v)", s.Header.Checksum, s.Checksum)
}
