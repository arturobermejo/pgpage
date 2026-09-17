package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// lpColumns is the number of columns of a "pgpage lp" row.
const lpColumns = 12

// readItemsCSV returns the rows of the fixture's heap_page_items() output
// for block, keyed by column name.
func readItemsCSV(t *testing.T, block int) []map[string]string {
	t.Helper()

	f, err := os.Open(fixtureHeap + ".items.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	var rows []map[string]string

	for _, record := range records[1:] {
		row := make(map[string]string, len(records[0]))
		for i, name := range records[0] {
			row[name] = record[i]
		}

		if row["blkno"] == strconv.Itoa(block) {
			rows = append(rows, row)
		}
	}

	return rows
}

// Every row of "pgpage lp" must show what heap_page_items() reports for
// the same line pointer of the fixture.
func TestLPMatchesPageinspect(t *testing.T) {
	states := map[string]string{"0": "UNUSED", "1": "NORMAL", "2": "REDIRECT", "3": "DEAD"}

	for block := range 3 {
		t.Run(fmt.Sprintf("block %d", block), func(t *testing.T) {
			code, stdout, stderr := runCommand("lp", fixtureHeap, "--block", strconv.Itoa(block))
			if code != exitOK {
				t.Fatalf("exit code %d, want %d (stderr: %s)", code, exitOK, stderr)
			}

			lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
			rows := readItemsCSV(t, block)

			if got := strings.Fields(lines[0]); len(got) != lpColumns || got[0] != "LP" {
				t.Fatalf("header = %q, want %d columns starting with LP", lines[0], lpColumns)
			}

			if len(lines)-1 != len(rows) {
				t.Fatalf("%d rows, heap_page_items() has %d", len(lines)-1, len(rows))
			}

			for i, row := range rows {
				fields := strings.Fields(lines[i+1])
				if len(fields) != lpColumns {
					t.Fatalf("row %q has %d columns, want %d", lines[i+1], len(fields), lpColumns)
				}

				want := []string{row["lp"], states[row["lp_flags"]], row["lp_off"], row["lp_len"]}

				if row["t_xmin"] == "" {
					// No tuple header: every tuple column is a dash.
					want = append(want, "-", "-", "-", "-", "-", "-", "-")
				} else {
					want = append(want,
						row["t_xmin"], row["t_xmax"], row["t_field3"], row["t_ctid"],
						hex4(t, row["t_infomask"]), hex4(t, row["t_infomask2"]), row["t_hoff"])
				}

				if got := fields[:lpColumns-1]; strings.Join(got, " ") != strings.Join(want, " ") {
					t.Errorf("line pointer %s:\n got %q\nwant %q", row["lp"], got, want)
				}
			}
		})
	}
}

// hex4 formats a decimal pageinspect column as the command does, "0x0902".
func hex4(t *testing.T, decimal string) string {
	t.Helper()

	n, err := strconv.ParseUint(decimal, 10, 16)
	if err != nil {
		t.Fatal(err)
	}

	return fmt.Sprintf("%#04x", n)
}

// The DETAIL column explains what the other columns cannot.
func TestLPDetail(t *testing.T) {
	code, stdout, _ := runCommand("lp", fixtureHeap, "--block", "2")
	if code != exitOK {
		t.Fatalf("exit code %d, want %d", code, exitOK)
	}

	tests := map[string]string{
		"dead":            "1 DEAD 0 0 - - - - - - - -",
		"redirect":        "10 REDIRECT 168 0 - - - - - - - →168",
		"unused":          "172 UNUSED 0 0 - - - - - - - -",
		"heap-only tuple": "168 NORMAL 2872 40 776 0 0 (2,168) 0x2902 0x8002 24 HASVARWIDTH,XMIN_COMMITTED,XMAX_INVALID,UPDATED,ONLY_TUPLE",
	}

	rows := make(map[string]bool)
	for _, line := range strings.Split(stdout, "\n") {
		rows[strings.Join(strings.Fields(line), " ")] = true
	}

	for name, want := range tests {
		if !rows[want] {
			t.Errorf("%s: no row %q in output:\n%s", name, want, stdout)
		}
	}
}

// A corrupt line pointer is listed with its error instead of stopping.
func TestLPCorruptLinePointer(t *testing.T) {
	fixture, err := os.ReadFile(fixtureHeap)
	if err != nil {
		t.Fatal(err)
	}

	page := bytes.Clone(fixture[:8192])
	page[24] |= 1 // line pointer 1: lp_off 8152 → 8153

	code, stdout, _ := runCommand("lp", writePage(t, page))
	if code != exitOK {
		t.Fatalf("exit code %d, want %d", code, exitOK)
	}

	lines := strings.Split(stdout, "\n")

	const want = "1 NORMAL 8153 35 - - - - - - - error: pgpage: line pointer 1 offset 8153 is not aligned to 8: invalid line pointer"
	if got := strings.Join(strings.Fields(lines[1]), " "); got != want {
		t.Errorf("row 1:\n got %q\nwant %q", got, want)
	}

	if !strings.HasPrefix(lines[2], "2 ") {
		t.Errorf("row 2 = %q, want the listing to continue with line pointer 2", lines[2])
	}
}

// Pages without a valid header have no line pointers: the command shows
// their status as inspect does.
func TestLPNotOK(t *testing.T) {
	tests := []struct {
		name string
		page []byte
		want string
	}{
		{name: "new page", page: make([]byte, 8192), want: "Block:  0\nStatus: NEW\n"},
		{
			name: "invalid page",
			page: invalidPage(),
			want: "Block:  0\nStatus: INVALID\nError:  pgpage: invalid page boundaries: lower=500 upper=100 special=8192: invalid page header\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, _ := runCommand("lp", writePage(t, tt.page))

			if code != exitOK {
				t.Errorf("exit code %d, want %d", code, exitOK)
			}

			if stdout != tt.want {
				t.Errorf("stdout = %q, want %q", stdout, tt.want)
			}
		})
	}
}

