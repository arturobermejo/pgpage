package pgpage

import (
	"errors"
	"fmt"
	"testing"
)

func TestPageStatusString(t *testing.T) {
	tests := []struct {
		status PageStatus
		want   string
	}{
		{status: StatusUnknown, want: "UNKNOWN"},
		{status: StatusOK, want: "OK"},
		{status: StatusNew, want: "NEW"},
		{status: StatusInvalid, want: "INVALID"},
		{status: StatusInvalid + 1, want: "PageStatus(4)"},
	}

	for _, tt := range tests {
		// fmt uses String through the fmt.Stringer interface.
		if got := fmt.Sprint(tt.status); got != tt.want {
			t.Errorf("fmt.Sprint(PageStatus(%d)) = %q, want %q", uint8(tt.status), got, tt.want)
		}
	}
}

// The zero value must not claim that a page is fine.
func TestPageStatusZeroValue(t *testing.T) {
	var s PageStatus
	if s != StatusUnknown {
		t.Errorf("zero PageStatus = %v, want %v", s, StatusUnknown)
	}
}

func TestPageStatusOf(t *testing.T) {
	corrupt := makePage(devPage)
	corrupt[12], corrupt[13] = 0xFF, 0xFF // pd_lower past pd_upper

	tests := []struct {
		name string
		page []byte
		want PageStatus
	}{
		{name: "valid page", page: makePage(devPage), want: StatusOK},
		{name: "all-zero page", page: make([]byte, PageSize), want: StatusNew},
		{name: "corrupt header", page: corrupt, want: StatusInvalid},
		{name: "wrong length", page: make([]byte, PageSize-1), want: StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PageStatusOf(ParsePageHeader(tt.page)); got != tt.want {
				t.Errorf("PageStatusOf(ParsePageHeader(page)) = %v, want %v", got, tt.want)
			}
		})
	}
}

// Classification must look through wrapping, as callers add context.
func TestPageStatusOfWrappedError(t *testing.T) {
	err := fmt.Errorf("block 7: %w", fmt.Errorf("pgpage: bad flags: %w", ErrInvalidPageHeader))
	if got := PageStatusOf(PageHeader{}, err); got != StatusInvalid {
		t.Errorf("PageStatusOf(wrapped ErrInvalidPageHeader) = %v, want %v", got, StatusInvalid)
	}

	if got := PageStatusOf(PageHeader{}, errors.New("something else")); got != StatusUnknown {
		t.Errorf("PageStatusOf(unrelated error) = %v, want %v", got, StatusUnknown)
	}
}

func TestSummarizePage(t *testing.T) {
	summary := SummarizePage(makePage(devPage), 0)

	if summary.Status != StatusOK || summary.Err != nil {
		t.Fatalf("SummarizePage = %v, %v; want %v, nil", summary.Status, summary.Err, StatusOK)
	}

	if summary.Header != devPage {
		t.Errorf("Header = %+v, want %+v", summary.Header, devPage)
	}

	// 8104 free bytes out of 8192. The page size is a power of two, so the
	// division is exact in floating point and can be compared with ==.
	percent, ok := summary.FreeSpacePercent()
	if !ok || percent != 98.92578125 {
		t.Errorf("FreeSpacePercent() = %v, %v; want 98.92578125, true", percent, ok)
	}

	// devPage stores a checksum that was not computed from these bytes.
	if summary.Checksum != ChecksumMismatch || summary.ComputedChecksum == devPage.Checksum {
		t.Errorf("Checksum = %v, computed %d; want %v and a checksum other than %d",
			summary.Checksum, summary.ComputedChecksum, ChecksumMismatch, devPage.Checksum)
	}
}

// A page PostgreSQL wrote verifies at its own block and nowhere else.
func TestSummarizePageChecksum(t *testing.T) {
	page := fixturePages(t)[1]

	if s := SummarizePage(page, 1); s.Checksum != ChecksumOK || s.ComputedChecksum != s.Header.Checksum {
		t.Errorf("at block 1: Checksum = %v, computed %d, stored %d; want OK", s.Checksum, s.ComputedChecksum, s.Header.Checksum)
	}

	if s := SummarizePage(page, 2); s.Checksum != ChecksumMismatch {
		t.Errorf("at block 2: Checksum = %v, want %v", s.Checksum, ChecksumMismatch)
	}
}

// Pages that cannot be decoded are still summarized, never an error.
func TestSummarizePageNotOK(t *testing.T) {
	corrupt := makePage(devPage)
	corrupt[12], corrupt[13] = 0xFF, 0xFF // pd_lower past pd_upper

	tests := []struct {
		name    string
		page    []byte
		status  PageStatus
		wantErr error
	}{
		{name: "all-zero page", page: make([]byte, PageSize), status: StatusNew},
		{name: "corrupt header", page: corrupt, status: StatusInvalid, wantErr: ErrInvalidPageHeader},
		{name: "wrong length", page: make([]byte, PageSize-1), status: StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := SummarizePage(tt.page, 0)

			if summary.Status != tt.status {
				t.Errorf("Status = %v, want %v", summary.Status, tt.status)
			}

			if tt.wantErr != nil && !errors.Is(summary.Err, tt.wantErr) {
				t.Errorf("errors.Is(Err, %v) = false, want true (Err = %v)", tt.wantErr, summary.Err)
			}

			if tt.status == StatusNew && summary.Err != nil {
				t.Errorf("Err = %v, want nil for a new page", summary.Err)
			}

			if tt.status == StatusUnknown && summary.Err == nil {
				t.Error("Err = nil, want the reason the page could not be decoded")
			}

			if percent, ok := summary.FreeSpacePercent(); ok {
				t.Errorf("FreeSpacePercent() = %v, true; want ok = false", percent)
			}

			// Only a page with a valid header has a checksum to verify.
			if summary.Checksum != ChecksumUnknown {
				t.Errorf("Checksum = %v, want %v", summary.Checksum, ChecksumUnknown)
			}
		})
	}
}

// A summary nobody filled in must not report free space.
func TestPageSummaryZeroValue(t *testing.T) {
	var summary PageSummary

	if summary.Status != StatusUnknown {
		t.Errorf("zero PageSummary Status = %v, want %v", summary.Status, StatusUnknown)
	}

	if _, ok := summary.FreeSpacePercent(); ok {
		t.Error("zero PageSummary FreeSpacePercent() ok = true, want false")
	}
}
