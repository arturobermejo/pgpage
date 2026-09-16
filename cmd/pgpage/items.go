package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/arturobermejo/pgpage"
)

// runItems implements "pgpage items".
func runItems(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("items", "usage: pgpage items <relation-file> [--block N]", stderr)

	block, page, code := readBlockArg(fs, args, stderr)
	if page == nil {
		return code
	}

	summary := pgpage.SummarizePage(page, block)
	if summary.Status != pgpage.StatusOK {
		// Without a valid header there are no line pointers to list.
		printInspect(stdout, block, summary)
		return exitOK
	}

	printItems(stdout, page, summary.Header)

	return exitOK
}

// printItems writes one row per line pointer of page, with the header of the
// tuple it references, in the columns of pageinspect's heap_page_items().
func printItems(w io.Writer, page []byte, h pgpage.PageHeader) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintln(tw, "LP\tSTATE\tOFF\tLEN\tXMIN\tXMAX\tFIELD3\tCTID\tINFOMASK\tINFOMASK2\tHOFF\tDETAIL")

	for n := pgpage.FirstOffsetNumber; int(n) <= h.ItemCount(); n++ {
		// ItemIDAt cannot fail: n is within the count of a valid header.
		id, _ := pgpage.ItemIDAt(page, h, n)
		fmt.Fprintf(tw, "%d\t%v\t%d\t%d\t", n, id.State(), id.Offset(), id.Length())

		tuple, err := pgpage.HeapTupleAt(page, h, n)

		switch {
		case err == nil:
			th := tuple.Header
			fmt.Fprintf(tw, "%d\t%d\t%d\t%v\t%#04x\t%#04x\t%d\t%s\n",
				th.Xmin, th.Xmax, th.Field3, th.Ctid, uint16(th.Infomask), uint16(th.Infomask2), th.Hoff, flagNames(th))
		case errors.Is(err, pgpage.ErrInvalidItemID) || errors.Is(err, pgpage.ErrInvalidTupleHeader):
			fmt.Fprintf(tw, "-\t-\t-\t-\t-\t-\t-\terror: %v\n", err)
		case id.State() == pgpage.ItemRedirect:
			fmt.Fprintf(tw, "-\t-\t-\t-\t-\t-\t-\t→%d\n", id.Offset())
		default:
			fmt.Fprintf(tw, "-\t-\t-\t-\t-\t-\t-\t-\n")
		}
	}

	tw.Flush()
}

// flagNames lists the infomask flags of a tuple header without their HEAP_
// prefix, for example "HASVARWIDTH,XMIN_COMMITTED,XMAX_INVALID".
func flagNames(h pgpage.HeapTupleHeader) string {
	names := append(h.Infomask.Names(), h.Infomask2.Names()...)
	for i, name := range names {
		names[i] = strings.TrimPrefix(name, "HEAP_")
	}

	if len(names) == 0 {
		return "-"
	}

	return strings.Join(names, ",")
}
