package pgpage

import (
	"fmt"
	"strings"
)

// HexBytesPerLine is the number of bytes in each line of a hex dump.
const HexBytesPerLine = 16

// HexLine is one line of a hex dump.
type HexLine struct {
	// Offset is the position of the first byte, counted from the base given
	// to HexLines, so that a dump of part of a page shows page offsets.
	Offset int
	// Bytes holds up to HexBytesPerLine bytes. It is a sub-slice of the
	// dumped data, not a copy.
	Bytes []byte
}

// HexLines splits data into lines of HexBytesPerLine bytes, the last one
// possibly shorter, numbering them from base. Callers that highlight a range
// of bytes can locate byte i of a line at offset line.Offset+i.
func HexLines(data []byte, base int) []HexLine {
	lines := make([]HexLine, 0, (len(data)+HexBytesPerLine-1)/HexBytesPerLine)

	for i := 0; i < len(data); i += HexBytesPerLine {
		end := min(i+HexBytesPerLine, len(data))
		lines = append(lines, HexLine{Offset: base + i, Bytes: data[i:end:end]})
	}

	return lines
}

// ASCII returns the bytes as text, with printable ASCII characters shown as
// themselves and every other byte as '.'.
func (l HexLine) ASCII() string {
	text := make([]byte, len(l.Bytes))

	for i, b := range l.Bytes {
		if b >= ' ' && b <= '~' {
			text[i] = b
		} else {
			text[i] = '.'
		}
	}

	return string(text)
}

// String formats the line like hexdump -C with a 4-digit offset, for example
//
//	0000  00 00 00 00 10 58 9a 01  73 b0 00 00 48 00 f0 1f  |.....X..s...H...|
//
// A short line is padded so its text column lines up with full lines.
func (l HexLine) String() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "%04x ", l.Offset)

	for i := range HexBytesPerLine {
		if i%8 == 0 {
			sb.WriteByte(' ')
		}

		if i < len(l.Bytes) {
			fmt.Fprintf(&sb, "%02x ", l.Bytes[i])
		} else {
			sb.WriteString("   ")
		}
	}

	sb.WriteString(" |")
	sb.WriteString(l.ASCII())
	sb.WriteByte('|')

	return sb.String()
}
