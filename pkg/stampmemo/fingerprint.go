package stampmemo

import "sync"

// Fingerprints skips a side effect when the payload fingerprint is unchanged.
type Fingerprints struct {
	m sync.Map
}

// Unchanged is true when key was Remember'd with the same fingerprint.
func (f *Fingerprints) Unchanged(key, fingerprint string) bool {
	prev, ok := f.m.Load(key)
	return ok && prev.(string) == fingerprint
}

// Remember records fingerprint after a successful side effect.
func (f *Fingerprints) Remember(key, fingerprint string) {
	f.m.Store(key, fingerprint)
}
