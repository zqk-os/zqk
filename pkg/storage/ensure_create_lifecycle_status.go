package storage

import (
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// ensureCreateLifecycleStatus sets status on create when missing or invalid for the kind,
// and coerces non-preliminary statuses down to the lifecycle origin so creates park on the
// draft plane until promote/update to shovel_ready (active/ready/…) permeates the membrane.
//
// When promoteOnCreate is true (CLI --promote / pkgctx.WithPromoteOnCreate), a valid
// non-preliminary status is kept so Create writes CAS directly — system/agent payloads
// that are already promote-ready must not land as draft-plane ghosts.
//
// TRACK: REDACTED — draft-first create / promote membrane.
// TRACK: REDACTED — CapOrchestratorHandler still passes
// status=pending; this coerce makes creates succeed. Prefer passing proposed explicitly
// for clarity when CAP is next touched.
func ensureCreateLifecycleStatus(obj map[string]any, promoteOnCreate bool) {
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
	// Promote-ready create: keep shovel-ready status → skip draft plane (CAS write).
	if promoteOnCreate {
		return
	}
	// Non-preliminary create (e.g. policy status=active): force origin when origin is preliminary.
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
	// TRACK: REDACTED
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Warn("create at non-preliminary lifecycle origin; membrane park incomplete").
		Kind(kind).
		String("status", status).
		String("origin_status", origin).
		Log()
}
