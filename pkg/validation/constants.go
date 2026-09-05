package validation

import "github.com/lanceman/zqk/pkg/shovelready"

// Validation cache and state cache constants.
// Use these instead of magic strings so updates and discovery are in one place.

const (
	// MaxObjectIDLength is the maximum allowed length for an object ID.
	// Enforced by IDValidator, base_object spec (max_length), and WAL append. Single source of truth for ID length cap.
	// Prevents malformed or corrupted IDs (e.g. from YAML id: | blocks) from causing unbounded WAL lines.
	MaxObjectIDLength = 2048

	// MaxValidationCacheEntries is the maximum number of validation states held in memory.
	// When exceeded on Load we stop loading; when exceeded on Set we evict oldest by LastValidated.
	// Prevents validation cache from holding multi-GB as object volume grows.
	MaxValidationCacheEntries = 300_000

	// ValidationCacheVersion is the version written to validation_cache.json.
	// Bump when cache schema or semantics change.
	ValidationCacheVersion = "1.0.0"

	// NoSourceCodeAvailableChecksum is returned only when neither the running
	// executable nor any checker/lifecycle source files can be read. Production
	// binaries still fingerprint os.Executable so a rebuild invalidates the cache.
	NoSourceCodeAvailableChecksum = "no-source-code-available"
)

const (
	PathCheckInstanceValidationHelpers = "cmd/zqk/system/check_instance_validation_helpers.go"
	PathCheckImplValidators            = "cmd/zqk/system/check_impl_validators.go"
	PathSpecAutoFixer                  = "cmd/zqk/system/spec_auto_fixer.go"
	PathGoValidator                    = "pkg/validation/go_validator.go"
	PathInstanceValidator              = "pkg/validation/instance_validator.go"
)

const (
	PathCriteriaLifecycleYAML           = "docs/process/_internal/lifecycles/criteria_lifecycle.yaml"
	PathConvergenceSessionLifecycleYAML = "docs/process/_internal/lifecycles/convergence_session_lifecycle.yaml"
	PathBacklogItemLifecycleYAML        = "docs/process/_internal/lifecycles/backlog_item_lifecycle.yaml"
	PathPriorityPlanLifecycleYAML       = "docs/process/_internal/lifecycles/priority_plan_lifecycle.yaml"
)

// ValidationCodeChecksumFiles are named paths always unioned into the checker
// fingerprint (sparse test roots still hash these when present). Production
// coverage is ValidationCodeChecksumGlobs plus the running executable — do not
// grow this list as the primary way to catch checker edits.
// TRACK: PRI-CEF-R26-LIFECYCLE-EXAM-001 / REDACTED
var ValidationCodeChecksumFiles = []string{
	PathCheckInstanceValidationHelpers,
	PathCheckImplValidators,
	PathSpecAutoFixer,
	PathGoValidator,
	PathInstanceValidator,
	PathCriteriaLifecycleYAML,
	PathConvergenceSessionLifecycleYAML,
	PathBacklogItemLifecycleYAML,
	PathPriorityPlanLifecycleYAML,
}

// ValidationCodeChecksumGlobs are filepath.Glob patterns from project root.
// check*.go is the Layer 1 checker tree (including check_references_helpers.go);
// parser covers ExtractReferenceFields; validation package + all lifecycle YAMLs
// cover validator and occupancy-machine edits. *_test.go is skipped.
// TRACK: PRI-CEF-R26-LIFECYCLE-EXAM-001 — --clear-cache is break-glass, not the reload path.
var ValidationCodeChecksumGlobs = []string{
	"cmd/zqk/system/check*.go",
	"cmd/zqk/system/async_check*.go",
	"cmd/zqk/system/*validation*.go",
	"cmd/zqk/system/spec_auto_fixer*.go",
	"pkg/validation/*.go",
	"pkg/migration/parser/*.go",
	"docs/process/_internal/lifecycles/*.yaml",
}

const (
	KindValidationRule      = "validation_rule"
	DirValidationRules      = "validation_rules"
	RuleTypeFieldPresence   = "field_presence"
	RuleTypeActiveReference = "active_reference"
	RuleTypeAlignment       = "alignment"
)

// Lock operation names for validation state cache (used with RunInLockWithLogger).
// Centralized so metrics/tracing use consistent names.
const (
	LockOpValidationCacheLoad           = "validation_state_cache_load"
	LockOpValidationCacheSaveCopy       = "validation_state_cache_save_copy"
	LockOpValidationCacheGet            = "validation_state_cache_get"
	LockOpValidationCacheGetMaxAge      = "validation_state_cache_get_max_age"
	LockOpValidationCacheGetAll         = "validation_state_cache_get_all"
	LockOpValidationCacheSet            = "validation_state_cache_set"
	LockOpValidationCacheInvalidate     = "validation_state_cache_invalidate"
	LockOpValidationCacheInvalidateKind = "validation_state_cache_invalidate_by_kind"
	LockOpValidationCacheInvalidateCat  = "validation_state_cache_invalidate_by_category"
	LockOpValidationCacheInvalidatePat  = "validation_state_cache_invalidate_by_pattern"
	LockOpValidationCacheClear          = "validation_state_cache_clear"
	LockOpValidationCacheGetStale       = "validation_state_cache_get_stale"
	LockOpValidationCacheGetByTier      = "validation_state_cache_get_by_tier"
	LockOpValidationCacheCount          = "validation_state_cache_count"
)

