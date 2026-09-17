# Pluggable Validator System Architecture

**Last Verified:** 2026-08-31


**Version**: 1.0  
**Status**: Design  
**Date**: 2025-12-25  
**Related**: BLI-631, REQ-029

## Overview

The validation system is designed to be pluggable, allowing users to specify their own validators while defaulting to a Go-based validator modeled after SHACL patterns.

## Architecture

### Validator Interface

All validators implement the `Validator` interface:

```go
type Validator interface {
    Validate(ctx context.Context, obj map[string]interface{}, kind string, options *ValidationOptions) (*ValidationResult, error)
    Name() string
    SupportsFeature(feature string) bool
}
```

### Validator Registry

The `ValidatorRegistry` manages available validators:

```go
type ValidatorRegistry struct {
    validators map[string]Validator
    defaultValidator string
}
```

### Default Validator: Go Validator (SHACL-Inspired)

The default `GoValidator` is modeled after SHACL patterns:

- **Shapes**: Object specifications (YAML specs)
- **Property Shapes**: Field definitions with constraints
- **Constraints**: SHACL-inspired constraints:
  - `minCount` (required fields)
  - `maxCount` (optional fields)
  - `datatype` (type validation)
  - `pattern` (regex validation)
  - `in` (enum validation)
  - `minLength` / `maxLength` (string length)
  - `lifecycle` (state transitions)
  - `semantic_type` (semantic validation)

## Usage

### Default Usage

```go
// Uses default "go" validator
validator := GetGlobalRegistry().Get("")
result, err := validator.Validate(ctx, obj, "backlog_item", options)
```

### Custom Validator

```go
// Register custom validator
registry := GetGlobalRegistry()
registry.Register("custom", myCustomValidator)

// Use custom validator
validator := registry.Get("custom")
result, err := validator.Validate(ctx, obj, "backlog_item", options)
```

### Configuration

Validators can be configured via:
- Context variables
- Configuration files
- Environment variables
- Command-line flags

## SHACL Mapping

### SHACL Concepts → Go Validator

| SHACL | Go Validator |
|-------|--------------|
| NodeShape | Object Specification |
| PropertyShape | Field Definition |
| sh:targetClass | Object Kind |
| sh:path | Field Name |
| sh:datatype | Field Type |
| sh:minCount | required: true |
| sh:maxCount | required: false |
| sh:pattern | pattern: "regex" |
| sh:in | enum: [...] |
| sh:minLength | min_length: N |
| sh:maxLength | max_length: N |

## Custom Validator Example

```go
type CustomValidator struct {
    // Custom validator implementation
}

func (cv *CustomValidator) Name() string {
    return "custom"
}

func (cv *CustomValidator) SupportsFeature(feature string) bool {
    switch feature {
    case "lifecycle":
        return true
    case "custom_rules":
        return true
    default:
        return false
    }
}

func (cv *CustomValidator) Validate(ctx context.Context, obj map[string]interface{}, kind string, options *ValidationOptions) (*ValidationResult, error) {
    // Custom validation logic
    result := &ValidationResult{
        IsValid: true,
        Errors: []ValidationError{},
        Warnings: []ValidationWarning{},
    }
    
    // Perform validation...
    
    return result, nil
}

// Register
registry := validation.GetGlobalRegistry()
registry.Register("custom", &CustomValidator{})
```

## Benefits

1. **Flexibility**: Users can implement custom validators
2. **Standardization**: Default validator uses SHACL-inspired patterns
3. **Extensibility**: Easy to add new validators
4. **Backward Compatibility**: Existing code continues to work
5. **Performance**: Go validator is fast and efficient

## Future Enhancements

1. **SHACL Validator**: Full SHACL validator integration
2. **JSON Schema Validator**: Support JSON Schema validation
3. **Custom Rule Engine**: Allow spec-defined custom rules
4. **Validator Plugins**: Load validators from external plugins
5. **Validation Chains**: Support multiple validators in sequence


