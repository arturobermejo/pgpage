package pgpage

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"testing"
)

// fixtureHeap is a heap relation file copied from PostgreSQL 18 by
// scripts/make-fixture.sh, next to the pageinspect output for its pages.
const fixtureHeap = "testdata/heap_small"

// readFixtureCSV returns the rows of a CSV file written by make-fixture.sh,
// each one keyed by column name.
func readFixtureCSV(t *testing.T, path string) []map[string]string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}

	if len(records) < 2 {
		t.Fatalf("%s: no rows", path)
	}

	columns := records[0]
	rows := make([]map[string]string, 0, len(records)-1)

	for _, record := range records[1:] {
		row := make(map[string]string, len(columns))
		for i, name := range columns {
			row[name] = record[i]
		}

		rows = append(rows, row)
	}

	return rows
}

// parseInt parses a pageinspect integer column that fits in bits bits.
func parseInt(t *testing.T, s string, bits int) int64 {
	t.Helper()

	n, err := strconv.ParseInt(s, 10, bits)
	if err != nil {
		t.Fatalf("parse %q as int%d: %v", s, bits, err)
	}

	return n
}

// parseLSN parses the "X/X" text form of a pg_lsn.
func parseLSN(t *testing.T, s string) LSN {
	t.Helper()

	var hi, lo uint32
	if _, err := fmt.Sscanf(s, "%X/%X", &hi, &lo); err != nil {
		t.Fatalf("parse %q as LSN: %v", s, err)
	}

	return LSN(uint64(hi)<<32 | uint64(lo))
}

// Every page of the fixture must decode to what page_header() reports.
func TestFixturePageHeaders(t *testing.T) {
	f, err := os.Open(fixtureHeap)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	rows := readFixtureCSV(t, fixtureHeap+".page_header.csv")

	if got, want := info.Size(), int64(len(rows))*PageSize; got != want {
		t.Fatalf("%s is %d bytes, want %d for %d pages", fixtureHeap, got, want, len(rows))
	}

	for _, row := range rows {
		block := BlockNumber(parseInt(t, row["blkno"], 32))

		t.Run(fmt.Sprintf("block %d", block), func(t *testing.T) {
			page, err := ReadPage(f, block)
			if err != nil {
				t.Fatalf("ReadPage returned error: %v", err)
			}

			got, err := ParsePageHeader(page)
			if err != nil {
				t.Fatalf("ParsePageHeader returned error: %v", err)
			}

			// page_header() returns smallint columns, which are signed:
			// a checksum above 32767 comes back negative.
			want := PageHeader{
				LSN:           parseLSN(t, row["lsn"]),
				Checksum:      uint16(parseInt(t, row["checksum"], 16)),
				Flags:         PageFlags(parseInt(t, row["flags"], 16)),
				Lower:         uint16(parseInt(t, row["lower"], 16)),
				Upper:         uint16(parseInt(t, row["upper"], 16)),
				Special:       uint16(parseInt(t, row["special"], 16)),
				PageSize:      uint16(parseInt(t, row["pagesize"], 32)),
				LayoutVersion: uint8(parseInt(t, row["version"], 16)),
				PruneXID:      TransactionID(parseInt(t, row["prune_xid"], 64)),
			}

			if got != want {
				t.Errorf("ParsePageHeader =\n  %+v\nwant\n  %+v", got, want)
			}

			if s := got.LSN.String(); s != row["lsn"] {
				t.Errorf("LSN.String() = %q, pageinspect shows %q", s, row["lsn"])
			}
		})
	}
}