// Precondition constraint constants used for validation and bitmask evaluation.
const (
	PrecondProblemStatementAndAcceptance = "problem statement and acceptance considerations defined"
	PrecondPriorityAssigned              = "priority assigned"
	PrecondOwnerIdentified               = "owner identified"
	PrecondOwnerIsSet                    = "owner is set"
	PrecondOwnerRefIsSet                 = "owner_ref is set"
	PrecondPriorityPlanRefIsSet          = "priority_plan_ref is set"
	PrecondActiveMilestoneRefLinked      = "at least one active milestone_ref linked"
	PrecondActiveGoalRefLinked           = "at least one active goal_ref linked"
	PrecondActiveCriteriaRefLinked       = "at least one active criteria_ref linked"
	PrecondMilestoneRefLinked            = "at least one milestone_ref linked"
	PrecondActiveTestCaseRefLinked       = "at least one active test_case_ref linked"
	PrecondMilestoneGoalAlignment        = "milestone_refs must link back to goal_refs"
	PrecondCriteriaGoalAlignment         = "criteria_refs must link back to goal_refs"
	PrecondCommitRefsNotEmpty            = "commit_refs is not empty"
	// PrecondCommitRefsGitMutationEvidence fail-closes BLI→complete when agents cite
	// empty refs, merge SHAs on main, or docs/process-only CAS renames.
	// TRACK: REDACTED
	PrecondCommitRefsGitMutationEvidence = "commit_refs have git mutation evidence for this backlog_item"
	// PrecondBranchRefIsAncestorOfTrunk enforces that a plan cannot complete until its branch_ref
	// is an ancestor of trunk.
	PrecondBranchRefIsAncestorOfTrunk = "branch_ref is an ancestor of trunk"
	// PrecondMachineCheckableClosureEvidence gates promotion to complete on machine-checkable
	// scheduler job id, bundle log, and re-read green fingerprint in health.jsonl.
	// TRACK: BLI-CEF-R19-CLOSURE-GATE-001 / REQ-CEF-R19-EVIDENCE-001
	PrecondMachineCheckableClosureEvidence = "machine-checkable evidence with green scheduler fingerprint is verified"
	// PrecondAllLinkedCriteriaValidatedOrComplete gates backlog complete transitions/holds.
	PrecondAllLinkedCriteriaValidatedOrComplete = "all linked criteria_refs are validated or complete"
	// PrecondPriorityPlanRefExecutionFacing gates planned→in_progress: work may only start
	// under a plan that is itself executing. Bound to a row in refStatusRules, and matched as a
	// substring so the lifecycle's "(execution-facing)" suffix still resolves to it.
	PrecondPriorityPlanRefExecutionFacing = "priority_plan_ref target must be in active or in_progress status"
	// PrecondReadyBacklogReferencesPlan is child-owned priority_plan membership:
	// ≥1 backlog_item with priority_plan_ref=this plan and status planned (conversational "ready").
	// TRACK: [REDACTED-ID] — naming may rename planned→ready later.
	PrecondReadyBacklogReferencesPlan = "at least one ready backlog_item references this plan via priority_plan_ref"
	// PrecondTeamOrPersonaDispatchRefs gates priority_plan shovel-ready / execution-locked
	// transitions so CAP cannot fall back to every persona. TRACK: REDACTED
	PrecondTeamOrPersonaDispatchRefs = "at least one team_configuration_ref or persona_refs"
	// PrecondCRIShovelReady is the MMORCH DoR gate for promote into planned/in_progress.
	// Must be the exact lifecycle precondition string so checkPrecondition cannot no-op.
	// TRACK: REDACTED — CAP dor-gap vs empty-column split.
	PrecondCRIShovelReady = shovelready.Precondition
	// PrecondAllLinkedBacklogReadyOrLater gates execution lock (→in_progress), including shockwave:
	// every linked backlog_item must be planned+ (ready-or-later). Vacuous true when no BLI dependents.
	// TRACK: [REDACTED-ID]
	PrecondAllLinkedBacklogReadyOrLater = "all linked backlog_items referencing this plan are ready or later"
	// PrecondNoLinkedBacklogInProgressOrComplete enforces active (shovel-ready) constraints:
	PrecondNoLinkedBacklogInProgressOrComplete = "no linked backlog_items referencing this plan are in progress or complete"
	// PrecondLinkedBacklogAllTerminal gates priority_plan status=complete (hold + →complete):
	// planned/in_progress/exploring children must not coexist with a complete plan.
	// Vacuous true when no BLI dependents. Fail-closed without lookups.
	// TRACK: REDACTED — was complete with 9 planned L1–L10 BLIs.
	PrecondLinkedBacklogAllTerminal = "all linked backlog_items referencing this plan are terminal"
	// PrecondWorkflowConstraintsIfSet gates priority_plan seal when workflow_ref is present:
	// the referenced workflow must exist, be kind workflow, and enabled. Vacuous true when unset.
	// Must match the lifecycle YAML token (case-insensitive). TRACK: REDACTED
	PrecondWorkflowConstraintsIfSet = "workflow constraints validated (if workflow_ref is set)"
	// PrecondPriorityPlanValidated is the remaining grooming→active YAML token:
	// inherited title or specialized description, plus workstream_refs or singular workstream_ref.
	// Persona/team and planned-child are separate tokens. TRACK: BLI-CEF-R26-REMAINING-KINDS-001
	PrecondPriorityPlanValidated = "priority plan validated"
)

// Precondition sub-string constraints for generic pattern extraction.
const (
	SubprecondLinkBackTo         = "link back to"
	SubprecondLinksBackTo        = "links back to"
	SubprecondBelongsTo          = "belongs to"
	SubprecondActive             = "active"
	SubprecondAtLeast            = "at least"
	SubprecondIsSet              = "is set"
	SubprecondIsNotEmpty         = "is not empty"
	SubprecondOr                 = " or "
	SubprecondActiveMilestoneRef = "active milestone_ref"
	SubprecondActiveGoalRef      = "active goal_ref"
	SubprecondActiveCriteriaRef  = "active criteria_ref"
)
