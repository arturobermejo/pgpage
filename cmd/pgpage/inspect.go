package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/arturobermejo/pgpage"
)

// runInspect implements "pgpage inspect".
func runInspect(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: pgpage inspect <relation-file> [--block N]")
		fs.PrintDefaults()
	}

	block := fs.Uint("block", 0, "block `number` of the page to inspect")

	path, err := parseArgs(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}

	if err != nil {
		return exitUsage // parseArgs already reported it
	}

	rel, err := pgpage.OpenRelation(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	defer rel.Close()

	// Compare before converting: a huge --block must not wrap around to a
	// valid block number.
	if *block >= uint(rel.PageCount()) {
		fmt.Fprintf(stderr, "pgpage: block %d is out of range: %s has %d pages\n", *block, path, rel.PageCount())
		return exitError
	}

	page, err := rel.ReadPage(pgpage.BlockNumber(*block))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}

	printInspect(stdout, pgpage.BlockNumber(*block), pgpage.SummarizePage(page))

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
