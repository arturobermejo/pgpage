package main

import (
	"strings"
	"testing"
)

// Values from testdata/heap_small.page_header.csv.
func TestInspect(t *testing.T) {
	const want = `Block:          2
LSN:            0/21A8AF0
Checksum:       7780 (OK)
Items:          180
Free space:     1728
Layout version: 4
Status:         OK
`

	tests := map[string][]string{
		"block after file":  {"inspect", fixtureHeap, "--block", "2"},
		"block before file": {"inspect", "--block", "2", fixtureHeap},
		"single dash":       {"inspect", fixtureHeap, "-block=2"},
	}

	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := runCommand(args...)

			if code != exitOK {
				t.Fatalf("exit code %d, want %d (stderr: %s)", code, exitOK, stderr)
			}

			if stdout != want {
				t.Errorf("stdout =\n%s\nwant\n%s", stdout, want)
			}

			if stderr != "" {
				t.Errorf("stderr = %q, want nothing", stderr)
			}
		})
	}
}

func TestInspectDefaultBlock(t *testing.T) {
	code, stdout, _ := runCommand("inspect", fixtureHeap)

	if code != exitOK {
		t.Fatalf("exit code %d, want %d", code, exitOK)
	}

	if !strings.HasPrefix(stdout, "Block:          0\nLSN:            0/21A8A18\n") {
		t.Errorf("stdout does not describe block 0:\n%s", stdout)
	}
}

// Pages that cannot be decoded are reported, not treated as a failure.
func TestInspectNotOK(t *testing.T) {
	tests := []struct {
		name string
		page []byte
		want string
	}{
		{
			name: "new page",
			page: make([]byte, 8192),
			want: "Block:  0\nStatus: NEW\n",
		},
		{
			name: "invalid page",
			page: invalidPage(),
			want: "Block:  0\nStatus: INVALID\nError:  pgpage: invalid page boundaries: lower=500 upper=100 special=8192: invalid page header\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCommand("inspect", writePage(t, tt.page))

			if code != exitOK {
				t.Fatalf("exit code %d, want %d (stderr: %s)", code, exitOK, stderr)
			}

			if stdout != tt.want {
				t.Errorf("stdout = %q, want %q", stdout, tt.want)
			}
		})
	}
}

func TestInspectErrors(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{name: "missing file", args: []string{"inspect"}, code: exitUsage, stderr: "missing relation file"},
		{name: "extra argument", args: []string{"inspect", fixtureHeap, "other"}, code: exitUsage, stderr: `unexpected argument "other"`},
		{name: "unknown flag", args: []string{"inspect", fixtureHeap, "--blok", "1"}, code: exitUsage, stderr: "flag provided but not defined: -blok"},
		{name: "block not a number", args: []string{"inspect", fixtureHeap, "--block", "one"}, code: exitUsage, stderr: `invalid value "one" for flag -block`},
		{name: "negative block", args: []string{"inspect", fixtureHeap, "--block", "-1"}, code: exitUsage, stderr: `invalid value "-1" for flag -block`},
		{name: "file does not exist", args: []string{"inspect", "no-such-file"}, code: exitError, stderr: "no such file or directory"},
		{name: "block past the end", args: []string{"inspect", fixtureHeap, "--block", "3"}, code: exitError, stderr: "block 3 is out of range: " + fixtureHeap + " has 3 pages"},
		{name: "block past 32 bits", args: []string{"inspect", fixtureHeap, "--block", "4294967298"}, code: exitError, stderr: "block 4294967298 is out of range"},
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

			// Usage errors print the message once, then the usage once.
			if tt.code == exitUsage {
				if n := strings.Count(stderr, "usage:"); n != 1 {
					t.Errorf("stderr shows the usage %d times, want once:\n%s", n, stderr)
				}
			}
		})
	}
}
