package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/arturobermejo/pgpage"
	"github.com/arturobermejo/pgpage/internal/tui"
)

const tuiUsage = "usage: pgpage [tui] <relation-file> [--block N]"

// runTUI opens the relation and hands it to the interactive explorer, which
// owns the screen until the user quits.
func runTUI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("tui", tuiUsage, stderr)
	n := fs.Uint("block", 0, "block `number` of the page to start on")

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

	// An empty relation has no block to start on, but can still be opened:
	// the explorer says it has no pages. Only a block that was asked for
	// must exist.
	block, err := blockInRange(rel, *n)
	if err != nil && *n > 0 {
		fmt.Fprintln(stderr, err)
		return exitError
	}

	if err := tui.Run(rel, block, stdin, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}

	return exitOK
}
