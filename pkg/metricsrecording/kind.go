package metricsrecording

import "sync"

var deniedKinds sync.Map // string (object/kind) -> struct{}

const emptyKind = ""

// DenyKind excludes recording for metric events / metric objects of this kind when Enabled()
// is true. Used from tests that want most metrics on but need to silence one kind.
func DenyKind(kind string) {
	if kind == emptyKind {
		return
	}
	deniedKinds.Store(kind, struct{}{})
}

// AllowKind removes a kind previously passed to DenyKind.
func AllowKind(kind string) {
	deniedKinds.Delete(kind)
}

// ClearDeniedKinds removes all kind denials (e.g. in TestMain cleanup).
func ClearDeniedKinds() {
	deniedKinds.Range(func(key, _ any) bool {
		deniedKinds.Delete(key)
		return true
	})
}

// EnabledForKind returns true if recording is allowed for this object/kind. When kind is
// empty, only the global Enabled() gate applies.
func EnabledForKind(kind string) bool {
	if !Enabled() {
		return false
	}
	if kind == emptyKind {
		return true
	}
	_, denied := deniedKinds.Load(kind)
	return !denied
}
