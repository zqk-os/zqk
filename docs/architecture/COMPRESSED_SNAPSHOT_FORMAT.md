# Compressed Snapshot Format Design

**Date**: 2026-01-05  
**Status**: Design  
**Purpose**: Design a compressed snapshot format with explicit expansion rules, similar to HDT (Header, Dictionary, Triples)

## Overview

Snapshots should be stored in a compressed format that:
1. **Reduces storage size** through dictionary-based compression
2. **Enables efficient querying** without full decompression
3. **Provides explicit expansion rules** for reconstruction
4. **Supports incremental/delta snapshots** for versioning

## HDT-Inspired Architecture

Similar to HDT (Header, Dictionary, Triples), our compressed snapshot format has three main components:

### 1. Header
Contains metadata about the snapshot:
- Snapshot version/format identifier
- Timestamp (UTC, nanosecond precision)
- Object count
- Dictionary size
- Compression algorithm used
- Expansion rules version
- Checksums/validation data

### 2. Dictionary
Maps common strings/values to unique identifiers:
- **Field names**: Common field names (id, kind, title, status, etc.)
- **Common values**: Frequently used values (status values, types, etc.)
- **Namespace IDs**: Common namespace identifiers
- **Kind names**: Object kind names
- **Reference patterns**: Common reference patterns

### 3. Data Section
Objects represented using dictionary references:
- Objects encoded using dictionary IDs instead of full strings
- Structure preserved (nested objects, arrays, etc.)
- References to other objects in snapshot
- Delta encoding for incremental snapshots

## Format Structure

```
┌─────────────────────────────────────┐
│           HEADER                    │
│  - Format version                   │
│  - Timestamp                        │
│  - Object count                     │
│  - Dictionary metadata              │
│  - Expansion rules                  │
│  - Checksums                        │
└─────────────────────────────────────┘
┌─────────────────────────────────────┐
│         DICTIONARY                   │
│  - String → ID mappings              │
│  - Value → ID mappings               │
│  - Sorted/indexed for fast lookup   │
└─────────────────────────────────────┘
┌─────────────────────────────────────┐
│           DATA                      │
│  - Objects encoded with dict IDs    │
│  - Structure preserved              │
│  - References maintained            │
└─────────────────────────────────────┘
```

## Dictionary Design

### Dictionary Types

1. **Field Dictionary**: Common field names
   ```
   ID  | String
   ----|--------
   0   | id
   1   | kind
   2   | title
   3   | status
   4   | created_at
   5   | updated_at
   ...
   ```

2. **Value Dictionary**: Common values
   ```
   ID  | String
   ----|--------
   0   | active
   1   | completed
   2   | planned
   3   | in_progress
   4   | zqk:kernel
   5   | backlog_item
   ...
   ```

3. **Reference Dictionary**: Common reference patterns
   ```
   ID  | Pattern
   ----|--------
   0   | BLI-{n}
   1   | GOAL-{n}
   2   | SCH-{n}
   ...
   ```

### Dictionary Compression Strategies

1. **Frequency-based ordering**: Most common strings get lowest IDs (better compression)
2. **Prefix compression**: Similar strings share prefixes
3. **Pattern recognition**: Reference patterns (e.g., "BLI-001", "BLI-002") stored as templates
4. **Type-specific dictionaries**: Separate dictionaries for different value types

## Data Encoding

### Object Encoding

Objects are encoded using dictionary references:

**Original Object:**
```yaml
id: BLI-001
kind: backlog_item
title: Implement feature X
status: active
namespace_id: zqk:kernel
```

**Compressed Representation:**
```json
{
  "0": "BLI-001",      // id (dict ID 0)
  "1": 5,              // kind: backlog_item (dict ID 5)
  "2": "Implement feature X",  // title (not in dict, stored as-is)
  "3": 0,              // status: active (dict ID 0)
  "4": 4               // namespace_id: zqk:kernel (dict ID 4)
}
```

### Expansion Rules

Explicit rules for expanding compressed data:

