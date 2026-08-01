package scheduler

import (
	"context"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"
)

// convRouteMetaKey* are keys for ResolveConvergenceRoutingSession metadata maps (not object FieldKey*).
const (
	convRouteMetaKeyConvergenceSessionID  = "convergence_session_id"
	convRouteMetaKeyCurrentPhaseSource    = "current_phase_source"
	convRouteMetaKeyEffectiveCurrentPhase = "effective_current_phase"
	convRouteMetaKeyEffectiveFlowVariant  = "effective_flow_variant"
	convRouteMetaKeyFlowVariantSource     = "flow_variant_source"
	convRouteMetaKeyKindWarning           = "kind_warning"
	convRouteMetaKeyObjectKind            = "object_kind"
	convRouteMetaKeyReadAttempted         = "read_attempted"
	convRouteMetaKeyReadError             = "read_error"
	convRouteMetaKeyReadOK                = "read_ok"
	convRouteMetaKeySkipSessionContext    = "skip_session_context"
)

// ResolveConvergenceRoutingSession loads convergence_session fields when sessionID is set (unless
// skipSessionContext). Flags override object fields. Emits metadata for suggested_convergence_session_fields.
// When the session object is read successfully, beforeStateSnapshot is the persisted before_state_snapshot
// (iteration tombstone) for disparity vs current measurement. predictions is the session's predictions
// map (for debrief vs health snapshot), or nil if unread. sessionThresholds is extracted from the same
// read when read_ok; nil when session context is skipped, unread, or thresholds absent.
func ResolveConvergenceRoutingSession(
	ctx context.Context,
	storage storagepkg.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	sessionID, flagCurrentPhase, flagFlowVariant string,
	skipSessionContext bool,
) (effCurrentPhase, effFlowVariant string, meta map[string]any, beforeStateSnapshot map[string]any, predictions map[string]any, sessionThresholds map[string]any, err error) {
	log := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	meta = map[string]any{
		convRouteMetaKeyReadAttempted: false,
	}
	if strings.TrimSpace(sessionID) == emptyValue {
		return flagCurrentPhase, flagFlowVariant, nil, nil, nil, nil, nil
	}
	meta[convRouteMetaKeyConvergenceSessionID] = sessionID
	if skipSessionContext {
		meta[convRouteMetaKeySkipSessionContext] = true
		mergeObjectFieldsIntoRoutingMeta(nil, flagCurrentPhase, flagFlowVariant, meta)
		return effectivePhaseFromMeta(meta), effectiveFlowFromMeta(meta), meta, nil, nil, nil, nil
	}
	meta[convRouteMetaKeyReadAttempted] = true
	obj, readErr := storage.Read(ctx, secCtx, sessionID)
	if readErr != nil {
		meta[convRouteMetaKeyReadOK] = false
		meta[convRouteMetaKeyReadError] = readErr.Error()
		ConvergenceRoutingLog(log).Debug(LogEventConvergenceRoutingReadSessionFailed).
			SessionID(sessionID).
			WithError(readErr).
			Log()
		mergeObjectFieldsIntoRoutingMeta(nil, flagCurrentPhase, flagFlowVariant, meta)
		return strings.TrimSpace(flagCurrentPhase), strings.TrimSpace(flagFlowVariant), meta, nil, nil, nil, nil
	}
	meta[convRouteMetaKeyReadOK] = true
	if kind, _ := obj[objects.FieldKeyKind].(string); kind != emptyValue {
		meta[convRouteMetaKeyObjectKind] = kind
		if kind != objects.KindConvergenceSession {
			meta[convRouteMetaKeyKindWarning] = "object kind is not convergence_session; current_phase / flow_variant may be missing"
			ConvergenceRoutingLog(log).Debug(LogEventConvergenceRoutingKindNotSession).
				Kind(kind).
				SessionID(sessionID).
				Log()
		}
	}
	mergeObjectFieldsIntoRoutingMeta(obj, flagCurrentPhase, flagFlowVariant, meta)
	beforeStateSnapshot = extractBeforeStateSnapshot(obj)
	return effectivePhaseFromMeta(meta), effectiveFlowFromMeta(meta), meta, beforeStateSnapshot, extractPredictions(obj), ThresholdsMapFromObject(obj), nil
}

func extractPredictions(obj map[string]any) map[string]any {
	if obj == nil {
		return nil
	}
	raw, ok := obj[objects.FieldKeyPredictions]
	if !ok {
		return nil
	}
	raw, ok = nildecode.DecodeNonNilPayload[any](raw)
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}
	return m
}

func extractBeforeStateSnapshot(obj map[string]any) map[string]any {
	if obj == nil {
		return nil
	}
	raw, ok := obj[objects.FieldKeyBeforeStateSnapshot]
	if !ok {
		return nil
	}
	raw, ok = nildecode.DecodeNonNilPayload[any](raw)
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}
	return m
}

func mergeObjectFieldsIntoRoutingMeta(obj map[string]any, flagCP, flagFV string, meta map[string]any) {
	when.When(func() bool { return strings.TrimSpace(flagCP) != emptyValue }).Then(func() {
		meta[convRouteMetaKeyEffectiveCurrentPhase] = strings.TrimSpace(flagCP)
		meta[convRouteMetaKeyCurrentPhaseSource] = "flag"
	}).OrElseWhen(func() bool { return obj != nil }).Then(func() {
		cp := strings.TrimSpace(FieldAsString(obj[objects.FieldKeyCurrentPhase]))
		meta[convRouteMetaKeyEffectiveCurrentPhase] = cp
		when.When(func() bool { return cp != emptyValue }).
			Then(func() { meta[convRouteMetaKeyCurrentPhaseSource] = "object" }).
			OrElse(func() { meta[convRouteMetaKeyCurrentPhaseSource] = "empty" }).
			Run()
	}).OrElse(func() {
		meta[convRouteMetaKeyEffectiveCurrentPhase] = ""
		meta[convRouteMetaKeyCurrentPhaseSource] = "empty"
	}).Run()

	when.When(func() bool { return strings.TrimSpace(flagFV) != emptyValue }).Then(func() {
		meta[convRouteMetaKeyEffectiveFlowVariant] = strings.TrimSpace(flagFV)
		meta[convRouteMetaKeyFlowVariantSource] = "flag"
	}).OrElseWhen(func() bool { return obj != nil }).Then(func() {
		fv := strings.TrimSpace(FieldAsString(obj[objects.FieldKeyFlowVariant]))
		meta[convRouteMetaKeyEffectiveFlowVariant] = fv
		when.When(func() bool { return fv != emptyValue }).
			Then(func() { meta[convRouteMetaKeyFlowVariantSource] = "object" }).
			OrElse(func() { meta[convRouteMetaKeyFlowVariantSource] = "empty" }).
			Run()
	}).OrElse(func() {
		meta[convRouteMetaKeyEffectiveFlowVariant] = ""
		meta[convRouteMetaKeyFlowVariantSource] = "empty"
	}).Run()
}

func effectivePhaseFromMeta(meta map[string]any) string {
	s, _ := meta[convRouteMetaKeyEffectiveCurrentPhase].(string)
	return strings.TrimSpace(s)
}

func effectiveFlowFromMeta(meta map[string]any) string {
	s, _ := meta[convRouteMetaKeyEffectiveFlowVariant].(string)
	return strings.TrimSpace(s)
}

// FieldAsString coerces common object field values to string for routing metadata.
func FieldAsString(v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return ""
}
