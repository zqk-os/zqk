package scheduler

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// kindsDestroyedByAutoEnrollment is the victim set from the 2026-08-24 incident: 270 archived CAS
// objects hard-deleted in one retention window, which then showed up as GhostRefs ("Referenced
// object <id> (kind: criteria) in Backlog item") on the next system check.
//
// Retained as data rather than prose because it is the evidence that the exemption must not be
// hand-maintained: every one of these was absent from isCatalogKind, and that omission was the bug.
var kindsDestroyedByAutoEnrollment = []string{
	objects.KindCriteria,
	objects.KindConvergenceSession,
	objects.KindPriorityPlan,
	objects.KindBacklogItem,
	"mission",
	"strategic_plan",
	"technical_debt",
	"test_case",
}

// TestRetentionAutoEnrollmentSkipsKernelCriticalKinds pins the discriminator that protects process
// history from the retention sweep.
//
// The sweep's own defaults are aggressive by design: `archived` is deliberately not in
// DefaultProtectStatuses, so anything enrolled becomes collectable once it ages past cleanup_after.
// Enrollment is therefore the decision that matters, and for unconfigured kinds it happens
// automatically. kernel_critical already means "must not be silently hard-deleted", so it is the
// right gate; the alternative (naming each kind in isCatalogKind) is what failed.
func TestRetentionAutoEnrollmentSkipsKernelCriticalKinds(t *testing.T) {
	for _, kind := range kindsDestroyedByAutoEnrollment {
		if !objects.IsKernelCriticalKind(kind) {
			t.Errorf("%s is no longer kernel_critical, so retention auto-enrollment will collect its "+
				"archived objects again; this kind lost 100%% of its archived instances to that path "+
				"on 2026-08-24", kind)
		}
	}
}

// TestMergeStrategyToleranceConsultsKernelCritical checks that the discovery loop actually asks the
// question, not merely that the answer would be right.
//
// This is a source-level assertion rather than a behavioral one, which is a compromise worth naming:
// mergeStrategyTolerance needs live storage to count instances per kind, so exercising enrollment
// end-to-end means standing up a seeded store. Without this check the two tests above are vacuous —
// they assert kernel_critical's values, so deleting the `continue` from the loop leaves them green
// while restoring the exact defect. TRACK: replace with a behavioral test when a seeded retention
// harness exists.
func TestMergeStrategyToleranceConsultsKernelCritical(t *testing.T) {
	src := readHandlerSource(t)
	loop := discoveryLoopBody(t, src)
	if !strings.Contains(loop, "IsKernelCriticalKind(kind)") {
		t.Error("the unconfigured-kind discovery loop in mergeStrategyTolerance no longer consults " +
			"IsKernelCriticalKind; without it, every kernel_critical kind is auto-enrolled and its " +
			"archived objects become collectable (archived is not in DefaultProtectStatuses), which " +
			"hard-deleted 270 objects on 2026-08-24")
	}
	// The skip must precede the Count call that gates enrollment, otherwise it is decoration.
	kc := strings.Index(loop, "IsKernelCriticalKind(kind)")
	cnt := strings.Index(loop, "h.storage.Count(")
	if kc >= 0 && cnt >= 0 && kc > cnt {
		t.Error("the kernel_critical skip appears after the instance Count that gates enrollment; it " +
			"must short-circuit before a kind can be added to the effective tolerance map")
	}
}

func readHandlerSource(t *testing.T) string {
	t.Helper()
	data, err := fileutil.ReadFile("handlers_retention_tolerance.go")
	if err != nil {
		t.Fatalf("read retention handler source: %v", err)
	}
	return string(data)
}

// discoveryLoopBody returns mergeStrategyTolerance's body, where unconfigured kinds are enrolled.
func discoveryLoopBody(t *testing.T, src string) string {
	t.Helper()
	const sig = "func (h *RetentionToleranceHandler) mergeStrategyTolerance("
	start := strings.Index(src, sig)
	if start < 0 {
		t.Fatalf("mergeStrategyTolerance not found; if it was renamed, update this test rather than " +
			"deleting it — it is the only check that the enrollment guard is wired in")
	}
	rest := src[start:]
	if end := strings.Index(rest, "\nfunc "); end > 0 {
		rest = rest[:end]
	}
	return rest
}

// TestRetentionStillTrimsNonCriticalKinds is the other half: the fix must not turn retention off.
// If these became kernel_critical, high-volume audit and scheduler records would accumulate forever
// and the guard above would be the reason, so it needs to fail loudly rather than silently stop
// collecting.
func TestRetentionStillTrimsNonCriticalKinds(t *testing.T) {
	trimmable := []string{
		objects.KindSchedulerJob,
		objects.KindChangeJournalEntry,
		objects.KindAgentTask,
		"audit_event",
	}
	for _, kind := range trimmable {
		if objects.IsKernelCriticalKind(kind) {
			t.Errorf("%s is now kernel_critical, so retention auto-enrollment will skip it and these "+
				"high-volume records will accumulate without bound; either exempt it explicitly in "+
				"retention_tolerance.yaml or reconsider the kernel_critical marking", kind)
		}
		if isCatalogKind(kind) {
			t.Errorf("%s was added to isCatalogKind, which also stops retention from trimming it", kind)
		}
	}
}