func TestLPErrors(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{name: "missing file", args: []string{"lp"}, code: exitUsage, stderr: "missing relation file"},
		{name: "block not a number", args: []string{"lp", fixtureHeap, "--block", "x"}, code: exitUsage, stderr: `invalid value "x" for flag -block`},
		{name: "block past the end", args: []string{"lp", fixtureHeap, "--block", "3"}, code: exitError, stderr: "block 3 is out of range"},
		{name: "help", args: []string{"lp", "-h"}, code: exitOK, stderr: "usage: pgpage lp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCommand(tt.args...)

			if code != tt.code {
				t.Errorf("exit code %d, want %d", code, tt.code)
			}

			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}

			if !strings.Contains(stderr, tt.stderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tt.stderr)
			}
		})
	}
}

// Live tuples in the fixture all have t_xmax and t_field3 set to 0, so a test
// on the fixture alone could not tell those columns apart.
func TestLPColumnOrder(t *testing.T) {
	fixture, err := os.ReadFile(fixtureHeap)
	if err != nil {
		t.Fatal(err)
	}

	// The tuple of line pointer 1 starts at byte 8152 of block 0.
	page := bytes.Clone(fixture[:8192])
	page[8152+4] = 0x84 // t_xmax = 900 (0x0384)
	page[8152+5] = 0x03
	page[8152+8] = 3 // t_field3 = 3

	code, stdout, _ := runCommand("lp", writePage(t, page))
	if code != exitOK {
		t.Fatalf("exit code %d, want %d", code, exitOK)
	}

	const want = "1 NORMAL 8152 35 775 900 3 (0,1) 0x0902 0x0002 24 HASVARWIDTH,XMIN_COMMITTED,XMAX_INVALID"
	if got := strings.Join(strings.Fields(strings.Split(stdout, "\n")[1]), " "); got != want {
		t.Errorf("row 1:\n got %q\nwant %q", got, want)
	}
}

// items, the name the command had first, still runs it.
func TestLPItemsAlias(t *testing.T) {
	_, want, _ := runCommand("lp", fixtureHeap, "--block", "2")

	if code, got, stderr := runCommand("items", fixtureHeap, "--block", "2"); code != exitOK || got != want {
		t.Errorf("items: exit code %d, stderr %q, output differs from lp:\n%s", code, stderr, got)
	}
}
