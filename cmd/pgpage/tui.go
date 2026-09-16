package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/arturobermejo/pgpage"
	"github.com/arturobermejo/pgpage/internal/tui"
)

const tuiUsage = "usage: pgpage tui <relation-file>"

// runTUI opens the relation and hands it to the interactive explorer, which
// owns the screen until the user quits.
func runTUI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("tui", tuiUsage, stderr)

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

	if err := tui.Run(rel, stdin, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}

	return exitOK
}
