# Field Discovery System v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Status**: Implemented  
**Date**: 2025-12-26  
**Related**: Object Specs, CLI Help Generation, Validation

## Problem Statement

The CLI and validation systems need to know what fields are available for each object kind, but:
1. Fields are defined in object specs with inheritance (base_object, auditable)
2. Common fields should be shown for all objects
3. Specialized fields differentiate between kinds
4. Field information should be cached for performance
5. Help menus should dynamically list available fields

## Solution: FieldRegistry

A cached field discovery system that:
1. **Identifies Common Fields**: Extracts fields from `base_object` and `auditable` specs
2. **Discovers Specialized Fields**: Scans all spec files and identifies kind-specific fields
3. **Groups by Kind**: Organizes fields into common vs specialized per kind
4. **Caches Results**: Stores field information in memory until reload is requested
5. **Provides API**: Exposes methods to get fields for a kind, all kinds, common fields

## Architecture

### FieldRegistry

```go
type FieldRegistry struct {
    specLoader   *SpecLoader
    cache        map[string]*KindFields  // kind -> field information
    commonFields []FieldInfo             // Fields common to all objects
    mu           sync.RWMutex
    loaded       bool
}
```

### FieldInfo

```go
type FieldInfo struct {
    Name         string   // Field name
    Type         string   // Field type (string, integer, list, etc.)
    SemanticType string   // Semantic type (statement, reference, quantity, etc.)
    Traits       []string // Traits (listable, readable, writable, etc.)
    Required     bool     // Whether field is required
    Description  string   // Field description from checklist
    Kind         string   // Object kind this field belongs to (for specialized fields)
    Inherited    bool     // Whether field is inherited from base/auditable
}
```

### KindFields

```go
type KindFields struct {
    Kind              string      // Object kind
    AllFields         []FieldInfo // All fields (common + specialized, sorted)
    CommonFields      []FieldInfo // Fields inherited from base/auditable
    SpecializedFields []FieldInfo // Fields specific to this kind
}
```

## Implementation Details

### Loading Process

1. **Load Base Specs**: Load `base_object.yaml` and `auditable.yaml`
2. **Extract Common Fields**: Extract all fields from base specs (using `ResolvedFields`)
3. **Scan Spec Directory**: Dynamically discover all spec files
4. **Load Each Spec**: Load spec with inheritance resolution
5. **Identify Specialized Fields**: Fields not in common fields are specialized
6. **Cache Results**: Store in memory with thread-safe access

### Field Extraction

- Uses `ResolvedFields` from specs (includes inherited fields)
- Extracts: type, semantic_type, traits, required, description
- Sorts fields alphabetically for consistent ordering
- Groups into common vs specialized

### Caching Strategy

- **Lazy Loading**: Fields loaded on first access
- **Thread-Safe**: Uses `sync.RWMutex` for concurrent access
- **Reload Support**: `Reload()` clears cache and reloads
- **Global Instance**: Singleton pattern via `GetGlobalFieldRegistry()`

## Usage Examples

### Get Fields for a Kind

```go
registry := GetGlobalFieldRegistry()
kindFields, err := registry.GetFieldsForKind("backlog_item")
if err != nil {
    return err
}

// All fields (common + specialized)
for _, field := range kindFields.AllFields {
    fmt.Printf("%s: %s\n", field.Name, field.Description)
}

// Only specialized fields
for _, field := range kindFields.SpecializedFields {
    fmt.Printf("%s (specialized): %s\n", field.Name, field.Type)
}
```

### Get All Kinds

```go
registry := GetGlobalFieldRegistry()
kinds, err := registry.GetAllKinds()
if err != nil {
    return err
}

for _, kind := range kinds {
    fmt.Printf("Available kind: %s\n", kind)
}
```

### Get Common Fields

```go
registry := GetGlobalFieldRegistry()
commonFields, err := registry.GetCommonFields()
if err != nil {
    return err
}

fmt.Printf("Common fields (available for all objects):\n")
for _, field := range commonFields {
    fmt.Printf("  - %s: %s\n", field.Name, field.Description)
}
```

### Reload Cache

```go
registry := GetGlobalFieldRegistry()
err := registry.Reload()
if err != nil {
    return fmt.Errorf("failed to reload fields: %w", err)
}
```

## CLI Integration

### Help Menu Generation

```go
// Generate help for a specific kind
func generateFieldHelp(kind string) string {
    registry := GetGlobalFieldRegistry()
    kindFields, err := registry.GetFieldsForKind(kind)
    if err != nil {
        return fmt.Sprintf("Error: %v", err)
    }

    var help strings.Builder
    help.WriteString(fmt.Sprintf("Available fields for %s:\n\n", kind))
    
    help.WriteString("Common Fields (inherited):\n")
    for _, field := range kindFields.CommonFields {
        help.WriteString(fmt.Sprintf("  - %s (%s): %s\n", 
            field.Name, field.Type, field.Description))
    }
    
    help.WriteString("\nSpecialized Fields:\n")
    for _, field := range kindFields.SpecializedFields {
        help.WriteString(fmt.Sprintf("  - %s (%s): %s\n", 
            field.Name, field.Type, field.Description))
    }
    
    return help.String()
}
```

### Filter/Sort Field Suggestions

```go
// Get filterable fields for a kind
func getFilterableFields(kind string) []string {
    registry := GetGlobalFieldRegistry()
    kindFields, err := registry.GetFieldsForKind(kind)
    if err != nil {
        return []string{}
    }

    var filterable []string
    for _, field := range kindFields.AllFields {
        // Check if field has "filterable" trait
        for _, trait := range field.Traits {
            if trait == "filterable" {
                filterable = append(filterable, field.Name)
                break
            }
        }
    }
    return filterable
}
```

## Performance Considerations

1. **Lazy Loading**: Fields only loaded when first accessed
2. **Caching**: Results cached in memory, no repeated file I/O
3. **Thread-Safe**: Concurrent reads supported via `RWMutex`
4. **Dynamic Discovery**: Automatically discovers new spec files
5. **Sorted Output**: Fields sorted for consistent ordering

## Future Enhancements

1. **Field Validation**: Validate field values against spec
2. **Field Suggestions**: Auto-complete field names in CLI
3. **Field Documentation**: Generate markdown docs from field info
4. **Field Dependencies**: Track field dependencies and relationships
5. **Field Usage Analytics**: Track which fields are most commonly used

