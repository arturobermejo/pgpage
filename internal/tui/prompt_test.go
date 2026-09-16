package tui

import (
	"strings"
	"testing"

	"github.com/arturobermejo/pgpage"
)

// What the user types is read in decimal or, with the 0x prefix, in the
// hexadecimal the page offsets are shown in.
func TestParseBlock(t *testing.T) {
	const pages = 128

	tests := []struct {
		name  string
		text  string
		want  pgpage.BlockNumber
		error string
	}{
		{name: "decimal", text: "42", want: 42},
		{name: "the first block", text: "0", want: 0},
		{name: "the last block", text: "127", want: 127},
		{name: "spaces around it", text: "  7 ", want: 7},
		{name: "hexadecimal", text: "0x2a", want: 42},
		{name: "hexadecimal in capitals", text: "0X2A", want: 42},
		{name: "nothing typed", text: "", error: "type a block number"},
		{name: "only spaces", text: "   ", error: "type a block number"},
		{name: "not a number", text: "abc", error: "not a block number"},
		{name: "decimal digits with letters", text: "12x", error: "not a block number"},
		{name: "0x without digits", text: "0x", error: "not a block number"},
		{name: "negative", text: "-1", error: "not a block number"},
		{name: "past the last block", text: "128", error: "past the last one"},
		{name: "far past the last block", text: "99999999", error: "past the last one"},
		{name: "past the last block in hex", text: "0xff", error: "past the last one"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, err := parseBlock(tt.text, pages)

			switch {
			case tt.error == "" && err != nil:
				t.Fatalf("parseBlock(%q) failed: %v", tt.text, err)
			case tt.error != "" && err == nil:
				t.Fatalf("parseBlock(%q) = %d, want an error about %q", tt.text, block, tt.error)
			case tt.error != "":
				if !strings.Contains(err.Error(), tt.error) {
					t.Errorf("parseBlock(%q) failed with %q, want it to mention %q", tt.text, err, tt.error)
				}

				return
			}

			if block != tt.want {
				t.Errorf("parseBlock(%q) = %d, want %d", tt.text, block, tt.want)
			}
		})
	}
}

// A relation with a single page accepts only block 0: the range check must
// not underflow when it builds its message either.
func TestParseBlockOnePage(t *testing.T) {
	if block, err := parseBlock("0", 1); err != nil || block != 0 {
		t.Errorf("parseBlock(\"0\", 1) = %d, %v; want 0 and no error", block, err)
	}

	if _, err := parseBlock("1", 1); err == nil {
		t.Error("block 1 was accepted in a relation with one page")
	}
}

// The prompt shows what can be typed and, after a bad attempt, what was
// wrong with it.
func TestPromptView(t *testing.T) {
	p, _ := newPrompt().open()

	view := p.view(128)

	for _, want := range []string{"GO TO BLOCK", "block", "range 0-127", "Esc cancel", "0x for hex"} {
		if !strings.Contains(view, want) {
			t.Errorf("prompt does not contain %q:\n%s", want, view)
		}
	}

	p.err = "block 300 is past the last one, 127"

	view = p.view(128)

	if !strings.Contains(view, "past the last one") {
		t.Errorf("prompt does not show the error:\n%s", view)
	}

	if strings.Contains(view, "Esc cancel") {
		t.Errorf("the error did not replace the key hints:\n%s", view)
	}
}

// Opening the prompt clears whatever was typed the last time.
func TestPromptOpenClears(t *testing.T) {
	p, _ := newPrompt().open()
	p.input.SetValue("41")
	p.err = "boom"

	p = p.close()

	if p.active {
		t.Error("the prompt stayed active after close")
	}

	p, _ = p.open()

	if p.input.Value() != "" || p.err != "" {
		t.Errorf("the prompt reopened with %q and error %q", p.input.Value(), p.err)
	}

	if !p.active {
		t.Error("the prompt is not active after open")
	}
}
