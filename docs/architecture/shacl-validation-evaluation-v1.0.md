# SHACL-Based Validation System Evaluation

**Last Verified:** 2026-08-31


**Version**: 1.0  
**Status**: Design  
**Date**: 2025-12-25  
**Related**: BLI-631, REQ-029, REQ-026, REQ-027

## Overview

SHACL (Shapes Constraint Language) is a W3C standard for validating RDF data graphs. While designed for RDF/SPARQL, its concepts are generalizable and could provide a more formal, declarative approach to object instance validation in zqk.

## SHACL Concepts

### Core Components

1. **Shapes**: Define the structure and constraints for data
2. **Constraints**: Rules that data must satisfy (e.g., minCount, maxCount, pattern, datatype)
3. **Targets**: Define which nodes a shape applies to
4. **Property Shapes**: Define constraints on specific properties
5. **Node Shapes**: Define constraints on the node itself

### Example SHACL Shape

```turtle
ex:BacklogItemShape
  a sh:NodeShape ;
  sh:targetClass ex:BacklogItem ;
  sh:property [
    sh:path ex:status ;
    sh:datatype xsd:string ;
    sh:in ( "exploring" "validated" "planned" "in_progress" "complete" ) ;
  ] ;
  sh:property [
    sh:path ex:title ;
    sh:minCount 1 ;
    sh:maxCount 1 ;
    sh:datatype xsd:string ;
  ] ;
  sh:property [
    sh:path ex:priority_plan_ref ;
    sh:minCount 0 ;
    sh:maxCount 1 ;
    sh:pattern "^PRI-\\d{3,}$" ;
  ] .
```

## Current Validation Approach

Our current `InstanceValidator` uses:
- Direct Go code for validation rules
- YAML spec files with `validation` sections
- Hardcoded validation logic in `validateField()`

### Current Spec Structure

```yaml
fields:
  status:
    type: enum
    validation:
      required: true
      enum: ["exploring", "validated", "planned", "in_progress", "complete"]
  title:
    type: string
    validation:
      required: true
      min_length: 1
  priority_plan_ref:
    type: string
    validation:
      required: false
      pattern: "^PRI-\\d{3,}$"
```

## SHACL Adaptation for YAML

### Option 1: SHACL-Inspired YAML Syntax

Define validation rules using SHACL concepts but in YAML:

```yaml
shapes:
  backlog_item:
    target: backlog_item
    properties:
      status:
        path: status
        datatype: string
        required: true
        in: ["exploring", "validated", "planned", "in_progress", "complete"]
      title:
        path: title
        datatype: string
        minCount: 1
        maxCount: 1
      priority_plan_ref:
        path: priority_plan_ref
        datatype: string
        pattern: "^PRI-\\d{3,}$"
        minCount: 0
        maxCount: 1
```

### Option 2: SHACL JSON-LD Integration

Use SHACL JSON-LD format directly:

```json
{
  "@context": {
    "sh": "http://www.w3.org/ns/shacl#",
    "ex": "http://example.org/zqk#"
  },
  "@type": "sh:NodeShape",
  "sh:targetClass": "ex:BacklogItem",
  "sh:property": [
    {
      "sh:path": "ex:status",
      "sh:datatype": "http://www.w3.org/2001/XMLSchema#string",
      "sh:in": ["exploring", "validated", "planned", "in_progress", "complete"]
    }
  ]
}
```

### Option 3: Hybrid Approach

Keep current YAML spec structure but add SHACL-compatible validation:

```yaml
fields:
  status:
    type: string
    semantic_type: statement
    validation:
      required: true
      enum: ["exploring", "validated", "planned", "in_progress", "complete"]
    shacl:
      datatype: xsd:string
      in: ["exploring", "validated", "planned", "in_progress", "complete"]
```

## Benefits of SHACL Approach

1. **Standardization**: W3C standard, well-documented
2. **Declarative**: Validation rules are data, not code
3. **Reusability**: Shapes can be composed and extended
4. **Tooling**: Existing SHACL validators and tooling
5. **Formal Semantics**: Precise validation semantics
6. **Interoperability**: Can export/import SHACL shapes

## Challenges

1. **RDF/SPARQL Focus**: SHACL is designed for RDF graphs, not YAML
2. **Complexity**: May be overkill for simple validation
3. **Learning Curve**: Team needs to learn SHACL concepts
4. **Tooling**: Need to adapt or build SHACL validator for YAML
5. **Performance**: RDF-based validation may be slower than direct Go code

## Integration Options

### Option A: SHACL as Validation Language

Replace current validation logic with SHACL-based validator:
- Convert YAML specs to SHACL shapes
- Use SHACL validator library (e.g., TopBraid SHACL API, Apache Jena)
- Validate instances against SHACL shapes

### Option B: SHACL-Inspired Validation

Keep current approach but adopt SHACL concepts:
- Use SHACL terminology and patterns
- Maintain Go-based validator
- Add SHACL export capability

### Option C: Dual Validation

Support both approaches:
- Current Go-based validator for performance
- SHACL validator for formal validation
- Allow choosing validation method

## Recommendation

**Hybrid Approach (Option B + C)**:

1. **Short-term**: Enhance current validation with SHACL-inspired patterns
   - Adopt SHACL terminology (shapes, constraints, property shapes)
   - Add SHACL-compatible validation rules to specs
   - Keep Go-based validator for performance

2. **Medium-term**: Add SHACL export capability
   - Generate SHACL shapes from YAML specs
   - Enable interoperability with SHACL tooling
   - Support SHACL validation as optional validation method

3. **Long-term**: Evaluate full SHACL integration
   - Assess performance and complexity
   - Consider RDF-based backend option
   - Migrate if benefits outweigh costs

## Implementation Plan

### Phase 1: SHACL-Inspired Enhancements
- Add SHACL terminology to spec structure
- Enhance validation rules with SHACL concepts
- Document SHACL mapping

### Phase 2: SHACL Export
- Generate SHACL shapes from YAML specs
- Export validation rules as SHACL
- Test with SHACL validators

### Phase 3: SHACL Validator Integration
- Integrate SHACL validator library
- Support SHACL-based validation as option
- Performance comparison

## References

- [SHACL W3C Specification](https://www.w3.org/TR/shacl/)
- [SHACL Primer](https://www.w3.org/TR/shacl-primer/)
- [TopBraid SHACL API](https://github.com/TopQuadrant/shacl)
- [Apache Jena SHACL](https://jena.apache.org/documentation/shacl/)

