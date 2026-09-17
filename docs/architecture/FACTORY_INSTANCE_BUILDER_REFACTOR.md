# Factory Instance Builder Refactor

**Last Verified:** 2026-08-31


**Date**: 2026-01-09  
**Status**: Design  
**Purpose**: Refactor factories and streaming elicitation logic to use instance builders

## Overview

Update object creation factories and streaming elicitation logic to use instance builders instead of:
- Temporary files + CLI execution
- Manual map[string]any construction
- Direct YAML file writing

## Current State

### HandleCreateObjectInteractive
**Location**: `pkg/mcp/tools_interactive_handlers.go`

**Current Flow**:
1. Use TemplateLoop/TemplateGenerator for validation
2. When complete: Write filled template to temp file
3. Execute CLI command: `zqk object create <kind> --file <temp_file>`
4. Clean up temp file

**Issues**:
- Requires file I/O (temp file)
- Spawns CLI process
- Indirect (goes through CLI bridge)

### Test Object Builder
**Location**: `pkg/storage/test_object_builder.go`

**Current Flow**:
1. Manually construct `map[string]any`
2. Call `populateRequiredFieldsFromConfig()` to fill fields
3. Use storage.Create() directly

**Issues**:
- Manual field construction
- No type safety
- Duplicated logic

## Proposed Refactor

### HandleCreateObjectInteractive

**New Flow**:
1. Load instance builder in background (first call, store in session)
2. Buffer provided values into builder using `SetField()`
3. When complete: `builder.Build()` + create via storage/CLI

**Benefits**:
- No temp files (can use builder directly)
- Can use storage.Create() directly (if storage available)
- Type-safe (via builder)
- Can use sequence format in future

### Implementation Steps

#### Step 1: Extend InteractiveSessionState
Store instance builder reference in session:

```go
type InteractiveSessionState struct {
    SessionID     string
    Kind          string
    LoopState     *LoopState
    Builder       instance_builders.InstanceBuilder // NEW: Builder reference
    SchemaVersion string                            // NEW: Schema version for builder
    CreatedAt     time.Time
    LastUpdatedAt time.Time
}
```

#### Step 2: Refactor HandleCreateObjectInteractive

**First Call (sessionID == "")**:
1. Get schema_version from template generator (already available in TemplateGenerator)
2. Load instance builder from registry: `registry.GetBuilder(kind, schemaVersion)`
3. Store builder in session state
4. Process loop (as before)
5. Buffer provided values: `builder.SetField(fieldName, value)`

**Subsequent Calls (sessionID != "")**:
1. Retrieve session (includes builder reference)
2. Buffer new provided values: `builder.SetField(fieldName, value)`
3. Process loop (as before)

**When Complete (loopState.IsComplete)**:
1. Build instance: `instance, err := builder.Build()`
2. Create via storage or CLI bridge
3. Return result
4. Clean up session

#### Step 3: Update Other Factories

**Test Object Builder** (`pkg/storage/test_object_builder.go`):
- Replace manual `map[string]any` construction
- Use instance builder from registry
- Call `builder.SetField()` for each field
- Use `builder.Build()` + `storage.Create()`

**Scenario Builder** (`cmd/zqk/utility/scenario_builder.go`):
- Similar pattern: use instance builders for generated objects

## API Design

### Instance Builder Registry Integration

```go
// Get builder from registry
registry := instance_builders.GetGlobalRegistry()
builder, err := registry.GetBuilder(kind, schemaVersion)
if err != nil {
    // Fallback: use template-based approach (backward compatibility)
    // OR: return error requiring builder registration
}
```

### Builder Usage Pattern

```go
// Load builder (once per session)
builder, err := registry.GetBuilder(kind, schemaVersion)

// Buffer values as they come in
for fieldName, value := range providedValues {
    builder.SetField(fieldName, value)
}

// When complete, build and create
instance, err := builder.Build()
if err != nil {
    return nil, fmt.Errorf("failed to build instance: %w", err)
}

// Create via storage or CLI bridge
if err := storage.Create(ctx, secCtx, instance); err != nil {
    return nil, fmt.Errorf("failed to create object: %w", err)
}
```

## Schema Version Handling

The template generator already gets schema_version from spec:

```go
// From TemplateGenerator.GenerateTemplateWithTokens
schemaVersion := "2.0.0" // Default fallback
if stg.specLoader != nil {
    specFile := kind + ".yaml"
    if spec, err := stg.specLoader.LoadSpecWithInheritance(specFile); err == nil {
        if spec.SchemaVersion != "" {
            schemaVersion = spec.SchemaVersion
        }
    }
}
```

We can extract this and use it for builder lookup.

## Backward Compatibility

### Option 1: Require Builders (Strict)
- If builder not found, return error
- Requires all object types to have builders

### Option 2: Fallback to Template (Lenient)
- If builder not found, use current template+CLI approach
- Allows gradual migration

**Recommendation**: Option 2 for gradual migration

## Storage Access

The MCP Server currently uses CLI bridge. Options:

1. **Add storage field to Server struct** (preferred long-term)
2. **Use CLI bridge for now, migrate later** (easier short-term)
3. **Use storage factory pattern**

**Recommendation**: Option 2 for now (use CLI bridge), Option 1 later

## Migration Plan

### Phase 1: HandleCreateObjectInteractive ✅ (In Progress)
- Extend session state
- Load builder in background
- Buffer values into builder
- Use builder.Build() + creation

### Phase 2: Test Object Builder
- Refactor `populateRequiredFieldsFromConfig`
- Use instance builders
- Update test helpers

### Phase 3: Scenario Builder
- Use instance builders for generated objects
- Replace manual construction

### Phase 4: Other Factories
- Identify all object creation factories
- Migrate to instance builders

## Open Questions

1. **Storage Access**: How does MCP Server get storage provider?
   - Currently uses CLI bridge
   - Could add storage field to Server struct

2. **Builder Registration**: When are builders registered?
   - Need to ensure builders are registered before use
   - May need initialization code

3. **Error Handling**: What if builder not found?
   - Fallback to template approach?
   - Or require builders?

4. **BaseInstanceBuilder Methods**: Need to verify SetField/SetID/Build exist
   - File appears incomplete (only 39 lines)
   - May need to restore full implementation

## Critical Note

**base_builder.go appears corrupted** - only contains `splitSequenceLine` function, missing:
- Package declaration
- BaseInstanceBuilder struct
- SetField, SetID, Build methods
- NewBaseInstanceBuilder constructor

**Action Required**: Restore base_builder.go before refactor will work
