package featureflags

import "sync/atomic"

// active is the process store built at gateway boot. Request paths read it
// so a Settings toggle changes behavior on the next call without a restart.
var active atomic.Pointer[Store]

// SetActive publishes the store used by ActiveEnabled. Passing nil clears it
// (tests and processes that have not finished boot).
func SetActive(store *Store) {
	active.Store(store)
}

// Active returns the store set at boot, or nil.
func Active() *Store {
	return active.Load()
}

// ActiveEnabled reports the live value of a flag. Before the store exists,
// the code default is used so features that ship on stay on during early boot.
// Unknown ids are off.
func ActiveEnabled(id string) bool {
	if store := active.Load(); store != nil {
		return store.IsEnabled(id)
	}
	if def, ok := LookupDef(id); ok {
		return def.Default
	}
	return false
}
