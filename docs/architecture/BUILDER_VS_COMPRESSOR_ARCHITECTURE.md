# Builder vs Compressor Architecture

**Last Verified:** 2026-08-31


**Date**: 2026-01-09  
**Status**: Architecture Overview  
**Purpose**: Clarify the relationship between builders (construction) and compressors (compression/decompression)

## Overview

The system has two distinct but complementary concepts:

1. **Builders** - Construct/create things (specs, instances, configs, etc.)
2. **Compressors** - Compress/decompress data for storage efficiency

## Builders (Construction)

Builders are in `pkg/specbuilder/` and create/manage system definitions and instances.

### Spec Builders
**Purpose**: Build object specifications (structure/definition)  
**Location**: `pkg/specbuilder/builders/`, `pkg/specbuilder/bldr_v1/`  
**Output**: `*objects.Spec` (YAML spec files)  
**Example**: `PolicySpecBuilder` creates `policy.yaml` spec file

### Instance Builders
**Purpose**: Build object instances (actual data)  
**Location**: `pkg/specbuilder/instance_builders/`, `pkg/specbuilder/instance_builders/bldr_instance_v1/`  
**Output**: `map[string]any` (YAML or sequence format instance files)  
**Example**: `PolicyInstanceBuilder` creates `POL-CODE-009.yaml` or `POL-CODE-009.seq`

### Other Builders
- **Lifecycle Builders**: `pkg/specbuilder/lifecycle_builders/` - Build lifecycle definitions
- **Config Builders**: `pkg/specbuilder/config_builders/` - Build configuration definitions
- **Profile Builders**: `pkg/specbuilder/profile_builders/` - Build profile definitions
- **Routing Builders**: `pkg/specbuilder/routing_builders/` - Build routing rule definitions
- **Trait Builders**: `pkg/specbuilder/trait_builders/` - Build trait definitions

### Builder Pattern
- **Input**: Programmatic construction (fluent API or code)
- **Output**: YAML files (human-readable definitions/instances)
- **Purpose**: Code-driven, version-controlled generation

## Compressors (Compression/Decompression)

Compressors are in `pkg/storage/` and compress/decompress data for efficient storage.

### Compressed Snapshot Format
**Purpose**: Compress snapshot data for storage efficiency  
**Location**: `pkg/storage/compressed_snapshot.go`  
**Components**:
- **DictionaryBuilder**: Analyzes objects and builds compression dictionary
- **CompressObjects**: Compresses objects using dictionary
- **ExpandObjects**: Decompresses objects back to original format

### Compression Strategy
- **Dictionary Compression**: Maps common strings (field names, values) to IDs
- **Format**: Header + Dictionary + Data
- **Output**: `.csnap` files (compressed snapshots)

## Relationship

### Builders → Compressors
Builders create objects, compressors compress them:

```
Instance Builder
  ↓ creates
Object Instances (YAML/sequence files)
  ↓ snapshot
Compressed Snapshot (dictionary compression)
```

### Sequence Format Bridge
The sequence format from instance builders can enhance compression:

```
Instance Builder (defines field order)
  ↓
Sequence Format (value-only)
  ↓
Compressed Snapshot (dictionary + sequence = even smaller)
```

See `SNAPSHOT_SEQUENCE_FORMAT_ENHANCEMENT.md` for details.

## Key Differences

| Aspect | Builders | Compressors |
|--------|----------|-------------|
| **Purpose** | Construct/create | Compress/decompress |
| **Input** | Code/fluent API | Data structures |
| **Output** | YAML files | Compressed format |
| **Direction** | Creation (one-way) | Bidirectional (compress/expand) |
| **Location** | `pkg/specbuilder/` | `pkg/storage/` |
| **Scope** | System definitions/instances | Storage optimization |

## Design Principles

### Builders
1. **Code-driven**: Built from code, generate YAML
2. **Versioned**: Support multiple versions (v1_0_0, v2_0_0)
3. **Type-specific**: One builder per type (spec, instance, lifecycle, etc.)
4. **Human-readable output**: Generate YAML files

### Compressors
1. **Storage optimization**: Reduce storage size
2. **Reversible**: Must support decompression/expansion
3. **Format-specific**: Optimized for specific use cases (snapshots)
4. **Dictionary-based**: Use frequency analysis for compression

## Future Considerations

### Generic Compression Interface?
Currently compression is snapshot-specific. Should we have:

1. **Generic Compression Interface**:
   ```go
   type Compressor interface {
       Compress(data []map[string]any) (CompressedData, error)
       Expand(CompressedData) ([]map[string]any, error)
   }
   ```

2. **Multiple Compression Strategies**:
   - Dictionary compression (current)
   - Sequence format compression (proposed)
   - Delta compression (for incremental snapshots)
   - Gzip/bzip2 (for additional compression)

3. **Compression Plugins**:
   - Allow different compression strategies
   - Configurable compression level
   - Format-specific optimizations

### Builder-Compressor Integration
The sequence format enhancement demonstrates integration:
- **Builders** define field order (canonical structure)
- **Compressors** use field order for more efficient compression
- **Synergy**: Builders provide metadata, compressors optimize storage

## Summary

- **Builders** = Construction/Creation (specs, instances, configs)
- **Compressors** = Storage Optimization (snapshots, backups)
- **Relationship**: Builders create data, compressors optimize storage of that data
- **Integration**: Sequence format from builders enhances compression efficiency
