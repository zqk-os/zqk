package system

// This file has been split into focused modules for better maintainability:
//
// - auto_fix_helpers_types_init.go: Types, initialization (AutoFixContext, HashMismatchInfo, initializeAutoFixContext)
// - auto_fix_helpers_issue_processing.go: Issue processing logic (shouldProcessIssue, determineFixKind, getOriginalHash, isImmutableObject)
// - auto_fix_helpers_hash_fixing.go: Hash fixing functions (fixImmutableObjectHash, selectRegistryForUpdate, fixHashViaRegistry)
// - auto_fix_helpers_integrity.go: Integrity issue fixing (fixIntegrityIssue)
// - auto_fix_helpers_cas.go: CAS-specific fixes (fixCASIndexOutOfSync, fixCASMissingHash, fixCASMissingHashWithCAS)
// - auto_fix_helpers_batch.go: Batch processing (batchFixHashMismatches)
//
// All public APIs remain unchanged - this split is purely organizational.
