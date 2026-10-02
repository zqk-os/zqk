package scheduler

import (
	"context"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/objects"
)

// ConvergenceMeasureResult is the normalized output of an evaluation surface adapter.
type ConvergenceMeasureResult struct {
	ObjectUpdateBody          map[string]any
	HealthWatermarkRFC3339    string
	DeltaAssessment           string
	ReadyForSessionCompletion bool
	PrimaryMeasurementOutcome string
	NextActionHint            string
	IsDuplicateWatermark      bool
	AuditUpdateBody           map[string]any
	Skip                      bool
}

// EvaluationSurfaceAdapter defines the interface for pluggable convergence measure surfaces.
type EvaluationSurfaceAdapter interface {
	Measure(ctx context.Context, job *ScheduledJob, sessionID string, obj map[string]any, h *ConvergenceSessionTickHandler) (*ConvergenceMeasureResult, error)
}

var (
	evalSurfaceAdapters   = make(map[string]EvaluationSurfaceAdapter)
	evalSurfaceAdaptersMu sync.RWMutex
)

// RegisterEvaluationSurfaceAdapter registers an evaluation surface adapter.
func RegisterEvaluationSurfaceAdapter(id string, adapter EvaluationSurfaceAdapter) {
	evalSurfaceAdaptersMu.Lock()
	defer evalSurfaceAdaptersMu.Unlock()
	evalSurfaceAdapters[id] = adapter
}

func getEvaluationSurfaceAdapter(id string) EvaluationSurfaceAdapter {
	evalSurfaceAdaptersMu.RLock()
	defer evalSurfaceAdaptersMu.RUnlock()
	return evalSurfaceAdapters[id]
}

// Evaluation surface ids for pluggable convergence measure adapters.
// registry growth beyond CEF + health.jsonl.
const (
	// EvaluationSurfaceCEFDiamondScorecard measures dual-seat CEF diamond axes via matrix CSV.
	EvaluationSurfaceCEFDiamondScorecard = "cef_diamond_scorecard"

	convRouteMetaKeyEvaluationSurfaceID = "evaluation_surface_id"
	convRouteMetaKeyEvaluationSurface   = "evaluation_surface"
)

// EvaluationSurfaceIDFromMaps picks the session's evaluation surface from after_state_snapshot,
// then before_state_snapshot. Empty maps / missing keys default to the health.jsonl adapter
// (legacy measure path) so existing CVS ticks stay unchanged.
func EvaluationSurfaceIDFromMaps(afterState, beforeState map[string]any) string {
	if id := evaluationSurfaceIDFromMap(afterState); id != emptyValue {
		return id
	}
	if id := evaluationSurfaceIDFromMap(beforeState); id != emptyValue {
		return id
	}
	return EvaluationSurfaceSchedulerTestBundleHealthJSONL
}

// EvaluationSurfaceIDFromSessionObject reads after/before snapshots on a convergence_session object.
func EvaluationSurfaceIDFromSessionObject(obj map[string]any) string {
	return EvaluationSurfaceIDFromMaps(extractAfterStateSnapshot(obj), extractBeforeStateSnapshot(obj))
}

// EvaluationSurfaceIDFromRoutingMeta returns the surface stamped into ResolveConvergenceRoutingSession meta.
func EvaluationSurfaceIDFromRoutingMeta(meta map[string]any) string {
	if meta == nil {
		return EvaluationSurfaceSchedulerTestBundleHealthJSONL
	}
	if id, _ := meta[convRouteMetaKeyEvaluationSurfaceID].(string); strings.TrimSpace(id) != emptyValue {
		return strings.TrimSpace(id)
	}
	return EvaluationSurfaceSchedulerTestBundleHealthJSONL
}

func evaluationSurfaceIDFromMap(m map[string]any) string {
	if m == nil {
		return emptyValue
	}
	for _, key := range []string{convSugKeyEvaluationSurface, "evaluation_surface_id", convRouteMetaKeyEvaluationSurfaceID} {
		if id := strings.TrimSpace(FieldAsString(m[key])); id != emptyValue {
			return id
		}
	}
	return emptyValue
}

func extractAfterStateSnapshot(obj map[string]any) map[string]any {
	return ExtractNonEmptySubMap(obj, objects.FieldKeyAfterStateSnapshot)
}
