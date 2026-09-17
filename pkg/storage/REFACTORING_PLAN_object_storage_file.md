# Refactoring Plan: object_storage_file.go

**File:** `pkg/storage/object_storage_file.go`  
**Original Size:** 3,619 lines  
**Current Size:** 670 lines (81% reduction)  
**Target:** Split into focused files (~600-2,000 lines each)

## Analysis

### Function Distribution
- **Total Functions:** 118
- **Main Operations:** Create, Read, Update, Delete, List, Bulk, Transaction, Query, Search
- **Helper Functions:** Validation, Filtering, Sorting, Grouping, File I/O, Hash Registry, CAS operations

### Proposed File Structure

#### 1. `object_storage_file_core.go` (~800 lines)
**Purpose:** Core types, initialization, and shared infrastructure

**Contents:**
- `FileObjectStorage` struct definition
- `NewFileObjectStorage()` constructor
- Global variables (cache handlers, lifecycle hooks)
- Helper getters (`GetProjectRoot`, `GetLifecycleLoader`)
- CAS detection (`usesContentAddressableStorage`, `getContentAddressableStorage`)
- Hash registry creation (`newHashRegistry`)
- File path resolution (`getObjectFilePath`)

**Dependencies:** Minimal - core types only

---

#### 2. `object_storage_file_crud.go` (~1,200 lines)
**Purpose:** Core CRUD operations (Create, Read, Update, Delete)

**Contents:**
- `Create()` - Main create operation
- `Read()` - Main read operation  
- `Update()` - Main update operation
- `Delete()` - Main delete operation
- `Move()` - Move operation
- Helper functions:
  - `validateAndPrepareObjectForCreation()`
  - `ensureObjectID()`
  - `prepareObjectPath()`
  - `checkObjectExists()`
  - `validateObjectBeforeCreation()`
  - `marshalObjectForCreation()`
  - `writeObjectToStorage()`
  - `writeObjectToCAS()`
  - `writeObjectToFile()`
  - `finalizeObjectCreation()`
  - `updateReferencesForMovedObject()`
  - `referenceMatches()`
  - `buildNewReference()`
  - `createMoveAuditEvent()`
  - `findDependents()`
  - `objectReferences()`
  - `cascadeDelete()`

**Dependencies:** Core, Helpers

---

#### 3. `object_storage_file_list.go` (~1,500 lines)
**Purpose:** List, Query, Search, Filtering, Sorting, Grouping

**Contents:**
- `List()` - Main list operation
- `Query()` - Query operation
- `Count()` - Count operation
- `Exists()` - Exists check
- Filtering:
  - `matchesFiltersParsed()`
  - `matchesFilters()` (legacy)
  - `matchesFilterOperator()`
  - `matchesFilterOperatorGeneric()`
  - `compareEqual()`
  - `compareValues()`
  - `stringContains()`, `stringStartsWith()`, `stringEndsWith()`
  - `valueInList()`
  - `arrayContains()`, `arrayContainsAll()`, `arrayContainsAny()`
  - `getFieldSemanticType()`
  - `parseTimestampForFilter()`
  - `isDateSemanticType()`
- Sorting:
  - `sortParsedObjectsSlice()`
  - `sortObjects()`
- Grouping:
  - `groupObjects()`
  - `flattenGroups()`
- File collection:
  - `collectFilePaths()`
  - `collectFilePathsWithStrategy()`
  - `collectFilePathsUsingStrategy()`
  - `walkBucketedStorageByDateRange()`
  - `extractTimeRangeFromFilters()`
  - `isBucketInTimeRange()`
  - `hasDateSubdirectories()`
  - `usesBucketedStorage()`
  - `scanIDBasedFiles()`
  - `getBucketStrategyRegistry()`

**Dependencies:** Core, Helpers

---

#### 4. `object_storage_file_bulk.go` (~400 lines)
**Purpose:** Bulk operations

**Contents:**
- `BulkCreate()`
- `BulkUpdate()`
- `BulkGet()`
- `BulkDelete()`

**Dependencies:** Core, CRUD

---

#### 5. `object_storage_file_transaction.go` (~200 lines)
**Purpose:** Transaction support

**Contents:**
- `BeginTransaction()`
- `BeginEnhancedTransaction()`
- `FileObjectTransaction` struct
- `fileTransactionOp` struct
- Transaction methods: `Create()`, `Read()`, `Update()`, `Delete()`, `Commit()`, `Rollback()`

**Dependencies:** Core, CRUD

---

#### 6. `object_storage_file_validation.go` (~800 lines)
**Purpose:** Validation and reference checking

**Contents:**
- `validateObject()`
- `validateObjectBeforeCreation()`
- `validateReferences()`
- `checkForBlockingIssuesBeforeWrite()`
- `detectBatchCreationFromCache()`
- `detectPartialData()`
- `registerValidationError()`
- `getNonBlockingValidationErrors()`
- `normalizeObjectValues()`

**Dependencies:** Core

