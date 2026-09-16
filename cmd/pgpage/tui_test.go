package main

import (
	"strings"
	"testing"
)

// The command draws the relation and exits when the user presses q. A
// relation file with no command opens the explorer too, and --block selects
// the page it starts on, before or after the file.
func TestTUI(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		selected string
	}{
		{name: "tui command", args: []string{"tui", fixtureHeap}, selected: "> 0"},
		{name: "no command", args: []string{fixtureHeap}, selected: "> 0"},
		{name: "block after the file", args: []string{fixtureHeap, "--block", "2"}, selected: "> 2"},
		{name: "block before the file", args: []string{"--block", "1", fixtureHeap}, selected: "> 1"},
		{name: "tui command with a block", args: []string{"tui", fixtureHeap, "--block=2"}, selected: "> 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCommandInput("q", tt.args...)
			if code != exitOK {
				t.Fatalf("exit code %d, want %d (stderr: %s)", code, exitOK, stderr)
			}

			if !strings.Contains(stdout, fixtureHeap) {
				t.Errorf("stdout does not name the relation:\n%q", stdout)
			}

			if !strings.Contains(stdout, tt.selected) {
				t.Errorf("stdout does not select %q:\n%q", tt.selected, stdout)
			}
		})
	}
}

// An empty relation has no block to start on, and opens all the same; only
// a block asked for must exist.
func TestTUIEmptyRelation(t *testing.T) {
	path := writePage(t, nil)

	code, stdout, stderr := runCommandInput("q", path)
	if code != exitOK || !strings.Contains(stdout, "0 pages") {
		t.Errorf("exit code %d, stderr %q, stdout:\n%q", code, stderr, stdout)
	}

	code, _, stderr = runCommandInput("q", path, "--block", "1")
	if code != exitError || !strings.Contains(stderr, "block 1 is out of range") {
		t.Errorf("--block 1: exit code %d, stderr %q", code, stderr)
	}
}

func TestTUIErrors(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{name: "missing file", args: []string{"tui"}, code: exitUsage, stderr: "missing relation file"},
		{name: "unknown file", args: []string{"tui", "nope"}, code: exitError, stderr: "no such file"},
		{name: "extra argument", args: []string{"tui", fixtureHeap, "x"}, code: exitUsage, stderr: `unexpected argument "x"`},
		{name: "help", args: []string{"tui", "-h"}, code: exitOK, stderr: "usage: pgpage [tui] <relation-file> [--block N]"},
		{name: "block out of range", args: []string{fixtureHeap, "--block", "3"}, code: exitError, stderr: "block 3 is out of range"},
		{name: "huge block", args: []string{"tui", fixtureHeap, "--block", "4294967298"}, code: exitError, stderr: "out of range"},
		{name: "block of a missing file", args: []string{"--block", "1", "nope"}, code: exitError, stderr: "no such file"},
		{name: "bad block", args: []string{fixtureHeap, "--block", "two"}, code: exitUsage, stderr: "invalid value"},
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
