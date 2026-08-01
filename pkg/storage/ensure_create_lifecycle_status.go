package storage

import (
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// ensureCreateLifecycleStatus sets status on create when missing or invalid for the kind.
// Uses the lifecycle origin (e.g. agent_task → proposed, backlog_item → exploring).
// Valid non-origin statuses are left alone so intentional create-with-status still works.
// Invalid create statuses (e.g. pending) are coerced to the lifecycle origin so creates succeed;
// prefer passing the origin status explicitly at call sites when practical.
func ensureCreateLifecycleStatus(obj map[string]any) {
	kind, _ := obj[objects.FieldKeyKind].(string)
	if kind == "" {
		return
	}
	loader := objects.GetGlobalLifecycleLoader()
	if loader == nil {
		return
	}
	origin, err := loader.GetOriginStatus(kind)
	if err != nil || origin == "" {
		return
	}

	status := objects.GetString(obj, objects.FieldKeyStatus)
	if status == "" {
		obj[objects.FieldKeyStatus] = origin
		return
	}

	valid, err := loader.IsValidStatus(kind, status)
	if err != nil || valid {
		return
	}

	requested := status
	obj[objects.FieldKeyStatus] = origin
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Warn("create status coerced to lifecycle origin").
		Kind(kind).
		String("requested_status", requested).
		String("origin_status", origin).
		Log()
}
