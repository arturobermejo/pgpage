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

// target is what a prompt asks for: the view whose list it moves, what the
// numbers in that list are called, and the range of the valid ones. Blocks
// are numbered from 0, line pointers from 1, as PostgreSQL numbers them.
type target struct {
	view        view
	noun        string
	first, last uint64
}

// blockTarget asks for a block of a relation of pages pages.
func blockTarget(pages pgpage.BlockNumber) target {
	return target{view: viewPages, noun: "block", first: 0, last: uint64(pages) - 1}
}

// itemTarget asks for a line pointer of a page that has count of them.
func itemTarget(count int) target {
	return target{view: viewItems, noun: "line pointer", first: uint64(pgpage.FirstOffsetNumber), last: uint64(count)}
}

// prompt is the "go to" line: a text input, what it asks for, whether it is
// taking keys and what was wrong with the last thing typed.
//
// While it is active it captures every key, so that typing "q" writes a q
// instead of quitting.
type prompt struct {
	input  textinput.Model
	target target
	active bool
	err    string
}

// newPrompt returns the prompt closed and ready to be opened.
func newPrompt() prompt {
	input := textinput.New()
	input.CharLimit = blockDigits
	input.Width = blockDigits

	return prompt{input: input}
}

// open clears the prompt, sets what it asks for and gives it the keyboard.
// It returns the command that makes the cursor blink.
func (p prompt) open(t target) (prompt, tea.Cmd) {
	p.target = t
	p.active = true
	p.err = ""
	p.input.Prompt = t.noun + " "
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
func (p prompt) view() string {
	rng := fieldStyle.Render(fmt.Sprintf("  range %d-%d", p.target.first, p.target.last))

	hint := moreStyle.Render("  ↵ jump · Esc cancel · 0x for hex")
	if p.err != "" {
		hint = invalidStyle.Render("  " + p.err)
	}

	title := "GO TO " + strings.ToUpper(p.target.noun) + "  "

	return titleStyle.Render(title) + p.input.View() + rng + hint
}

// parse reads a number the way the user typed it: in decimal, or in
// hexadecimal with the 0x prefix that page offsets are shown with.
//
// The error is meant to be read in the prompt, so it says what to do rather
// than what happened.
func (t target) parse(text string) (uint64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("type a %s number between %d and %d", t.noun, t.first, t.last)
	}

	base := 10
	if digits, found := strings.CutPrefix(strings.ToLower(text), "0x"); found {
		base, text = 16, digits
	}

	// ParseUint rejects a leading minus, so a negative number never reaches
	// the range check as a huge unsigned one.
	n, err := strconv.ParseUint(text, base, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a %s number", text, t.noun)
	}

	switch {
	case n < t.first:
		return 0, fmt.Errorf("%s numbers start at %d", t.noun, t.first)
	case n > t.last:
		return 0, fmt.Errorf("%s %d is past the last one, %d", t.noun, n, t.last)
	}

	return n, nil
}

// parseBlock reads a block number of a relation of pages pages.
func parseBlock(text string, pages pgpage.BlockNumber) (pgpage.BlockNumber, error) {
	n, err := blockTarget(pages).parse(text)

	return pgpage.BlockNumber(n), err
}
