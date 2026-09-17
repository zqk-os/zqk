# Namespace System Specification v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Design  
**Related**: REQ-036, CRIT-9000, Multi-Layered Ontology v1.0, GOAL-6370

## Purpose

This document specifies the complete namespace system for zqk, including semantic structures for namespace origin, applicability, integration patterns, and isolation mechanisms. It ensures that namespaces can be discovered, validated, and integrated while maintaining clear boundaries and preventing conflicts.

## Namespace Concept

A **namespace** is a logical container that groups related object types and provides a naming scope to prevent conflicts and enable clear categorization. Namespaces serve multiple purposes:

1. **Categorization**: Group objects by their origin and purpose
2. **Isolation**: Prevent naming conflicts between different domains
3. **Integration**: Enable cross-namespace references while maintaining boundaries
4. **Discovery**: Allow system to discover and understand available object types
5. **Authority**: Define who can create and modify objects in each namespace

## Namespace Structure

### Namespace Identifier Format

Namespaces use a hierarchical dot-separated format:

```
{layer}:{domain}:{subdomain}
```

**Examples**:
- `zqk:kernel` - zqk kernel objects
- `domain:organizational` - Organizational domain objects
- `domain:financial:accounting` - Financial accounting subdomain
- `integration:jira` - Jira integration objects
- `integration:github:actions` - GitHub Actions integration

### Namespace Layers

#### Layer 1: zqk Kernel (`zqk:kernel`)

**Purpose**: Core zqk system objects that are essential for system operation

**Characteristics**:
- **Fixed**: Object types defined by zqk, not extensible by users
- **Stable**: Changes require zqk version updates
- **Protected**: Cannot be modified by external domains
- **Core**: Essential for zqk operation

**Object Types**:
- `goal`, `milestone`, `workstream`, `priority_plan`, `backlog_item`
- `requirement`, `criteria`, `test_case`
- `policy`, `decision`, `question`
- `mission`, `vision`, `roadmap`
- `account`, `role`, `component`

**Authority**: zqk core team only

**Isolation**: Complete - external domains cannot create or modify kernel objects

#### Layer 2: Domain Namespaces (`domain:{domain_name}`)

**Purpose**: Domain-specific objects that extend zqk for specific industries, organizations, or use cases

**Characteristics**:
- **Discoverable**: Registered in domain registry
- **Extensible**: Can add new object types
- **Evolvable**: Can change without breaking zqk kernel
- **Contextual**: Domain-specific semantics

**Domain Categories**:

1. **Organizational Domain** (`domain:organizational`):
   - `organization`, `division`, `department`, `team`
   - `partnership`, `contract`, `agreement`
   - `role`, `authority`, `permission`

2. **Financial Domain** (`domain:financial`):
   - `account`, `transaction`, `budget`
   - `regulatory_object`, `compliance_requirement`
   - `audit_trail`, `certification`

3. **Healthcare Domain** (`domain:healthcare`):
   - `patient`, `provider`, `treatment`
   - `regulatory_object`, `compliance_requirement`
   - `audit_trail`, `certification`

4. **Process Domain** (`domain:process`):
   - `workflow`, `process_step`, `approval_chain`
   - `sla`, `kpi`, `metric`

**Authority**: Domain owners (organizations, teams, or individuals)

**Isolation**: Partial - domain objects can reference kernel objects, but kernel objects cannot reference domain objects directly

#### Layer 3: Integration Namespaces (`integration:{system_name}`)

**Purpose**: External system objects and integration mappings

**Characteristics**:
- **External**: Objects from external systems (Jira, GitHub, etc.)
- **Mapped**: Linked to zqk objects via integration adapters
- **Isolated**: Changes in external systems don't break zqk
- **Read-Only**: Typically read-only from zqk perspective

**Integration Types**:

1. **Issue Tracking** (`integration:jira`, `integration:github:issues`):
   - External issue objects
   - Integration mappings to backlog items

2. **Version Control** (`integration:github`, `integration:gitlab`):
   - Pull request objects
   - Commit objects
   - Branch objects

3. **CI/CD** (`integration:github:actions`, `integration:jenkins`):
   - Pipeline objects
   - Build objects
   - Deployment objects

