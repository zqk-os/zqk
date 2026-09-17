// Package nildecode provides generic helpers for type assertions with non-zero checks.
// Kept separate from pkg/pipeline so low-level packages (e.g. pkg/objects) can use them without
// importing pipeline (the pipeline package depends on pkg/objects in other files).
package nildecode

// DecodeNonNilPayload asserts payload has dynamic type P and is not P's zero value. For pointer
// payloads P (e.g. *FooPayload), the zero value is nil. P must be [comparable]; function and slice
// types cannot be used (use a manual assert for those).
func DecodeNonNilPayload[P comparable](payload any) (P, bool) {
	var z P
	p, ok := payload.(P)
	if !ok {
		return z, false
	}
	if p == z {
		return z, false
	}
	return p, true
}
