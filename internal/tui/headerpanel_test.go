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

	for _, line := range strings.Split(panel, "\n")[1:] {
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

// splitField takes a "name   value" line apart at the field column.
func splitField(line string) (name, value string, ok bool) {
	if len(line) < fieldColumn {
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

	for _, line := range strings.Split(panel, "\n")[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}

		name, _, ok := splitField(line)
		if !ok {
			t.Errorf("line %q does not have a field name", line)
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
			want:   []string{"PAGE HEADER", "reading…"},
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
		want  string
	}{
		{flags: 0, want: "—"},
		{flags: pgpage.PageHasFreeLines, want: "HAS_FREE_LINES"},
		{flags: pgpage.PageAllVisible, want: "ALL_VISIBLE"},
		{flags: pgpage.PageFull | pgpage.PageAllVisible, want: "FULL,ALL_VISIBLE"},
		{
			flags: pgpage.PageHasFreeLines | pgpage.PageFull | pgpage.PageAllVisible,
			want:  "HAS_FREE_LINES,FULL,ALL_VISIBLE",
		},
	}

	for _, tt := range tests {
		if got := pageFlagNames(tt.flags); got != tt.want {
			t.Errorf("pageFlagNames(%#04x) = %q, want %q", uint16(tt.flags), got, tt.want)
		}
	}
}