**Authority**: Integration system owners

**Isolation**: Complete - integration objects are isolated from kernel and domain objects

## Namespace Object Specification

### Namespace Object Structure

```yaml
id: NAMESPACE-{sequence}
kind: namespace
title: "Namespace Title"
namespace_id: "zqk:kernel" | "domain:{domain}" | "integration:{system}"
layer: "kernel" | "domain" | "integration"
domain: "organizational" | "financial" | "healthcare" | "process" | null
subdomain: string | null
origin: namespace_origin
applicability: namespace_applicability
integration: namespace_integration
isolation: namespace_isolation
schema_version: 2.0.0
status: "active" | "deprecated" | "experimental"
```

### Namespace Origin

**Purpose**: Describes where the namespace comes from and who has authority over it

```yaml
origin:
  type: "zqk_core" | "domain_registry" | "integration_adapter" | "user_defined"
  authority: "zqk_core_team" | "domain_owner" | "integration_owner" | "user"
  authority_ref: "account:zqk_core" | "account:{owner_id}" | "organization:{org_id}"
  registration_date: "2025-12-30T00:00:00Z"
  registration_method: "built_in" | "discovered" | "manual" | "imported"
  source: "zqk_core" | "domain_registry" | "external_system" | "user_upload"
  version: "1.0.0"
  lifecycle: "stable" | "experimental" | "deprecated"
```

**Examples**:

```yaml
# zqk Kernel Namespace
origin:
  type: "zqk_core"
  authority: "zqk_core_team"
  authority_ref: "account:zqk_core"
  registration_date: "2025-01-01T00:00:00Z"
  registration_method: "built_in"
  source: "zqk_core"
  version: "1.0.0"
  lifecycle: "stable"

# Organizational Domain Namespace
origin:
  type: "domain_registry"
  authority: "domain_owner"
  authority_ref: "organization:acme_corp"
  registration_date: "2025-12-30T00:00:00Z"
  registration_method: "discovered"
  source: "domain_registry"
  version: "1.0.0"
  lifecycle: "experimental"

# Jira Integration Namespace
origin:
  type: "integration_adapter"
  authority: "integration_owner"
  authority_ref: "account:devops_team"
  registration_date: "2025-12-30T00:00:00Z"
  registration_method: "manual"
  source: "external_system"
  version: "1.0.0"
  lifecycle: "stable"
```

### Namespace Applicability

**Purpose**: Defines when, where, and how the namespace can be used

```yaml
applicability:
  project_types: ["individual", "small_team", "enterprise"]
  organization_sizes: ["startup", "mid_size", "enterprise"]
  user_roles: ["developer", "product_manager", "executive", "agent"]
  security_levels: ["low", "medium", "high", "critical"]
  compliance_requirements: ["none", "soc2", "hipaa", "pci_dss"]
  geographic_regions: ["us", "eu", "global"] | null
  industry_domains: ["software", "healthcare", "financial", "general"]
  maturity_requirements: ["production", "development", "prototype"]
  feature_flags: ["namespace_discovery", "cross_namespace_queries"]
  constraints:
    - type: "requires_kernel_version"
      value: ">=1.0.0"
    - type: "requires_feature"
      value: "graph_backend"
    - type: "conflicts_with"
      value: "domain:legacy"
```

**Examples**:

```yaml
# zqk Kernel - Universal Applicability
applicability:
  project_types: ["individual", "small_team", "enterprise"]
  organization_sizes: ["startup", "mid_size", "enterprise"]
  user_roles: ["developer", "product_manager", "executive", "agent"]
  security_levels: ["low", "medium", "high", "critical"]
  compliance_requirements: ["none", "soc2", "hipaa", "pci_dss"]
  geographic_regions: ["global"]
  industry_domains: ["software", "healthcare", "financial", "general"]
  maturity_requirements: ["production"]
  feature_flags: []
  constraints: []

# Healthcare Domain - Restricted Applicability
applicability:
  project_types: ["enterprise"]
  organization_sizes: ["enterprise"]
  user_roles: ["developer", "product_manager"]
  security_levels: ["high", "critical"]
  compliance_requirements: ["hipaa"]
  geographic_regions: ["us", "eu"]
  industry_domains: ["healthcare"]
  maturity_requirements: ["production", "development"]
  feature_flags: ["namespace_discovery"]
  constraints:
    - type: "requires_compliance"
      value: "hipaa"
```

