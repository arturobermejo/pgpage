package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestValidateFixture(t *testing.T) {
	code, stdout, stderr := runCommand("validate", fixtureHeap)

	if code != exitOK {
		t.Fatalf("exit code %d, want %d (stderr: %s)", code, exitOK, stderr)
	}

	if want := "scanned 3 pages: 3 OK, 0 NEW, 0 INVALID, 0 invalid items\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestValidateEmptyFile(t *testing.T) {
	code, stdout, _ := runCommand("validate", writePage(t, nil))

	if code != exitOK {
		t.Errorf("exit code %d, want %d", code, exitOK)
	}

	if want := "scanned 0 pages: 0 OK, 0 NEW, 0 INVALID, 0 invalid items\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// One relation with every kind of page and problem the scan reports.
func TestValidateProblems(t *testing.T) {
	fixture, err := os.ReadFile(fixtureHeap)
	if err != nil {
		t.Fatal(err)
	}

	good := fixture[:8192]

	// Line pointer 1 of the fixture's block 0 is at byte 24: lp_off 8152 → 8153.
	badItem := bytes.Clone(good)
	badItem[24] |= 1

	// Its tuple starts at byte 8152, and t_hoff is byte 22 of the tuple.
	badTuple := bytes.Clone(good)
	badTuple[8152+22] = 16

	var relation []byte
	for _, page := range [][]byte{good, make([]byte, 8192), invalidPage(), badItem, badTuple} {
		relation = append(relation, page...)
	}

	relation = append(relation, make([]byte, 100)...) // a torn sixth page

	const want = `block 2: pgpage: invalid page boundaries: lower=500 upper=100 special=8192: invalid page header
block 3: pgpage: line pointer 1 offset 8153 is not aligned to 8: invalid line pointer
block 4: pgpage: line pointer 1: pgpage: t_hoff 16 is not an aligned offset between 23 and the tuple length 35: invalid heap tuple header
block 5: partial page of 100 bytes
scanned 5 pages: 3 OK, 1 NEW, 1 INVALID, 2 invalid items
`

	code, stdout, stderr := runCommand("validate", writePage(t, relation))

	if code != exitError {
		t.Errorf("exit code %d, want %d", code, exitError)
	}

	if stdout != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout, want)
	}

	if stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
}

// A partial last page alone is a problem.
func TestValidatePartialPageOnly(t *testing.T) {
	code, stdout, _ := runCommand("validate", writePage(t, make([]byte, 8192+1)))

	if code != exitError {
		t.Errorf("exit code %d, want %d", code, exitError)
	}

	if !strings.HasPrefix(stdout, "block 1: partial page of 1 bytes\n") {
		t.Errorf("stdout = %q, want the partial page reported", stdout)
	}
}

func TestValidateErrors(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{name: "missing file", args: []string{"validate"}, code: exitUsage, stderr: "missing relation file"},
		{name: "extra argument", args: []string{"validate", fixtureHeap, "other"}, code: exitUsage, stderr: `unexpected argument "other"`},
		{name: "unknown flag", args: []string{"validate", fixtureHeap, "--block", "1"}, code: exitUsage, stderr: "flag provided but not defined: -block"},
		{name: "file does not exist", args: []string{"validate", "no-such-file"}, code: exitError, stderr: "no such file or directory"},
		{name: "help", args: []string{"validate", "-h"}, code: exitOK, stderr: "usage: pgpage validate"},
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
