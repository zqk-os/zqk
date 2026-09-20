package storage

import (
	"github.com/zqk-os/zqk/pkg/objects"
)

// canonicalizePersistedLifecycleStatus maps aliases onto the kind's lifecycle
// and refuses unknown values. Persist must not write statuses the lifecycle
// does not name (e.g. technical_debt "accepted").
// TRACK: BLI-KERNEL-UNPAIRED-DELETE-INBOUND-001 — fail-closed unknown lifecycle persist.
func canonicalizePersistedLifecycleStatus(loader *objects.LifecycleLoader, kind, status string) (string, error) {
	if status == emptyValue {
		return status, nil
	}
	if loader == nil {
		return objects.NormalizeStatus(status), nil
	}
	return loader.NormalizeStatusForKind(kind, status)
}
