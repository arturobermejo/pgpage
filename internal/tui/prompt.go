package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/arturobermejo/pgpage"
)

// blockDigits is the longest block number a relation can have, so that no
// typing can overflow the parser.
const blockDigits = 10

// prompt is the "go to block" line: a text input, whether it is taking keys
// and what was wrong with the last thing typed.
//
// While it is active it captures every key, so that typing "q" writes a q
// instead of quitting.
type prompt struct {
	input  textinput.Model
	active bool
	err    string
}

// newPrompt returns the prompt closed and ready to be opened.
func newPrompt() prompt {
	input := textinput.New()
	input.Prompt = "block "
	input.CharLimit = blockDigits
	input.Width = blockDigits

	return prompt{input: input}
}

// open clears the prompt and gives it the keyboard. It returns the command
// that makes the cursor blink.
func (p prompt) open() (prompt, tea.Cmd) {
	p.active = true
	p.err = ""
	p.input.SetValue("")

	return p, p.input.Focus()
}

// close gives the keyboard back to the explorer.
func (p prompt) close() prompt {
	p.active = false
	p.err = ""
	p.input.Blur()

	return p
}

// update hands the key to the text input, which is a model of its own with
// its own Update: it is the one that knows about the cursor, the backspace
// and the rest of line editing.
func (p prompt) update(msg tea.Msg) (prompt, tea.Cmd) {
	var cmd tea.Cmd

	p.input, cmd = p.input.Update(msg)

	return p, cmd
}

// view renders the prompt line: the input, the range that is valid and
// either the keys or the error of the last attempt.
func (p prompt) view(pages pgpage.BlockNumber) string {
	rng := fieldStyle.Render(fmt.Sprintf("  range 0-%d", pages-1))

	hint := moreStyle.Render("  ↵ jump · Esc cancel · 0x for hex")
	if p.err != "" {
		hint = invalidStyle.Render("  " + p.err)
	}

	return titleStyle.Render("GO TO BLOCK  ") + p.input.View() + rng + hint
}

// parseBlock reads a block number the way the user typed it: in decimal, or
// in hexadecimal with the 0x prefix that page offsets are shown with.
//
// The error is meant to be read in the prompt, so it says what to do rather
// than what happened.
func parseBlock(text string, pages pgpage.BlockNumber) (pgpage.BlockNumber, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("type a block number between 0 and %d", pages-1)
	}

	base := 10
	if digits, found := strings.CutPrefix(strings.ToLower(text), "0x"); found {
		base, text = 16, digits
	}

	// ParseUint rejects a leading minus, so a negative block never reaches
	// the range check as a huge unsigned number.
	block, err := strconv.ParseUint(text, base, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a block number", text)
	}

	if block >= uint64(pages) {
		return 0, fmt.Errorf("block %d is past the last one, %d", block, pages-1)
	}

	return pgpage.BlockNumber(block), nil
}
