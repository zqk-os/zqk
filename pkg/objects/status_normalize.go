package objects

import (
	"sync"

	pkgctx "github.com/zqk-os/zqk/pkg/context"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/logging"
)

// statusAliases maps variant values to the preferred lifecycle status.
// Apply only when the preferred value is valid for the kind (see ApplyAliasesForStatus).
// Built-in: completed↔complete (kind-dependent), archive→archived, canceled→cancelled.
var (
	statusAliases = map[string]string{
		"completed": "complete",
		"archive":   "archived",
		"canceled":  "cancelled", // US spelling → preferred in lifecycles (e.g. priority_plan)
	}
	statusAliasesMu sync.RWMutex
)

// RegisterStatusAlias registers an alias so NormalizeStatus maps alias to preferred.
// Safe for concurrent use. Typically called at init or when loading config.
func RegisterStatusAlias(alias, preferred string) {
	if alias == emptyValue {
		return
	}
	_ = concurrency.RunInLockWithLogger(&statusAliasesMu, LockNameStatusNormalizeRegisterAlias, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		statusAliases[alias] = preferred
		return nil
	})
}

func lookupStatusAlias(status string) (string, bool) {
	if status == emptyValue {
		return emptyValue, false
	}
	var preferred string
	var ok bool
	_ = concurrency.RunInRLockWithLogger(&statusAliasesMu, LockNameStatusNormalizeLookup, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		preferred, ok = statusAliases[status]
		return nil
	})
	return preferred, ok
}

// ApplyAliasesForStatus returns the preferred status when status is a registered
// alias and preferred is in validTargets (kind's lifecycle statuses). Use this for
// kind-aware normalization so "completed" maps to "complete" only for kinds that
// have "complete", and is left as "completed" for kinds that use "completed".
func ApplyAliasesForStatus(status string, validTargets map[string]bool) string {
	if preferred, ok := lookupStatusAlias(status); ok && validTargets[preferred] {
		return preferred
	}
	return status
}

// NormalizeStatus maps common status variants to the preferred value without
// kind context. Prefer NormalizeStatusForKind (or ApplyAliasesForStatus with
// the kind's status set) when the kind is known, so "completed" is not forced
// to "complete" for kinds that use "completed" (e.g. audit_event).
func NormalizeStatus(status string) string {
	if preferred, ok := lookupStatusAlias(status); ok {
		return preferred
	}
	return status
}
