# Semantic Types and Formal Ontology Integration

**Version**: 1.0  
**Status**: Design  
**Date**: 2025-12-29  
**Related**: BLI-631, REQ-027, REQ-029

## Overview

This document describes the formal ontology integration for semantic type validation in the zqk system. It provides mappings between our semantic types and formal ontologies, and outlines the validation approach.

## Current Semantic Types

The zqk system currently defines the following semantic types:

1. **statement**: A declarative statement or assertion
2. **list**: An ordered collection of items
3. **reference**: A reference to another object
4. **comparison**: A comparative relation
5. **expression**: A computed or derived value

## Formal Ontology Selection

After research, we have selected the following ontologies for semantic type validation:

### Primary: Schema.org

**Rationale:**
- Widely adopted and maintained
- Comprehensive vocabulary for common data types
- Good support for statements, references, and structured data
- JSON-LD compatible
- Well-documented with active community

**Key Mappings:**
- `statement` → `schema:Text` or `schema:CreativeWork`
- `reference` → `schema:Thing` (with `@id` or `schema:identifier`)
- `list` → `schema:ItemList` or `schema:Collection`
- `comparison` → `schema:Comparison` (custom extension) or `schema:PropertyValue`
- `expression` → `schema:Value` or computed property

### Secondary: ISO 11179 (Data Element Semantics)

**Rationale:**
- Standard for data element semantics and metadata registry
- Provides formal definitions for data element types
- Useful for structured metadata validation

**Key Concepts:**
- Data Element: A unit of data for which the definition, identification, representation, and permissible values are specified
- Semantic Type: The meaning or interpretation of a data element
- Value Domain: The set of permissible values for a data element

### Reference: BFO (Basic Formal Ontology)

**Rationale:**
- ISO/IEC 21838-2 standardized top-level ontology
- Provides foundational categories for all entities
- Useful for high-level categorization

**Key Concepts:**
- Continuant: Entities that exist in full at any time they exist
- Occurrent: Entities that unfold over time
- Quality: A characteristic of an entity

## Semantic Type Mappings

### statement

**Definition**: A declarative statement or assertion that expresses a fact, opinion, or claim.

**Schema.org Mapping:**
- Primary: `schema:Text` - for simple text statements
- Structured: `schema:CreativeWork` - for structured statements with metadata
- Alternative: `schema:Statement` (if available in future versions)

**ISO 11179 Mapping:**
- Data Element Type: Text
- Semantic Type: Statement/Assertion
- Value Domain: Free text or structured text

**Validation Rules:**
- Must be a string or object
- If object, should have `text` or `content` field
- Can include metadata: `author`, `created_at`, `source`

**Example:**
```yaml
description: "This is a statement"  # schema:Text
# or
description:
  text: "This is a statement"
  author: "account:user1"
  created_at: "2025-12-29T00:00:00Z"
```

### reference

**Definition**: A reference to another object in the system, identified by its ID.

**Schema.org Mapping:**
- Primary: `schema:Thing` with `@id` or `schema:identifier`
- Alternative: `schema:URL` for external references

**ISO 11179 Mapping:**
- Data Element Type: Identifier
- Semantic Type: Reference/Identifier
- Value Domain: Object ID pattern (e.g., "BLI-001", "account:user1")

**Validation Rules:**
- Must be a non-empty string
- Must match object ID pattern for the referenced kind
- Referenced object must exist (validated separately)

**Example:**
```yaml
goal_refs: ["GOAL-001", "GOAL-002"]  # schema:Thing with @id
```

### list

**Definition**: An ordered collection of items.

**Schema.org Mapping:**
- Primary: `schema:ItemList` - for ordered lists
- Alternative: `schema:Collection` - for unordered collections

**ISO 11179 Mapping:**
- Data Element Type: List/Array
- Semantic Type: Collection
- Value Domain: Array of items

**Validation Rules:**
- Must be an array/slice
- Items can be validated individually based on their semantic types
- Order is preserved (for `schema:ItemList`)

**Example:**
```yaml
milestone_refs: ["MIL-001", "MIL-002"]  # schema:ItemList
```

### comparison

**Definition**: A comparative relation expressing a relationship between two or more entities.

**Schema.org Mapping:**
- Primary: `schema:PropertyValue` - for value comparisons
- Custom: `schema:Comparison` (extension) - for explicit comparisons
- Alternative: `schema:Relation` - for general relations

**ISO 11179 Mapping:**
- Data Element Type: Text or Enum
- Semantic Type: Comparison/Relation
- Value Domain: Enum of comparison operators (e.g., "greater_than", "less_than", "equal_to")

**Validation Rules:**
- Must be a string
- Should be one of: "greater_than", "less_than", "equal_to", "not_equal_to", "greater_than_or_equal", "less_than_or_equal", "contains", "not_contains"
- Can be extended with custom comparison operators

**Example:**
```yaml
comparison_operator: "greater_than"  # schema:PropertyValue
```

### expression

**Definition**: A computed or derived value that is calculated from other fields.

**Schema.org Mapping:**
- Primary: `schema:Value` - for computed values
- Alternative: `schema:PropertyValue` - for property-based expressions

**ISO 11179 Mapping:**
- Data Element Type: Computed/Derived
- Semantic Type: Expression/Formula
- Value Domain: Result of computation

**Validation Rules:**
- Can be any type (result of computation)
- Should not be set directly by users (computed only)
- Validation depends on the expression's result type

**Example:**
```yaml
percent_complete: 75  # Computed from milestone completion
```

## Implementation Strategy

### Phase 3.1: Ontology Registry

Create an ontology registry that maps semantic types to formal ontology definitions:

```go
type OntologyRegistry struct {
    mappings map[string]OntologyMapping
}

type OntologyMapping struct {
    SemanticType    string
    SchemaOrgTypes  []string
    ISO11179Type    string
    BFOType         string
    ValidationRules []ValidationRule
}
```

### Phase 3.2: Enhanced Validation

Enhance `GoValidator.validateSemanticTypeItem` to use ontology-based rules:

1. Look up semantic type in ontology registry
2. Apply Schema.org validation rules
3. Apply ISO 11179 validation rules (if applicable)
4. Apply custom validation rules

### Phase 3.3: Documentation

Document all mappings and provide examples for each semantic type.

## Validation Rules by Semantic Type

### statement
- Type: string or object
- If object: must have `text` or `content` field
- Optional metadata: `author`, `created_at`, `source`

### reference
- Type: string
- Pattern: Must match object ID pattern
- Existence: Referenced object must exist (validated separately)

### list
- Type: array/slice
- Items: Each item validated based on its semantic type
- Order: Preserved for ordered lists

### comparison
- Type: string
- Enum: Must be one of valid comparison operators
- Extensible: Can add custom operators

### expression
- Type: any (result of computation)
- Computed: Should not be set directly
- Result type: Validated based on expression's return type

## Future Enhancements

1. **JSON-LD Support**: Generate JSON-LD annotations using Schema.org types
2. **Ontology Reasoning**: Use ontology reasoning to infer relationships
3. **Custom Ontologies**: Support domain-specific ontologies
4. **Validation Hooks**: Allow custom validation rules per semantic type
5. **Performance**: Cache ontology mappings and validation rules

## References

- [Schema.org](https://schema.org/)
- [ISO/IEC 11179](https://www.iso.org/standard/50340.html)
- [ISO/IEC 21838-2 (BFO)](https://www.iso.org/standard/74572.html)
- [ISO 15926](https://www.iso.org/standard/62341.html)

