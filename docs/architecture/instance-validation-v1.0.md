# Instance Validation System Architecture

**Version**: 1.0  
**Status**: Design  
**Date**: 2025-12-25  
**Related**: BLI-631, REQ-026, REQ-027, REQ-028

## Problem Statement

The zqk system needs a comprehensive instance validation system that validates object instances against their object specifications. Currently, validation is fragmented:
- Some validation exists in `zqk check` command
- Lifecycle validation is partially implemented
- Field validation rules in specs are not fully enforced
- Semantic types (statement, list, reference, comparison, expression) lack formal definitions

## Requirements

### Core Requirements

1. **Spec-Based Validation**: Validate instances against their object specifications
2. **Lifecycle Validation**: Enforce lifecycle states and transitions from lifecycle definitions
3. **Field Validation**: Enforce type, pattern, enum, range, and required field rules
4. **Semantic Type Validation**: Validate semantic types using formal ontologies
5. **Required Fields Per State**: Validate required fields based on lifecycle state

### Formal Ontology Integration

Research and adopt formal ontologies for semantic types:
- **Basic Formal Ontology (BFO)** - ISO/IEC 21838-2 standardized top-level ontology
- **ISO 15926** - Lifecycle integration standard
- **Schema.org** - Common data types and semantic types
- **ISO 11179** - Data element semantic types and metadata registry
- **OntoUML** - Formal relations and comparisons

## Architecture

### InstanceValidator

```go
type InstanceValidator struct {
    specLoader *objects.SpecLoader
    lifecycleLoader *objects.LifecycleLoader // Future
}

func (iv *InstanceValidator) ValidateInstance(
    obj map[string]interface{},
    kind string,
    currentState string,
) (*ValidationResult, error)
```

### ValidationResult

```go
type ValidationResult struct {
    IsValid  bool
    Errors   []ValidationError
    Warnings []ValidationWarning
}

type ValidationError struct {
    Field   string
    Message string
    Rule    string // "required", "type", "pattern", "enum", "lifecycle"
}
```

### Validation Rules

1. **Type Validation**: Check value matches spec `type` (string, int, list, object, etc.)
2. **Pattern Validation**: Validate string fields against regex patterns
3. **Enum Validation**: Check value is in allowed enum values
4. **Required Validation**: Check required fields are present
5. **Semantic Type Validation**: Validate semantic types (statement, list, reference, etc.)
6. **Lifecycle Validation**: Validate state transitions and required fields per state

### Semantic Type Definitions

Current semantic types (to be formalized):

- **statement**: A declarative statement or assertion (string/text)
- **list**: An ordered collection of items (array/slice)
- **reference**: A reference to another object (string matching ID pattern)
- **comparison**: A comparative relation (string, formal ontology needed)
- **expression**: A computed or derived value (any type, computed)

### Lifecycle Validation

Future enhancement:
- Load lifecycle definitions from `{kind}_lifecycle.yaml` files
- Validate current state is valid
- Validate transition from current to target state is allowed
- Check preconditions for transition
- Validate required fields for target state

## Implementation Plan

### Phase 1: Basic Instance Validation (Complete)
- ✅ Type validation
- ✅ Pattern validation
- ✅ Enum validation
- ✅ Required field validation
- ✅ Basic semantic type validation (placeholder)

### Phase 2: Lifecycle Integration (Complete)
- ✅ Load lifecycle definitions (LifecycleLoader)
- ✅ Validate state transitions
- ✅ Precondition checking
- ⏳ Required fields per state (partial - uses preconditions)

### Phase 3: Formal Ontology Integration
- Research and select ontology(ies)
- Map semantic types to formal definitions
- Implement ontology-based validation
- Document ontology mappings

### Phase 4: Integration
- Integrate with `zqk check` command
- Add validation to object creation/update
- Add validation to MCP tools
- Performance optimization

## Testing Strategy

- Unit tests for each validation rule type
- Integration tests with real specs
- Test invalid instances
- Test lifecycle transitions
- Test semantic type validation

## Future Enhancements

1. **Formal Ontology**: Adopt BFO or ISO 11179 for semantic types
2. **Custom Validators**: Allow spec-defined custom validation functions
3. **Validation Hooks**: Pre/post validation hooks for extensibility
4. **Performance**: Caching of compiled patterns and validators
5. **Error Messages**: More detailed, actionable error messages

