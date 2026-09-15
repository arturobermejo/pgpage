package pgpage

import (
	"slices"
	"testing"
)

// The flag values are part of the on-disk format and must match
// htup_details.h. Names were taken from heap_tuple_infomask_flags() in
// PostgreSQL 18, one bit at a time.
func TestInfoMaskFlags(t *testing.T) {
	tests := []struct {
		flag  InfoMask
		value uint16
		name  string
	}{
		{flag: HeapHasNull, value: 0x0001, name: "HEAP_HASNULL"},
		{flag: HeapHasVarWidth, value: 0x0002, name: "HEAP_HASVARWIDTH"},
		{flag: HeapHasExternal, value: 0x0004, name: "HEAP_HASEXTERNAL"},
		{flag: HeapHasOIDOld, value: 0x0008, name: "HEAP_HASOID_OLD"},
		{flag: HeapXmaxKeyShrLock, value: 0x0010, name: "HEAP_XMAX_KEYSHR_LOCK"},
		{flag: HeapComboCID, value: 0x0020, name: "HEAP_COMBOCID"},
		{flag: HeapXmaxExclLock, value: 0x0040, name: "HEAP_XMAX_EXCL_LOCK"},
		{flag: HeapXmaxLockOnly, value: 0x0080, name: "HEAP_XMAX_LOCK_ONLY"},
		{flag: HeapXminCommitted, value: 0x0100, name: "HEAP_XMIN_COMMITTED"},
		{flag: HeapXminInvalid, value: 0x0200, name: "HEAP_XMIN_INVALID"},
		{flag: HeapXmaxCommitted, value: 0x0400, name: "HEAP_XMAX_COMMITTED"},
		{flag: HeapXmaxInvalid, value: 0x0800, name: "HEAP_XMAX_INVALID"},
		{flag: HeapXmaxIsMulti, value: 0x1000, name: "HEAP_XMAX_IS_MULTI"},
		{flag: HeapUpdated, value: 0x2000, name: "HEAP_UPDATED"},
		{flag: HeapMovedOff, value: 0x4000, name: "HEAP_MOVED_OFF"},
		{flag: HeapMovedIn, value: 0x8000, name: "HEAP_MOVED_IN"},
	}

	for _, tt := range tests {
		if uint16(tt.flag) != tt.value {
			t.Errorf("%s = %#04x, want %#04x", tt.name, uint16(tt.flag), tt.value)
		}

		if got := tt.flag.Names(); !slices.Equal(got, []string{tt.name}) {
			t.Errorf("InfoMask(%#04x).Names() = %q, want [%q]", tt.value, got, tt.name)
		}
	}
}

func TestInfoMask2Flags(t *testing.T) {
	tests := []struct {
		flag  InfoMask2
		value uint16
		name  string
	}{
		{flag: HeapKeysUpdated, value: 0x2000, name: "HEAP_KEYS_UPDATED"},
		{flag: HeapHotUpdated, value: 0x4000, name: "HEAP_HOT_UPDATED"},
		{flag: HeapOnlyTuple, value: 0x8000, name: "HEAP_ONLY_TUPLE"},
	}

	for _, tt := range tests {
		if uint16(tt.flag) != tt.value {
			t.Errorf("%s = %#04x, want %#04x", tt.name, uint16(tt.flag), tt.value)
		}

		if got := tt.flag.Names(); !slices.Equal(got, []string{tt.name}) {
			t.Errorf("InfoMask2(%#04x).Names() = %q, want [%q]", tt.value, got, tt.name)
		}
	}
}

func TestInfoMask2Names(t *testing.T) {
	tests := []struct {
		name string
		mask InfoMask2
		want []string
	}{
		{name: "attribute count only", mask: 0x07FF, want: nil},
		{name: "heap-only tuple with 2 attributes", mask: 0x8002, want: []string{"HEAP_ONLY_TUPLE"}},
		{name: "all named flags", mask: 0xE000, want: []string{"HEAP_KEYS_UPDATED", "HEAP_HOT_UPDATED", "HEAP_ONLY_TUPLE"}},
		{name: "unused bits", mask: 0x1801, want: []string{"0x0800", "0x1000"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mask.Names(); !slices.Equal(got, tt.want) {
				t.Errorf("InfoMask2(%#04x).Names() = %q, want %q", uint16(tt.mask), got, tt.want)
			}
		})
	}
}