### Namespace Integration

**Purpose**: Defines how namespaces integrate with each other while maintaining boundaries

```yaml
integration:
  can_reference:
    - namespace_id: "zqk:kernel"
      object_types: ["goal", "milestone", "workstream"]
      reference_direction: "outbound"
      validation: "strict" | "lenient"
    - namespace_id: "domain:organizational"
      object_types: ["organization", "team"]
      reference_direction: "bidirectional"
      validation: "strict"
  can_be_referenced_by:
    - namespace_id: "domain:organizational"
      object_types: ["organization"]
      reference_direction: "inbound"
      validation: "strict"
  integration_patterns:
    - type: "direct_reference"
      description: "Direct reference using namespace:object_id format"
      example: "domain:organizational:organization:ACME-CORP"
    - type: "adapter_mapping"
      description: "Integration adapter maps external objects to zqk objects"
      example: "integration:jira:issue:PROJ-123 → backlog_item:BLI-456"
    - type: "semantic_inference"
      description: "System infers relationships based on semantic similarity"
      example: "domain:financial:account → zqk:kernel:component"
  conflict_resolution:
    strategy: "namespace_prefix" | "explicit_mapping" | "semantic_disambiguation"
    priority: ["zqk:kernel", "domain:organizational", "integration:jira"]
```

**Examples**:

```yaml
# zqk Kernel Integration
integration:
  can_reference: []  # Kernel doesn't reference other namespaces
  can_be_referenced_by:
    - namespace_id: "domain:organizational"
      object_types: ["organization", "team"]
      reference_direction: "inbound"
      validation: "strict"
    - namespace_id: "domain:financial"
      object_types: ["account", "budget"]
      reference_direction: "inbound"
      validation: "strict"
  integration_patterns:
    - type: "direct_reference"
      description: "Domain objects reference kernel objects directly"
      example: "domain:organizational:organization:ACME-CORP → zqk:kernel:workstream:WS-001"
  conflict_resolution:
    strategy: "namespace_prefix"
    priority: ["zqk:kernel"]

# Domain Organizational Integration
integration:
  can_reference:
    - namespace_id: "zqk:kernel"
      object_types: ["goal", "milestone", "workstream", "backlog_item"]
      reference_direction: "outbound"
      validation: "strict"
  can_be_referenced_by:
    - namespace_id: "domain:financial"
      object_types: ["budget"]
      reference_direction: "inbound"
      validation: "strict"
  integration_patterns:
    - type: "direct_reference"
      description: "Organizational objects reference kernel objects"
      example: "domain:organizational:team:ENG-001 → zqk:kernel:workstream:WS-001"
    - type: "semantic_inference"
      description: "System infers team ownership of workstreams"
      example: "domain:organizational:team:ENG-001 → zqk:kernel:workstream:WS-001 (inferred)"
  conflict_resolution:
    strategy: "namespace_prefix"
    priority: ["zqk:kernel", "domain:organizational"]
```

### Namespace Isolation

**Purpose**: Defines mechanisms to keep namespaces separate and prevent conflicts

```yaml
isolation:
  naming_conflicts:
    prevention: "namespace_prefix" | "explicit_mapping" | "semantic_disambiguation"
    resolution: "error" | "warning" | "auto_resolve"
  access_control:
    read: ["all" | "namespace_owner" | "authorized_users"]
    write: ["namespace_owner" | "authorized_users"]
    delete: ["namespace_owner"]
  validation:
    strict: true | false
    cross_namespace_validation: true | false
    reference_validation: "strict" | "lenient" | "none"
  boundaries:
    object_creation: "namespace_owner_only" | "authorized_users" | "all"
    object_modification: "namespace_owner_only" | "authorized_users" | "all"
    object_deletion: "namespace_owner_only" | "authorized_users" | "all"
  conflict_detection:
    enabled: true | false
    detection_method: "id_collision" | "semantic_similarity" | "reference_validation"
    resolution_strategy: "error" | "warning" | "auto_resolve" | "manual"
```

