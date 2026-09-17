// Command pgpage inspects the pages of PostgreSQL relation files.
//
// Usage:
//
//	pgpage <relation-file> [--block N]
//	pgpage tui <relation-file> [--block N]
//	pgpage inspect <relation-file> [--block N]
//	pgpage lp <relation-file> [--block N]
//	pgpage validate <relation-file>
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/arturobermejo/pgpage"
)

// Exit codes.
const (
	exitOK    = 0 // success
	exitError = 1 // the command ran but failed, for example a missing file
	exitUsage = 2 // the command line is wrong
)

const usage = `usage: pgpage <relation-file> [--block N]
       pgpage <command> [arguments]

With a relation file and no command, pgpage opens the interactive explorer.

commands:
  tui <relation-file> [--block N]       explore the relation interactively
  inspect <relation-file> [--block N]   print the header of one page
  lp <relation-file> [--block N]        list the line pointers of one page and their tuples
  validate <relation-file>              check every page of a heap relation
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run executes the command line args, without the program name, and returns
// the exit code. It reads keys from stdin, writes results to stdout and
// messages to stderr, so that tests can run a command without a terminal.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}

	switch args[0] {
	case "tui":
		return runTUI(args[1:], stdin, stdout, stderr)
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "lp", "items": // items is the name the command had first
		return runLinePointers(args[1:], stdout, stderr)
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	default:
		if isRelationArg(args[0]) {
			return runTUI(args, stdin, stdout, stderr)
		}

		fmt.Fprintf(stderr, "pgpage: %q is neither a command nor a file\n\n%s", args[0], usage)

		return exitUsage
	}
}

// isRelationArg reports whether the first argument, which is not a command,
// starts the explorer's arguments: a file that exists, or a flag such as
// --block before the file.
//
// A name that is neither stays an error. Taking every unknown word for a file
// would turn a typo like "pgpage inspec ./24576" into "no such file inspec",
// which points the user at the wrong mistake. A file named like a command
// can still be opened as ./inspect.
func isRelationArg(arg string) bool {
	if strings.HasPrefix(arg, "-") {
		return true
	}

	_, err := os.Stat(arg)

	return err == nil
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

// newFlagSet returns the flag set of a command, which reports errors and the
// usage line to stderr instead of exiting.
func newFlagSet(name, usageLine string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, usageLine)
		fs.PrintDefaults()
	}

	return fs
}

// readBlockArg parses "<relation-file> [--block N]" and reads that page. It
// reports every problem to stderr itself: when it returns a nil page, the
// command must exit with code.
func readBlockArg(fs *flag.FlagSet, args []string, stderr io.Writer) (block pgpage.BlockNumber, page []byte, code int) {
	n := fs.Uint("block", 0, "block `number` of the page to read")

	path, err := parseArgs(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		return 0, nil, exitOK
	}

	if err != nil {
		return 0, nil, exitUsage // parseArgs already reported it
	}

	rel, err := pgpage.OpenRelation(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 0, nil, exitError
	}
	defer rel.Close()

	block, err = blockInRange(rel, *n)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 0, nil, exitError
	}

	page, err = rel.ReadPage(block)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 0, nil, exitError
	}

	return block, page, exitOK
}

// blockInRange converts n to a block of rel, or fails if rel has no such
// block.
func blockInRange(rel *pgpage.Relation, n uint) (pgpage.BlockNumber, error) {
	// Compare before converting: a huge --block must not wrap around to a
	// valid block number.
	if n >= uint(rel.PageCount()) {
		return 0, fmt.Errorf("pgpage: block %d is out of range: %s has %d pages", n, rel.Path(), rel.PageCount())
	}

	return pgpage.BlockNumber(n), nil
}
