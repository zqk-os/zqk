# Lifecycle Definitions as Internal Objects: Design

**Last Verified:** 2026-08-31


**Created:** 2026-01-12  
**Status:** Preferred Implementation  
**Purpose:** Lifecycle definitions are internal objects with built-in immutable defaults and overridable system objects

## Executive Summary

Lifecycle definitions should be **internal objects** (`kind: lifecycle`) that can be managed through the normal object storage system, while maintaining built-in immutable defaults generated from lifecycle builders. This design follows the existing pattern of built-in objects with overrides.

## Current State

### What Exists

1. **Lifecycle Builders** (`pkg/specbuilder/lifecycle_builders/`)
   - Builders generate lifecycle definitions programmatically
   - Versioned builders (e.g., `bldr_lifecycle_v1/`)
   - Generator command: `zqk system generate-lifecycle-builders`

2. **Built-in Object Pattern** (`pkg/storage/builtin.go`)
   - `IsBuiltIn()` function identifies built-in objects
   - Built-in objects are immutable (except with admin role)
   - `source_type: "internal"` indicates system objects

3. **Lifecycle Reference Field** (`extensible_object.yaml`)
   - Objects can reference lifecycle definitions via `lifecycle_ref`
   - Pattern: `^docs/process/_internal/lifecycles/.*\.yaml$`
   - Optional field (defaults to naming convention: `{kind}_lifecycle.yaml`)

4. **Graph Backend Support** (`pkg/zqkcli/get.go`, `list.go`)
   - Code already checks for `kind: lifecycle` objects
   - Can list/get lifecycle definitions as objects

5. **LifecycleLoader** (`pkg/objects/lifecycle_loader.go`)
   - Currently file-based (reads YAML files directly)
   - Caches loaded lifecycles
   - Supports inheritance (`extends`)

