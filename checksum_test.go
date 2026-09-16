package pgpage

import (
	"encoding/binary"
	"fmt"
	"os"
	"testing"
)

// fixturePages returns every page of the fixture with the checksum
// page_header() reports for it. PostgreSQL 18 enables data checksums by
// default, and make-fixture.sh copies the file from disk, so the stored
// values are the ones PostgreSQL computed when it wrote each page.
func fixturePages(t *testing.T) map[BlockNumber][]byte {
	t.Helper()

	f, err := os.Open(fixtureHeap)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	pages := make(map[BlockNumber][]byte)

	for _, row := range readFixtureCSV(t, fixtureHeap+".page_header.csv") {
		block := BlockNumber(parseInt(t, row["blkno"], 32))

		page, err := ReadPage(f, block)
		if err != nil {
			t.Fatal(err)
		}

		pages[block] = page
	}

	return pages
}

// storedChecksum returns the pd_checksum of a page.
func storedChecksum(page []byte) uint16 {
	return binary.LittleEndian.Uint16(page[8:10])
}

// The checksum of every fixture page must be the one PostgreSQL stored.
func TestFixtureChecksums(t *testing.T) {
	for block, page := range fixturePages(t) {
		t.Run(fmt.Sprintf("block %d", block), func(t *testing.T) {
			if got, want := PageChecksum(page, block), storedChecksum(page); got != want {
				t.Errorf("PageChecksum = %d, PostgreSQL stored %d", got, want)
			}

			if status, _ := VerifyChecksum(page, block); status != ChecksumOK {
				t.Errorf("VerifyChecksum = %v, want %v", status, ChecksumOK)
			}
		})
	}
}

// The stored checksum is not part of the checksum: writing a different
// value in pd_checksum must not change the result.
func TestPageChecksumIgnoresStoredValue(t *testing.T) {
	page := fixturePages(t)[0]
	want := PageChecksum(page, 0)

	for _, stored := range []uint16{0, 1, 0xFFFF, want + 1} {
		binary.LittleEndian.PutUint16(page[8:10], stored)

		if got := PageChecksum(page, 0); got != want {
			t.Errorf("with pd_checksum %d: PageChecksum = %d, want %d", stored, got, want)
		}
	}
}

// The block number is mixed in, so the same bytes at another block have
// another checksum: a page written in the wrong place is detected.
func TestPageChecksumDependsOnBlock(t *testing.T) {
	page := fixturePages(t)[0]
	stored := storedChecksum(page)

	for _, block := range []BlockNumber{1, 2, 1 << 20, MaxBlockNumber} {
		if got := PageChecksum(page, block); got == stored {
			t.Errorf("as block %d: PageChecksum = %d, the same as at block 0", block, got)
		}
	}

	if status, _ := VerifyChecksum(page, 1); status != ChecksumMismatch {
		t.Errorf("VerifyChecksum as block 1 = %v, want %v", status, ChecksumMismatch)
	}
}

// Any byte of the page counts: a change anywhere, in the header, a line
// pointer, free space or a tuple, changes the checksum.
func TestPageChecksumDetectsChanges(t *testing.T) {
	page := fixturePages(t)[0]
	want := storedChecksum(page)

	for _, offset := range []int{0, 10, 23, 24, 1000, 2472, 8191} {
		damaged := append([]byte(nil), page...)
		damaged[offset] ^= 0x01

		if got := PageChecksum(damaged, 0); got == want {
			t.Errorf("flipping a bit at byte %d left the checksum at %d", offset, got)
		}

		status, computed := VerifyChecksum(damaged, 0)
		if status != ChecksumMismatch || computed == want {
			t.Errorf("VerifyChecksum after flipping byte %d = %v, %d; want %v and a different checksum",
				offset, status, computed, ChecksumMismatch)
		}
	}
}

// A stored 0 is not a mismatch: PostgreSQL never stores 0 with checksums
// on, so it means the page was written with them off.
func TestVerifyChecksumDisabled(t *testing.T) {
	page := fixturePages(t)[0]
	binary.LittleEndian.PutUint16(page[8:10], 0)

	status, computed := VerifyChecksum(page, 0)

	if status != ChecksumDisabled {
		t.Errorf("VerifyChecksum = %v, want %v", status, ChecksumDisabled)
	}

	if computed == 0 {
		t.Error("computed checksum is 0, which PostgreSQL never produces")
	}
}

func TestChecksumStatusString(t *testing.T) {
	tests := []struct {
		status ChecksumStatus
		want   string
	}{
		{status: ChecksumUnknown, want: "UNKNOWN"},
		{status: ChecksumOK, want: "OK"},
		{status: ChecksumMismatch, want: "MISMATCH"},
		{status: ChecksumDisabled, want: "DISABLED"},
		{status: ChecksumDisabled + 1, want: "ChecksumStatus(4)"},
	}

	for _, tt := range tests {
		if got := fmt.Sprint(tt.status); got != tt.want {
			t.Errorf("fmt.Sprint(ChecksumStatus(%d)) = %q, want %q", uint8(tt.status), got, tt.want)
		}
	}
}

// The zero value must not claim that a checksum was verified.
func TestChecksumStatusZeroValue(t *testing.T) {
	var s ChecksumStatus
	if s != ChecksumUnknown {
		t.Errorf("zero ChecksumStatus = %v, want %v", s, ChecksumUnknown)
	}
}

// A page shorter than PageSize has no checksum to compute.
func TestPageChecksumShortPage(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("PageChecksum of a short page did not panic")
		}
	}()

	PageChecksum(make([]byte, PageSize-1), 0)
}

// The 32 partial sums live on the stack: verifying every page of a
// relation must not allocate.
func TestPageChecksumDoesNotAllocate(t *testing.T) {
	page := makePage(devPage)

	allocs := testing.AllocsPerRun(100, func() {
		PageChecksum(page, 0)
	})
	if allocs != 0 {
		t.Errorf("PageChecksum allocated %v times per call, want 0", allocs)
	}
}

// Whatever the bytes, the checksum is deterministic and never 0.
func FuzzPageChecksum(f *testing.F) {
	f.Add(make([]byte, PageSize), uint32(0))
	f.Add(makePage(devPage), uint32(7))

	f.Fuzz(func(t *testing.T, data []byte, block uint32) {
		page := make([]byte, PageSize)
		copy(page, data)

		got := PageChecksum(page, BlockNumber(block))
		if got == 0 {
			t.Fatal("PageChecksum returned 0")
		}

		if again := PageChecksum(page, BlockNumber(block)); again != got {
			t.Fatalf("PageChecksum returned %d, then %d, for the same page", got, again)
		}
	})
}

func BenchmarkPageChecksum(b *testing.B) {
	page := makePage(devPage)

	b.SetBytes(PageSize)

	for b.Loop() {
		PageChecksum(page, 0)
	}
}
