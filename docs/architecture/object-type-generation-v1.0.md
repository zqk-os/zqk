# Object Type Generation v1.0 - "Croptop" Approach

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Status**: Implemented  
**Date**: 2025-12-26  
**Related**: BLI-634, Object Storage Provider

## Problem Statement

Currently, all objects are represented as `map[string]any` throughout the codebase. This provides flexibility but has significant downsides for complex nested structures:

1. **No Type Safety for Complex Types**: Status history, change logs are just `[]interface{}`
2. **Runtime Errors**: Field access errors only discovered at runtime
3. **Poor Developer Experience**: Hard to work with nested structures in Go code
4. **No IDE Support**: No autocomplete for complex nested types

## Solution: Hybrid "Croptop" Approach

Keep objects as `map[string]any` for flexibility (lightweight, extensible), but use typed Go structs for complex nested structures (type-safe, better DX).

**Philosophy**: Lightweight and flexible like a croptop, structured where it matters.

## Design

### Generated Structs

For each object spec (e.g., `backlog_item.yaml`), generate a Go struct:

```go
type BacklogItem struct {
    BaseObject
    
    // Backlog item specific fields
    Category         string   `yaml:"category,omitempty" json:"category,omitempty"`
    Priority         string   `yaml:"priority,omitempty" json:"priority,omitempty"`
    DateCaptured     *time.Time `yaml:"date_captured,omitempty" json:"date_captured,omitempty"`
    PriorityPlanRef  string   `yaml:"priority_plan_ref,omitempty" json:"priority_plan_ref,omitempty"`
    
    // References
    GoalRefs         []string `yaml:"goal_refs,omitempty" json:"goal_refs,omitempty"`
    MilestoneRefs    []string `yaml:"milestone_refs,omitempty" json:"milestone_refs,omitempty"`
    // ...
}
```

### Base Object

All objects embed `BaseObject` which contains common fields:

```go
type BaseObject struct {
    ID            string    `yaml:"id" json:"id"`
    Kind          string    `yaml:"kind" json:"kind"`
    SchemaVersion string    `yaml:"schema_version" json:"schema_version"`
    Title         string    `yaml:"title,omitempty" json:"title,omitempty"`
    CreatedAt     time.Time `yaml:"created_at" json:"created_at"`
    CreatedBy     string    `yaml:"created_by" json:"created_by"`
    // ... all common fields from base_object.yaml
}
```

### Type Mapping

Map spec field types to Go types:

| Spec Type | Go Type | Notes |
|-----------|---------|-------|
| `string` | `string` | Direct mapping |
| `integer` | `int` or `*int` | Pointer if optional |
| `float` | `float64` or `*float64` | Pointer if optional |
| `boolean` | `bool` or `*bool` | Pointer if optional |
| `date` | `time.Time` or `*time.Time` | Pointer if optional |
| `list` | `[]string`, `[]int`, etc. | Based on item type |
| `enum` | `string` | Enum values validated at runtime |
| `object` | `map[string]any` | Complex nested objects |
| `reference` | `string` or `[]string` | Reference IDs |

### Generation Strategy

#### Option 1: Code Generation Tool

Create a tool that:
1. Scans `docs/process/_internal/object_specs/`
2. Parses each YAML spec
3. Generates Go structs in `pkg/objects/types.go` (or separate files)
4. Can be run via `go generate` or `make generate`

**Pros:**
- Single source of truth (specs)
- Automatic updates when specs change
- Can include field documentation from specs

**Cons:**
- Requires build step
- Generated code needs to be committed

#### Option 2: Runtime Type Generation

Generate structs at runtime using reflection or code generation libraries.

**Pros:**
- No build step
- Always in sync with specs

**Cons:**
- More complex
- Less IDE support
- Performance overhead

#### Option 3: Hybrid Approach

1. Generate base structs from specs (Option 1)
2. Provide conversion utilities between maps and structs
3. Storage layer can work with either

**Recommended**: Option 3 (Hybrid)

### Conversion Utilities

Provide utilities to convert between maps and structs:

```go
// Convert map to typed struct
func ToBacklogItem(m map[string]any) (*BacklogItem, error) {
    // Uses reflection or manual mapping
}

// Convert typed struct to map
func (b *BacklogItem) ToMap() map[string]any {
    // Uses reflection or manual mapping
}
```

### Storage Layer Integration

Storage layer can accept either:

```go
// Option 1: Map (current, backward compatible)
storage.Create(ctx, secCtx, map[string]any{...})

// Option 2: Typed struct (new, type-safe)
backlogItem := &BacklogItem{
    BaseObject: BaseObject{
        ID: "BLI-001",
        Kind: "backlog_item",
        // ...
    },
    Category: "Product",
    Priority: "high",
}
storage.CreateTyped(ctx, secCtx, backlogItem)
```

### Implementation Plan

1. **Phase 1: Manual Structs** (Current)
   - Create `pkg/objects/types.go` with manual struct definitions
   - Provide conversion utilities
   - Update storage layer to accept both maps and structs

2. **Phase 2: Code Generation**
   - Create code generation tool
   - Generate structs from specs
   - Add `go generate` directive

3. **Phase 3: Full Integration**
   - Update all code to use typed structs where beneficial
   - Keep map support for dynamic operations
   - Add comprehensive tests

## Benefits

1. **Type Safety**: Compile-time checking prevents runtime errors
2. **Better DX**: IDE autocomplete and refactoring support
3. **Documentation**: Field types visible in code
4. **Performance**: Structs are more efficient than maps
5. **Backward Compatible**: Map-based code continues to work

## Migration Strategy

1. Add struct definitions alongside existing map-based code
2. Provide conversion utilities
3. Gradually migrate code to use structs
4. Keep map support for dynamic/flexible operations
5. Eventually, most code uses structs, maps only for edge cases

## Example Usage

```go
// Create with typed struct
backlogItem := &BacklogItem{
    BaseObject: BaseObject{
        ID: "BLI-001",
        Kind: "backlog_item",
        Title: "New Feature",
        Status: "exploring",
        SchemaVersion: "2.0.0",
        CreatedAt: time.Now(),
        CreatedBy: "account:user",
    },
    Category: "Product",
    Priority: "high",
    GoalRefs: []string{"goal:GOAL-001"},
}

// Type-safe access
fmt.Println(backlogItem.Category)  // Compile-time checked
fmt.Println(backlogItem.GoalRefs)  // Type-safe slice

// Convert to map for storage (if needed)
m := backlogItem.ToMap()
storage.Create(ctx, secCtx, m)
```

## Related Work

- Object Storage Provider interface already supports `map[string]any`
- Spec loader already parses object specifications
- Validation system works with both maps and could work with structs

