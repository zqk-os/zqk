package authcred

import (
	"fmt"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// PrototypeAccountError is the sentinel error for prototype/test account rejection.
var PrototypeAccountError = &prototypeAccountError{}

type prototypeAccountError struct{}

func (*prototypeAccountError) Error() string { return "prototype/test account" }
func (e *prototypeAccountError) Is(err error) bool {
	_, ok := err.(*prototypeAccountError)
	return ok
}

// accountRefPrefix is the canonical account ID prefix (compared lowercased).
const accountRefPrefix = "acc-"

// prototypeAccountIDMarkers are substrings in an ACC-* suffix that mark a prototype/test account.
var prototypeAccountIDMarkers = []string{"proto", "test", "mock", "dev", "sbox", "pvt"}

// defaultPrototypeAccountRefs are the fixture accounts shipped by the bootstrap seed whose IDs
// carry no "proto"/"test" marker, so the heuristic below cannot catch them. They are a
// *replaceable default*, not a fixed truth: deployments override or extend the set with
// [RegisterPrototypeAccountRefs] or the PROTOTYPE_ACCOUNT_REFS env list.
//
// TRACK: the durable fix is resolving the ref against the
// account index (a prototype or ghost account is one that does not resolve to a real, non-fixture
// account object) instead of matching IDs the kernel has memorized.
var defaultPrototypeAccountRefs = []string{
	"ACC-1785920548450214015-3df55bd1", // seeded fixture: test-user
	"ACC-1785920548450214016-ace2aae1", // seeded fixture: test-agent
}

var (
	prototypeAccountRefsMu   sync.RWMutex
	prototypeAccountRefs     map[string]struct{} // lowercased refs
	prototypeAccountRefsOnce sync.Once
)

// loadPrototypeAccountRefs seeds the registry from the defaults plus the PROTOTYPE_ACCOUNT_REFS
// env list (comma-separated) on first use.
func loadPrototypeAccountRefs() {
	prototypeAccountRefsOnce.Do(func() {
		refs := defaultPrototypeAccountRefs
		if extra := strings.TrimSpace(zqkenv.PrototypeAccountRefs().Get()); extra != "" {
			refs = append(append([]string{}, refs...), strings.Split(extra, ",")...)
		}
		storePrototypeAccountRefs(refs, false)
	})
}

// storePrototypeAccountRefs writes refs into the registry, replacing the current set when reset.
func storePrototypeAccountRefs(refs []string, reset bool) {
	prototypeAccountRefsMu.Lock()
	defer prototypeAccountRefsMu.Unlock()
	if reset || prototypeAccountRefs == nil {
		prototypeAccountRefs = make(map[string]struct{}, len(refs))
	}
	for _, ref := range refs {
		if normalized := strings.ToLower(strings.TrimSpace(ref)); normalized != "" {
			prototypeAccountRefs[normalized] = struct{}{}
		}
	}
}

// RegisterPrototypeAccountRefs adds account refs that must be rejected in production. Use this
// (or the PROTOTYPE_ACCOUNT_REFS env list) instead of editing the defaults so fixture account IDs
// live with whoever seeds them.
func RegisterPrototypeAccountRefs(refs ...string) {
	loadPrototypeAccountRefs()
	storePrototypeAccountRefs(refs, false)
}

// ResetPrototypeAccountRefs replaces the registry with exactly refs (empty clears it).
func ResetPrototypeAccountRefs(refs ...string) {
	loadPrototypeAccountRefs()
	storePrototypeAccountRefs(refs, true)
}

// isRegisteredPrototypeRef reports whether lowerRef is a registered prototype account ref.
func isRegisteredPrototypeRef(lowerRef string) bool {
	loadPrototypeAccountRefs()
	prototypeAccountRefsMu.RLock()
	defer prototypeAccountRefsMu.RUnlock()
	_, ok := prototypeAccountRefs[lowerRef]
	return ok
}

// IsPrototypeAccount returns true if the given owner_ref is a prototype/test account.
func IsPrototypeAccount(ref string) bool {
	lowerRef := strings.ToLower(strings.TrimSpace(ref))
	if lowerRef == "" {
		return false
	}
	if isRegisteredPrototypeRef(lowerRef) {
		return true
	}
	suffix, isAccount := strings.CutPrefix(lowerRef, accountRefPrefix)
	if !isAccount {
		return false
	}
	for _, marker := range prototypeAccountIDMarkers {
		if strings.Contains(suffix, marker) {
			return true
		}
	}
	return false
}

// ValidateOwnerRefProduction validates that owner_ref is a valid account ID and not a prototype/test account.
func ValidateOwnerRefProduction(owner_ref string) error {
	raw := strings.TrimSpace(owner_ref)
	if raw == "" {
		return nil // empty is tolerated
	}

	lower := strings.ToLower(raw)
	if !strings.HasPrefix(lower, accountRefPrefix) {
		return fmt.Errorf("owner_ref %q is not a valid account ID (must start with ACC-)", raw)
	}

	if IsPrototypeAccount(raw) {
		return fmt.Errorf("owner_ref %q is a prototype/test account and is not allowed in production", raw)
	}
	return nil
}
