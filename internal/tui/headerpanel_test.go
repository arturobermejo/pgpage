package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// The panel shows the stored fields with the values PostgreSQL wrote, and
// the values derived from them.
func TestHeaderPanel(t *testing.T) {
	panel := headerPanel(pgpage.SummarizePage(fixturePage(t, 0)), true)

	// Same numbers page_header() reports for block 0 of the fixture.
	fields := map[string]string{
		"pd_lsn":         "0/21A8A18",
		"pd_checksum":    "6769",
		"pd_flags":       "0x0000",
		"pd_lower":       "764",
		"pd_upper":       "2472",
		"pd_special":     "8192",
		"page size":      "8192",
		"layout version": "4",
		"pd_prune_xid":   "0",
		"items":          "185",
		"free space":     "1708 B",
		"free":           "21 %",
		"flags decoded":  "—",
		"status":         "OK",
	}

	rows := map[string]string{}

	for _, line := range strings.Split(panel, "\n") {
		if name, value, ok := splitField(line); ok {
			rows[name] = value
		}
	}

	if len(rows) != len(fields) {
		t.Errorf("%d fields in the panel, want %d:\n%s", len(rows), len(fields), panel)
	}

	for name, want := range fields {
		if rows[name] != want {
			t.Errorf("%s = %q, want %q", name, rows[name], want)
		}
	}
}

// splitField takes a "name   value" line apart at the field column. Blank
// lines and the rule between the two groups of fields are not fields.
func splitField(line string) (name, value string, ok bool) {
	if len(line) < fieldColumn || strings.HasPrefix(line, borderHorizontal) {
		return "", "", false
	}

	name = strings.TrimSpace(line[:fieldColumn])
	value = strings.TrimSpace(line[fieldColumn:])

	return name, value, name != ""
}

// Values start at the same column whatever the name, which is what makes
// the panel readable.
func TestHeaderPanelAligns(t *testing.T) {
	panel := headerPanel(pgpage.SummarizePage(fixturePage(t, 0)), true)

	for _, line := range strings.Split(panel, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, borderHorizontal) {
			continue
		}

		name, _, ok := splitField(line)
		if !ok {
			// A flag after the first one continues the field above it.
			if !strings.HasPrefix(line, strings.Repeat(" ", fieldColumn)) {
				t.Errorf("line %q does not have a field name", line)
			}

			continue
		}

		if got := lipgloss.Width(name); got >= fieldColumn {
			t.Errorf("field %q is %d cells wide, the column is %d", name, got, fieldColumn)
		}
	}
}

// A page without a valid header describes nothing: the panel says what is
// wrong instead of printing zeroes as if they were data.
func TestHeaderPanelNotOK(t *testing.T) {
	tests := []struct {
		name    string
		summary pgpage.PageSummary
		cached  bool
		want    []string
		absent  []string
	}{
		{
			name:   "not read yet",
			want:   []string{"reading…"},
			absent: []string{"pd_lsn", "status"},
		},
		{
			name:    "new page",
			summary: pgpage.SummarizePage(make([]byte, pgpage.PageSize)),
			cached:  true,
			want:    []string{"status", "NEW"},
			absent:  []string{"pd_lsn", "free space"},
		},
		{
			name:    "invalid page",
			summary: pgpage.PageSummary{Status: pgpage.StatusInvalid, Err: errors.New("lower past upper")},
			cached:  true,
			want:    []string{"status", "INVALID", "error", "lower past upper"},
			absent:  []string{"pd_lsn", "items"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			panel := headerPanel(tt.summary, tt.cached)

			for _, want := range tt.want {
				if !strings.Contains(panel, want) {
					t.Errorf("panel does not contain %q:\n%s", want, panel)
				}
			}

			for _, absent := range tt.absent {
				if strings.Contains(panel, absent) {
					t.Errorf("panel contains %q, which it cannot know:\n%s", absent, panel)
				}
			}
		})
	}
}

func TestPageFlagNames(t *testing.T) {
	tests := []struct {
		flags pgpage.PageFlags
		want  []string
	}{
		{flags: 0, want: []string{"—"}},
		{flags: pgpage.PageHasFreeLines, want: []string{"HAS_FREE_LINES"}},
		{flags: pgpage.PageAllVisible, want: []string{"ALL_VISIBLE"}},
		{flags: pgpage.PageFull | pgpage.PageAllVisible, want: []string{"FULL", "ALL_VISIBLE"}},
		{
			flags: pgpage.PageHasFreeLines | pgpage.PageFull | pgpage.PageAllVisible,
			want:  []string{"HAS_FREE_LINES", "FULL", "ALL_VISIBLE"},
		},
	}

	for _, tt := range tests {
		if got := pageFlagNames(tt.flags); strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("pageFlagNames(%#04x) = %q, want %q", uint16(tt.flags), got, tt.want)
		}
	}
}

