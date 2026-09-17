# Domain Registry and Namespace System Integration

**Last Verified:** 2026-08-31


**Date**: 2026-01-05  
**Status**: Design Document

## Overview

This document describes how the **Domain Registry** system and the **Namespace System** work together to provide domain discovery, validation, and namespace management in zqk.

## Two Complementary Systems

### 1. Domain Registry (`domain_registry` object)

**Purpose**: Metadata registry that tracks registered domain ontologies

**Characteristics**:
- **Registry Object**: Lives in `zqk:kernel` namespace
- **Metadata Store**: Contains domain metadata (description, location, object types, version, etc.)
- **Discovery**: Enables domain discovery and validation
- **Integration**: Supports integration of external domain models

**Structure**:
```yaml
kind: domain_registry
namespace_id: zqk:kernel
domains:
  - domain_id: organizational
    namespace_id: domain:organizational
    namespace_ref: NAMESPACE-002
    object_types: [organization, division, department, team, partnership]
    location: .zqk/process/organizations
    status: active
    version: 1.0.0
```

### 2. Namespace System (`NamespaceRegistry`)

**Purpose**: Runtime mapping of object kinds to namespaces

**Characteristics**:
- **Runtime Mapping**: Maps object kinds to their default namespaces
- **Hierarchical**: Supports hierarchical namespaces (`zqk:kernel`, `domain:organizational`)
- **Subordinate Namespaces**: Supports subordinate namespaces (`zqk:kernel:cli`, `zqk:kernel:metrics`)
- **Inference**: Infers namespaces from object kinds when not explicitly specified

**Structure**:
```yaml
# namespaces_config.yaml
namespaces:
  zqk:kernel:
    subordinate_namespaces:
      - zqk:kernel:cli
      - zqk:kernel:metrics
    kinds: [goal, milestone, backlog_item, ...]
  
  domain:organizational:
    kinds: [organization, division, department, team, partnership]
```

## Relationship and Integration

### How They Work Together

1. **Domain Registry** = **"What domains exist?"**
   - Declarative: Lists all registered domains
   - Metadata: Contains domain metadata, versions, locations
   - Discovery: Enables discovery of available domains

2. **Namespace System** = **"Which namespace does this object belong to?"**
   - Runtime: Maps object kinds to namespaces at runtime
   - Validation: Ensures objects are in correct namespaces
   - Inference: Automatically assigns namespaces to objects

### Integration Points

#### 1. Domain Registry References Namespace Objects

Domain registry entries reference `namespace` objects:

```yaml
domains:
  - namespace_id: domain:organizational
    namespace_ref: NAMESPACE-002  # References namespace object
```

**Requirement**: Domain registry entries should reference valid `namespace` objects that define the namespace structure.

#### 2. Namespace System Uses Domain Registry for Discovery

The namespace system can query the domain registry to discover available domains:

```go
// Pseudo-code
func DiscoverDomains() []Domain {
    registry := GetDomainRegistry()
    return registry.GetDomains()
}

func GetNamespaceForDomain(domainID string) string {
    // Query domain registry for namespace_id
    domain := GetDomainRegistry().GetDomain(domainID)
    return domain.NamespaceID
}
```

#### 3. Domain Registry Validates Against Namespace System

When registering a domain, validate that:
- The `namespace_id` is valid
- The `namespace_ref` points to a valid namespace object
- The object types listed are correctly mapped to the namespace

## Subordinate Namespaces and Domain Registries

### Current State

Domain registries currently track **domain namespaces** (e.g., `domain:organizational`), not subordinate namespaces (e.g., `zqk:kernel:cli`).

### Design Decision

**Domain registries track domain-level namespaces only**, not kernel subordinate namespaces:

- ✅ **Track**: `domain:organizational`, `domain:financial`
- ❌ **Don't Track**: `zqk:kernel:cli`, `zqk:kernel:metrics`

**Rationale**:
- Domain registries are for **external domain ontologies**
- Subordinate namespaces are for **internal module organization**
- Kernel subordinate namespaces are managed by the namespace system, not domain registries

### Future: Domain Subordinate Namespaces

If domains need subordinate namespaces (e.g., `domain:organizational:hr`), they should be:
1. Registered in the domain registry
2. Defined as namespace objects
3. Mapped in the namespace system

