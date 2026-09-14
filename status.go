package pgpage

import (
	"errors"
	"fmt"
)

// PageStatus classifies a page by whether its header could be trusted.
type PageStatus uint8

const (
	// StatusUnknown means the page has not been examined. It is the zero
	// value, so a PageStatus nobody set never claims a page is fine.
	StatusUnknown PageStatus = iota
	// StatusOK means the page header is valid.
	StatusOK
	// StatusNew means the page is all zeroes and was never initialized.
	StatusNew
	// StatusInvalid means the page header is corrupt.
	StatusInvalid
)

// String returns the status as shown to users, for example "OK".
func (s PageStatus) String() string {
	switch s {
	case StatusUnknown:
		return "UNKNOWN"
	case StatusOK:
		return "OK"
	case StatusNew:
		return "NEW"
	case StatusInvalid:
		return "INVALID"
	default:
		return fmt.Sprintf("PageStatus(%d)", uint8(s))
	}
}

// PageStatusOf classifies the results of ParsePageHeader, so it can be
// called as PageStatusOf(ParsePageHeader(page)).
//
// An error that does not wrap ErrInvalidPageHeader says nothing about the
// page itself, such as a slice of the wrong length, and yields StatusUnknown.
func PageStatusOf(h PageHeader, err error) PageStatus {
	switch {
	case errors.Is(err, ErrInvalidPageHeader):
		return StatusInvalid
	case err != nil:
		return StatusUnknown
	case h.IsNew():
		return StatusNew
	default:
		return StatusOK
	}
}

// PageSummary is the overview of one page shown when listing pages.
type PageSummary struct {
	// Header is the decoded page header. It is the zero value unless
	// Status is StatusOK.
	Header PageHeader
	Status PageStatus
	// Err explains why the page could not be decoded, and is nil when
	// Status is StatusOK or StatusNew.
	Err error
}

// SummarizePage decodes the header of page and classifies it. It never
// fails: a page that cannot be decoded is reported through Status and Err.
func SummarizePage(page []byte) PageSummary {
	h, err := ParsePageHeader(page)

	return PageSummary{
		Header: h,
		Status: PageStatusOf(h, err),
		Err:    err,
	}
}

// FreeSpacePercent returns the free space between pd_lower and pd_upper as
// a percentage of the page size. ok is false unless Status is StatusOK,
// because only a valid header says where the free space is.
func (s PageSummary) FreeSpacePercent() (percent float64, ok bool) {
	if s.Status != StatusOK {
		return 0, false
	}

	return float64(s.Header.FreeSpace()) * 100 / PageSize, true
}
