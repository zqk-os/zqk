package storage

import "time"

// CreatePipelineOrderedStages is the synchronous Create path (after early validation) when WAL
// write-behind does not short-circuit. Order matches object_storage_file_create.go (entry through exit).
//
// Write-behind returns after write_to_storage and exit, skipping index_flush and finalize.
// Nested write_content_addressed sub-steps (get_content_addressed_storage, bucket_strategy_lookup,
// content_addressed_put, …) are not listed here; those are implementation details of the
// content-addressable write path.
//
// Tests and callers that bound total Create latency must allow for the index_flush stage: it waits
// up to IndexFlushAfterCreateTimeout unless a shorter context deadline applies (FlushKindContext).
var CreatePipelineOrderedStages = []PersistenceStep{
	PersistenceStepEntry,
	PersistenceStepValidateAndPrepare,
	PersistenceStepEnsureID,
	PersistenceStepPreparePath,
	PersistenceStepCheckExists,
	PersistenceStepValidateBeforeWriteStep,
	PersistenceStepMarshal,
	PersistenceStepWriteToStorage,
	PersistenceStepIndexFlush,
	PersistenceStepFinalize,
	PersistenceStepExit,
}

// IndexFlushAfterCreateTimeout bounds how long Create waits after a successful write for the
// per-kind listing/index materialization so List() and similar readers observe the new object.
// It is a stage budget, not the whole Create latency.
const IndexFlushAfterCreateTimeout = 5 * time.Second
