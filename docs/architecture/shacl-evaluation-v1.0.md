# SHACL-Based Validation System Evaluation

**Last Verified:** 2026-08-31


**Version**: 1.0  
**Status**: Evaluation  
**Date**: 2025-12-29  
**Related**: BLI-631, REQ-029

## Overview

This document evaluates SHACL (Shapes Constraint Language) as a potential validation system for zqk, comparing it with the current Go-based validator and providing recommendations.

## SHACL Overview

**SHACL (Shapes Constraint Language)** is a W3C standard (2017) for validating RDF data against a set of conditions (shapes). It provides:

- **Declarative Validation**: Define validation rules declaratively
- **RDF-Based**: Works with RDF data models
- **Extensible**: Supports custom constraint components
- **Standardized**: W3C recommendation, widely adopted

## Current Go-Based Validator

### Architecture

The current `GoValidator` implements SHACL-inspired patterns:

- **Property Shapes**: Field-level validation (datatype, pattern, minCount, maxCount, etc.)
- **Node Shapes**: Object-level validation
- **Constraints**: Type, pattern, enum, required field validation
- **Lifecycle Integration**: Custom lifecycle state and transition validation

### Strengths

1. **Native Go Implementation**: Fast, no external dependencies
2. **Type Safety**: Compile-time type checking
3. **Lifecycle Support**: Integrated with zqk lifecycle system
4. **Custom Rules**: Easy to add domain-specific validation
5. **Performance**: Direct memory access, no serialization overhead
6. **Debugging**: Native Go debugging tools

### Limitations

1. **No Standard Export**: Cannot export validation rules as SHACL shapes
2. **Limited Interoperability**: Rules are Go-specific
3. **Manual Maintenance**: Validation rules must be coded manually
4. **No RDF Support**: Does not work with RDF data models

## SHACL Evaluation

### Advantages of SHACL

1. **Standardization**: W3C standard, widely adopted
2. **Interoperability**: Can be used across different systems
3. **Declarative**: Rules defined in RDF/Turtle, not code
4. **Tooling**: Rich ecosystem of SHACL validators and tools
5. **Extensibility**: Custom constraint components
6. **Export/Import**: Can export validation rules as SHACL shapes

### Disadvantages of SHACL

1. **RDF Requirement**: Requires RDF data model (zqk uses YAML/JSON)
2. **Performance Overhead**: RDF serialization/deserialization
3. **Complexity**: Learning curve for SHACL syntax
4. **Go Integration**: Would require RDF library (e.g., go-rdf)
5. **Lifecycle Support**: Would need custom SHACL extensions for lifecycle validation
6. **Type System**: Less type-safe than native Go

## Comparison Matrix

| Feature | Go Validator | SHACL |
|---------|-------------|-------|
| **Performance** | ⭐⭐⭐⭐⭐ Fast (native) | ⭐⭐⭐ Moderate (RDF overhead) |
| **Type Safety** | ⭐⭐⭐⭐⭐ Compile-time | ⭐⭐⭐ Runtime only |
| **Standardization** | ⭐⭐ Custom | ⭐⭐⭐⭐⭐ W3C Standard |
| **Interoperability** | ⭐⭐ Go-specific | ⭐⭐⭐⭐⭐ Cross-platform |
| **Lifecycle Support** | ⭐⭐⭐⭐⭐ Native | ⭐⭐ Custom extensions |
| **Ease of Use** | ⭐⭐⭐⭐ Go developers | ⭐⭐⭐ SHACL learning curve |
| **Tooling** | ⭐⭐⭐ Go tools | ⭐⭐⭐⭐⭐ Rich ecosystem |
| **Export/Import** | ⭐ Not supported | ⭐⭐⭐⭐⭐ Standard format |
| **Maintenance** | ⭐⭐⭐ Code-based | ⭐⭐⭐⭐ Declarative |

## Hybrid Approach

### Option 1: SHACL Export Only

**Approach**: Keep Go validator, add SHACL export capability

**Pros:**
- Maintains current performance
- Adds interoperability without changing core
- Minimal code changes

**Cons:**
- Export may not capture all validation rules
- Two sources of truth (code and SHACL)

### Option 2: SHACL as Source of Truth

**Approach**: Define validation rules in SHACL, generate Go validator from SHACL

**Pros:**
- Single source of truth (SHACL)
- Interoperability
- Can export/import rules

**Cons:**
- Requires RDF infrastructure
- Code generation complexity
- Performance overhead

### Option 3: Dual Validator Support

**Approach**: Support both Go and SHACL validators, allow switching

**Pros:**
- Best of both worlds
- Flexibility
- Can use SHACL for interoperability

**Cons:**
- Maintenance overhead
- Complexity
- Potential inconsistencies

## Recommendation

### Recommended: Hybrid Approach (Option 1) - SHACL Export

**Rationale:**

1. **Current System Works Well**: The Go validator is performant and well-integrated
2. **Interoperability Without Overhead**: SHACL export provides interoperability without RDF overhead
3. **Incremental Adoption**: Can add SHACL export incrementally
4. **Best of Both Worlds**: Performance of Go + interoperability of SHACL

### Implementation Plan

1. **Phase 1**: Add SHACL export capability to GoValidator
   - Export validation rules as SHACL shapes
   - Support basic constraints (datatype, pattern, minCount, maxCount, enum)
   - Document export format

2. **Phase 2**: Add SHACL import capability (optional)
   - Import SHACL shapes and generate Go validation rules
   - Validate imported shapes

3. **Phase 3**: SHACL validator support (future)
   - Add optional SHACL validator for RDF data
   - Use for interoperability scenarios

### SHACL Export Format

Example SHACL shape for a `backlog_item`:

```turtle
@prefix sh: <http://www.w3.org/ns/shacl#> .
@prefix schema: <https://schema.org/> .
@prefix zqk: <https://zqk.example.org/ns#> .

zqk:BacklogItemShape
    a sh:NodeShape ;
    sh:targetClass zqk:BacklogItem ;
    sh:property [
        sh:path zqk:id ;
        sh:datatype xsd:string ;
        sh:pattern "^BLI-\\d+$" ;
        sh:minCount 1 ;
        sh:maxCount 1 ;
    ] ;
    sh:property [
        sh:path zqk:title ;
        sh:datatype xsd:string ;
        sh:minCount 1 ;
        sh:minLength 1 ;
    ] ;
    sh:property [
        sh:path zqk:status ;
        sh:datatype xsd:string ;
        sh:in ("exploring" "validated" "planned" "in_progress" "complete" "archived" "rejected") ;
    ] .
```

## Conclusion

**Recommendation**: **Hybrid Approach - SHACL Export**

- Keep the current Go-based validator for performance and integration
- Add SHACL export capability for interoperability
- Consider SHACL import and full SHACL validator support in the future

This approach provides:
- ✅ Performance of native Go implementation
- ✅ Interoperability through SHACL export
- ✅ Minimal disruption to current system
- ✅ Incremental adoption path

## References

- [SHACL W3C Recommendation](https://www.w3.org/TR/shacl/)
- [SHACL Primer](https://www.w3.org/TR/shacl-primer/)
- [SHACL Advanced Features](https://www.w3.org/TR/shacl-af/)

