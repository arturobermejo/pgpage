package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/arturobermejo/pgpage"
)

// fieldColumn is the width of the field names of a panel, the longest one
// plus a space.
const fieldColumn = 16

// headerPanel renders what the page header of the selected page says: first
// the fields as they are stored, then the values derived from them.
//
//	PAGE HEADER
//	pd_lsn          0/21A8A18
//	pd_checksum     45171
//	...
//	items           185
//	free space      1708 B
//	status          OK
func headerPanel(summary pgpage.PageSummary, cached bool) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("PAGE HEADER"))

	if !cached {
		b.WriteString("\n" + moreStyle.Render("reading…"))
		return b.String()
	}

	if summary.Status != pgpage.StatusOK {
		// A header that did not parse describes nothing. Step 25 gives these
		// pages a view of their own.
		b.WriteString("\n" + field("status", statusStyle(summary.Status).Render(summary.Status.String())))

		if summary.Err != nil {
			b.WriteString("\n" + field("error", invalidStyle.Render(summary.Err.Error())))
		}

		return b.String()
	}

	h := summary.Header

	free := "—"
	if percent, ok := summary.FreeSpacePercent(); ok {
		free = fmt.Sprintf("%.0f %%", percent)
	}

	rows := []struct{ name, value string }{
		{"pd_lsn", h.LSN.String()},
		{"pd_checksum", fmt.Sprint(h.Checksum)},
		{"pd_flags", fmt.Sprintf("%#04x", uint16(h.Flags))},
		{"pd_lower", fmt.Sprint(h.Lower)},
		{"pd_upper", fmt.Sprint(h.Upper)},
		{"pd_special", fmt.Sprint(h.Special)},
		{"page size", fmt.Sprint(h.PageSize)},
		{"layout version", fmt.Sprint(h.LayoutVersion)},
		{"pd_prune_xid", fmt.Sprint(uint32(h.PruneXID))},
		{"", ""}, // the line between what is stored and what is derived
		{"items", fmt.Sprint(h.ItemCount())},
		{"free space", fmt.Sprintf("%d B", h.FreeSpace())},
		{"free", free},
		{"flags decoded", pageFlagNames(h.Flags)},
	}

	for _, row := range rows {
		if row.name == "" {
			b.WriteString("\n")
			continue
		}

		b.WriteString("\n" + field(row.name, numberStyle.Render(row.value)))
	}

	b.WriteString("\n" + field("status", statusStyle(summary.Status).Render(summary.Status.String())))

	return b.String()
}

// field renders one "name  value" line of a panel.
func field(name, value string) string {
	return fieldStyle.Render(padRight(name, fieldColumn)) + valueStyle.Render(value)
}

// padRight left aligns s in a field of width cells, counting cells and not
// bytes, as padLeft does.
func padRight(s string, width int) string {
	if pad := width - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}

	return s + " "
}

// pageFlagNames returns the names of the pd_flags bits that are set, as
// PostgreSQL spells them, or a dash when none is.
func pageFlagNames(flags pgpage.PageFlags) string {
	var names []string

	if flags.HasFreeLines() {
		names = append(names, "HAS_FREE_LINES")
	}

	if flags.IsFull() {
		names = append(names, "FULL")
	}

	if flags.IsAllVisible() {
		names = append(names, "ALL_VISIBLE")
	}

	if len(names) == 0 {
		return "—"
	}

	return strings.Join(names, ",")
}
