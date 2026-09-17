# UPDATE Loop Design

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-08  
**Status**: Design Document  
**Purpose**: Design the interactive UPDATE loop for object modification via MCP elicitation

## Overview

The UPDATE loop is similar to the CREATE loop but has key differences:
- Requires object ID to load existing object
- Allows partial updates (only specified fields)
- Must validate lifecycle transitions
- Shows diff/preview before confirmation
- Requires explicit confirmation before applying changes

## Design Questions

### 1. How does the system know what object to load?

**Options:**

**Option A: Object ID as initial parameter**
- User provides object ID when initiating update
- System loads object immediately
- Pros: Clear, explicit, matches CLI pattern
- Cons: User must know object ID

**Option B: Object ID as first elicitation**
- System first asks for object ID
- Then loads object and proceeds
- Pros: More interactive, can validate ID exists
- Cons: Extra round-trip

**Option C: Object identifier (ID, title, etc.)**
- User can provide ID, title, or other identifier
- System resolves to object ID
- Pros: More flexible, user-friendly
- Cons: More complex, potential ambiguity

**Recommendation: Option A (Object ID as parameter)**
- Matches CLI pattern (`zqk object update <id>`)
- Clear and explicit
- Can extend later to support identifiers if needed

### 2. How does the system know what field or fields are being updated?

**Options:**

**Option A: Explicit field specification**
- User specifies which fields to update (e.g., `--field title --field status`)
- Only those fields are elicited
- Pros: Clear intent, efficient
- Cons: User must know field names

**Option B: Implicit detection (all fields)**
- System loads object, shows all fields with current values
- User provides new values for fields they want to change
- System compares and detects changes
- Pros: Flexible, user-friendly
- Cons: More complex, potential for accidental changes

**Option C: Hybrid (fields parameter + detection)**
- User can optionally specify fields to update
- If not specified, system detects changes from provided values
- Pros: Flexible, supports both patterns
- Cons: More complex implementation

**Option D: Iterative field-by-field**
- System shows one field at a time (or group of related fields)
- User confirms each change
- Pros: Very controlled, clear changes
- Cons: Many round-trips, verbose

**Recommendation: Option C (Hybrid)**
- Supports explicit field specification (matches CLI pattern)
- Falls back to change detection if fields not specified
- Provides flexibility while maintaining efficiency

### 3. How does the system know when the user has provided all of the updates and is ready to confirm?

**Options:**

**Option A: Explicit confirmation command**
- User provides all field values
- System shows diff/preview
- User sends confirmation command/parameter
- Pros: Clear, explicit, prevents accidental changes
- Cons: Extra step

**Option B: Automatic after validation passes**
- System validates changes
- If valid, automatically applies (no confirmation)
- Pros: Faster, fewer round-trips
- Cons: Risk of accidental changes

**Option C: Confirmation flag in final request**
- User provides all field values + confirmation flag
- System validates and applies
- Pros: Single round-trip for confirmation
- Cons: Less interactive

**Option D: Iterative confirmation**
- After each field change, system asks "Continue updating? (yes/no/field_name)"
- User can continue or confirm
- Pros: Very controlled, step-by-step
- Cons: Many round-trips, verbose

**Recommendation: Option A (Explicit confirmation)**
- Shows diff/preview before applying
- Requires explicit confirmation (prevents accidental changes)
- Matches user expectation for destructive operations
- Can optimize later if needed

## UPDATE Loop Flow

### Phase 1: Initialization
1. User initiates update with object ID (and optionally field names)
2. System loads existing object
3. System validates object exists and is readable
4. System loads lifecycle for validation

### Phase 2: Template Generation
1. System generates template with current values (not tokens)
2. If fields specified, only show those fields
3. If no fields specified, show all mutable fields
4. System filters out immutable fields (created_at, created_by, etc.)

### Phase 3: Value Elicitation
1. System shows current values for fields to update
2. User provides new values (via MCP elicitation)
3. System validates each value (type, constraints, lifecycle transitions)
4. System detects which fields have changed
5. Loop continues until:
   - User provides values for all requested fields, OR
   - User indicates they're done (e.g., empty field list)

### Phase 4: Validation & Preview
1. System validates all changes together
2. System checks lifecycle transitions (if status changed)
3. System generates diff/preview of changes
4. System returns preview to user

### Phase 5: Confirmation
1. User reviews preview
2. User sends confirmation (explicit parameter/command)
3. System applies changes
4. System returns success/error

## State Management

```go
type UpdateLoopState struct {
    ObjectID          string                 // Object ID being updated
    ExistingObject    map[string]interface{} // Current object state
    FieldsToUpdate    []string               // Fields to update (if specified)
    ProvidedValues    map[string]interface{} // New values provided by user
    ChangedFields     []string               // Fields that have changed
    ValidationErrors  map[string]string      // Field-level validation errors
    LifecycleErrors   []string               // Lifecycle transition errors
    IsValid           bool                   // Whether all changes are valid
    PreviewDiff       string                 // Diff/preview of changes
    AwaitingConfirmation bool                // Whether waiting for confirmation
}
```

## Key Differences from CREATE Loop

1. **Object Loading**: UPDATE requires loading existing object first
2. **Field Filtering**: UPDATE filters immutable fields (created_at, created_by) but allows updates to mutable fields
3. **Change Detection**: UPDATE compares new values with existing values
4. **Lifecycle Validation**: UPDATE validates transitions, not just initial state
5. **Partial Updates**: UPDATE supports updating only specified fields
6. **Confirmation**: UPDATE requires explicit confirmation before applying changes
7. **Preview/Diff**: UPDATE shows what will change before confirmation

## Implementation Considerations

### Immutable Fields
Fields that should NOT be updatable:
- `created_at` (immutable)
- `created_by` (immutable)
- `id` (immutable)
- `kind` (immutable)
- `schema_version` (system-managed)

Fields that ARE updatable but auto-managed:
- `updated_at` (auto-updated on save)
- `updated_by` (auto-updated on save)
- `status` (requires lifecycle transition validation)

### Lifecycle Transition Validation
- Must validate status changes against lifecycle transitions
- Must check preconditions for target status
- Must validate transition is allowed (manual vs auto)

### Partial Update Semantics
- If field not provided, keep existing value
- If field provided with empty value, clear field (if allowed)
- If field provided with value, update field
- Validate only changed fields (optimization)

### Error Handling
- Object not found → return error immediately
- Field not found → return error for that field
- Validation error → continue loop, show error
- Lifecycle error → continue loop, show error
- Confirmation timeout → abort update

## Example Flow

```
1. User: "Update object CRIT-9080, change title and category"
   
2. System: Loads CRIT-9080
           Generates template with current values:
           title: "Bucketing Strategy Integration for All CRUD Operations"
           category: "acceptance"
           [Other fields...]
   
3. System: Elicits new values:
           - title: {title}
           - category: {category}
   
4. User: Provides values:
           title: "Updated Title"
           category: "acceptance"
   
5. System: Detects changes:
           - title: changed
           - category: unchanged
           Validates changes
           Shows preview:
           --- title
           +++ title
           - Bucketing Strategy Integration for All CRUD Operations
           + Updated Title
   
6. System: Requests confirmation
   
7. User: Confirms
   
8. System: Applies changes
           Returns success
```

## Next Steps

1. Implement `UpdateLoopState` struct
2. Implement `UpdateLoopProcessor` (similar to `StreamingTemplateLoop`)
3. Implement object loading and change detection
4. Implement lifecycle transition validation
5. Implement diff/preview generation
6. Implement confirmation mechanism
7. Wire into MCP tool
