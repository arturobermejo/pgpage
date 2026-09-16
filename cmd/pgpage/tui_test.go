package main

import (
	"strings"
	"testing"
)

// The command draws the relation and exits when the user presses q.
func TestTUI(t *testing.T) {
	code, stdout, stderr := runCommandInput("q", "tui", fixtureHeap)
	if code != exitOK {
		t.Fatalf("exit code %d, want %d (stderr: %s)", code, exitOK, stderr)
	}

	if !strings.Contains(stdout, fixtureHeap) {
		t.Errorf("stdout does not name the relation:\n%q", stdout)
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
		{name: "help", args: []string{"tui", "-h"}, code: exitOK, stderr: "usage: pgpage tui"},
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
