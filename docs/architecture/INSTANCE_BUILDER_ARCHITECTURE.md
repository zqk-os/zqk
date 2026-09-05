# Instance Builder Architecture

**Last Verified:** 2026-08-31


**Version**: 2.0.0 (Revised)  
**Status**: Design  
**Date**: 2026-01-09  
**Related**: Spec Builder Pattern, Instance Validation, Compact Storage

## Overview

Instance builders extend the spec builder pattern to programmatically create and manage object instances. Unlike spec builders (one per spec), there's **ONE instance builder per object type** that can handle many instances of that type. Instance builders support both YAML and compact sequence file formats.

## Key Distinction

| Type | Count | Builder Pattern |
|------|-------|----------------|
| **Specs** | ONE per object type | One spec builder per spec version |
| **Instances** | MANY per object type | ONE instance builder per object type |

Example:
- **Policy spec**: ONE spec → ONE `PolicySpecBuilder`
- **Policy instances**: MANY (POL-CODE-009, POL-CODE-010, etc.) → ONE `PolicyInstanceBuilder`

## Problem Statement

Currently, object instances are created:
1. **Manually** via YAML files
2. **Via CLI commands** (`zqk object create`)
3. **Via MCP tools** (`zqk_object_create`)
4. **In tests** using helper functions

There's no programmatic, code-driven way to:
- Create instances with type safety
- Store instances in compact format (sequence files: just values in order)
- Read/write instances efficiently
- Support version-aware instance handling

## Architecture

### Core Concept

**Instance Builder**: A builder that creates and manages instances of a specific object type. One builder handles all instances of that type.

**Sequence File Format**: Compact storage format where instances are stored as ordered values (no field names), reducing storage size significantly.

### Instance Builder Capabilities

An instance builder can:
1. **Build instances programmatically** (fluent API)
2. **Load instances from YAML files** (full format with field names)
3. **Load instances from sequence files** (compact format: values in order)
4. **Write instances to YAML** (human-readable)
5. **Write instances to sequence format** (compact storage)

### Sequence File Format

Sequence files store instances more compactly by:
- Omitting field names (builder knows the order)
- Storing only values in a canonical order
- Reducing file size significantly (especially for many instances)

Example:
```yaml
# YAML format (verbose)
id: POL-CODE-009
kind: policy
schema_version: 2.0.0
title: Code Quality Maintenance
category: code_quality
status: active
# ... many more fields

# Sequence format (compact) - just values in order
POL-CODE-009|policy|2.0.0|Code Quality Maintenance|code_quality|active|...
```

The builder knows the field order, so it can serialize/deserialize without field names.

## Design

### InstanceBuilder Interface

```go
// InstanceBuilder is an interface for builders that create and manage object instances
type InstanceBuilder interface {
    // Build creates a new instance map (fluent API construction)
    Build() (map[string]any, error)
    
    // GetKind returns the object kind this builder handles
    GetKind() string
    
    // GetSchemaVersion returns the schema version this builder supports
    GetSchemaVersion() string
    
    // LoadFromYAML loads an instance from a YAML file
    LoadFromYAML(yamlPath string) (map[string]any, error)
    
    // LoadFromSequence loads an instance from a sequence file (compact format)
    LoadFromSequence(sequencePath string) (map[string]any, error)
    
    // WriteToYAML writes an instance to a YAML file
    WriteToYAML(instance map[string]any, yamlPath string) error
    
    // WriteToSequence writes an instance to a sequence file (compact format)
    WriteToSequence(instance map[string]any, sequencePath string) error
}
```

### BaseInstanceBuilder

```go
// BaseInstanceBuilder provides common functionality for instance builders
type BaseInstanceBuilder struct {
    kind          string
    schemaVersion string
    fieldOrder    []string  // Canonical field order for sequence format
    specBuilder   SpecBuilderInterface  // Optional: for field definitions
}

// Field Order
// System fields first (id, kind, schema_version, created_at, etc.)
// Then spec-defined fields in canonical order
func (b *BaseInstanceBuilder) GetFieldOrder() []string {
    order := []string{
        "id",
        "kind", 
        "schema_version",
        "created_at",
        "created_by",
        "updated_at",
        "updated_by",
        // ... system fields
    }
    // Append spec-defined fields from spec builder
    // ... 
    return order
}

// LoadFromYAML loads instance from YAML file
func (b *BaseInstanceBuilder) LoadFromYAML(yamlPath string) (map[string]any, error) {
    // Read and parse YAML file
    // Return instance map
}

// LoadFromSequence loads instance from sequence file
func (b *BaseInstanceBuilder) LoadFromSequence(sequencePath string) (map[string]any, error) {
    // Read sequence file (values in order, separated by delimiter)
    // Map values to fields using fieldOrder
    // Return instance map
}

// WriteToYAML writes instance to YAML file
func (b *BaseInstanceBuilder) WriteToYAML(instance map[string]any, yamlPath string) error {
    // Marshal instance to YAML
    // Write to file
}

// WriteToSequence writes instance to sequence file
func (b *BaseInstanceBuilder) WriteToSequence(instance map[string]any, sequencePath string) error {
    // Get field order
    // Extract values in order
    // Write as sequence (e.g., pipe-delimited or CSV)
}
```

