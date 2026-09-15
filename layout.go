package pgpage

import "fmt"

// RegionKind identifies one of the areas a page header divides a page into.
type RegionKind uint8

// Regions in the order they appear in a page.
const (
	RegionHeader       RegionKind = iota // PageHeaderData
	RegionLinePointers                   // the line pointer array, up to pd_lower
	RegionFree                           // free space, from pd_lower to pd_upper
	RegionTuples                         // tuple data, from pd_upper to pd_special
	RegionSpecial                        // access-method data, from pd_special to the end
)

// String returns the region name as shown to users, for example "Free space".
func (k RegionKind) String() string {
	switch k {
	case RegionHeader:
		return "Header"
	case RegionLinePointers:
		return "Line pointers"
	case RegionFree:
		return "Free space"
	case RegionTuples:
		return "Tuples"
	case RegionSpecial:
		return "Special"
	default:
		return fmt.Sprintf("RegionKind(%d)", uint8(k))
	}
}

// Region is a range of bytes of a page. Like a Go slice expression, it
// includes Start and excludes End, so adjacent regions share a boundary and
// an empty region has Start == End.
type Region struct {
	Kind  RegionKind
	Start int
	End   int
}

// Len returns the number of bytes in the region.
func (r Region) Len() int {
	return r.End - r.Start
}

// PageRegions divides a page into its five regions, in order, as described
// by its header. Regions can be empty, such as the special space of a heap
// page. Together they cover the page from byte 0 to PageSize.
//
// It returns nil for a new page, which has no layout yet, and for a header
// that ParsePageHeader would reject.
func PageRegions(h PageHeader) []Region {
	if h.IsNew() || h.validate() != nil {
		return nil
	}

	return []Region{
		{Kind: RegionHeader, Start: 0, End: PageHeaderSize},
		{Kind: RegionLinePointers, Start: PageHeaderSize, End: int(h.Lower)},
		{Kind: RegionFree, Start: int(h.Lower), End: int(h.Upper)},
		{Kind: RegionTuples, Start: int(h.Upper), End: int(h.Special)},
		{Kind: RegionSpecial, Start: int(h.Special), End: PageSize},
	}
}