---

#### 7. `object_storage_file_helpers.go` (~1,200 lines)
**Purpose:** File I/O, hash operations, metadata, permissions

**Contents:**
- File I/O:
  - `writeObjectFile()`
  - `writeObjectFileWithPerm()`
  - `writeObjectFileWithPermAndData()`
  - `writeObjectFileViaQueue()`
  - `readObjectFile()`
  - `readObjectFileViaQueue()`
  - `readObjectFileNoCache()`
- Hash operations:
  - `calculateHash()`
  - `CalculateSHA256Hash()`
  - `saveHashRegistryWithRetry()`
  - `verifyHashRegistrySave()`
- Metadata:
  - `ensureObjectMetadata()`
- Permissions:
  - `checkPermission()`
- Keystore:
  - `prepareKeystoreEntry()`
  - `applyKeystoreAccessControl()`
- Tracking:
  - `trackPersistenceStep()`
- Utilities:
  - `touchProcessDirectory()`
  - `generateID()`
  - `generateIDs()`
  - `getObjectID()`

**Dependencies:** Core

---

#### 8. `object_storage_file_graph.go` (~600 lines)
**Purpose:** Graph-like operations (GetRelated, GetPath, GetNeighbors)

**Contents:**
- `GetRelated()`
- `GetPath()`
- `GetNeighbors()`

**Dependencies:** Core, CRUD, List

---

## Migration Strategy

### Phase 1: Extract Helpers (Low Risk)
1. Create `object_storage_file_helpers.go`
2. Move helper functions (file I/O, hash, metadata, permissions)
3. Test - should compile and pass tests

### Phase 2: Extract Validation (Low Risk)
1. Create `object_storage_file_validation.go`
2. Move validation functions
3. Test

### Phase 3: Extract List/Query (Medium Risk)
1. Create `object_storage_file_list.go`
2. Move List, Query, Count, Exists, filtering, sorting, grouping
3. Test thoroughly

### Phase 4: Extract Bulk (Low Risk) ✅ COMPLETED
1. Created `object_storage_file_bulk.go` (316 lines)
2. Moved BulkCreate, BulkUpdate, BulkGet, BulkDelete
3. Reduced main file by 306 lines
4. All tests passing

### Phase 5: Extract Transaction (Low Risk) ✅ COMPLETED
1. Created `object_storage_file_transaction.go` (182 lines)
2. Moved BeginTransaction, BeginEnhancedTransaction, FileObjectTransaction
3. Reduced main file by 173 lines
4. All tests passing

### Phase 6: Extract Graph Operations (Low Risk) ✅ COMPLETED
1. Created `object_storage_file_graph.go` (404 lines)
2. Moved GetRelated, GetPath, GetNeighbors
3. Reduced main file by 389 lines
4. All tests passing

### Phase 7: Extract CRUD (High Risk - Core Operations) ✅ COMPLETED
1. Created `object_storage_file_crud.go` (2,095 lines)
2. Moved Create, Read, Update, Delete, Move and all helper functions
3. Reduced main file by 2,081 lines
4. All tests passing

### Phase 8: Finalize Core (Low Risk) ✅ COMPLETED
**Final State:**
- Main file is 670 lines (down from 3,619 lines - 81% reduction)
- Contains only core infrastructure:
  - Global variables (cache handlers, lifecycle hooks, test mode mutex)
  - FileObjectStorage struct definition
  - NewFileObjectStorage() constructor
  - Helper getters (GetProjectRoot, GetLifecycleLoader)
  - CAS detection and management (usesContentAddressableStorage, getContentAddressableStorage)
  - Hash registry creation (newHashRegistry)
  - File path resolution (getObjectFilePath)
  - Cache operation handlers (executeCacheOperation, executeCacheOperationWithID)
  - Lifecycle hook handlers (executeLifecycleHook)

**Summary:**
- Total reduction: 2,949 lines extracted (81% reduction)
- File split into 8 focused files:
  - `object_storage_file.go`: 670 lines (core)
  - `object_storage_file_crud.go`: 2,095 lines
  - `object_storage_file_list.go`: 2,070 lines
  - `object_storage_file_helpers.go`: 707 lines
  - `object_storage_file_validation.go`: 682 lines
  - `object_storage_file_graph.go`: 404 lines
  - `object_storage_file_bulk.go`: 316 lines
  - `object_storage_file_transaction.go`: 182 lines
  - `object_storage_file_search.go`: 330 lines (already existed)
- All tests passing
- Refactoring complete!

## Testing Strategy

1. **Unit Tests:** Each extracted file should have corresponding test file
2. **Integration Tests:** Full CRUD operations
3. **Regression Tests:** Run full test suite after each phase
4. **Performance Tests:** Ensure no performance regression

## Risk Mitigation

- **Incremental:** One file at a time
- **Test After Each Phase:** Don't proceed until tests pass
- **Keep Original:** Don't delete original until all phases complete
- **Review Dependencies:** Ensure imports are correct