### Example: PolicyInstanceBuilder

```go
type PolicyInstanceBuilder struct {
    *BaseInstanceBuilder
}

func NewPolicyInstanceBuilder(schemaVersion string) *PolicyInstanceBuilder {
    // Define field order for sequence format
    fieldOrder := []string{
        "id", "kind", "schema_version",
        "title", "category", "policy_type", "status",
        "body", "examples", "related_patterns",
        // ... all fields in canonical order
    }
    
    builder := &PolicyInstanceBuilder{
        BaseInstanceBuilder: NewBaseInstanceBuilder("policy", schemaVersion, fieldOrder),
    }
    
    return builder
}

// Fluent API methods
func (b *PolicyInstanceBuilder) SetTitle(title string) *PolicyInstanceBuilder {
    b.SetField("title", title)
    return b
}
```

### Registry Pattern

```go
// VersionedInstanceBuilderRegistry manages instance builders by kind and version
type VersionedInstanceBuilderRegistry struct {
    builders map[string]map[string]InstanceBuilder  // kind -> schema_version -> builder
}

// GetBuilder returns the builder for a kind and schema version
func (r *VersionedInstanceBuilderRegistry) GetBuilder(kind, schemaVersion string) (InstanceBuilder, error)

// Unlike spec builders, we have ONE builder per kind/version (not one per instance)
```

## Usage Examples

### Creating Instances Programmatically

```go
builder := NewPolicyInstanceBuilder("2.0.0")
instance, err := builder.
    SetID("POL-CODE-009").
    SetTitle("Code Quality Maintenance").
    SetCategory("code_quality").
    Build()
```

### Loading from YAML

```go
builder := NewPolicyInstanceBuilder("2.0.0")
instance, err := builder.LoadFromYAML("docs/process/policies/POL-CODE-009.yaml")
```

### Loading from Sequence File

```go
builder := NewPolicyInstanceBuilder("2.0.0")
instance, err := builder.LoadFromSequence("docs/process/policies/POL-CODE-009.seq")
```

### Writing to Compact Format

```go
builder := NewPolicyInstanceBuilder("2.0.0")
instance := map[string]any{
    "id": "POL-CODE-009",
    "title": "Code Quality Maintenance",
    // ...
}
err := builder.WriteToSequence(instance, "policies/POL-CODE-009.seq")
```

## Field Order Definition

Field order for sequence format can be:
1. **Canonical order**: System fields first, then spec fields in defined order
2. **From spec builder**: Extract field order from spec builder (if available)
3. **Explicit definition**: Builder defines field order explicitly

Recommended approach: **Explicit definition** in builder for clarity and control.

## Sequence File Format Details

### Delimiter Options

- **Pipe-delimited**: `value1|value2|value3`
- **Tab-delimited**: `value1\tvalue2\tvalue3`
- **CSV**: `value1,value2,value3` (with escaping for commas)
- **JSON Lines**: One JSON array per line

Recommended: **Pipe-delimited** for simplicity and readability.

### Escaping

Values containing the delimiter must be escaped:
- Escape character: `\`
- Example: `value with \| pipe` → stored as `value with \\| pipe`

### Multi-line Values

Multi-line values (e.g., `body` field with markdown) can be:
- Escaped with newlines: `line1\nline2\nline3`
- Or use a different format (base64, length-prefixed)

## Benefits

1. **Storage Efficiency**: Sequence files are much smaller than YAML
2. **Performance**: Faster parsing (no YAML parsing overhead)
3. **Consistency**: One builder per type ensures consistent handling
4. **Flexibility**: Support both human-readable (YAML) and compact (sequence) formats
5. **Snapshot Optimization**: Sequence format can be used in compressed snapshots for even smaller storage (see `SNAPSHOT_SEQUENCE_FORMAT_ENHANCEMENT.md`)

## Implementation Plan

### Phase 1: Core Infrastructure ✅
- BaseInstanceBuilder with YAML support
- InstanceBuilder interface
- Registry

### Phase 2: Sequence Format Support
- Field order definition
- Sequence file reading/writing
- Escaping and multi-line value handling

### Phase 3: Example Implementation
- PolicyInstanceBuilder with sequence support
- Test with existing policy instances

### Phase 4: Code Generation
- Generate instance builders from spec builders
- Auto-generate field order from spec

## Open Questions

1. **Field Order Source**: Should field order come from spec builder, or be explicitly defined?
   - **Recommendation**: Explicitly defined for clarity

2. **Sequence Format**: What delimiter and escaping strategy?
   - **Recommendation**: Pipe-delimited with backslash escaping

3. **Multi-line Values**: How to handle fields with newlines?
   - **Options**: Escape sequences, base64, length-prefixed
   - **Recommendation**: Escape sequences (`\n`)

4. **Backward Compatibility**: How to handle instances without sequence files?
   - Always support YAML as fallback
   - Sequence format is optional optimization
