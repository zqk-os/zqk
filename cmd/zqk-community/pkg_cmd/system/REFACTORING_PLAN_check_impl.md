# Refactoring Plan: check_impl.go

**File:** `cmd/zqk/system/check_impl.go`  
**Original Size:** 3,322 lines  
**Current Size:** 720 lines (78% reduction)  
**Status:** ✅ COMPLETE - All phases finished successfully

**Line count verification (BLI-REDACTED):** Main file `check_impl.go` is 720 lines (under 2000). No further split required. Logic already lives in check_impl_core-style entry points plus `check_impl_validators.go`, `check_impl_integrity.go`, `check_impl_helpers.go`, `check_impl_output.go`, `check_impl_autofix.go`, `check_impl_audit_buffer.go`, `check_impl_hash.go`.

## Analysis

### Function Distribution
- **Main Entry Points:** `runCheck`, `runCheckSync`, `checkAll`, `checkKind`, `checkIDs`
- **Core Check Logic:** `checkObject`, `checkObjectWithCache`, `checkObjectWithCacheAndContent`
- **Validation Checks:** `checkRegistration`, `checkLifecycle`, `checkInstanceValidation`, `checkPolicy`, `checkReferences`
- **Integrity Checks:** `checkIntegrity`, `checkIntegrityWithRegistry`, `checkIntegrityWithRegistryAndContent`
- **Hash Operations:** `updateHashInRegistry`, `updateHashInRegistryWithInstance`, `reloadHashRegistriesForKinds`, hash mismatch fixes
- **Cache Operations:** `checkDuplicateIDs`, cache audit events, cache coordination
- **Output Functions:** `outputResults`, `outputJSON`, `outputJSONL`, `outputYAML`, `outputTable`
- **Auto-Fix:** `autoFixIssues`, `processIssueForAutoFix`
- **Helpers:** `validateIDFormat`, `filterResultsByTier`, `prepareOutputData`, `generateSystemCheckReminders`

### Proposed File Structure

#### 1. `check_impl_core.go` (~800 lines)
**Purpose:** Core check orchestration and main entry points

**Contents:**
- `runCheck()` - Main entry point
- `runCheckSync()` - Synchronous check entry point
- `checkAll()` - Check all objects
- `checkKind()` - Check objects of a kind
- `checkIDs()` - Check specific object IDs
- `checkKindObjects()` - Check objects of a kind (internal)
- `checkObject()` - Main check function
- `checkObjectWithCache()` - Check with cache support
- `checkObjectWithCacheAndContent()` - Check with cache and pre-read content
- `CheckKindObjectsWithCache()` - Bulk check with cache
- `performBlockingCheck()` - Blocking check for write operations
- Types: `CheckResult`, `Issue`
- Context initialization helpers

**Dependencies:** All other check modules

---

#### 2. `check_impl_validators.go` (~600 lines)
**Purpose:** Validation checks (registration, lifecycle, instance validation, policy, references)

**Contents:**
- `checkRegistration()` - Object registration validation
- `checkLifecycle()` - Lifecycle state validation
- `checkLifecycleWithLoader()` - Lifecycle validation with loader
- `checkInstanceValidation()` - Instance validation
- `checkInstanceValidationWithValidator()` - Instance validation with validator
- `checkInstanceValidationWithValidatorAndData()` - Instance validation with data
- `checkPolicy()` - Policy validation
- `checkReferences()` - Reference validation
- `checkReferencesWithCache()` - Reference validation with cache
- `checkDuplicateIDs()` - Duplicate ID detection
- `validateIDFormat()` - ID format validation (deprecated)
- `validateAllSpecs()` - Validate all object specs

**Dependencies:** Core, Helpers

---

#### 3. `check_impl_integrity.go` (~500 lines)
**Purpose:** Integrity checks (hash verification, CAS integrity)

**Contents:**
- `checkIntegrity()` - Basic integrity check
- `checkIntegrityWithRegistry()` - Integrity check with registry
- `checkIntegrityWithRegistryAndContent()` - Integrity check with pre-read content
- `casHashFileExists()` - Check if CAS hash file exists
- Integrity-related helper functions

**Dependencies:** Core, Hash

---

#### 4. `check_impl_hash.go` (~400 lines)
**Purpose:** Hash registry operations and hash mismatch fixes

**Contents:**
- `updateHashInRegistry()` - Update hash in registry
- `updateHashInRegistryWithInstance()` - Update hash with registry instance
- `reloadHashRegistriesForKinds()` - Reload hash registries
- Hash mismatch fix functions
- `createHashMismatchFixAuditEvent()` - Create audit event for hash fixes
- Hash-related coordination functions

**Dependencies:** Core, Integrity

---

#### 5. `check_impl_cache.go` (~500 lines)
**Purpose:** Cache-related checks and operations

**Contents:**
- `checkDuplicateIDs()` - Duplicate ID detection using cache
- Cache audit event creation
- `createCacheAuditEvent()` - Create cache audit event
- `createCacheAuditEventWithBuilder()` - Create cache audit event with builder
- Cache coordination functions
- Cache freshness checks

