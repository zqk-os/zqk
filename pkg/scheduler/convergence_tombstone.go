package scheduler

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// Tombstone / iteration-anchor identifiers for test-bundle health convergence (see
// docs/architecture/CONVERGENCE_PHASE_ROUTER_AND_COORDINATOR_DESIGN.md).
const (
	// EvaluationSurfaceSchedulerTestBundleHealthJSONL is the stable id for health.jsonl-derived measurement.
	EvaluationSurfaceSchedulerTestBundleHealthJSONL = "scheduler_test_bundle_health_jsonl"
	// TombstoneSnapshotSchemaVersion bumps when the tombstone map shape changes.
	TombstoneSnapshotSchemaVersion = "1"
)

// ScopeFingerprintSHA256FromFingerprintMap returns a deterministic hash of the scoped bundle set:
// sorted fingerprint keys, joined by newlines, SHA-256 hex. Empty map yields hash of empty string.
func ScopeFingerprintSHA256FromFingerprintMap(fpToOutcome map[string]string) string {
	if len(fpToOutcome) == 0 {
		return sha256Hex([]byte{})
	}
	keys := make([]string, 0, len(fpToOutcome))
	for k := range fpToOutcome {
		if strings.TrimSpace(k) != emptyValue {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return sha256Hex([]byte(strings.Join(keys, "\n")))
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ComputeTombstoneDisparity compares a persisted before_state_snapshot (tombstone) to the current
// after_state_snapshot-shaped measurement. Both maps use keys from buildTestBundleTombstoneFields.
func ComputeTombstoneDisparity(before, after map[string]any) map[string]any {
	out := map[string]any{}
	if len(before) == 0 {
		out["active_tombstone"] = false
		out[objects.FieldKeyNote] = "no before_state_snapshot on session; use --stamp-tombstone to record an iteration anchor"
		return out
	}
	out["active_tombstone"] = true
	bf := strings.TrimSpace(stringFieldAny(before["scope_fingerprint_sha256"]))
	af := strings.TrimSpace(stringFieldAny(after["scope_fingerprint_sha256"]))
	scopeMatch := bf != emptyValue && bf == af
	out["scope_fingerprint_match"] = scopeMatch
	wb := strings.TrimSpace(stringFieldAny(before["health_watermark_rfc3339"]))
	wa := strings.TrimSpace(stringFieldAny(after["health_watermark_rfc3339"]))
	wmMatch := wb != emptyValue && wb == wa
	out["watermark_match"] = wmMatch
	out["watermark_before"] = wb
	out["watermark_after"] = wa
	switch {
	case !scopeMatch:
		out["disparity_summary"] = "scope fingerprint changed since tombstone (bundle set membership changed)"
	case !wmMatch:
		out["disparity_summary"] = "watermark advanced; scope fingerprint unchanged"
	default:
		out["disparity_summary"] = "tombstone anchor fields match current measurement"
	}
	return out
}

func stringFieldAny(v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return ""
}

// BuildTestBundleTombstoneFields returns minimal iteration-anchor fields for before_state_snapshot
// or for embedding in after_state_snapshot (same shape for disparity).
func BuildTestBundleTombstoneFields(snap *TestBundleConvergenceSnapshot) map[string]any {
	if snap == nil {
		return map[string]any{}
	}
	return map[string]any{
		"evaluation_surface_id":    EvaluationSurfaceSchedulerTestBundleHealthJSONL,
		"snapshot_schema_version":  TombstoneSnapshotSchemaVersion,
		"scope_fingerprint_sha256": ScopeFingerprintSHA256FromFingerprintMap(snap.FingerprintLatestOutcome),
		"health_watermark_rfc3339": snap.HealthWatermarkRFC3339,
		"fingerprint_count":        len(snap.FingerprintLatestOutcome),
	}
}