```yaml
expansion_rules:
  version: "1.0.0"
  dictionary_mapping:
    field_names:
      "0": "id"
      "1": "kind"
      "2": "title"
      "3": "status"
      "4": "namespace_id"
    values:
      "0": "active"
      "1": "completed"
      "4": "zqk:kernel"
      "5": "backlog_item"
  reference_patterns:
    "BLI-{n}": "backlog_item:{n}"
    "GOAL-{n}": "goal:{n}"
  compression_strategies:
    - type: "dictionary_reference"
      apply_to: ["field_names", "common_values"]
    - type: "pattern_matching"
      apply_to: ["object_ids", "references"]
    - type: "delta_encoding"
      apply_to: ["incremental_snapshots"]
```

## Implementation Design

### File Format

**Option 1: Binary Format (Recommended)**
- Header: Fixed-size binary structure
- Dictionary: Indexed binary structure with fast lookup
- Data: Binary-encoded objects
- **Pros**: Maximum compression, fast access
- **Cons**: Requires custom reader/writer

**Option 2: Structured YAML/JSON**
- Header: YAML metadata section
- Dictionary: YAML mapping section
- Data: YAML objects section (with dict references)
- **Pros**: Human-readable, easy to debug
- **Cons**: Less compression, slower parsing

**Option 3: Hybrid (Recommended for MVP)**
- Header: YAML (metadata)
- Dictionary: Binary index + YAML mapping
- Data: Compressed JSON/YAML with dict references
- **Pros**: Balance of readability and compression
- **Cons**: More complex implementation

### Expansion Process

```go
type CompressedSnapshot struct {
    Header     *SnapshotHeader
    Dictionary *SnapshotDictionary
    Data       *SnapshotData
}

type SnapshotHeader struct {
    FormatVersion    string
    Timestamp        time.Time
    ObjectCount      int
    DictionarySize   int
    CompressionAlgo  string
    ExpansionRules   ExpansionRules
    Checksum         string
}

type SnapshotDictionary struct {
    FieldNames map[int]string
    Values     map[int]string
    Patterns   map[int]string
}

type ExpansionRules struct {
    Version           string
    DictionaryMapping map[string]map[int]string
    ReferencePatterns map[string]string
    CompressionStrategies []CompressionStrategy
}

// Expand decompresses a snapshot
func (cs *CompressedSnapshot) Expand() ([]map[string]any, error) {
    // 1. Validate header and checksums
    // 2. Load dictionary
    // 3. Expand each object using dictionary
    // 4. Apply expansion rules
    // 5. Return expanded objects
}
```

## Benefits

1. **Storage Efficiency**: 50-90% reduction in snapshot size
2. **Fast Queries**: Can query compressed data without full expansion
3. **Version Control**: Smaller diffs between snapshots
4. **Explicit Rules**: Clear expansion rules ensure reproducibility
5. **Incremental Snapshots**: Delta encoding for version history

## Use Cases

1. **Snapshot Storage**: Store snapshots efficiently
2. **Scenario Creation**: Fast scenario generation from compressed snapshots
3. **Version History**: Track changes between snapshots
4. **Backup/Restore**: Efficient backup format
5. **Data Migration**: Compressed format for migration scenarios

## Migration Path

1. **Phase 1**: Implement compressed format alongside current format
2. **Phase 2**: Add compression option to snapshot command
3. **Phase 3**: Make compression default for new snapshots
4. **Phase 4**: Add expansion tooling and validation
5. **Phase 5**: Support incremental/delta snapshots

## Requirements and Criteria

### Requirements
- **REQ-039**: Compressed Snapshot Format with Explicit Expansion Rules

### Criteria
- **CRIT-9071**: Dictionary-based compression for common strings
- **CRIT-9072**: Explicit expansion rules for reconstruction
- **CRIT-9073**: Storage efficiency (50-90% reduction)
- **CRIT-9074**: Efficient querying without full decompression
- **CRIT-9075**: Incremental/delta snapshot support
- **CRIT-9076**: Data integrity and validation
- **CRIT-9077**: Lossless compression and expansion

### Backlog Items
- **BLI-708**: Implement Compressed Snapshot Format with Expansion Rules

## Related Documentation

- [Snapshot Implementation Plan](./SNAPSHOT_IMPLEMENTATION_PLAN.md)
- [Timestamp Snapshot Design](./TIMESTAMP_SNAPSHOT_DESIGN.md)
- [CAS Snapshot Integrity](./CAS_SNAPSHOT_INTEGRITY.md)