**Dependencies:** Core

---

#### 6. `check_impl_output.go` (~600 lines)
**Purpose:** Output formatting and result presentation

**Contents:**
- `outputResults()` - Main output orchestrator
- `outputJSON()` - JSON output
- `outputJSONL()` - JSONL output
- `outputYAML()` - YAML output
- `outputTable()` - Table output
- `prepareOutputData()` - Prepare output data structure
- `filterResultsByTier()` - Filter results by tier
- `generateSystemCheckReminders()` - Generate reminders
- Output context initialization

**Dependencies:** Core

---

#### 7. `check_impl_autofix.go` (~400 lines)
**Purpose:** Auto-fix operations

**Contents:**
- `autoFixIssues()` - Main auto-fix orchestrator
- `processIssueForAutoFix()` - Process individual issue for auto-fix
- Auto-fix context and helpers
- Auto-fix coordination

**Dependencies:** Core, Validators, Integrity, Hash

---

#### 8. `check_impl_audit_buffer.go` (~300 lines)
**Purpose:** Audit event buffering

**Contents:**
- `AuditEventBuffer` struct
- `BufferedEvent` struct
- `GetAuditEventBuffer()` - Get global buffer
- `ResetAuditEventBuffer()` - Reset buffer
- `Add()` - Add event to buffer
- `AddHashMismatchFix()` - Add hash fix event
- `Flush()` - Flush buffer
- `generateKey()` - Generate event key

**Dependencies:** Core

---

#### 9. `check_impl_helpers.go` (~400 lines)
**Purpose:** Helper functions and utilities

**Contents:**
- `logInitialDebugState()` - Log initial debug state
- `logFinalDebugState()` - Log final debug state
- `getObjectFilePath()` - Get file path for object
- `isHashMismatchFixMode()` - Check if in hash fix mode
- `isInternalKind()` - Check if kind is internal
- `discoverObjectKinds()` - Discover object kinds
- `getKindDirectory()` - Get directory for kind
- Other utility functions

**Dependencies:** Core

---

## Migration Strategy

### Phase 1: Extract Helpers (Low Risk)
1. Create `check_impl_helpers.go`
2. Move helper functions
3. Test

### Phase 2: Extract Output (Low Risk)
1. Create `check_impl_output.go`
2. Move output functions
3. Test

### Phase 3: Extract Audit Buffer (Low Risk)
1. Create `check_impl_audit_buffer.go`
2. Move audit buffer types and functions
3. Test

### Phase 4: Extract Validators (Medium Risk)
1. Create `check_impl_validators.go`
2. Move validation check functions
3. Test thoroughly

### Phase 5: Extract Integrity (Medium Risk)
1. Create `check_impl_integrity.go`
2. Move integrity check functions
3. Test

### Phase 6: Extract Hash Operations (Medium Risk)
1. Create `check_impl_hash.go`
2. Move hash-related functions
3. Test

### Phase 7: Extract Cache Operations (Low Risk)
1. Create `check_impl_cache.go`
2. Move cache-related functions
3. Test

### Phase 8: Extract Auto-Fix (Medium Risk)
1. Create `check_impl_autofix.go`
2. Move auto-fix functions
3. Test

### Phase 9: Finalize Core (Low Risk) ✅ COMPLETE
1. ✅ Kept only core orchestration in main file
2. ✅ Cleaned up comment stubs
3. ✅ Final testing passed

## Completion Summary

**Original File:** 3,322 lines  
**Final Core File:** 650 lines  
**Reduction:** 2,672 lines (80% reduction)

**Extracted Files Created:**
1. ✅ `check_impl_helpers.go` (355 lines) - Helper functions
2. ✅ `check_impl_output.go` (399 lines) - Output formatting
3. ✅ `check_impl_audit_buffer.go` (673 lines) - Audit event buffering
4. ✅ `check_impl_validators.go` (466 lines) - Validation checks
5. ✅ `check_impl_integrity.go` (320 lines) - Integrity checks
6. ✅ `check_impl_hash.go` (140 lines) - Hash operations
7. ✅ `check_impl_autofix.go` (421 lines) - Auto-fix operations

**Total Lines in Extracted Files:** 2,774 lines  
**Core File Remaining:** 650 lines  
**Total:** 3,424 lines (slight increase due to file headers and organization)

**Benefits:**
- ✅ Improved maintainability - each file has a focused responsibility
- ✅ Better testability - easier to test individual components
- ✅ Reduced cognitive load - smaller, focused files
- ✅ Better organization - logical grouping of related functions
- ✅ All builds passing - no functionality lost

## Testing Strategy

1. **Unit Tests:** Each extracted file should have corresponding test file
2. **Integration Tests:** Full check operations
3. **Regression Tests:** Run full test suite after each phase
4. **Performance Tests:** Ensure no performance regression

## Risk Mitigation

- **Incremental:** One file at a time
- **Test After Each Phase:** Don't proceed until tests pass
- **Keep Original:** Don't delete original until all phases complete
- **Review Dependencies:** Ensure imports are correct