## Implementation Requirements

### 1. Ensure `domain_registry` is in Namespace Config

The `domain_registry` object kind should be mapped to `zqk:kernel`:

```yaml
# namespaces_config.yaml
zqk:kernel:
  kinds:
    - domain_registry  # Registry object lives in kernel
```

### 2. Domain Registry Validation

When creating/updating a domain registry entry, validate:
- `namespace_id` matches a registered namespace
- `namespace_ref` points to a valid namespace object
- Object types are correctly mapped to the namespace

### 3. Namespace System Integration

The namespace system should:
- Support querying domain registries for domain discovery
- Validate namespace IDs against domain registry entries
- Ensure domain namespaces are properly registered

## Example: Organizational Domain

### Domain Registry Entry

```yaml
id: DOMAIN-REG-001
kind: domain_registry
namespace_id: zqk:kernel
domains:
  - domain_id: organizational
    namespace_id: domain:organizational
    namespace_ref: NAMESPACE-002
    object_types:
      - organization
      - division
      - department
      - team
      - partnership
```

### Namespace Object

```yaml
id: NAMESPACE-002
kind: namespace
namespace_id: domain:organizational
layer: domain
domain: organizational
```

### Namespace Config

```yaml
domain:organizational:
  description: "Organizational domain objects"
  kinds:
    - organization
    - division
    - department
    - team
    - partnership
```

### Integration Flow

1. **Domain Registry** declares: "organizational domain exists with namespace `domain:organizational`"
2. **Namespace Object** defines: "`domain:organizational` is a domain namespace"
3. **Namespace Config** maps: "these object kinds belong to `domain:organizational`"
4. **Runtime**: Objects of kind `organization` are assigned to `domain:organizational`

## Conflict Prevention

### Potential Conflicts

1. **Namespace ID Mismatch**: Domain registry says `domain:organizational`, but namespace config doesn't have it
   - **Solution**: Validate domain registry entries against namespace config

2. **Object Type Mismatch**: Domain registry lists object types that aren't in namespace config
   - **Solution**: Validate object types are correctly mapped

3. **Missing Namespace Object**: Domain registry references `namespace_ref` that doesn't exist
   - **Solution**: Validate `namespace_ref` points to valid namespace object

### Validation Rules

```go
func ValidateDomainRegistryEntry(entry DomainEntry) error {
    // 1. Validate namespace_id exists in namespace system
    if !NamespaceRegistry.HasNamespace(entry.NamespaceID) {
        return fmt.Errorf("namespace_id %s not found in namespace system", entry.NamespaceID)
    }
    
    // 2. Validate namespace_ref points to valid namespace object
    if entry.NamespaceRef != "" {
        namespaceObj := GetNamespaceObject(entry.NamespaceRef)
        if namespaceObj == nil {
            return fmt.Errorf("namespace_ref %s not found", entry.NamespaceRef)
        }
        if namespaceObj.NamespaceID != entry.NamespaceID {
            return fmt.Errorf("namespace_ref %s has different namespace_id", entry.NamespaceRef)
        }
    }
    
    // 3. Validate object types are mapped to namespace
    for _, objectType := range entry.ObjectTypes {
        expectedNamespace := NamespaceRegistry.GetNamespaceForKind(objectType)
        if expectedNamespace != entry.NamespaceID {
            return fmt.Errorf("object type %s mapped to %s, but domain registry says %s",
                objectType, expectedNamespace, entry.NamespaceID)
        }
    }
    
    return nil
}
```

## Future Enhancements

1. **Domain Registry API**: Provide API to query domain registries
2. **Auto-Registration**: Automatically register domains when namespace objects are created
3. **Domain Discovery**: Use domain registry for runtime domain discovery
4. **Namespace Validation**: Validate namespace objects against domain registry entries

## Summary

- **Domain Registry**: Metadata about what domains exist
- **Namespace System**: Runtime mapping of objects to namespaces
- **Integration**: Domain registries reference namespace objects; namespace system can query domain registries
- **No Conflicts**: They complement each other - domain registries are declarative, namespace system is operational
- **Subordinate Namespaces**: Domain registries track domain-level namespaces only, not kernel subordinate namespaces