**Examples**:

```yaml
# zqk Kernel Isolation
isolation:
  naming_conflicts:
    prevention: "namespace_prefix"
    resolution: "error"
  access_control:
    read: ["all"]
    write: ["zqk_core_team"]
    delete: ["zqk_core_team"]
  validation:
    strict: true
    cross_namespace_validation: true
    reference_validation: "strict"
  boundaries:
    object_creation: "namespace_owner_only"
    object_modification: "namespace_owner_only"
    object_deletion: "namespace_owner_only"
  conflict_detection:
    enabled: true
    detection_method: "id_collision"
    resolution_strategy: "error"

# Domain Organizational Isolation
isolation:
  naming_conflicts:
    prevention: "namespace_prefix"
    resolution: "warning"
  access_control:
    read: ["all"]
    write: ["namespace_owner", "authorized_users"]
    delete: ["namespace_owner"]
  validation:
    strict: true
    cross_namespace_validation: true
    reference_validation: "strict"
  boundaries:
    object_creation: "authorized_users"
    object_modification: "authorized_users"
    object_deletion: "namespace_owner_only"
  conflict_detection:
    enabled: true
    detection_method: "semantic_similarity"
    resolution_strategy: "warning"
```

## Reference Scheme

### Full Reference Format

References use the format: `{namespace_id}:{object_type}:{object_id}`

**Examples**:
- `zqk:kernel:goal:GOAL-6369`
- `domain:organizational:organization:ACME-CORP`
- `integration:jira:issue:PROJ-123`

### Short Reference Format

Within the same namespace, short references can be used:
- `goal:GOAL-6369` (assumes `zqk:kernel`)
- `organization:ACME-CORP` (assumes current domain namespace)

### Cross-Namespace References

Cross-namespace references must use full format:
- `domain:organizational:organization:ACME-CORP → zqk:kernel:workstream:WS-001`

## Namespace Registry

### Registry Structure

The namespace registry is a system object that tracks all registered namespaces:

```yaml
id: NAMESPACE-REGISTRY-001
kind: namespace_registry
title: "zqk Namespace Registry"
namespaces:
  - namespace_id: "zqk:kernel"
    namespace_ref: "NAMESPACE-001"
    status: "active"
    registered_at: "2025-01-01T00:00:00Z"
  - namespace_id: "domain:organizational"
    namespace_ref: "NAMESPACE-002"
    status: "active"
    registered_at: "2025-12-30T00:00:00Z"
```

### Namespace Discovery

Namespaces can be discovered through:
1. **Built-in**: zqk kernel namespaces are built-in
2. **Domain Registry**: Domain namespaces are registered in domain registry
3. **Integration Adapters**: Integration namespaces are discovered via adapters
4. **Manual Registration**: Users can manually register namespaces

### Namespace Validation

Before a namespace can be used, it must be validated:
1. **Format Validation**: Namespace ID follows correct format
2. **Uniqueness Validation**: Namespace ID is unique
3. **Authority Validation**: Requester has authority to create namespace
4. **Integration Validation**: Integration patterns are valid
5. **Isolation Validation**: Isolation rules are consistent

## Implementation Requirements

### REQ-036: Multi-Layered Ontology System Implementation

This requirement includes:
1. **Namespace Object Type**: Create `namespace` object specification
2. **Namespace Registry**: Implement namespace registry system
3. **Reference Resolution**: Implement namespace-aware reference resolution
4. **Cross-Namespace Validation**: Implement cross-namespace reference validation
5. **Namespace Discovery**: Implement namespace discovery mechanism

### CRIT-9000: Multi-Layered Ontology Namespace System

Success conditions:
1. Namespace system implemented with `zqk:kernel`, `domain:*`, `integration:*` prefixes
2. Reference scheme supports cross-layer references
3. Domain registry functional and discoverable
4. Namespace objects created with origin, applicability, integration, and isolation structures
5. Cross-namespace reference validation working

## Related Objects

- **REQ-036**: Multi-Layered Ontology System Implementation
- **CRIT-9000**: Multi-Layered Ontology Namespace System
- **GOAL-6370**: Multi-Layered Ontology System
- **MIL-038**: Core Ontology Foundation Complete

