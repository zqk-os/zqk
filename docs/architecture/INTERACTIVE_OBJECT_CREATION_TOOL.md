# Interactive Object Creation Tool Proposal

**Version**: 1.0.0  
**Created**: 2026-01-08  
**Status**: Proposal  
**Purpose**: Design a built-in MCP tool that guides object creation through spec-driven field elicitation

## Concept

Create a built-in MCP tool that:
1. **Streams template data** with token placeholders (e.g., `title={title}`)
2. **Detects required fields** from the object spec
3. **Elicits field values** from the agent using MCP's elicitation error mechanism
4. **Validates incrementally** as fields are provided
5. **Creates the object** when all required fields are provided and validation passes

## Key Insight

Use MCP's elicitation error mechanism to create a "conversation loop" that doesn't exit until validation passes. The tool guides the agent through providing all required fields based on the object spec.

**Important**: This approach does NOT use CLI interactive behavior. Instead, it uses MCP protocol-level elicitation (elicitation errors) to create a loop that:
- Returns elicitation errors with required fields
- Agent provides values via tool parameters (not write tool)
- Tool validates and either requests more fields or creates object
- Loop continues until validation passes

## Tool Design

### Tool Name

`zqk_create_object_interactive` or `zqk_create_object_guided`

### Interface

**First Call** (initialization):
```json
{
  "kind": "criteria",
  "partial_fields": {}  // Optional: pre-fill some fields
}
```

**Subsequent Calls** (field filling):
```json
{
  "kind": "criteria",
  "session_id": "uuid-here",  // Track the session
  "field_values": {
    "title": "Bucketing Strategy Integration",
    "category": "acceptance",
    "description": "...",
    // ... more fields
  }
}
```

### Behavior Flow

1. **Initial Call**
   - Load object spec for the kind
   - Extract required fields from spec
   - Generate template with placeholders: `title={title}`, `category={category}`, etc.
   - Return elicitation error listing all required fields
   - Include session ID for state tracking

2. **Field Elicitation**
   - Agent provides field values via tool parameters
   - Tool validates provided fields against spec
   - If more fields needed → return elicitation error with missing fields
   - If all required fields provided → proceed to validation

3. **Validation & Creation**
   - Validate complete object against spec
   - If validation fails → return elicitation error with validation issues
   - If validation passes → create object via CLI
   - Return success with object ID

### Template Generation

Generate template from spec with tokens:
```yaml
id: {id}
kind: criteria
title: {title}
category: {category}  # Required: enum [acceptance, functional, non_functional, ...]
description: {description}
status: {status}  # Optional, default: not_started
validation_method: {validation_method}  # Required
requirement_refs: {requirement_refs}  # Optional: array
```

**Token Format**: `{field_name}` - represents a placeholder that needs to be filled

**Detection**: During stream processing, detect tokens (e.g., `title={title}`) and identify that `title` requires input.

### Elicitation Pattern

Use MCP's elicitation error mechanism to create the loop:

1. **Template Generation**: Generate template with tokens (`title={title}`, `category={category}`)
2. **Token Detection**: Parse template, detect tokens, map to required fields from spec
3. **Elicitation Error**: Return elicitation error with all required fields
4. **Agent Response**: Agent provides field values via tool parameters (NOT write tool)
5. **Validation Loop**: Validate fields, if missing/invalid → return elicitation again
6. **Completion**: When validation passes → create object, return success

**Example Elicitation**:

```go
return nil, NewElicitationError(
    "Missing required fields for criteria object",
    []ElicitationParam{
        ElicitParamWithExample("title", "Title of the criteria", "string", true, "Bucketing Strategy Integration"),
        ElicitParamWithChoices("category", "Category", "enum", true, []interface{}{"acceptance", "functional", "non_functional"}),
        ElicitParamWithExample("description", "Description of the criteria", "string", true, "..."),
        ElicitParamWithExample("validation_method", "How this criteria is validated", "string", true, "manual_review"),
    },
)
```

**Key**: Agent provides values via tool parameters, NOT the write tool. This creates a protocol-level loop using MCP elicitation, not CLI interactivity.

## Implementation Approach

### Option 1: Stateful Session (Recommended)

**Store session state** (in-memory map keyed by session ID):
- Partial object data
- Required fields list
- Validation state