// Names of both masks together must list flags as heap_tuple_infomask_flags()
// does in raw_flags. The expected values were produced by PostgreSQL 18.
func TestInfoMaskNamesMatchPostgreSQL(t *testing.T) {
	tests := []struct {
		name      string
		infomask  InfoMask
		infomask2 InfoMask2
		want      []string
	}{
		{
			name:     "fixture tuple",
			infomask: 2306, infomask2: 2,
			want: []string{"HEAP_HASVARWIDTH", "HEAP_XMIN_COMMITTED", "HEAP_XMAX_INVALID"},
		},
		{
			name:     "fixture heap-only tuple",
			infomask: 10498, infomask2: 32770,
			want: []string{"HEAP_HASVARWIDTH", "HEAP_XMIN_COMMITTED", "HEAP_XMAX_INVALID", "HEAP_UPDATED", "HEAP_ONLY_TUPLE"},
		},
		{
			name:     "tuple locked FOR UPDATE",
			infomask: 0x21C2, infomask2: 0xA002,
			want: []string{
				"HEAP_HASVARWIDTH", "HEAP_XMAX_EXCL_LOCK", "HEAP_XMAX_LOCK_ONLY", "HEAP_XMIN_COMMITTED",
				"HEAP_UPDATED", "HEAP_KEYS_UPDATED", "HEAP_ONLY_TUPLE",
			},
		},
		{name: "no flags", infomask: 0, infomask2: 0, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := append(tt.infomask.Names(), tt.infomask2.Names()...)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Names() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInfoMaskHas(t *testing.T) {
	tests := []struct {
		name  string
		mask  InfoMask
		flags InfoMask
		want  bool
	}{
		{name: "single flag set", mask: 0x0902, flags: HeapXminCommitted, want: true},
		{name: "single flag unset", mask: 0x0902, flags: HeapXmaxCommitted, want: false},
		{name: "frozen", mask: HeapXminCommitted | HeapXminInvalid, flags: HeapXminFrozen, want: true},
		{name: "committed is not frozen", mask: HeapXminCommitted, flags: HeapXminFrozen, want: false},
		{name: "share lock needs both bits", mask: HeapXmaxExclLock, flags: HeapXmaxShrLock, want: false},
		{name: "no flags asked", mask: 0, flags: 0, want: true},
	}

	for _, tt := range tests {
		if got := tt.mask.Has(tt.flags); got != tt.want {
			t.Errorf("%s: InfoMask(%#04x).Has(%#04x) = %v, want %v", tt.name, uint16(tt.mask), uint16(tt.flags), got, tt.want)
		}
	}
}

// In the fixture every live tuple has its hint bits set, and the targets of
// REDIRECT line pointers are exactly the heap-only tuples.
func TestFixtureInfoMasks(t *testing.T) {
	rel := openRelation(t, fixtureHeap)

	for block := range rel.PageCount() {
		page, err := rel.ReadPage(block)
		if err != nil {
			t.Fatalf("ReadPage(%d) returned error: %v", block, err)
		}

		h, err := ParsePageHeader(page)
		if err != nil {
			t.Fatalf("block %d: ParsePageHeader returned error: %v", block, err)
		}

		items, err := ParseItemIDs(page, h, nil)
		if err != nil {
			t.Fatalf("block %d: ParseItemIDs returned error: %v", block, err)
		}

		redirectTargets := make(map[OffsetNumber]bool)

		for _, id := range items {
			if id.State() == ItemRedirect {
				redirectTargets[OffsetNumber(id.Offset())] = true
			}
		}

		for i, id := range items {
			if id.State() != ItemNormal {
				continue
			}

			n := OffsetNumber(i + 1)

			tuple, err := ParseHeapTupleHeader(page[id.Offset() : id.Offset()+id.Length()])
			if err != nil {
				t.Fatalf("block %d item %d: ParseHeapTupleHeader returned error: %v", block, n, err)
			}

			if !tuple.Infomask.Has(HeapXminCommitted | HeapXmaxInvalid) {
				t.Errorf("block %d item %d: flags %q, want HEAP_XMIN_COMMITTED and HEAP_XMAX_INVALID", block, n, tuple.Infomask.Names())
			}

			if heapOnly := tuple.Infomask2.Has(HeapOnlyTuple); heapOnly != redirectTargets[n] {
				t.Errorf("block %d item %d: heap-only = %v, REDIRECT target = %v", block, n, heapOnly, redirectTargets[n])
			}
		}
	}
}
