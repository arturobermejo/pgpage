package pgpage

import (
	"fmt"
	"slices"
	"testing"
)

func TestPageRegions(t *testing.T) {
	tests := []struct {
		name string
		h    PageHeader
		want []Region
	}{
		{
			// The page in the main screen mockup: 12 items and 7640 free bytes.
			name: "mockup page",
			h:    PageHeader{Lower: 72, Upper: 7712, Special: 8192, PageSize: PageSize, LayoutVersion: PageLayoutVersion},
			want: []Region{
				{Kind: RegionHeader, Start: 0, End: 24},
				{Kind: RegionLinePointers, Start: 24, End: 72},
				{Kind: RegionFree, Start: 72, End: 7712},
				{Kind: RegionTuples, Start: 7712, End: 8192},
				{Kind: RegionSpecial, Start: 8192, End: 8192},
			},
		},
		{
			name: "index-like page with special space",
			h:    PageHeader{Lower: 40, Upper: 8000, Special: 8176, PageSize: PageSize, LayoutVersion: PageLayoutVersion},
			want: []Region{
				{Kind: RegionHeader, Start: 0, End: 24},
				{Kind: RegionLinePointers, Start: 24, End: 40},
				{Kind: RegionFree, Start: 40, End: 8000},
				{Kind: RegionTuples, Start: 8000, End: 8176},
				{Kind: RegionSpecial, Start: 8176, End: 8192},
			},
		},
		{
			name: "empty page",
			h:    PageHeader{Lower: 24, Upper: 8192, Special: 8192, PageSize: PageSize, LayoutVersion: PageLayoutVersion},
			want: []Region{
				{Kind: RegionHeader, Start: 0, End: 24},
				{Kind: RegionLinePointers, Start: 24, End: 24},
				{Kind: RegionFree, Start: 24, End: 8192},
				{Kind: RegionTuples, Start: 8192, End: 8192},
				{Kind: RegionSpecial, Start: 8192, End: 8192},
			},
		},
		{
			name: "full page",
			h:    PageHeader{Lower: 4096, Upper: 4096, Special: 8192, PageSize: PageSize, LayoutVersion: PageLayoutVersion},
			want: []Region{
				{Kind: RegionHeader, Start: 0, End: 24},
				{Kind: RegionLinePointers, Start: 24, End: 4096},
				{Kind: RegionFree, Start: 4096, End: 4096},
				{Kind: RegionTuples, Start: 4096, End: 8192},
				{Kind: RegionSpecial, Start: 8192, End: 8192},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PageRegions(tt.h); !slices.Equal(got, tt.want) {
				t.Errorf("PageRegions =\n  %v\nwant\n  %v", got, tt.want)
			}
		})
	}
}

// Pages without a trustworthy layout have no regions.
func TestPageRegionsNoLayout(t *testing.T) {
	tests := []struct {
		name string
		h    PageHeader
	}{
		{name: "new page", h: PageHeader{}},
		{name: "lower past upper", h: PageHeader{Lower: 500, Upper: 100, Special: 8192, PageSize: PageSize, LayoutVersion: PageLayoutVersion}},
		{name: "wrong page size", h: PageHeader{Lower: 72, Upper: 7712, Special: 8192, PageSize: 4096, LayoutVersion: PageLayoutVersion}},
	}

	for _, tt := range tests {
		if got := PageRegions(tt.h); got != nil {
			t.Errorf("%s: PageRegions = %v, want nil", tt.name, got)
		}
	}
}

func TestRegionKindString(t *testing.T) {
	tests := []struct {
		kind RegionKind
		want string
	}{
		{kind: RegionHeader, want: "Header"},
		{kind: RegionLinePointers, want: "Line pointers"},
		{kind: RegionFree, want: "Free space"},
		{kind: RegionTuples, want: "Tuples"},
		{kind: RegionSpecial, want: "Special"},
		{kind: RegionSpecial + 1, want: "RegionKind(5)"},
	}

	for _, tt := range tests {
		if got := fmt.Sprint(tt.kind); got != tt.want {
			t.Errorf("fmt.Sprint(RegionKind(%d)) = %q, want %q", uint8(tt.kind), got, tt.want)
		}
	}
}

// checkRegions reports whether regions tile the page: five regions in order,
// each starting where the previous one ended, from 0 to PageSize.
func checkRegions(regions []Region) error {
	if len(regions) != 5 {
		return fmt.Errorf("%d regions, want 5", len(regions))
	}

	end := 0

	for i, r := range regions {
		if r.Kind != RegionKind(i) {
			return fmt.Errorf("region %d is %v, want %v", i, r.Kind, RegionKind(i))
		}

		if r.Start != end || r.Len() < 0 {
			return fmt.Errorf("%v spans %d-%d after a region ending at %d", r.Kind, r.Start, r.End, end)
		}

		end = r.End
	}

	if end != PageSize {
		return fmt.Errorf("regions end at %d, want %d", end, PageSize)
	}

	return nil
}

// The regions of every fixture page tile the page, and their sizes match the
// header: line pointers hold ItemCount entries and free space is FreeSpace.
func TestFixturePageRegions(t *testing.T) {
	rel := openRelation(t, fixtureHeap)

	for block := range rel.PageCount() {
		page, err := rel.ReadPage(block)
		if err != nil {
			t.Fatal(err)
		}

		h, err := ParsePageHeader(page)
		if err != nil {
			t.Fatal(err)
		}

		regions := PageRegions(h)
		if err := checkRegions(regions); err != nil {
			t.Fatalf("block %d: %v", block, err)
		}

		if got, want := regions[RegionLinePointers].Len(), h.ItemCount()*itemIDSize; got != want {
			t.Errorf("block %d: line pointers are %d bytes, want %d", block, got, want)
		}

		if got, want := regions[RegionFree].Len(), h.FreeSpace(); got != want {
			t.Errorf("block %d: free space is %d bytes, want %d", block, got, want)
		}
	}
}

// Any header ParsePageHeader accepts yields regions that tile the page.
func FuzzPageRegions(f *testing.F) {
	f.Add(uint16(72), uint16(7712), uint16(8192))
	f.Add(uint16(24), uint16(24), uint16(8176))

	f.Fuzz(func(t *testing.T, lower, upper, special uint16) {
		h := PageHeader{Lower: lower, Upper: upper, Special: special, PageSize: PageSize, LayoutVersion: PageLayoutVersion}

		regions := PageRegions(h)
		if _, err := ParsePageHeader(makePage(h)); err != nil {
			if regions != nil {
				t.Fatalf("PageRegions(%+v) = %v for a header ParsePageHeader rejects", h, regions)
			}

			return
		}

		if h.IsNew() {
			return
		}

		if err := checkRegions(regions); err != nil {
			t.Fatalf("PageRegions(%+v): %v", h, err)
		}
	})
}
