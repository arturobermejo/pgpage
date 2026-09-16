package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureHeap is the relation file shared with the pgpage package tests.
const fixtureHeap = "../../testdata/heap_small"

// runCommand runs the command line args with no input and returns its exit
// code and output.
func runCommand(args ...string) (code int, stdout, stderr string) {
	return runCommandInput("", args...)
}

// runCommandInput is runCommand with stdin, for the commands that read keys.
func runCommandInput(stdin string, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer

	code = run(args, strings.NewReader(stdin), &out, &errOut)

	return code, out.String(), errOut.String()
}

// writePage writes page as the only page of a relation file in a temporary
// directory and returns its path.
func writePage(t *testing.T, page []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "relation")
	if err := os.WriteFile(path, page, 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

// invalidPage returns the invalid page example of the PRD: a complete header
// whose pd_lower is past pd_upper.
func invalidPage() []byte {
	page := make([]byte, 8192)
	page[12], page[13] = 0xF4, 0x01 // pd_lower = 500
	page[14], page[15] = 0x64, 0x00 // pd_upper = 100
	page[16], page[17] = 0x00, 0x20 // pd_special = 8192
	page[18], page[19] = 0x04, 0x20 // page size 8192, layout version 4

	return page
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		stderr string
	}{
		{name: "no command", args: nil, stderr: "usage: pgpage <command>"},
		{name: "unknown command", args: []string{"dump"}, stderr: `unknown command "dump"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCommand(tt.args...)

			if code != exitUsage {
				t.Errorf("exit code %d, want %d", code, exitUsage)
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

func TestHelp(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		stdout string
		stderr string
	}{
		{name: "help command", args: []string{"help"}, stdout: "usage: pgpage <command>"},
		{name: "inspect -h", args: []string{"inspect", "-h"}, stderr: "usage: pgpage inspect"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCommand(tt.args...)

			if code != exitOK {
				t.Errorf("exit code %d, want %d", code, exitOK)
			}

			if !strings.Contains(stdout, tt.stdout) || !strings.Contains(stderr, tt.stderr) {
				t.Errorf("stdout = %q, stderr = %q; want them to contain %q and %q", stdout, stderr, tt.stdout, tt.stderr)
			}
		})
	}
}