## Proposed Design

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│ Built-in Lifecycle Definitions (Immutable)                  │
│ - Generated from lifecycle builders (code)                   │
│ - Embedded in binary or loaded at initialization            │
│ - ID pattern: LIFECYCLE-{ABBR}-001 (e.g., LIFECYCLE-BLI-001) │
│   * Standardized format: LIFECYCLE-[A-Z]+-\d{3,}$            │
│   * ABBR is abbreviation derived from object_type            │
│ - source_type: "built-in"                                   │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ (Fallback if override doesn't exist)
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ LifecycleLoader.resolveLifecycle(kind)                      │
│ 1. Check for override (internal object)                     │
│ 2. Fall back to built-in default                            │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ (Override)
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Internal Lifecycle Objects (Mutable)                        │
│ - Stored as objects (kind: lifecycle)                       │
│ - Can be created/updated via CLI                            │
│ - ID pattern: LIFECYCLE-{kind}-{custom-id}                  │
│ - source_type: "internal"                                   │
│ - object_type field: specifies which object kind uses it    │
└─────────────────────────────────────────────────────────────┘
```

### Lifecycle Object Structure

```yaml
# Internal object (kind: lifecycle)
id: LIFECYCLE-BLI-002      # Standardized format: LIFECYCLE-{ABBR}-{sequence}
kind: lifecycle
object_type: backlog_item  # Which object kind this lifecycle is for
source_type: internal      # vs. "built-in" for defaults
title: "Custom Backlog Item Lifecycle"

# Lifecycle definition fields (same as current YAML structure)
extends: base_lifecycle
statuses:
  - value: exploring
    display: Exploring
    initial: true
  # ... etc
transitions:
  - from: exploring
    to: validated
    # ... etc
percent_complete:
  method: milestone_based
  # ... etc

# Standard object metadata
created_at: "2026-01-12T10:00:00Z"
created_by: "account:system"
updated_at: "2026-01-12T10:00:00Z"
updated_by: "account:system"
```

### Built-in Lifecycle Structure

Built-in lifecycles would be generated from builders and stored similarly, but with:
- `source_type: "built-in"`
- Immutable (cannot be updated/deleted without admin role)
- ID pattern: `LIFECYCLE-{object_type}-{version}` (e.g., `LIFECYCLE-backlog_item-v1_0_0`)

## Implementation Plan

### Phase 1: Create Lifecycle Object Spec

Create `docs/process/_internal/object_specs/lifecycle.yaml`:

```yaml
schema_version: 2.0.0
ontology: lifecycle
extends: base_object
visibility: internal
description: |
  Lifecycle definitions define state machines for object kinds.
  Built-in lifecycles are immutable defaults generated from lifecycle builders.
  Internal lifecycle objects can override defaults for custom workflows.

traits:
  - base_object_traits
  - readable
  - writable  # Only for internal (non-built-in) objects

fields:
  object_type:
    type: string
    required: true
    semantic_type: identifier
    purpose: The object kind this lifecycle defines states for (e.g., "backlog_item", "goal")
    validation:
      pattern: ^[a-z_]+$
      required: true
  
  source_type:
    type: string
    semantic_type: identifier
    default: "internal"
    purpose: "built-in" for immutable defaults, "internal" for overrides
    validation:
      enum: ["built-in", "internal"]
      required: false
  
  # Lifecycle definition fields
  extends:
    type: string
    semantic_type: reference
    purpose: Parent lifecycle to extend (e.g., "base_lifecycle")
    validation:
      required: false
  
  statuses:
    type: list
    semantic_type: list
    purpose: List of valid statuses for this object type
    validation:
      required: true
      minCount: 1
  
  transitions:
    type: list
    semantic_type: list
    purpose: Valid state transitions
    validation:
      required: false
  
  percent_complete:
    type: object
    semantic_type: object
    purpose: Configuration for calculating percent complete
    validation:
      required: false

  # ... (full lifecycle structure as object fields)
```

### Phase 2: Enhance LifecycleLoader

Modify `LifecycleLoader.LoadLifecycle(kind string)` to:

1. **Check for override objects first:**
   ```go
   // Try to find internal lifecycle object for this kind
   lifecycleObj, err := storageProvider.List(ctx, secCtx, storageCtx, filter{
       Kind: "lifecycle",
       Filters: map[string]any{
           "object_type": kind,
           "source_type": "internal",
       },
   })
   
   if len(lifecycleObj.Objects) > 0 {
       // Use override (take first match, or most recent)
       return parseLifecycleFromObject(lifecycleObj.Objects[0])
   }
   ```

2. **Fall back to built-in default:**
   ```go
   // Load built-in lifecycle from builder or embedded files
   return loadBuiltInLifecycle(kind)
   ```

3. **Cache both types** (with separate cache keys for built-in vs. override)

### Phase 3: Initialize Built-in Lifecycles

Create initialization function that:

1. **Generates built-in lifecycle objects from builders:**
   ```go
   func InitializeBuiltInLifecycles(storageProvider ObjectStorageProvider) error {
       registry := lifecycle_builders.GetGlobalRegistry()
       
       for objectType := range registry.GetAllObjectTypes() {
           version, _ := registry.GetLatestVersion(objectType)
           builder, _ := registry.GetBuilder(objectType, version)
           lifecycle := builder.Build()
           
           // Create lifecycle object
           lifecycleObj := lifecycleToObject(lifecycle, objectType, version)
           lifecycleObj["source_type"] = "built-in"
           // ID uses standardized format: LIFECYCLE-{ABBR}-001
           abbreviation := getObjectTypeAbbreviation(objectType)
           lifecycleObj["id"] = fmt.Sprintf("LIFECYCLE-%s-001", abbreviation)
           
           // Store as object (only if doesn't exist)
           if _, err := storageProvider.Read(ctx, secCtx, lifecycleObj["id"]); err == ErrObjectNotFound {
               storageProvider.Create(ctx, secCtx, lifecycleObj)
           }
       }
   }
   ```

2. **Called during system initialization** (or on-demand)

3. **Part of Adding New Object Kinds:**
   
   **Requirement**: When adding a new object kind to the system, creating the built-in lifecycle object MUST be part of that process.

   **Workflow for Adding a New Object Kind:**
   
   1. Create object spec (`docs/process/_internal/object_specs/{kind}.yaml`)
   2. Create lifecycle builder (`pkg/specbuilder/bldr_lifecycle_v1/{kind}_builder.go`)
   3. Register lifecycle builder (builder automatically registers on init)
   4. **Initialize built-in lifecycle object** - This step MUST be included:
      - Call `InitializeBuiltInLifecycleForKind(kind)` (single-kind version of initialization)
      - Or ensure lifecycle is created during bootstrap/init for all kinds
      - The built-in lifecycle object should be created automatically when the object kind is added
   
   **Integration Points:**
   
   - **During `zqk system init`**: All built-in lifecycle objects are initialized for all registered object kinds
   - **During bootstrap file extraction**: Lifecycle objects should be initialized alongside specs
   - **Manual initialization**: New object kinds added to existing projects should trigger lifecycle initialization
   - **CI/CD checks**: Ensure lifecycle builder exists for all object kinds with lifecycle-enabled specs
   
   **Helper Function for Single Kind:**
   ```go
   func InitializeBuiltInLifecycleForKind(objectType string, storageProvider ObjectStorageProvider) error {
       registry := lifecycle_builders.GetGlobalRegistry()
       
       version, err := registry.GetLatestVersion(objectType)
       if err != nil {
           return fmt.Errorf("no lifecycle builder found for %s: %w", objectType, err)
       }
       
       builder, err := registry.GetBuilder(objectType, version)
       if err != nil {
           return fmt.Errorf("failed to get builder for %s@%s: %w", objectType, version, err)
       }
       
       lifecycle := builder.Build()
       lifecycleObj := lifecycleToObject(lifecycle, objectType, version)
       lifecycleObj["source_type"] = "built-in"
       // ID uses standardized format: LIFECYCLE-{ABBR}-001
       abbreviation := getObjectTypeAbbreviation(objectType)
       lifecycleObj["id"] = fmt.Sprintf("LIFECYCLE-%s-001", abbreviation)
       
       // Store as object (only if doesn't exist)
       if _, err := storageProvider.Read(ctx, secCtx, lifecycleObj["id"]); err == ErrObjectNotFound {
           return storageProvider.Create(ctx, secCtx, lifecycleObj)
       }
       
       return nil
   }
   ```

### Phase 4: CLI Commands

Add lifecycle management commands:

```bash
# List lifecycle definitions
zqk internal list lifecycle

# Get specific lifecycle
zqk internal get lifecycle LIFECYCLE-backlog_item-v1_0_0

# Create custom lifecycle (override)
zqk internal create lifecycle --file custom_lifecycle.yaml
# Helper: zqk internal create lifecycle --from-built-in backlog_item --id custom-001

# Update lifecycle (only internal, not built-in)
zqk internal update lifecycle LIFECYCLE-backlog_item-custom-001 --field statuses=...

# Note: Built-in lifecycles are immutable (can't update/delete)
```

### Phase 5: Object Reference Updates

Update `lifecycle_ref` field behavior:

1. **Default behavior:** Use naming convention (`{kind}_lifecycle.yaml`)
   - LifecycleLoader resolves to override object or built-in default

2. **Explicit reference:** `lifecycle_ref: "LIFECYCLE-BLI-002"`
   - LifecycleLoader loads specific lifecycle object by ID (standardized format)

3. **File path reference (legacy):** `lifecycle_ref: "docs/process/_internal/lifecycles/backlog_item_lifecycle.yaml"`
   - Continue to support for backward compatibility
   - LifecycleLoader resolves file path to object if possible

## Benefits

1. **Consistency:** Lifecycles managed like other system metadata (specs, etc.)
2. **Flexibility:** Can override defaults without code changes
3. **Traceability:** Lifecycle changes tracked as object updates (audit events)
4. **Validation:** Spec-based validation ensures lifecycle structure is correct
5. **Tooling:** Can use all object management tools (list, get, update, create)
6. **Backward Compatibility:** File-based loading still works (via object storage)

## Migration Strategy

1. **Phase 1:** Create spec, keep file-based loading
2. **Phase 2:** Add object storage support alongside files
3. **Phase 3:** Initialize built-in lifecycles as objects
4. **Phase 4:** Update LifecycleLoader to prefer objects, fall back to files
5. **Phase 5:** (Optional) Migrate existing lifecycle files to objects
6. **Phase 6:** (Optional) Deprecate file-based loading

## Backward Compatibility

- Continue to support file-based lifecycle definitions during migration
- LifecycleLoader checks objects first, then files
- Existing `lifecycle_ref` file paths continue to work
- Default naming convention (`{kind}_lifecycle.yaml`) resolves through object lookup

## Notes for Implementation

### Helper Notes in CLI

When creating a lifecycle object, provide helpful notes:

```
After creating this lifecycle, update the object spec for {object_type}:
  - Set lifecycle_ref field to: LIFECYCLE-{object_type}-{id}
  - Or rely on default naming convention (lifecycle_ref can be empty)
  
Note: Built-in lifecycles are immutable. To override, create an internal
lifecycle object with object_type="{object_type}" and source_type="internal".
```

### Builder Integration

Lifecycle builders already exist and generate lifecycle definitions. The initialization function can use these builders to create built-in lifecycle objects programmatically, ensuring defaults are always available.

### Integration with Object Kind Addition Process

**Critical Requirement**: Creating the built-in lifecycle object MUST be part of the process for adding a new object kind to the system.

**Why This Matters:**
- Every object kind that uses lifecycle state machines needs a lifecycle definition
- The built-in lifecycle provides the default state machine for that kind
- Without a built-in lifecycle, objects of that kind cannot have valid status values
- The system initialization should automatically ensure all object kinds have their lifecycle objects

**When Lifecycle Initialization Happens:**
1. **System Bootstrap (`zqk system init`)**: All built-in lifecycle objects are created for all registered object kinds
2. **New Object Kind Addition**: When a developer adds a new object kind, they must also:
   - Create the lifecycle builder
   - Ensure the lifecycle object is initialized (either manually or via a helper command)
3. **System Sync/Refresh**: The system should verify all object kinds have lifecycle objects and create missing ones

**Documentation Updates Needed:**
- Update "Adding New Object Types" documentation to include lifecycle initialization step
- Add lifecycle initialization to the guided spec management workflow
- Include lifecycle initialization in CI/CD checks for new object kinds

## Summary

Lifecycle definitions should be internal objects with:
- **Built-in defaults** (immutable, from builders)
- **Override objects** (mutable, via CLI)
- **Spec-based validation** (lifecycle.yaml spec)
- **Backward compatibility** (file-based loading still works)

This design aligns with the existing built-in object pattern and makes lifecycle definitions first-class system objects while maintaining the flexibility of defaults.