# Persistency Layer Gap Analysis

**Last Verified:** 2026-08-31


**Date:** 2025-12-28  
**Status:** Analysis Complete  
**Related:** Object storage API review

## Executive Summary

The persistency layer has a solid foundation with consistent CRUD operations, bulk operations, transactions, and proper privilege handling. However, there are several gaps that impact API fluency, efficiency, and completeness.

## Current State Assessment

### ✅ Strengths

1. **Complete CRUD Operations**
   - Create, Read, Update, Delete all implemented
   - Consistent across file and graph backends
   - Proper validation and metadata management

2. **Bulk Operations**
   - BulkCreate, BulkUpdate, BulkGet, BulkDelete
   - Transaction-based for atomicity
   - Detailed error reporting per object

3. **Query Capabilities**
   - List with filtering, sorting, pagination, grouping
   - Rich filter operators ($eq, $ne, $gt, $in, $contains, etc.)
   - Dynamic synonym resolution (e.g., "current" for priority_plan_ref)
   - Backend-specific Query() for advanced queries

4. **Privilege Handling**
   - All operations check appropriate permissions (read/write/delete)
   - Delete operations require CLI context (security)
   - Admin role has full access
   - Permission checks are consistent across backends

5. **Transaction Support**
   - BeginTransaction() for atomic multi-object operations
   - Transaction interface matches main operations
   - Proper rollback on errors

### ⚠️ Gaps Identified

#### 1. Missing Core Operations

**Exists(id string) (bool, error)**
- **Current Workaround:** Use `Read()` and check for `ErrObjectNotFound`
- **Problem:** Inefficient - loads entire object just to check existence
- **Impact:** Performance overhead for existence checks
- **Priority:** Medium

**Count(filter ListFilter) (int, error)**
- **Current Implementation:** Uses `List()` and counts results
- **Problem:** Loads all objects into memory just to count
- **Impact:** Memory inefficient for large datasets
- **Priority:** High (already have count command, but inefficient)

**Upsert(obj map[string]any) error**
- **Current Workaround:** Check existence, then Create or Update
- **Problem:** Not atomic, race conditions possible
- **Impact:** Requires manual transaction handling
- **Priority:** Medium

**Patch(id string, partialUpdates map[string]any) error**
- **Current Implementation:** `Update()` requires full merge
- **Problem:** Caller must read object, merge, then update
- **Impact:** Extra round-trip, more complex code
- **Priority:** Low (Update works, just less convenient)

#### 2. API Fluency Issues

**No Builder Pattern for Filters**
- **Current:** Manual map construction
  ```go
  filters := map[string]any{
      "status": "active",
      "priority": map[string]any{"$gt": 5},
  }
  ```
- **Desired:** Fluent builder
  ```go
  filter := NewFilter().
      Equal("status", "active").
      GreaterThan("priority", 5).
      Build()
  ```
- **Priority:** Low (nice-to-have, current API works)

**No Chainable Operations**
- **Current:** Separate method calls
- **Desired:** Fluent chaining (if needed)
- **Priority:** Low (Go idioms prefer explicit)

**Inconsistent Error Types**
- **Current:** Mix of `error`, `ErrObjectNotFound`, `ErrPermissionDenied`
- **Problem:** Hard to programmatically handle specific errors
- **Priority:** Medium (affects error handling)

#### 3. Efficiency Concerns

**Count Operation**
- Uses `List()` which loads all objects
- Should use backend-specific count queries
- File backend: count files without parsing
- Graph backend: use COUNT() in Cypher

**List with Large Result Sets**
- No streaming API for large datasets
- All objects loaded into memory
- Could add iterator pattern for large queries
- **Priority:** Low (pagination helps, but not perfect)

#### 4. Privilege Handling Gaps

**Bulk Operations Permission Checks**
- ✅ Currently correct: Each operation in bulk checks permissions
- ✅ Transaction operations check permissions
- **Status:** No gaps identified

**Missing Permission Context**
- All operations require SecurityContext
- No way to check permissions without attempting operation
- Could add `CheckPermission(operation, kind) bool`
- **Priority:** Low (can use Read() to check)

#### 5. API Consistency Issues

**Return Types**
- `Create()` returns `error` (no object returned)
- `Read()` returns `(map[string]any, error)`
- `Update()` returns `error` (no updated object returned)
- `BulkCreate()` returns `*BulkResult` with objects
- **Inconsistency:** Single operations don't return objects, bulk does
- **Priority:** Low (current design is reasonable)

**Error Messages**
- Some operations return generic errors
- Others return specific error types
- Could standardize error messages
- **Priority:** Low (errors are descriptive enough)

## Recommendations

### High Priority

1. **Add Efficient Count Operation**
   ```go
   Count(ctx context.Context, secCtx *SecurityContext, filter ListFilter) (int, error)
   ```
   - File backend: Count files without parsing YAML
   - Graph backend: Use COUNT() in Cypher query
   - Update count command to use this

2. **Add Exists Operation**
   ```go
   Exists(ctx context.Context, secCtx *SecurityContext, id string) (bool, error)
   ```
   - File backend: Check file existence
   - Graph backend: Check node existence
   - More efficient than Read() for existence checks

### Medium Priority

3. **Add Upsert Operation**
   ```go
   Upsert(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error
   ```
   - Atomic create-or-update
   - Useful for idempotent operations
   - Implement via transaction

4. **Standardize Error Types**
   - Create error type hierarchy
   - Make errors programmatically checkable
   - Add error codes for API consumers

### Low Priority

5. **Add Filter Builder (Optional)**
   - Fluent API for building filters
   - Improves readability
   - Not critical - current API works

6. **Add Patch Operation (Optional)**
   - Convenience wrapper around Update
   - Handles partial updates more elegantly
   - Current Update() works fine

## Implementation Plan

### Phase 1: Core Operations (High Priority)
1. Implement `Exists()` in both backends
2. Implement efficient `Count()` in both backends
3. Update count command to use new Count() method
4. Add tests for new operations

### Phase 2: Convenience Operations (Medium Priority)
1. Implement `Upsert()` using transactions
2. Standardize error types
3. Add error type documentation

### Phase 3: API Enhancements (Low Priority)
1. Add filter builder (if needed)
2. Add patch operation (if needed)
3. Consider streaming API for large queries

## Testing Requirements

For each new operation:
- Unit tests for both file and graph backends
- Integration tests with privilege checks
- Performance tests for efficiency improvements
- Error handling tests

## Backward Compatibility

All new operations are additive - existing code continues to work. No breaking changes required.

## Conclusion

The persistency layer is solid and production-ready. The identified gaps are primarily convenience and efficiency improvements rather than critical missing functionality. The highest priority items (Count and Exists) should be implemented to improve performance and API completeness.

