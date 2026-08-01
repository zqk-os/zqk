package validation

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

	// NoSourceCodeAvailableChecksum is returned when validation code files cannot be read
	// (e.g. production build without source). Cache cannot invalidate on code change in that case.
	NoSourceCodeAvailableChecksum = "no-source-code-available"
)

const (
	PathCheckInstanceValidationHelpers = "cmd/zqk/system/check_instance_validation_helpers.go"
	PathSpecAutoFixer                  = "cmd/zqk/system/spec_auto_fixer.go"
	PathGoValidator                    = "pkg/validation/go_validator.go"
	PathInstanceValidator              = "pkg/validation/instance_validator.go"
)

// ValidationCodeChecksumFiles are relative paths (from project root) included in the
// validation code checksum. Changes to these files trigger cache invalidation.
var ValidationCodeChecksumFiles = []string{
	PathCheckInstanceValidationHelpers,
	PathSpecAutoFixer,
	PathGoValidator,
	PathInstanceValidator,
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
	PrecondPriorityPlanRefIsSet          = "priority_plan_ref is set"
	PrecondActiveMilestoneRefLinked      = "at least one active milestone_ref linked"
	PrecondActiveGoalRefLinked           = "at least one active goal_ref linked"
	PrecondActiveCriteriaRefLinked       = "at least one active criteria_ref linked"
	PrecondMilestoneRefLinked            = "at least one milestone_ref linked"
	PrecondActiveTestCaseRefLinked       = "at least one active test_case_ref linked"
	PrecondMilestoneGoalAlignment        = "milestone_refs must link back to goal_refs"
	PrecondCriteriaGoalAlignment         = "criteria_refs must link back to goal_refs"
	PrecondCommitRefsNotEmpty            = "commit_refs is not empty"
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
