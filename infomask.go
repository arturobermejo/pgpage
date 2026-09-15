package pgpage

import "fmt"

// InfoMask holds the t_infomask flags of a heap tuple header.
type InfoMask uint16

// Flags of t_infomask, with the values and names used by PostgreSQL.
const (
	HeapHasNull        InfoMask = 0x0001 // has a null bitmap
	HeapHasVarWidth    InfoMask = 0x0002 // has variable-width attributes
	HeapHasExternal    InfoMask = 0x0004 // has attributes stored in TOAST
	HeapHasOIDOld      InfoMask = 0x0008 // has an OID field, before PostgreSQL 12
	HeapXmaxKeyShrLock InfoMask = 0x0010 // xmax is a FOR KEY SHARE locker
	HeapComboCID       InfoMask = 0x0020 // t_cid is a combo command ID
	HeapXmaxExclLock   InfoMask = 0x0040 // xmax is an exclusive locker
	HeapXmaxLockOnly   InfoMask = 0x0080 // xmax only locked the tuple, it did not update or delete it
	HeapXminCommitted  InfoMask = 0x0100 // hint: xmin is known committed
	HeapXminInvalid    InfoMask = 0x0200 // hint: xmin is known aborted
	HeapXmaxCommitted  InfoMask = 0x0400 // hint: xmax is known committed
	HeapXmaxInvalid    InfoMask = 0x0800 // hint: xmax is known aborted or unset
	HeapXmaxIsMulti    InfoMask = 0x1000 // xmax is a MultiXactId
	HeapUpdated        InfoMask = 0x2000 // this tuple is the new version from an UPDATE
	HeapMovedOff       InfoMask = 0x4000 // moved away by pre-9.0 VACUUM FULL
	HeapMovedIn        InfoMask = 0x8000 // moved here by pre-9.0 VACUUM FULL

	// HeapXmaxShrLock is a FOR SHARE locker, recorded as both lock bits.
	HeapXmaxShrLock = HeapXmaxExclLock | HeapXmaxKeyShrLock
	// HeapXminFrozen marks xmin as frozen, visible to every transaction.
	HeapXminFrozen = HeapXminCommitted | HeapXminInvalid
	// HeapMoved means t_field3 holds t_xvac instead of t_cid.
	HeapMoved = HeapMovedOff | HeapMovedIn
)

// infoMaskNames holds the PostgreSQL name of each t_infomask bit.
var infoMaskNames = [16]string{
	"HEAP_HASNULL", "HEAP_HASVARWIDTH", "HEAP_HASEXTERNAL", "HEAP_HASOID_OLD",
	"HEAP_XMAX_KEYSHR_LOCK", "HEAP_COMBOCID", "HEAP_XMAX_EXCL_LOCK", "HEAP_XMAX_LOCK_ONLY",
	"HEAP_XMIN_COMMITTED", "HEAP_XMIN_INVALID", "HEAP_XMAX_COMMITTED", "HEAP_XMAX_INVALID",
	"HEAP_XMAX_IS_MULTI", "HEAP_UPDATED", "HEAP_MOVED_OFF", "HEAP_MOVED_IN",
}

// Has reports whether every bit of flags is set, so it also works with
// combinations such as HeapXminFrozen.
func (m InfoMask) Has(flags InfoMask) bool {
	return m&flags == flags
}

// Names returns the PostgreSQL names of the bits set in m, from the lowest
// bit, as heap_tuple_infomask_flags() lists them in raw_flags.
func (m InfoMask) Names() []string {
	var names []string

	for bit, name := range infoMaskNames {
		if m&(1<<bit) != 0 {
			names = append(names, name)
		}
	}

	return names
}

// InfoMask2 holds t_infomask2: the number of attributes in its low 11 bits
// and flags in the rest.
type InfoMask2 uint16

// Flags of t_infomask2, with the values used by PostgreSQL.
const (
	// HeapKeysUpdated means the tuple was deleted, updated with a change to
	// its key columns, or locked FOR UPDATE.
	HeapKeysUpdated InfoMask2 = 0x2000
	// HeapHotUpdated means the tuple was updated and its new version is a
	// heap-only tuple on the same page.
	HeapHotUpdated InfoMask2 = 0x4000
	// HeapOnlyTuple marks a heap-only tuple, which no index entry points to.
	HeapOnlyTuple InfoMask2 = 0x8000
)

// heapNattsMask selects the number of attributes from t_infomask2.
const heapNattsMask InfoMask2 = 0x07FF

// infoMask2Names holds the PostgreSQL name of each t_infomask2 flag bit;
// bits 11 and 12 are unused.
var infoMask2Names = [16]string{
	13: "HEAP_KEYS_UPDATED",
	14: "HEAP_HOT_UPDATED",
	15: "HEAP_ONLY_TUPLE",
}

// Natts returns the number of attributes.
func (m InfoMask2) Natts() int {
	return int(m & heapNattsMask)
}

// Has reports whether every bit of flags is set.
func (m InfoMask2) Has(flags InfoMask2) bool {
	return m&flags == flags
}

// Names returns the PostgreSQL names of the flags set in m, from the lowest
// bit, ignoring the attribute count. Unused bits that are set, which
// PostgreSQL never writes, are named by their value, such as "0x0800", so
// they are not hidden.
func (m InfoMask2) Names() []string {
	var names []string

	for bit := 11; bit < 16; bit++ {
		flag := InfoMask2(1) << bit
		if m&flag == 0 {
			continue
		}

		if name := infoMask2Names[bit]; name != "" {
			names = append(names, name)
		} else {
			names = append(names, fmt.Sprintf("%#04x", uint16(flag)))
		}
	}

	return names
}
