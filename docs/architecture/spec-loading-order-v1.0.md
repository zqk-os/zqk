# Spec Loading Order and Trait Validation

**Last Verified:** 2026-08-31


**Version**: 1.0  
**Status**: Design  
**Date**: 2025-12-25

## Problem Statement

The current spec loading system loads specs recursively on-demand, but this approach has several issues:

1. **No explicit load order**: Specs with `extends` dependencies should be loaded in topological order
2. **Trait validation missing**: Traits are just lists, but they may have composition rules and validation criteria
3. **Simplified ontology**: The zqk design appears less rigorous than the legacy CLI system

## Spec Loading Order

### Dependency Graph

Specs form a directed acyclic graph (DAG) based on the `extends` field:

```
auditable (root - no parent)
  └─ base_object
      ├─ extensible_object
      │   └─ component
      ├─ criteria
      ├─ requirement
      ├─ backlog_item
      ├─ goal
      ├─ milestone
      └─ ... (many others)
```

### Required Load Order

1. **Root specs** (no `extends` or `extends: null`):
   - `auditable.yaml`

2. **First-level dependencies** (extend root):
   - `base_object.yaml` (extends: auditable)

3. **Second-level dependencies** (extend base_object):
   - `extensible_object.yaml` (extends: base_object)
   - `criteria.yaml` (extends: base_object)
   - `requirement.yaml` (extends: base_object)
   - `backlog_item.yaml` (extends: base_object)
   - `goal.yaml` (extends: base_object)
   - `milestone.yaml` (extends: base_object)
   - ... (all public object specs)

4. **Third-level dependencies** (extend extensible_object):
   - `component.yaml` (extends: extensible_object)

### Topological Sort Algorithm

We need to:
1. Build a dependency graph from all spec files
2. Perform topological sort to determine load order
3. Load specs in order, validating each level before proceeding
4. Detect circular dependencies

## Trait System

### Current State

Traits are currently just string lists:
```yaml
traits:
  - listable
  - readable
  - writable
  - constrainable  # domain-specific
```

### Trait Composition

Traits may have:
- **Dependencies**: Some traits require other traits (e.g., `modifiable` requires `readable`)
- **Composition**: Traits can be composed of multiple traits
- **Validation rules**: Traits must meet specific criteria
- **Field-level traits**: Fields can have traits that must be subsets of object-level traits

### Trait Validation Requirements

1. **Trait definitions**: Need trait specifications that define:
   - Trait name and description
   - Required dependencies
   - Composition rules
   - Validation criteria

2. **Trait inheritance**: Traits from parent specs must be validated
3. **Field trait validation**: Field-level traits must be subsets of object-level traits
4. **Domain-specific traits**: Must be declared and validated

## Implementation Plan

### Phase 1: Spec Load Order

1. Create `SpecLoadOrder` type that:
   - Scans all spec files
   - Builds dependency graph
   - Performs topological sort
   - Validates no circular dependencies

2. Update `SpecLoader` to:
   - Use `SpecLoadOrder` to determine load sequence
   - Load specs in dependency order
   - Validate each spec after loading

3. Add validation:
   - Circular dependency detection
   - Missing parent spec detection
   - Load order violations

### Phase 2: Trait System

1. Create trait definitions:
   - `.zqk/specs/traits/` directory
   - Trait spec format (YAML)
   - Trait dependency definitions

2. Create `TraitValidator`:
   - Validates trait dependencies
   - Validates trait composition
   - Validates field-level traits

3. Integrate with `SpecValidator`:
   - Validate traits during spec validation
   - Validate trait inheritance
   - Validate field trait subsets

### Phase 3: Rigor Parity

1. Compare with legacy system:
   - Document what legacy system validates
   - Identify gaps in zqk
   - Create requirements for parity

2. Implement missing validations:
   - Trait composition rules
   - Field trait validation
   - Spec completeness checks

## Requirements

- REQ-020: Spec Loading Order Enforcement
- REQ-021: Trait System Validation
- REQ-022: Trait Composition Support

## Criteria

- CRIT-8203: Specs loaded in topological order
- CRIT-8204: Circular dependency detection
- CRIT-8205: Trait dependency validation
- CRIT-8206: Trait composition validation
- CRIT-8207: Field trait subset validation

