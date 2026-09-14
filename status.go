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
