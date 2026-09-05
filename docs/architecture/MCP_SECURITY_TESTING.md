# MCP Security Testing

**Last Verified:** 2026-08-31


## Overview

Comprehensive security testing suite for the MCP permission system, including brute-force attack attempts, edge cases, and query operation validation.

## Test Coverage

### 1. Brute-Force Security Tests (`TestSecurityModel_BruteForce`)

Attempts to breach the security model by testing various permission/access/unix permission combinations:

- **Deny Scenarios:**
  - No kind-level read, field requires confidential
  - Kind-level read, but field requires admin
  - Kind-level read, field requires confidential, no access
  - Write operation on read-only field (r-x)
  - Execute operation on read-only field (r--)

- **Allow Scenarios:**
  - Kind-level read, unrestricted field
  - Kind-level read, confidential field, has access
  - Admin role, all fields
  - Wildcard access, confidential field
  - Read/execute on r-x field

### 2. Edge Case Tests (`TestSecurityModel_EdgeCases`)

Tests boundary conditions and edge cases:

- Nil security context
- Missing kind
- Empty permissions
- Wildcard read permission
- Field not in spec
- Multiple requires, one matches
- Deactivated user

### 3. Filtering Tests (`TestSecurityModel_Filtering`)

Verifies that field-level filtering respects permissions:

- Restricted fields are excluded from results
- Unrestricted fields are included
- `id` and `kind` are always included

### 4. Grouping Tests (`TestSecurityModel_Grouping`)

Ensures grouping operations respect field-level permissions:

- Restricted fields are filtered even when used for grouping
- Grouping works on non-restricted fields

### 5. Query Logic Tests (`TestSecurityModel_QueryLogic`)

Tests that query operations respect permissions:

- Query results are filtered by field-level permissions
- Restricted fields are not exposed in query results
- Object-level access is checked before field filtering

### 6. Cascading Access Tests (`TestSecurityModel_CascadingAccess`)

Tests cascading access patterns:

- Referenced objects accessible via parent object
- Field-level restrictions still apply to cascading access

### 7. Permission Cycle Detection (`TestSecurityModel_PermissionCycles`)

Detects cycles in permission dependencies:

- Direct cycles (A → B → A)
- Indirect cycles (A → B → C → A)
- Acyclic dependencies

### 8. Object-Parent Field Combinations (`TestSecurityModel_ObjectParentFieldCombinations`)

Tests acyclic access patterns for object.field and object.parent.field combinations:

- Access to parent fields
- Access to parent restricted fields (denied)
- Cascading access to child
- Access to child fields via cascading
- Access to child restricted fields (denied even with cascading)

### 9. Permission Traversal Acyclic (`TestSecurityModel_PermissionTraversalAcyclic`)

Ensures permission traversals are acyclic:

- Simple chains
- Direct cycles
- Indirect cycles
- No dependencies
- Multiple paths, no cycle

### 10. Complex Access Patterns (`TestSecurityModel_ComplexAccessPatterns`)

Tests complex access patterns with multiple restricted fields:

- Limited user vs full user permissions
- Multiple restricted fields (confidential, audit, read-only)
- Write operations on restricted fields

### 11. Privilege Escalation Tests (`TestSecurityModel_PrivilegeEscalation`)

Attempts privilege escalation attacks:

- Low-privilege user accessing admin-only field
- Low-privilege user accessing confidential field
- User writing to read-only field

### 12. Field Permission Combinations (`TestSecurityModel_FieldPermissionCombinations`)

Tests all permission combinations (rwx) with all operations (read/write/execute):

- All 8 permission combinations (---, r--, -w-, --x, rw-, r-x, -wx, rwx)
- All 3 operations (read, write, execute)
- 24 total combinations

### 13. Requirement Combinations (`TestSecurityModel_RequirementCombinations`)

Tests various requirement combinations:

- Single requirement, matches/no match
- Multiple requirements, all match/one missing
- Wildcard access with single/multiple requirements
- No requirements

### 14. Query Security Tests (`TestQuerySecurity_*`)

Tests query operation security:

- Filter restricted fields (rejected)
- Group by restricted fields (rejected)
- Sort by restricted fields (rejected)
- Validate filter/group/sort fields are accessible
- Complex query operations with mixed permissions

## Known Limitations

### Query Filtering/Grouping/Sorting

Currently, the system filters restricted fields from query results, but does not prevent using restricted fields for filtering/grouping/sorting operations. This is a known limitation that should be addressed in the storage layer:

- **Filtering:** Restricted fields should be rejected from filter criteria
- **Grouping:** Restricted fields should be rejected from group-by operations
- **Sorting:** Restricted fields should be rejected from sort-by operations

**Future Work:** Integrate permission validation into the storage layer's `ListFilter` processing to reject restricted fields before query execution.

## Test Execution

Run all security tests:

```bash
go test ./pkg/mcp -run "TestSecurityModel|TestQuerySecurity" -v
```

Run specific test suites:

```bash
# Brute-force tests
go test ./pkg/mcp -run "TestSecurityModel_BruteForce" -v

# Edge cases
go test ./pkg/mcp -run "TestSecurityModel_EdgeCases" -v

# Query security
go test ./pkg/mcp -run "TestQuerySecurity" -v
```

## Test Results

As of the latest run:

- ✅ **Brute-Force Tests:** All passing (11/11 scenarios)
- ✅ **Edge Cases:** All passing
- ✅ **Filtering:** All passing
- ✅ **Grouping:** All passing
- ✅ **Query Logic:** All passing
- ✅ **Cascading Access:** All passing
- ✅ **Permission Cycles:** All passing
- ✅ **Object-Parent Combinations:** All passing
- ✅ **Permission Traversal:** All passing
- ✅ **Complex Access Patterns:** All passing
- ✅ **Privilege Escalation:** All passing
- ✅ **Field Permission Combinations:** All passing (24/24 combinations)
- ✅ **Requirement Combinations:** All passing (7/7 scenarios)
- ⚠️ **Query Security:** Some tests failing (known limitation - filtering/grouping/sorting by restricted fields)

## Security Model Validation

The test suite validates:

1. **Permission Model:** Unix-like permissions (rwx) work correctly
2. **Access Requirements:** Role and access tag requirements are enforced
3. **Field-Level Filtering:** Restricted fields are excluded from results
4. **Cascading Access:** Referenced objects respect parent access
5. **Cycle Detection:** Permission dependencies are acyclic
6. **Privilege Escalation:** Low-privilege users cannot access restricted fields
7. **Query Operations:** Filtering/grouping/sorting respect field permissions (partial)

## Future Enhancements

1. **Storage Layer Integration:** Reject restricted fields from filter/group/sort operations
2. **Performance Testing:** Load testing with large permission caches
3. **Concurrency Testing:** Test permission checks under concurrent access
4. **Property-Based Testing:** Use property-based testing (e.g., QuickCheck) for exhaustive coverage

