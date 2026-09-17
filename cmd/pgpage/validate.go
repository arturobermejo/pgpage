package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/arturobermejo/pgpage"
)

// runValidate implements "pgpage validate". It exits with exitError if the
// relation has any problem.
func runValidate(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("validate", "usage: pgpage validate <relation-file>", stderr)

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

	report, err := validateRelation(rel, stdout)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}

	fmt.Fprintln(stdout, report)

	if report.problems() > 0 {
		return exitError
	}

	return exitOK
}

// validationReport counts what a scan of a relation found.
type validationReport struct {
	pages     int
	statuses  map[pgpage.PageStatus]int // pages by status
	items     int                       // invalid line pointers or tuple headers
	checksums int                       // pages whose checksum does not match
	trailing  int64                     // bytes of a partial last page
}

// problems returns the number of problems found, where a partial last page
// counts as one.
func (r validationReport) problems() int {
	n := r.statuses[pgpage.StatusInvalid] + r.items + r.checksums
	if r.trailing > 0 {
		n++
	}

	return n
}

// String formats the summary line, for example
// "scanned 3 pages: 3 OK, 0 NEW, 0 INVALID, 0 bad line pointers or tuples, 0 bad checksums".
func (r validationReport) String() string {
	return fmt.Sprintf("scanned %d pages: %d OK, %d NEW, %d INVALID, %d bad line pointers or tuples, %d bad checksums",
		r.pages, r.statuses[pgpage.StatusOK], r.statuses[pgpage.StatusNew], r.statuses[pgpage.StatusInvalid],
		r.items, r.checksums)
}

// validateRelation checks every page of rel as a heap relation, writing a
// line to w for each problem, and returns what it found. It returns an error
// only if the file cannot be read.
func validateRelation(rel *pgpage.Relation, w io.Writer) (validationReport, error) {
	report := validationReport{statuses: make(map[pgpage.PageStatus]int)}
	buf := make([]byte, pgpage.PageSize)

	for block := range rel.PageCount() {
		if err := rel.ReadPageInto(block, buf); err != nil {
			return report, err
		}

		report.pages++

		summary := pgpage.SummarizePage(buf, block)
		report.statuses[summary.Status]++

		if summary.Status != pgpage.StatusOK {
			if summary.Err != nil {
				fmt.Fprintf(w, "block %d: %v\n", block, summary.Err)
			}

			continue
		}

		// A page written with checksums off is not a problem; only a stored
		// checksum that disagrees with the bytes is.
		if summary.Checksum == pgpage.ChecksumMismatch {
			fmt.Fprintf(w, "block %d: checksum mismatch: stored %d, computed %d\n",
				block, summary.Header.Checksum, summary.ComputedChecksum)

			report.checksums++
		}

		report.items += validateItems(w, block, buf, summary.Header)
	}

	if report.trailing = rel.TrailingBytes(); report.trailing > 0 {
		fmt.Fprintf(w, "block %d: partial page of %d bytes\n", rel.PageCount(), report.trailing)
	}

	return report, nil
}

// validateItems checks every line pointer of a page with a valid header, and
// the tuple header each one references, and returns how many are invalid.
func validateItems(w io.Writer, block pgpage.BlockNumber, page []byte, h pgpage.PageHeader) int {
	invalid := 0

	for n := pgpage.FirstOffsetNumber; int(n) <= h.ItemCount(); n++ {
		// Line pointers without storage yield an error that is not
		// corruption, so only the sentinel errors count.
		_, err := pgpage.HeapTupleAt(page, h, n)
		if errors.Is(err, pgpage.ErrInvalidItemID) || errors.Is(err, pgpage.ErrInvalidTupleHeader) {
			fmt.Fprintf(w, "block %d: %v\n", block, err)

			invalid++
		}
	}

	return invalid
}
