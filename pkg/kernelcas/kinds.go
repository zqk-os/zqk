// Package kernelcas implements Kernel Mutation Pipeline kinds on top of pkg/pipeline.
// See docs/architecture/KERNEL_MUTATION_PIPELINE.md.
package kernelcas

// Named pipeline kinds (v1 closed set).
const (
	KindCreate         = "kernel.cas_object_create"
	KindUpdate         = "kernel.cas_object_update"
	KindTransition     = "kernel.cas_object_transition"
	KindErase          = "kernel.cas_object_erase"
	KindRestoreMerge   = "kernel.cas_object_restore_merge"
	KindReconcileIndex = "kernel.cas_object_reconcile_index"
	KindBlobGC         = "kernel.cas_blob_gc"
)

// AllKinds is the allowlist for coverage / kernel-integrity report.
func AllKinds() []string {
	return []string{
		KindCreate,
		KindUpdate,
		KindTransition,
		KindErase,
		KindRestoreMerge,
		KindReconcileIndex,
		KindBlobGC,
	}
}

// Plan values written to pipeline.Context.Outcome["plan"] in DECIDE.
const (
	PlanRefuse      = "refuse"
	PlanBreakGlass  = "break_glass"
	PlanDraftPlane  = "draft_plane"
	PlanCasSync     = "cas_sync"
	PlanEraseUnlink = "erase_unlink"
	PlanReindexOnly = "reindex_only"
	PlanBlobGC      = "blob_gc"
)

// Outcome map keys (also registered in pipeline_outcome_keys.yaml).
const (
	OutcomePlan          = "plan"
	OutcomeLifecycleOK   = "lifecycle_ok"
	OutcomeErasePolicy   = "erase_policy"
	OutcomeUnlinkPlanned = "unlink_planned"
	OutcomeObjectID      = "object_id"
	OutcomeObjectKind    = "object_kind"
	OutcomeIntent        = "intent"
	OutcomeRefuseReason  = "refuse_reason"
	OutcomeBreakGlass    = "break_glass_reason"
)

// Intent labels for NORMALIZE.
const (
	IntentCreate         = "Create"
	IntentUpdateFields   = "UpdateFields"
	IntentTransition     = "TransitionStatus"
	IntentEraseLogical   = "EraseLogical"
	IntentRestoreMerge   = "RestoreMerge"
	IntentReconcileIndex = "ReconcileIndex"
	IntentGcBlob         = "GcSupersededBlob"
)

// Erase policy OutcomeErasePolicy values.
const (
	ErasePolicyAllow  = "allow"
	ErasePolicyRefuse = "refuse"
	ErasePolicyBreak  = "break_glass"
)
