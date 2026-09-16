package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/arturobermejo/pgpage"
)

// TestMain turns colors off for every test in the package. Lip Gloss picks
// its color profile from the terminal, so without this the rendered strings
// would carry escape sequences when the tests run on a terminal and not when
// they run on CI.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

// The top bar names the relation and its shape, and pins the block counter
// to the right edge.
func TestTopBar(t *testing.T) {
	const width = 80

	bar := topBar(openFixture(t), 1, width)

	for _, want := range []string{"pgpage", fixtureHeap, "3 pages", "8192 B/page", "24.0 KB"} {
		if !strings.Contains(bar, want) {
			t.Errorf("top bar does not contain %q:\n%s", want, bar)
		}
	}

	if got := lipgloss.Width(bar); got != width {
		t.Errorf("width = %d, want %d:\n%s", got, width, bar)
	}

	if !strings.HasSuffix(bar, "blk 1/2") {
		t.Errorf("top bar does not end with the block counter:\n%s", bar)
	}
}

// A terminal too narrow for both sides keeps the relation and drops the
// block counter, never wrapping to a second line.
func TestTopBarNarrow(t *testing.T) {
	widths := []int{0, 1, 10, 30}

	for _, width := range widths {
		bar := topBar(openFixture(t), 0, width)

		if strings.Contains(bar, "blk") {
			t.Errorf("width %d: block counter does not fit but was drawn:\n%s", width, bar)
		}

		if strings.Contains(bar, "\n") {
			t.Errorf("width %d: top bar has more than one line:\n%s", width, bar)
		}

		// Width 0 means the size is still unknown: nothing is cut yet.
		if got := lipgloss.Width(bar); width > 0 && got > width {
			t.Errorf("width %d: bar is %d cells wide:\n%s", width, got, bar)
		}
	}
}

// A file with no complete page has no block to count.
func TestTopBarEmptyRelation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	rel, err := pgpage.OpenRelation(path)
	if err != nil {
		t.Fatal(err)
	}
	defer rel.Close()

	// Wide enough for the temporary directory's long path.
	bar := topBar(rel, 0, 200)

	if !strings.Contains(bar, "0 pages") || !strings.HasSuffix(bar, "no pages") {
		t.Errorf("top bar of an empty relation:\n%s", bar)
	}
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{size: 0, want: "0 B"},
		{size: 1023, want: "1023 B"},
		{size: 1024, want: "1.0 KB"},
		{size: 8192, want: "8.0 KB"},
		{size: 24576, want: "24.0 KB"},
		{size: 1048575, want: "1024.0 KB"}, // one byte short of a megabyte
		{size: 1048576, want: "1.0 MB"},
		{size: 1073741824, want: "1.0 GB"},
		{size: 1099511627776, want: "1024.0 GB"}, // a terabyte still counts in GB
	}

	for _, tt := range tests {
		if got := humanSize(tt.size); got != tt.want {
			t.Errorf("humanSize(%d) = %q, want %q", tt.size, got, tt.want)
		}
	}
}
