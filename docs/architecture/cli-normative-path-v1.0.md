# CLI as Normative Path for Object Operations v1.0

**Last Verified:** 2026-08-31


**Version:** 1.0.0  
**Created:** 2025-12-29  
**Status:** Design Complete  
**Related:** Hash Registry Design v1.0, CLI Ontology v1.0

## Overview

All object operations (create, update, delete) **must** be performed through the CLI. The CLI ensures proper system registration, integrity hash calculation, cache synchronization, and audit trail creation. The `system check` command is for **edge cases only** (detecting manual edits, corruption, migration issues), not the normative path for discovering or reconciling integrity issues.

## Core Principle

**The CLI is the single source of truth for object operations.**

When objects are created, updated, or deleted through the CLI:
1. ✅ Integrity hashes are automatically calculated and registered
2. ✅ Object ID cache is automatically synchronized
3. ✅ Audit events are automatically created
4. ✅ Metadata (created_at, updated_at, created_by, updated_by) is automatically managed
5. ✅ Validation (schema, lifecycle, references) is automatically performed
6. ✅ Security context is properly enforced

## Architecture

### Automatic Hash Registration

The storage layer automatically updates the hash registry for all operations:

#### Create Operation
```go
// pkg/storage/object_storage_file.go:483-508
// After writing object file:
hashRegistry := NewHashRegistry(kind, filepath.Dir(filePath))
hashRegistry.Load()
fileData, _ := os.ReadFile(filePath)
hash := calculateHash(fileData)
hashRegistry.SetHash(filepath.Base(filePath), hash)
hashRegistry.Save()
```

#### Update Operation
```go
// pkg/storage/object_storage_file.go:850-873
// After writing updated object file:
hashRegistry := NewHashRegistry(kind, filepath.Dir(oldFilePath))
hashRegistry.Load()
fileData, _ := os.ReadFile(oldFilePath)
hash := calculateHash(fileData)
hashRegistry.SetHash(filepath.Base(oldFilePath), hash)
hashRegistry.Save()
```

#### Delete Operation
```go
// pkg/storage/object_storage_file.go:1006-1011
// After deleting object file:
hashRegistry := NewHashRegistry(kind, filepath.Dir(filePath))
hashRegistry.Load()
hashRegistry.DeleteHash(filepath.Base(filePath))
hashRegistry.Save()
```

### CLI Command Flow

```
User/Agent → CLI Command → Processor → Storage Layer → Hash Registry
                                      ↓
                                   Object File
                                      ↓
                                   Cache Update
                                      ↓
                                   Audit Event
```

## Why CLI-Only?

### 1. Automatic Integrity Hash Registration

When objects are created through the CLI, hashes are **automatically calculated and stored** in the hash registry. This ensures:
- No manual hash calculation required
- No hash synchronization issues
- Consistent integrity verification from the start

### 2. Cache Synchronization

The CLI automatically updates the object ID cache after create/update/delete operations, ensuring:
- Fast reference integrity checks
- Accurate object existence queries
- No stale cache entries (when using CLI)

### 3. Audit Trail

All CLI operations create audit events automatically:
- Who created/updated/deleted the object
- When the operation occurred
- What changed (for updates)
- Cascade information (for deletes)

### 4. Validation

The CLI performs comprehensive validation:
- Schema validation (field types, required fields, patterns)
- Lifecycle validation (status transitions, required states)
- Reference validation (referenced objects exist)
- ID format validation

### 5. Security Context

The CLI enforces security context:
- Permission checks (read, write, delete)
- CLI authorization (delete operations require CLI context)
- User attribution (created_by, updated_by)

## System Check Command: Edge Cases Only

The `zqk system check` command is **not** the normative path for integrity verification. It is designed for:

### Edge Cases

1. **Manual File Edits**
   - Files edited outside the CLI (e.g., direct YAML editing)
   - Detects hash mismatches from manual changes
   - Can auto-fix missing hashes (with `--auto-fix`)

2. **Migration Issues**
   - Objects created before hash registry was implemented
   - Objects migrated from other systems
   - Legacy objects without registered hashes

3. **Corruption Detection**
   - File system corruption
   - Disk errors
   - Network issues (for graph backend)

4. **Cache Reconciliation**
   - Stale cache entries (with `--clean-cache`)
   - Cache out of sync with file system
   - Bulk cache refresh (with `--refresh-cache`)

### When to Use Check Command

✅ **Use `system check` when:**
- Investigating integrity violations reported by other systems
- After manual file edits (to detect changes)
- During migration or system upgrades
- Periodic health checks (CI/CD, monitoring)
- Debugging integrity issues

❌ **Don't use `system check` for:**
- Normal object operations (use CLI create/update/delete)
- Discovering missing hashes (shouldn't happen if using CLI)
- Regular integrity verification (hashes are registered automatically)

## Best Practices

### 1. Always Use CLI for Object Operations

```bash
# ✅ Correct: Use CLI
zqk object create test_case --file test.yaml

# ❌ Incorrect: Manual file creation
# Don't create files directly - they won't have registered hashes
```

### 2. Use Check Command for Edge Cases

```bash
# ✅ Correct: Check after manual edit
# (After manually editing a file)
zqk system check BLI-625 --auto-fix

# ✅ Correct: Periodic health check
zqk system check all --format json

# ❌ Incorrect: Using check as primary integrity mechanism
# (If you're doing this regularly, you're not using CLI properly)
```

### 3. Verify CLI Operations

```bash
# ✅ Correct: Verify object was created properly
zqk object get TEST-014

# ✅ Correct: Check specific object if needed
zqk system check TEST-014 --verbose
```

## Implementation Details

### Hash Registry Location

**File-Based Backend:**
- Location: `docs/process/{kind_dir}/.{kind}.hashes`
- Format: JSON file with hash map
- Example: `docs/process/tests/.test_case.hashes`

**Graph-Based Backend:**
- Location: Graph node `HashRegistry:{kind}`
- Format: JSON string in node property
- Example: Node ID `HashRegistry:test_case`

### Cache Location

**Object ID Cache:**
- Location: `.zqk/cache/object-id-cache.json`
- Updated automatically by CLI operations
- Used by `system check` for fast reference validation

### Audit Events

**Location:**
- File-Based: `docs/process/audit/YYYY-MM/audit-*.yaml`
- Graph-Based: Audit event nodes in graph

**Automatic Creation:**
- Create operations: `object_created` event
- Update operations: `object_updated` event
- Delete operations: `object_deleted` event

## Migration Path

If you have objects created before this principle was enforced:

1. **Run check command with auto-fix:**
   ```bash
   zqk system check all --auto-fix
   ```

2. **Verify all objects have hashes:**
   ```bash
   zqk system check all --format json | jq '.objects[] | select(.hash_status == "missing")'
   ```

3. **Going forward, use CLI for all operations**

## Benefits

1. **Consistency**: All objects have registered hashes from creation
2. **Reliability**: No hash synchronization issues
3. **Auditability**: Complete audit trail for all operations
4. **Performance**: Cache stays synchronized automatically
5. **Security**: Proper permission checks and user attribution
6. **Validation**: Comprehensive validation before persistence

## Related Documentation

- [Hash Registry Design v1.0](./hash-registry-design-v1.0.md)
- [CLI Ontology v1.0](./cli-ontology-v1.0.md)
- [CLI Standardization v1.0](./cli-standardization-v1.0.md)

