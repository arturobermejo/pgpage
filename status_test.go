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
