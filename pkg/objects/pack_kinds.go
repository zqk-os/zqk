package objects

// packOwnedKinds is the kind list a linked pack owns, in the order the pack gave.
// The composition root sets this before the object command tree is built.
var packOwnedKinds []string

// SetPackOwnedKinds records the kinds a linked pack owns.
// A nil or empty list clears the set.
func SetPackOwnedKinds(kinds []string) {
	if len(kinds) == 0 {
		packOwnedKinds = nil
		return
	}
	out := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		if kind == "" {
			continue
		}
		out = append(out, kind)
	}
	packOwnedKinds = out
}

// PackOwnedKinds returns the kinds recorded by SetPackOwnedKinds, in that order.
func PackOwnedKinds() []string {
	if len(packOwnedKinds) == 0 {
		return nil
	}
	out := make([]string, len(packOwnedKinds))
	copy(out, packOwnedKinds)
	return out
}

// WithoutKinds returns kinds that are not in owned, preserving order.
func WithoutKinds(kinds, owned []string) []string {
	if len(owned) == 0 {
		return kinds
	}
	drop := make(map[string]struct{}, len(owned))
	for _, kind := range owned {
		drop[kind] = struct{}{}
	}
	out := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		if _, ok := drop[kind]; ok {
			continue
		}
		out = append(out, kind)
	}
	return out
}
