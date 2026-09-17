// Package storage defines constants for storage operation names used by
// change notifications, persistence steps, metrics, and audit. Use these
// constants instead of string literals to prevent drift and typos (POL-CODE-011).
//
// Where possible, exported names and wire strings describe storage behavior (listing, content-addressed
// writes) rather than internal implementation details.
package storage

const (
	// OpCreate is the operation name for object creation.
	OpCreate = "create"
	// OpUpdate is the operation name for object updates.
	OpUpdate = "update"
	// OpDelete is the operation name for object deletion.
	OpDelete = "delete"
	// OpGet is the operation name for object read/get.
	OpGet = "get"

	// Sub-operations used as operation in trackPersistenceStep (e.g. write_content_addressed, validate_before_write).
	OpWriteContentAddressed     = "write_content_addressed"
	OpValidateBeforeWrite       = "validate_before_write"
	OpValidateObject            = "validate_object"
	OpGetBucketStrategyRegistry = "get_bucket_strategy_registry"
)

// PersistenceStep names a sub-step in trackPersistenceStep (create pipeline, content-addressed write path, validation, etc.).
// Wire values are stable for metrics and logs; use only the constants below (POL-CODE-011).
type PersistenceStep string

// String returns the wire-format name for logs and metrics.
func (p PersistenceStep) String() string { return string(p) }

const (
	PersistenceStepEntry                      PersistenceStep = "entry"
	PersistenceStepExit                       PersistenceStep = "exit"
	PersistenceStepValidateAndPrepare         PersistenceStep = "validate_and_prepare"
	PersistenceStepEnsureID                   PersistenceStep = "ensure_id"
	PersistenceStepPreparePath                PersistenceStep = "prepare_path"
	PersistenceStepCheckExists                PersistenceStep = "check_exists"
	PersistenceStepValidateBeforeWriteStep    PersistenceStep = "validate_before_write"
	PersistenceStepMarshal                    PersistenceStep = "marshal"
	PersistenceStepWriteToStorage             PersistenceStep = "write_to_storage"
	PersistenceStepFinalize                   PersistenceStep = "finalize"
	PersistenceStepGetContentAddressedStorage PersistenceStep = "get_content_addressed_storage"
	PersistenceStepGetBucketStrategyReg       PersistenceStep = "get_bucket_strategy_registry"
	PersistenceStepGetStrategyForKind         PersistenceStep = "get_strategy_for_kind"
	PersistenceStepBucketStrategyLookup       PersistenceStep = "bucket_strategy_lookup"
	PersistenceStepContentAddressedPut        PersistenceStep = "content_addressed_put"
	// PersistenceStepIndexFlush is after a successful content-addressed write: wait until the
	// per-kind listing/index view used by List() reflects the new object.
	PersistenceStepIndexFlush         PersistenceStep = "listing_index_flush"
	PersistenceStepValidateObject     PersistenceStep = "validate_object"
	PersistenceStepValidateReferences PersistenceStep = "validate_references"
	PersistenceStepInitialize         PersistenceStep = "initialize"
)
