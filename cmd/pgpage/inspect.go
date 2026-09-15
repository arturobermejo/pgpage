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

	printInspect(stdout, block, pgpage.SummarizePage(page))

	return exitOK
}

// printInspect writes the summary of a page as aligned "Label: value" lines.
func printInspect(w io.Writer, block pgpage.BlockNumber, s pgpage.PageSummary) {
	tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)

	fmt.Fprintf(tw, "Block:\t%d\n", block)

	if s.Status == pgpage.StatusOK {
		fmt.Fprintf(tw, "LSN:\t%v\n", s.Header.LSN)
		fmt.Fprintf(tw, "Items:\t%d\n", s.Header.ItemCount())
		fmt.Fprintf(tw, "Free space:\t%d\n", s.Header.FreeSpace())
		fmt.Fprintf(tw, "Layout version:\t%d\n", s.Header.LayoutVersion)
	}

	fmt.Fprintf(tw, "Status:\t%v\n", s.Status)

	if s.Err != nil {
		fmt.Fprintf(tw, "Error:\t%v\n", s.Err)
	}

	tw.Flush()
}
