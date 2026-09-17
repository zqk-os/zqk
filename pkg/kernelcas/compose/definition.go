// Package compose compiles kind×intent mutation DECIDE/FINALIZE configuration
// into pipeline_definition-shaped definitions (Kernel Mutation Pipeline v2).
// TRACK: BLI-REDACTED
package compose

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/pipeline"
)

// CompositionKey uniquely identifies a composed mutation pipeline.
type CompositionKey struct {
	ObjectKind   string
	PipelineKind string
	Intent       string
}

func (k CompositionKey) String() string {
	return k.ObjectKind + "|" + k.PipelineKind + "|" + k.Intent
}

// Title is the stable human/agent label used when materializing pipeline_definition rows.
func (k CompositionKey) Title() string {
	return fmt.Sprintf("%s :: %s", k.PipelineKind, k.ObjectKind)
}

// Definition is the in-memory composed pipeline (also serialized into pipeline_definition.stages).
type Definition struct {
	Key     CompositionKey `json:"key"`
	Sources []Source       `json:"sources"`
	Stages  []Stage        `json:"stages"`
}

// Source records where DECIDE rules were derived from.
type Source struct {
	Type string `json:"type"` // lifecycle | critical_policy | kind_overlay | verification_dsl
	Ref  string `json:"ref,omitempty"`
}

// Stage is one pipeline stage with optional composed rules.
type Stage struct {
	Name  string `json:"name"`
	Role  string `json:"role"` // fixed | composed
	Rules []Rule `json:"rules,omitempty"`
}

// Rule is a declarative DECIDE/FINALIZE predicate.
type Rule struct {
	ID     string         `json:"id"`
	Op     string         `json:"op"`
	Config map[string]any `json:"config,omitempty"`
}

// Pipeline kind / intent / plan strings — must match pkg/kernelcas (no import; avoid cycle).
const (
	KindCreate         = "kernel.cas_object_create"
	KindUpdate         = "kernel.cas_object_update"
	KindTransition     = "kernel.cas_object_transition"
	KindErase          = "kernel.cas_object_erase"
	KindRestoreMerge   = "kernel.cas_object_restore_merge"
	KindReconcileIndex = "kernel.cas_object_reconcile_index"
	KindBlobGC         = "kernel.cas_blob_gc"

	IntentCreate         = "Create"
	IntentUpdateFields   = "UpdateFields"
	IntentTransition     = "TransitionStatus"
	IntentEraseLogical   = "EraseLogical"
	IntentRestoreMerge   = "RestoreMerge"
	IntentReconcileIndex = "ReconcileIndex"
	IntentGcBlob         = "GcSupersededBlob"

	PlanRefuse      = "refuse"
	PlanBreakGlass  = "break_glass"
	PlanCasSync     = "cas_sync"
	PlanEraseUnlink = "erase_unlink"
	PlanReindexOnly = "reindex_only"
	PlanBlobGC      = "blob_gc"

	ErasePolicyAllow  = "allow"
	ErasePolicyRefuse = "refuse"
	ErasePolicyBreak  = "break_glass"
)

// Rule ops (closed set for v2 compiler).
const (
	OpAllowCasSync           = "allow_cas_sync"
	OpBreakGlassIfReason     = "break_glass_if_reason"
	OpEraseCriticalPolicy    = "erase_critical_policy"
	OpReconcileIndexOnly     = "reconcile_index_only"
	OpBlobGC                 = "blob_gc"
	OpRequireRefAny          = "require_ref_any"
	OpRequireFieldWhenStatus = "require_field_when_status"
	// OpRequireField refuses when a named field is empty (any status).
	// Used for CAS-vital fields that create may park on the draft plane.
	OpRequireField                    = "require_field"
	OpRefusePlanStatus                = "refuse_plan_status"
	OpRequireFieldWhenActive          = "require_field_when_active"
	OpMinStringLen                    = "min_string_len"
	OpLifecyclePreconditions          = "lifecycle_preconditions"
	OpRefuseExecutionFacingMembership = "refuse_execution_facing_membership"
	// OpRefuseFieldPresent refuses when a named field is present (including empty lists).
	// Used for removed parent→child keys such as priority_plan.backlog_item_refs.
	OpRefuseFieldPresent = "refuse_field_present"
	// OpRefuseFieldWhenStatus refuses a non-empty field when status is in Config.statuses.
	OpRefuseFieldWhenStatus = "refuse_field_when_status"
	// OpRefuseRefPrefix refuses IDs with configured prefixes in a reference list.
	OpRefuseRefPrefix   = "refuse_ref_prefix"
	OpRefuseChildStatus = "refuse_child_status"
	// OpRefuseSelfRef refuses when a ref field contains the object's own id.
	OpRefuseSelfRef = "refuse_self_ref"
	// OpRefuseTwoCycle refuses A→B when B's same field points back at A.
	OpRefuseTwoCycle = "refuse_two_cycle"
	// OpShovelReadyWhenStatus refuses in_progress BLIs that fail CRI-SHOVEL-READY.
	// Planned promote is the lifecycle token (except in_progress→planned demote).
	// TRACK: CRIT-REDACTED — promote must not skip DoR.
	OpShovelReadyWhenStatus = "shovel_ready_when_status"
	// FieldCRIShovelReady is the overlay error field for CRI-SHOVEL-READY (not a spec FieldKey).
	FieldCRIShovelReady = "cri_shovel_ready"
	// OpRefuseDuplicateRefs refuses intra-object duplicate target IDs across or within reference fields.
	// TRACK: TDE-CEF-CAS-SPEC-FIELD-DIFF-001
	OpRefuseDuplicateRefs = "refuse_duplicate_refs"
	// OpRefuseUnknownFields refuses fields not declared on the resolved object spec.
	// TRACK: TDE-CEF-CAS-SPEC-FIELD-DIFF-001
	OpRefuseUnknownFields = "refuse_unknown_fields"
	// OpValidatePriorityValues validates priority and priority_tier are legitimate values if present.
	OpValidatePriorityValues = "validate_priority_values"
)

// AllPipelineKinds is the closed mutation kind set (mirrors kernelcas.AllKinds).
func AllPipelineKinds() []string {
	return []string{
		KindCreate, KindUpdate, KindTransition, KindErase,
		KindRestoreMerge, KindReconcileIndex, KindBlobGC,
	}
}

// IntentsForPipelineKind maps pipeline kinds to NORMALIZE intents.
func IntentsForPipelineKind(pipelineKind string) string {
	switch pipelineKind {
	case KindCreate:
		return IntentCreate
	case KindUpdate:
		return IntentUpdateFields
	case KindTransition:
		return IntentTransition
	case KindErase:
		return IntentEraseLogical
	case KindRestoreMerge:
		return IntentRestoreMerge
	case KindReconcileIndex:
		return IntentReconcileIndex
	case KindBlobGC:
		return IntentGcBlob
	default:
		return ""
	}
}

// FixedStageNames is the closed stage order for mutation pipelines.
func FixedStageNames() []string {
	return []string{
		pipeline.StageIngest,
		pipeline.StageNormalize,
		pipeline.StageDecide,
		pipeline.StageCommit,
		pipeline.StageFinalize,
	}
}

// ParseCompositionKey parses "objectKind|pipelineKind|intent".
func ParseCompositionKey(s string) (CompositionKey, bool) {
	parts := strings.Split(s, "|")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return CompositionKey{}, false
	}
	return CompositionKey{ObjectKind: parts[0], PipelineKind: parts[1], Intent: parts[2]}, true
}
