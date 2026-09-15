package pgpage

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// The header bytes from the PRD must format exactly as the PRD shows them.
func TestHexLineStringPRD(t *testing.T) {
	data := []byte{
		0x00, 0x00, 0x00, 0x00, 0x10, 0x58, 0x9a, 0x01, 0x73, 0xb0, 0x00, 0x00, 0x48, 0x00, 0xf0, 0x1f,
		0xf0, 0x1f, 0x04, 0x20, 0x00, 0x00, 0x00, 0x00, 0x62, 0x31, 0x05, 0x00, 0x04, 0x00, 0x00, 0x00,
	}

	want := []string{
		"0000  00 00 00 00 10 58 9a 01  73 b0 00 00 48 00 f0 1f  |.....X..s...H...|",
		"0010  f0 1f 04 20 00 00 00 00  62 31 05 00 04 00 00 00  |... ....b1......|",
	}

	lines := HexLines(data, 0)
	if len(lines) != len(want) {
		t.Fatalf("HexLines returned %d lines, want %d", len(lines), len(want))
	}

	for i, line := range lines {
		if got := line.String(); got != want[i] {
			t.Errorf("line %d:\n got %q\nwant %q", i, got, want[i])
		}
	}
}

// Lines of the fixture must match hexdump -C, apart from the offset width.
// The expected text was produced with hexdump -C on testdata/heap_small.
func TestHexLinesFixture(t *testing.T) {
	rel := openRelation(t, fixtureHeap)

	page0, err := rel.ReadPage(0)
	if err != nil {
		t.Fatal(err)
	}

	page2, err := rel.ReadPage(2)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		data []byte
		base int
		want []string
	}{
		{
			name: "header and first line pointers of block 0",
			data: page0[:48],
			base: 0,
			want: []string{
				"0000  00 00 00 00 18 8a 1a 02  71 1a 00 00 fc 02 a8 09  |........q.......|",
				"0010  00 20 04 20 00 00 00 00  d8 9f 46 00 b0 9f 46 00  |. . ......F...F.|",
				"0020  88 9f 46 00 60 9f 46 00  38 9f 46 00 10 9f 46 00  |..F.`.F.8.F...F.|",
			},
		},
		{
			// hexdump -s 19256 -n 40 shows file offsets 00004b38-00004b5f.
			name: "tuple (2,168) at page offset 2872",
			data: page2[2872 : 2872+40],
			base: 2872,
			want: []string{
				"0b38  08 03 00 00 00 00 00 00  00 00 00 00 00 00 02 00  |................|",
				"0b48  a8 00 02 80 02 29 18 00  7c 01 00 00 19 75 73 65  |.....)..|....use|",
				"0b58  72 20 33 38 30 20 76 32                           |r 380 v2|",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := HexLines(tt.data, tt.base)
			if len(lines) != len(tt.want) {
				t.Fatalf("HexLines returned %d lines, want %d", len(lines), len(tt.want))
			}

			for i, line := range lines {
				if got := line.String(); got != tt.want[i] {
					t.Errorf("line %d:\n got %q\nwant %q", i, got, tt.want[i])
				}
			}
		})
	}
}

func TestHexLinesSplit(t *testing.T) {
	tests := []struct {
		size    int
		lines   int
		lastLen int
	}{
		{size: 0, lines: 0},
		{size: 1, lines: 1, lastLen: 1},
		{size: 15, lines: 1, lastLen: 15},
		{size: 16, lines: 1, lastLen: 16},
		{size: 17, lines: 2, lastLen: 1},
		{size: PageSize, lines: PageSize / HexBytesPerLine, lastLen: 16},
	}

	for _, tt := range tests {
		data := make([]byte, tt.size)
		lines := HexLines(data, 100)

		if len(lines) != tt.lines {
			t.Errorf("%d bytes: %d lines, want %d", tt.size, len(lines), tt.lines)
			continue
		}

		for i, line := range lines {
			if want := 100 + i*HexBytesPerLine; line.Offset != want {
				t.Errorf("%d bytes: line %d offset %d, want %d", tt.size, i, line.Offset, want)
			}
		}

		if tt.lines > 0 {
			if got := len(lines[len(lines)-1].Bytes); got != tt.lastLen {
				t.Errorf("%d bytes: last line has %d bytes, want %d", tt.size, got, tt.lastLen)
			}
		}
	}
}

// Joining all lines must give back the data, and byte i of a line must be
// the data byte at line.Offset+i-base.
func TestHexLinesCoverData(t *testing.T) {
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i * 7)
	}

	const base = 8000

	var joined []byte

	for _, line := range HexLines(data, base) {
		for i, b := range line.Bytes {
			if want := data[line.Offset+i-base]; b != want {
				t.Fatalf("byte at offset %d = %#x, want %#x", line.Offset+i, b, want)
			}
		}

		joined = append(joined, line.Bytes...)
	}

	if !bytes.Equal(joined, data) {
		t.Error("lines do not add up to the data")
	}
}

func TestHexLineASCII(t *testing.T) {
	line := HexLine{Bytes: []byte{
		0x00, 0x1f, // control characters
		' ', 'A', 'z', '~', // printable, from the first (space) to the last (tilde)
		0x7f, 0x80, 0xff, // DEL and bytes outside ASCII
		'.', '|', // printable characters that look like the placeholders
	}}

	if got, want := line.ASCII(), ".. Az~....|"; got != want {
		t.Errorf("ASCII() = %q, want %q", got, want)
	}
}

// Every line String must line up: offsets, hex and text columns at the same
// positions, whatever the number of bytes.
func TestHexLineStringAlignment(t *testing.T) {
	full := HexLine{Offset: 0x1ff0, Bytes: bytes.Repeat([]byte{'a'}, HexBytesPerLine)}.String()

	for n := range HexBytesPerLine + 1 {
		got := HexLine{Offset: 0x1ff0, Bytes: bytes.Repeat([]byte{'a'}, n)}.String()

		if bar := strings.IndexByte(got, '|'); bar != strings.IndexByte(full, '|') {
			t.Errorf("%d bytes: text column starts at %d, want %d:\n%s\n%s", n, bar, strings.IndexByte(full, '|'), got, full)
		}
	}
}

// Apart from the offset width, the format is that of hexdump -C, which the
// standard library also implements.
func TestHexLinesMatchesStandardLibrary(t *testing.T) {
	data := []byte("PostgreSQL stores tables in 8 kB pages.\x00\x01\x02")

	var got strings.Builder
	for _, line := range HexLines(data, 0) {
		got.WriteString("0000" + line.String() + "\n")
	}

	if want := hex.Dump(data); got.String() != want {
		t.Errorf("HexLines does not match hex.Dump:\n got\n%s\nwant\n%s", got.String(), want)
	}
}