// Each flag of a page goes on a line of its own, lined up under the first
// one, so that the panel stays narrow.
func TestHeaderPanelFlagsOnePerLine(t *testing.T) {
	summary := withFlags(pgpage.SummarizePage(fixturePage(t, 0)),
		pgpage.PageHasFreeLines|pgpage.PageFull|pgpage.PageAllVisible)

	lines := strings.Split(headerPanel(summary, true), "\n")

	want := []string{"flags decoded", "", ""}
	names := []string{"HAS_FREE_LINES", "FULL", "ALL_VISIBLE"}

	first := -1

	for i, line := range lines {
		if strings.HasPrefix(line, "flags decoded") {
			first = i
		}
	}

	if first < 0 || first+len(names) > len(lines) {
		t.Fatalf("no flags decoded field:\n%s", strings.Join(lines, "\n"))
	}

	for i, name := range names {
		line := lines[first+i]

		if got := strings.TrimSpace(line[:fieldColumn]); got != want[i] {
			t.Errorf("line %d has field %q, want %q", i, got, want[i])
		}

		if got := strings.TrimSpace(line[fieldColumn:]); got != name {
			t.Errorf("line %d shows %q, want the flag %q", i, got, name)
		}
	}
}

// The stored fields and the derived ones are separated by a rule.
func TestHeaderPanelRule(t *testing.T) {
	lines := strings.Split(headerPanel(pgpage.SummarizePage(fixturePage(t, 0)), true), "\n")

	var rule, before, after int

	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, borderHorizontal):
			rule = i
		case strings.HasPrefix(line, "pd_prune_xid"):
			before = i
		case strings.HasPrefix(line, "items"):
			after = i
		}
	}

	if rule == 0 || before >= rule || rule >= after {
		t.Errorf("no rule between pd_prune_xid (line %d) and items (line %d), rule at %d:\n%s",
			before, after, rule, strings.Join(lines, "\n"))
	}
}

// pd_lower and pd_upper take the colors of the regions they bound on the
// map, and the status the color of its state; the rest is plain text.
func TestHeaderPanelColors(t *testing.T) {
	withColor(t)

	lines := strings.Split(headerPanel(pgpage.SummarizePage(fixturePage(t, 0)), true), "\n")

	colors := map[string]string{
		"pd_lower": "38;5;109", // line pointers
		"pd_upper": "38;5;144", // tuples
		"status":   "38;5;114", // OK
		"pd_lsn":   "38;5;255", // plain value
	}

	for name, color := range colors {
		found := false

		for _, line := range lines {
			if strings.Contains(line, name) {
				found = true

				if !strings.Contains(line, color) {
					t.Errorf("%s is not drawn in %s:\n%q", name, color, line)
				}
			}
		}

		if !found {
			t.Errorf("no line for %s", name)
		}
	}
}

// The header panel is as wide on a page with every flag set as on a page
// with none, so moving the selection does not resize the panels around it.
func TestHeaderBoxWidthIsStable(t *testing.T) {
	plain := pgpage.SummarizePage(fixturePage(t, 0))

	flagged := plain
	flagged.Header.Flags = pgpage.PageHasFreeLines | pgpage.PageFull | pgpage.PageAllVisible

	contents := map[string]string{
		"no flags":      headerPanel(plain, true),
		"every flag":    headerPanel(flagged, true),
		"not read yet":  headerPanel(pgpage.PageSummary{}, false),
		"one flag only": headerPanel(withFlags(plain, pgpage.PageAllVisible), true),
	}

	want := headerBoxWidth(contents["no flags"])

	for name, content := range contents {
		if got := headerBoxWidth(content); got != want {
			t.Errorf("%s: header box is %d cells wide, want %d like every other page", name, got, want)
		}

		if lipgloss.Width(content)+panelFrame > want {
			t.Errorf("%s: content is wider than the box:\n%s", name, content)
		}
	}
}

// withFlags returns summary with its pd_flags replaced.
func withFlags(summary pgpage.PageSummary, flags pgpage.PageFlags) pgpage.PageSummary {
	summary.Header.Flags = flags

	return summary
}
