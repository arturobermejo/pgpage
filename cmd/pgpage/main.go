// Command pgpage inspects the pages of PostgreSQL relation files.
//
// Usage:
//
//	pgpage inspect <relation-file> [--block N]
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/arturobermejo/pgpage"
)

// Exit codes.
const (
	exitOK    = 0 // success
	exitError = 1 // the command ran but failed, for example a missing file
	exitUsage = 2 // the command line is wrong
)

const usage = `usage: pgpage <command> [arguments]

commands:
  inspect <relation-file> [--block N]   print the header of one page
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the command line args, without the program name, and returns
// the exit code. It writes results to stdout and messages to stderr.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}

	switch args[0] {
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	default:
		fmt.Fprintf(stderr, "pgpage: unknown command %q\n\n%s", args[0], usage)
		return exitUsage
	}
}

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

// parseArgs parses flags and returns the single positional argument, the
// relation file. Flags may come before or after it, as in
// "pgpage inspect ./24576 --block 42".
//
// Like fs.Parse, it reports errors and the usage message to fs.Output.
func parseArgs(fs *flag.FlagSet, args []string) (string, error) {
	if err := fs.Parse(args); err != nil {
		return "", err
	}

	if fs.NArg() == 0 {
		return "", usageError(fs, "missing relation file")
	}

	path := fs.Arg(0)

	// Parse stops at the first non-flag argument, so parse what follows it.
	if err := fs.Parse(fs.Args()[1:]); err != nil {
		return "", err
	}

	if fs.NArg() > 0 {
		return "", usageError(fs, "unexpected argument %q", fs.Arg(0))
	}

	return path, nil
}

// usageError reports a command line error the way the flag package does:
// the message, then the usage.
func usageError(fs *flag.FlagSet, format string, args ...any) error {
	err := fmt.Errorf(format, args...)

	fmt.Fprintln(fs.Output(), err)
	fs.Usage()

	return err
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