**Pros**:
- Clean separation of calls
- Can handle complex multi-step flows
- Agent can provide fields incrementally

**Cons**:
- Need session management
- State cleanup (timeout expired sessions)
- More complex

### Option 2: Stateless with Full Field Set

**Agent provides all fields in one call**:
- Tool returns elicitation error with ALL required fields
- Agent provides all fields at once
- Single validation and creation

**Pros**:
- Simpler (no session state)
- Faster (single round-trip)
- Stateless (no cleanup needed)

**Cons**:
- Agent must know all fields upfront
- Less guidance (can't validate incrementally)

### Option 3: Hybrid - Stateless with Incremental Validation

**No session state, but validate incrementally**:
- Tool returns template with all placeholders
- Agent provides ALL fields in one call
- Tool validates all fields
- If validation fails → return elicitation with specific errors
- Agent fixes and calls again with all fields

**Pros**:
- No session management
- Can validate all at once
- Clear error messages

**Cons**:
- Agent must provide all fields each time
- Multiple round-trips for validation errors

## Recommended Approach: Stateless with Full Field Set

Use stateless approach with full field set, using MCP elicitation (not CLI interactivity):

1. **First call**: Generate template with tokens, return elicitation with ALL required fields from spec
2. **Agent provides all fields**: Single call with complete field set (via tool parameters)
3. **Validation**: Validate all fields at once against spec
4. **If errors**: Return elicitation with specific validation errors (missing/invalid fields)
5. **If success**: Create object via CLI, return success

**Key Point**: The loop is created by MCP elicitation errors, NOT CLI interactive behavior. The agent uses tool parameters to provide values, not the write tool.

### Example Flow

**Call 1**: Initialize
```json
{
  "kind": "criteria"
}
```

**Response**: Elicitation error
```json
{
  "error": {
    "code": "elicitation_required",
    "message": "Missing required fields for criteria object",
    "elicitation_params": [
      {
        "name": "title",
        "description": "Title of the criteria",
        "type": "string",
        "required": true,
        "example": "Bucketing Strategy Integration"
      },
      {
        "name": "category",
        "description": "Category (acceptance, functional, non_functional, ...)",
        "type": "enum",
        "required": true,
        "enum_values": ["acceptance", "functional", "non_functional", ...],
        "example": "acceptance"
      },
      // ... more fields
    ]
  }
}
```

**Call 2**: Provide all fields
```json
{
  "kind": "criteria",
  "title": "Bucketing Strategy Integration",
  "category": "acceptance",
  "description": "All CRUD operations must load and use bucketing strategy from spec",
  "validation_method": "manual_review",
  "requirement_refs": ["REQ-999"]
}
```

**Response**: Success or validation errors
- If validation fails → Return elicitation with specific field errors
- If validation succeeds → Create object, return object ID

## Integration with Spec System

### Load Spec

```go
specLoader := objects.GetGlobalSpecLoader()
spec, err := specLoader.LoadSpecWithInheritance(kind + ".yaml")
```

### Extract Required Fields

```go
requiredFields := []string{}
for fieldName, fieldDef := range spec.Fields {
    if fieldDef.Required {
        requiredFields = append(requiredFields, fieldName)
    }
}
```

### Generate Template

```go
template := generateTemplateFromSpec(spec)
// Returns: "id: {id}\nkind: criteria\ntitle: {title}\n..."
```

### Validate Fields

```go
validator := validation.NewValidator(specLoader, storageProvider)
result, err := validator.ValidateObject(objData, kind)
if err != nil || len(result.Errors) > 0 {
    // Return elicitation with validation errors
}
```

## Advantages

1. **Spec-Driven**: Uses actual object specs (always up-to-date)
2. **Validation Feedback**: Clear errors for missing/invalid fields
3. **Agent-Friendly**: Guided experience (knows what fields are needed)
4. **No Write Tool Needed**: Agent provides values via tool parameters (not Cursor's write tool)
5. **Proper Routing**: Creates object via CLI (CAS, validation, audit trails)
6. **MCP Protocol-Level Loop**: Uses elicitation errors (not CLI interactivity) - works with any MCP client
7. **Template Streaming**: Provides template with tokens so agent understands structure
8. **Incremental Validation**: Validates as fields are provided, provides clear feedback

## Comparison: Interactive Tool vs. Write Tool

### Interactive Tool (This Proposal)
- ✅ **Guided**: Agent knows exactly what fields are required
- ✅ **Validated**: Validation happens before object creation
- ✅ **CAS-Compliant**: Always routes through CLI (CAS, validation, audit trails)
- ✅ **Spec-Driven**: Uses actual object specs (always accurate)
- ✅ **Protocol-Level**: Uses MCP elicitation (works with any MCP client)
- ✅ **Error-Friendly**: Clear feedback on missing/invalid fields

### Write Tool (Current Problem)
- ❌ **Unguided**: Agent must guess what fields are needed
- ❌ **Bypasses CLI**: Writes files directly (no CAS, no validation, no audit trails)
- ❌ **Policy Violation**: Direct YAML manipulation bypasses project integrity mechanisms
- ❌ **Error-Prone**: Validation errors discovered after creation
- ❌ **Client-Specific**: Cursor IDE built-in (can't be modified/intercepted)

## Challenges

1. **Large Field Sets**: Some objects have many fields (complex forms)
2. **Nested Structures**: How to handle nested objects/arrays?
3. **Enum Values**: Need to provide enum options in elicitation
4. **Default Values**: Should tool apply defaults or ask agent?
5. **Conditional Fields**: Fields that depend on other fields (e.g., if status=X, then Y is required)

## Questions

1. **Session Management**: Stateful (session ID) or stateless (all fields at once)?
2. **Field Ordering**: Present fields in spec order, or prioritize required fields?
3. **Default Values**: Auto-apply defaults or present to agent?
4. **Nested Objects**: How to handle complex nested structures?
5. **Validation Timing**: Validate incrementally or all at once?
6. **Template Format**: Return YAML template, JSON template, or just field list?

## Implementation Details

### Template Generation with Tokens

Use existing `FieldRegistry` to get field information:

```go
fieldRegistry := objects.GetGlobalFieldRegistry()
kindFields, err := fieldRegistry.GetFieldsForKind(kind)
// kindFields.AllFields contains all fields with Required flag, Type, EnumValues, etc.
```

Generate template with tokens:
```yaml
id: {id}
kind: criteria
title: {title}
category: {category}
description: {description}
```

### Token Detection

Parse template to detect tokens:
```go
tokenPattern := regexp.MustCompile(`\{(\w+)\}`)
matches := tokenPattern.FindAllStringSubmatch(template, -1)
// Extract field names from matches
```

### Elicitation Parameter Generation

Convert `FieldInfo` to `ElicitationParam`:

```go
func fieldInfoToElicitationParam(field *objects.FieldInfo) ElicitationParam {
    param := ElicitParam(
        field.Name,
        field.Description,
        field.Type,
        field.Required,
    )
    
    // Add enum choices if applicable
    if field.Type == "enum" && len(field.EnumValues) > 0 {
        choices := make([]interface{}, len(field.EnumValues))
        for i, v := range field.EnumValues {
            choices[i] = v
        }
        param = ElicitParamWithChoices(field.Name, field.Description, field.Type, field.Required, choices)
    }
    
    return param
}
```

### Validation Integration

Use existing validation system:

```go
validator := validation.NewValidator(specLoader, storageProvider)
result, err := validator.ValidateObject(objData, kind)
if err != nil || len(result.Errors) > 0 {
    // Return elicitation with validation errors
    return nil, NewElicitationError("Validation failed", convertValidationErrorsToElicitation(result.Errors))
}
```

### Object Creation

Create object via CLI bridge (ensures CAS, validation, audit trails):

```go
cmdArgs := map[string]any{
    "_command_path": "object create",
    "kind": kind,
    "file": tempFile, // Write provided fields to temp file
}
return ExecuteCLICommandViaMCPWithContext(ctx, cmdArgs, secCtx, initCtx)
```

## Next Steps

1. ✅ Design tool interface (stateless with full field set)
2. Implement spec loading and field extraction (use `FieldRegistry`)
3. Implement template generation with tokens (reuse `generateTemplate` logic)
4. Implement token detection and elicitation parameter generation
5. Implement validation integration (use existing `Validator`)
6. Implement object creation (via CLI bridge)
7. Test with various object kinds (criteria, requirement, backlog_item, etc.)
8. Document usage patterns and examples
