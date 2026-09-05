# Snapshot Sequence Format Enhancement

**Last Verified:** 2026-08-31


**Date**: 2026-01-09  
**Status**: Proposal  
**Purpose**: Use value-only sequence format from instance builders to further reduce snapshot size

## Overview

The compressed snapshot format currently uses dictionary compression with field references. By leveraging the sequence format from instance builders, we can eliminate field names/IDs from the data section entirely, further reducing snapshot size.

## Current Format

Current compressed snapshots store objects as maps with field IDs:

```yaml
data:
  objects:
    - f1: v5      # field ID 1 (e.g., "id") = value ID 5
      f2: v10     # field ID 2 (e.g., "kind") = value ID 10
      f3: v3      # field ID 3 (e.g., "status") = value ID 3
```

Each object includes field references (`f1`, `f2`, `f3`), which adds overhead.

## Proposed Enhancement: Sequence Format

Store objects as value-only sequences using field order from instance builders:

```yaml
data:
  objects_by_kind:
    policy:
      field_order: [id, kind, status, title, category, ...]  # Reference to instance builder field order
      sequences:
        - [v5, v10, v3, v20, v15, ...]  # Values only, in field order
        - [v6, v10, v3, v21, v15, ...]  # Next policy instance
    goal:
      field_order: [id, kind, status, title, ...]
      sequences:
        - [v7, v11, v3, v22, ...]
```

## Benefits

1. **Further Size Reduction**: Eliminates field IDs from data section entirely
   - Current: `f1: v5, f2: v10, f3: v3` (field IDs + values)
   - Sequence: `v5|v10|v3` (values only, pipe-delimited or array)

2. **Leverages Instance Builders**: Uses canonical field order already defined in instance builders
   - No need to store field order per object
   - Field order stored once per object type

3. **Dictionary Still Benefits**: Value compression (dictionary IDs) still applies
   - Values like "active", "policy", common IDs still compressed
   - Only field name compression is eliminated from data section

4. **Type Grouping**: Objects grouped by kind for efficient processing
   - All policies together, all goals together, etc.
   - Can process/expand by type

## Format Structure

```yaml
header:
  format_version: "1.1.0"  # Increment version for sequence format
  compression_algo: "dictionary+sequence"
  
dictionary:
  values:  # Still compresses common values
    1: "active"
    2: "policy"
    3: "inactive"
    # ...

data:
  objects_by_kind:
    policy:
      schema_version: "2.0.0"
      field_order_id: "policy_2.0.0"  # Reference to instance builder field order
      sequences:
        - "v5|v10|v3|v20|v15"  # Pipe-delimited values (or JSON array)
        - "v6|v10|v3|v21|v15"
    goal:
      schema_version: "2.0.0"
      field_order_id: "goal_2.0.0"
      sequences:
        - "v7|v11|v3|v22"
```

## Implementation Approach

### Option 1: Field Order Registry (Recommended)

1. **Header includes field order registry**:
   ```yaml
   header:
     field_orders:
       policy_2.0.0: [id, kind, status, title, category, ...]
       goal_2.0.0: [id, kind, status, title, ...]
   ```

2. **Data section references field order by ID**:
   ```yaml
   data:
     objects_by_kind:
       policy:
         field_order_id: "policy_2.0.0"
         sequences: [...]
   ```

3. **Benefits**:
   - Field order stored once per (kind, schema_version) combination
   - Can be generated from instance builders automatically
   - Supports multiple schema versions per kind

### Option 2: Instance Builder Integration

1. **Use instance builder registry directly**:
   - Look up field order from `VersionedInstanceBuilderRegistry`
   - No need to store field order in snapshot (reconstructed from builders)

2. **Benefits**:
   - No field order storage in snapshot
   - Always uses latest field order from builders
   - Smaller snapshot files

3. **Considerations**:
   - Requires instance builders to be available during expansion
   - Version compatibility: snapshot schema version must match builder version

## Storage Format Options

### Pipe-Delimited (Compact)
```
v5|v10|v3|v20|v15
```
- Most compact
- Uses instance builder sequence format directly
- Requires escaping for special characters

### JSON Array (Readable)
```json
["v5", "v10", "v3", "v20", "v15"]
```
- More readable
- Standard JSON parsing
- Slightly larger than pipe-delimited

### Binary Format (Most Compact)
- Pack values as binary with length prefixes
- Most efficient but requires custom parsing
- Best for very large snapshots

**Recommendation**: Start with pipe-delimited (matches instance builder sequence format), add JSON array as option.

## Migration Path

1. **Phase 1**: Add sequence format as optional enhancement
   - Keep current format as default
   - Add `--sequence-format` flag to snapshot commands
   - Test size reduction

2. **Phase 2**: Make sequence format default for new snapshots
   - Both formats supported for expansion
   - Migration tool to convert old snapshots

3. **Phase 3**: Optimize field order storage
   - Integrate with instance builder registry
   - Support multiple schema versions

## Expected Size Reduction

**Current format example**:
```yaml
objects:
  - f1: v5
    f2: v10
    f3: v3
    f4: v20
    f5: v15
```
Size: ~50 bytes (with YAML formatting and field IDs)

**Sequence format**:
```
v5|v10|v3|v20|v15
```
Size: ~20 bytes (values only, pipe-delimited)

**Reduction**: ~60% additional reduction on data section (beyond dictionary compression)

Combined with dictionary compression:
- Dictionary compression: 50-90% reduction
- Sequence format: Additional 60% reduction on data section
- **Total**: Potentially 70-95% reduction from original YAML

## Integration with Instance Builders

The sequence format enhancement directly leverages the instance builder system:

1. **Field Order Source**: Use `BaseInstanceBuilder.GetFieldOrder()`
2. **Format Compatibility**: Use same pipe-delimited format as instance builders
3. **Escaping**: Use same escaping logic (`escapeSequenceValue`, `splitSequenceLine`)
4. **Version Awareness**: Support different schema versions per object type

## Open Questions

1. **Backward Compatibility**: How to handle snapshots without field order info?
   - Store field order in snapshot header
   - Or require instance builders for expansion

2. **Nested Objects**: How to handle nested objects/arrays in sequence format?
   - Option A: Store nested structures as JSON/YAML within sequence
   - Option B: Flatten nested structures (more complex)
   - Option C: Use special markers for nested structures

3. **Mixed Formats**: Should snapshots support mixed formats (some objects as maps, some as sequences)?
   - Probably not - choose one format per snapshot for simplicity

4. **Field Order Evolution**: What happens when field order changes between schema versions?
   - Store field order per schema version
   - Support migration/expansion to new field order
