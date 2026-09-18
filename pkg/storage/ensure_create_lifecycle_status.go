package storage

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/audit"
)

// ensureCreateLifecycleStatus sets status on create when missing or invalid for the kind,
// and coerces non-preliminary statuses down to the lifecycle origin for interactive CLI
// creates so they park on the draft plane until promote/update permeates the membrane.
//
// When promoteOnCreate is true (CLI --promote / pkgctx.WithPromoteOnCreate), when the create
// is non-CLI (system-generated; !audit.HasCLIMarker(ctx)), or when the kind is a bypass kind
// (objects.IsBypassKind(kind)), a valid non-preliminary status is preserved so Create writes
// CAS directly — system/agent payloads that are already shovel-ready must never land as
// draft-plane ghosts.
//
// TRACK: BLI-1785639926306245000-cf2ac4b1 — draft-first create / promote membrane.
func ensureCreateLifecycleStatus(ctx context.Context, obj map[string]any, promoteOnCreate bool) {
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
		status = origin
	} else {
		valid, err := loader.IsValidStatus(kind, status)
		if err != nil || !valid {
			requested := status
			obj[objects.FieldKeyStatus] = origin
			status = origin
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn("create status coerced to lifecycle origin").
				Kind(kind).
				String("requested_status", requested).
				String("origin_status", origin).
				Log()
		}
	}

	checker := objects.GetGlobalStatusChecker()
	if checker.IsPreliminary(kind, status) {
		return
	}
	// Promote-ready create, non-CLI (system-generated) create, or bypass kinds:
	// keep shovel-ready status → skip draft plane (CAS write).
	isCLI := audit.HasCLIMarker(ctx)
	if promoteOnCreate || !isCLI || objects.IsBypassKind(kind) {
		return
	}
	// Non-preliminary CLI create: force origin when origin is preliminary.
	if checker.IsPreliminary(kind, origin) {
		requested := status
		obj[objects.FieldKeyStatus] = origin
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn("create status coerced to lifecycle origin (draft-first membrane)").
			Kind(kind).
			String("requested_status", requested).
			String("origin_status", origin).
			Log()
		return
	}
	// Origin is already shovel_ready (glossary_term, scheduler_job, …). Status coerce cannot
	// park on the draft plane until those lifecycles gain a preliminary origin.
	// TRACK: BLI-1785639926306245000-cf2ac4b1
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Warn("create at non-preliminary lifecycle origin; membrane park incomplete").
		Kind(kind).
		String("status", status).
		String("origin_status", origin).
		Log()
}
