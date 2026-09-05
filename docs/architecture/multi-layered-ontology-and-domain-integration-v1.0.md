# Multi-Layered Ontology and Domain Integration v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Design  
**Related**: System Ontology v1.0, Knowledge Kernel Separation v1.0, Extensible Objects

## Overview

This document defines a multi-layered ontology system that distinguishes between zqk-specific objects and organizational/domain objects, accommodates evolving organizational structures, and enables domain-specific ontologies to be discovered and integrated smoothly into challenging political ecosystems without creating barriers for evolution.

## Problem Statement

**Challenges**:

1. **Ontology Layering**: How to distinguish zqk kernel objects from organizational/domain objects
2. **Organizational Evolution**: Organizations have divisions and sub-divisions that change/evolve outside zqk context
3. **Change Accommodation**: How to accommodate organizational changes and understand their impact on software delivery
4. **Domain Ontologies**: Multiple domain-specific ontologies need to be discoverable and accurately described
5. **Political Ecosystems**: Integration into challenging political ecosystems without creating barriers
6. **Evolution Support**: Ensure system can incorporate and understand how organizational shifts impact software delivery

**Example Scenarios**:

- **Organizational Restructuring**: Company reorganizes divisions, zqk must adapt
- **Partnership Changes**: Partnership structure evolves, authority model must update
- **Domain-Specific Objects**: Financial services domain has regulatory objects, healthcare has compliance objects
- **Political Dynamics**: Different teams have different priorities, system must accommodate

## Solution: Multi-Layered Ontology with Domain Integration

### Core Principles

1. **Layered Ontology**: Clear separation between zqk kernel and domain/organizational layers
2. **Namespace Isolation**: Distinct namespaces prevent contamination
3. **Domain Discovery**: Domain ontologies discoverable and integrable
4. **Organizational Modeling**: Organizational structures modeled as external domain objects
5. **Change Propagation**: Organizational changes propagate to impact analysis
6. **Political Awareness**: System understands and accommodates political dynamics

## Architecture

### 1. Ontology Layers

**Three-Layer Architecture**:

```
┌─────────────────────────────────────────────────────────┐
│ Layer 1: zqk Kernel Ontology (Protected Core)        │
│ - goal, milestone, workstream, backlog_item, etc.      │
│ - Fixed, stable, protected from external changes       │
│ - Namespace: zqk:kernel:*                             │
└─────────────────────────────────────────────────────────┘
                        ↕ (references)
┌─────────────────────────────────────────────────────────┐
│ Layer 2: Domain Ontologies (Discoverable, Extensible)   │
│ - organizational_structure, division, team, etc.   │
│ - partnership, contract, regulatory_object, etc.       │
│ - Discoverable via domain registry                     │
│ - Namespace: domain:{domain_name}:*                     │
└─────────────────────────────────────────────────────────┘
                        ↕ (references)
┌─────────────────────────────────────────────────────────┐
│ Layer 3: Integration Layer (External Systems)           │
│ - External system objects (Jira, GitHub, etc.)          │
│ - Integration adapters and mappings                     │
│ - Namespace: integration:{system_name}:*                │
└─────────────────────────────────────────────────────────┘
```

**Layer 1: zqk Kernel Ontology** (Protected Core)

**Namespace**: `zqk:kernel:*`

**Object Types**:
- `goal`, `milestone`, `workstream`, `priority_plan`, `backlog_item`
- `requirement`, `criteria`, `test_case`
- `policy`, `decision`, `question`
- `mission`, `vision`, `roadmap`

**Characteristics**:
- **Fixed**: Object types defined by zqk, not extensible
- **Stable**: Changes require zqk version updates
- **Protected**: Cannot be modified by external domains
- **Core**: Essential for zqk operation

**Layer 2: Domain Ontologies** (Discoverable, Extensible)

**Namespace**: `domain:{domain_name}:*`

**Domain Categories**:

1. **Organizational Domain** (`domain:organizational:*`):
   - `organization`, `division`, `department`, `team`
   - `partnership`, `contract`, `agreement`
   - `role`, `authority`, `permission`

2. **Industry Domain** (`domain:financial:*`, `domain:healthcare:*`, etc.):
   - `regulatory_object`, `compliance_requirement`
   - `audit_trail`, `certification`
   - Domain-specific objects

3. **Process Domain** (`domain:process:*`):
   - `workflow`, `process_step`, `approval_chain`
   - `sla`, `kpi`, `metric`

**Characteristics**:
- **Discoverable**: Registered in domain registry
- **Extensible**: Can add new object types
- **Evolvable**: Can change without breaking zqk kernel
- **Contextual**: Domain-specific semantics

**Layer 3: Integration Layer** (External)

**Namespace**: `integration:{system_name}:*`

**Object Types**:
- External system objects (Jira issues, GitHub PRs, etc.)
- Integration mappings and adapters

**Characteristics**:
- **External**: Objects from external systems
- **Mapped**: Mapped to zqk/domain objects
- **Isolated**: Changes don't affect kernel

### 2. Namespace and Reference Schemes

**Reference Scheme Format**:
```
{layer}:{domain}:{object_type}:{id}
```

**Examples**:
```
# zqk kernel object
zqk:kernel:goal:GOAL-001

# Organizational domain object
domain:organizational:division:DIV-001

# Financial domain object
domain:financial:regulatory_object:REG-001

# Integration object
integration:jira:issue:JIRA-123
```

**Cross-Layer References**:
```yaml
# zqk kernel object referencing organizational object
id: GOAL-001
kind: goal
organizational_context_ref: "domain:organizational:division:DIV-001"

# Organizational object referencing zqk kernel object
id: DIV-001
kind: division
zqk_goals_refs:
  - "zqk:kernel:goal:GOAL-001"
  - "zqk:kernel:goal:GOAL-002"
```

### 3. Domain Ontology Discovery

**Command**: `zqk domain discover`

Discovers and registers domain ontologies:

**Discovery Process**:

#### Step 1: Domain Registry
```bash
# Discover domain ontologies
zqk domain discover --scan

# Output:
# Domain Ontologies Discovered:
#   1. Organizational Domain (domain:organizational)
#      - Objects: organization, division, department, team, partnership
#      - Location: .zqk/domains/organizational/
#      - Status: registered
#   
#   2. Financial Services Domain (domain:financial)
#      - Objects: regulatory_object, compliance_requirement, audit_trail
#      - Location: .zqk/domains/financial/
#      - Status: registered
#   
#   3. Healthcare Domain (domain:healthcare)
#      - Objects: compliance_object, certification, patient_data_policy
#      - Location: .zqk/domains/healthcare/
#      - Status: registered
```

**Domain Registry Object**:
```yaml
id: DOMAIN-REG-001
kind: domain_registry
title: "Domain Ontology Registry"
domains:
  - domain_id: "organizational"
    namespace: "domain:organizational:*"
    description: "Organizational structure and relationships"
    objects:
      - organization
      - division
      - department
      - team
      - partnership
      - contract
    location: ".zqk/domains/organizational/"
    status: registered
    version: "1.0.0"
  
  - domain_id: "financial"
    namespace: "domain:financial:*"
    description: "Financial services regulatory and compliance objects"
    objects:
      - regulatory_object
      - compliance_requirement
      - audit_trail
    location: ".zqk/domains/financial/"
    status: registered
    version: "1.0.0"
```

#### Step 2: Domain Ontology Registration
```bash
# Register a domain ontology
zqk domain register \
  --domain-id "organizational" \
  --namespace "domain:organizational:*" \
  --location ".zqk/domains/organizational/" \
  --spec-file "organizational_ontology.yaml"

# Output:
# Domain: registered
# Domain ID: organizational
# Namespace: domain:organizational:*
# Objects: 6
# Status: active
```

**Domain Ontology Specification**:
```yaml
# .zqk/domains/organizational/organizational_ontology.yaml
domain:
  id: organizational
  namespace: domain:organizational:*
  version: "1.0.0"
  description: "Organizational structure and relationships"

objects:
  - id: organization
    kind: organization
    extends: extensible_object
    domain: organizational
    fields:
      - name: organization_name
        type: string
        required: true
      - name: divisions
        type: list
        item_type: division_ref
      - name: partnerships
        type: list
        item_type: partnership_ref
  
  - id: division
    kind: division
    extends: extensible_object
    domain: organizational
    fields:
      - name: division_name
        type: string
        required: true
      - name: parent_division_ref
        type: division_ref
        required: false
      - name: child_divisions
        type: list
        item_type: division_ref
      - name: teams
        type: list
        item_type: team_ref
      - name: zqk_goals_refs
        type: list
        item_type: zqk_kernel_goal_ref
```

### 4. Organizational Structure Modeling

**Organizational Domain Objects**:

#### Organization Object
```yaml
id: ORG-001
kind: organization
domain: organizational
namespace: domain:organizational:organization:ORG-001
title: "Acme Corp"
organization_name: "Acme Corp"
divisions:
  - domain:organizational:division:DIV-001
  - domain:organizational:division:DIV-002
partnerships:
  - domain:organizational:partnership:PART-001
zqk_context:
  code_owner: true
  authority_level: full_control
```

#### Division Object
```yaml
id: DIV-001
kind: division
domain: organizational
namespace: domain:organizational:division:DIV-001
title: "Engineering Division"
division_name: "Engineering"
parent_division_ref: null  # Top-level division
child_divisions:
  - domain:organizational:division:DIV-003  # Infrastructure
  - domain:organizational:division:DIV-004  # Product
teams:
  - domain:organizational:team:TEAM-001
  - domain:organizational:team:TEAM-002
zqk_goals_refs:
  - zqk:kernel:goal:GOAL-001
  - zqk:kernel:goal:GOAL-002
organizational_changes:
  - change_type: restructure
    date: "2025-12-01"
    description: "Split into Infrastructure and Product divisions"
    impact_analysis: "IMPACT-001"
```

#### Team Object
```yaml
id: TEAM-001
kind: team
domain: organizational
namespace: domain:organizational:team:TEAM-001
title: "Infrastructure Team"
team_name: "Infrastructure"
division_ref: domain:organizational:division:DIV-003
members:
  - account:alice
  - account:bob
zqk_workstreams_refs:
  - zqk:kernel:workstream:WS-001
  - zqk:kernel:workstream:WS-002
```

### 5. Organizational Change Impact Analysis

**Command**: `zqk organizational analyze-impact`

Analyzes impact of organizational changes on software delivery:

**Impact Analysis Process**:

#### Step 1: Change Detection
```bash
# Detect organizational changes
zqk organizational detect-changes

# Output:
# Organizational Changes Detected:
#   1. Division Restructure (2025-12-01)
#      - Division: Engineering (DIV-001)
#      - Change: Split into Infrastructure (DIV-003) and Product (DIV-004)
#      - Impact: 2 teams, 5 workstreams, 12 backlog items
#   
#   2. Team Reassignment (2025-12-15)
#      - Team: Data Science (TEAM-005)
#      - Change: Moved from Research to Product division
#      - Impact: 3 workstreams, 8 backlog items
```

#### Step 2: Impact Analysis
```bash
# Analyze impact of organizational change
zqk organizational analyze-impact --change CHANGE-001

# Output:
# Impact Analysis:
#   Change: Division Restructure (Engineering → Infrastructure + Product)
#   
#   Affected zqk Objects:
#     - Workstreams: 5 (WS-001, WS-002, WS-003, WS-004, WS-005)
#     - Goals: 3 (GOAL-001, GOAL-002, GOAL-003)
#     - Backlog Items: 12 (BLI-001 through BLI-012)
#     - Milestones: 4 (MIL-001, MIL-002, MIL-003, MIL-004)
#   
#   Impact Categories:
#     - Authority Changes: 5 workstreams need authority reassignment
#     - Goal Alignment: 3 goals need division reassignment
#     - Work Distribution: 12 backlog items need team reassignment
#     - Policy Compliance: 2 policies need division updates
#   
#   Recommended Actions:
#     1. Reassign workstream ownership to new divisions
#     2. Update goal-division mappings
#     3. Reassign backlog items to appropriate teams
#     4. Update policy applicability
```

**Impact Analysis Object**:
```yaml
id: IMPACT-001
kind: organizational_impact_analysis
title: "Impact of Engineering Division Restructure"
organizational_change_ref: "domain:organizational:change:CHANGE-001"
change_type: division_restructure
affected_objects:
  workstreams:
    - zqk:kernel:workstream:WS-001
    - zqk:kernel:workstream:WS-002
    - zqk:kernel:workstream:WS-003
    - zqk:kernel:workstream:WS-004
    - zqk:kernel:workstream:WS-005
  goals:
    - zqk:kernel:goal:GOAL-001
    - zqk:kernel:goal:GOAL-002
    - zqk:kernel:goal:GOAL-003
  backlog_items:
    - zqk:kernel:backlog_item:BLI-001
    # ... 11 more
  milestones:
    - zqk:kernel:milestone:MIL-001
    # ... 3 more

impact_categories:
  - category: authority_changes
    severity: high
    affected_count: 5
    description: "Workstream ownership needs reassignment"
  
  - category: goal_alignment
    severity: medium
    affected_count: 3
    description: "Goals need division reassignment"
  
  - category: work_distribution
    severity: medium
    affected_count: 12
    description: "Backlog items need team reassignment"
  
  - category: policy_compliance
    severity: low
    affected_count: 2
    description: "Policies need division updates"

recommended_actions:
  - action: reassign_workstream_ownership
    priority: high
    affected_objects: [WS-001, WS-002, WS-003, WS-004, WS-005]
  
  - action: update_goal_division_mappings
    priority: medium
    affected_objects: [GOAL-001, GOAL-002, GOAL-003]
  
  - action: reassign_backlog_items
    priority: medium
    affected_objects: [BLI-001 through BLI-012]
  
  - action: update_policy_applicability
    priority: low
    affected_objects: [POL-001, POL-002]
```

#### Step 3: Change Propagation
```bash
# Propagate organizational changes to zqk objects
zqk organizational propagate --change CHANGE-001 --confirm

# Output:
# Propagating organizational changes...
# ✓ Reassigned 5 workstreams to new divisions
# ✓ Updated 3 goal-division mappings
# ✓ Reassigned 12 backlog items to appropriate teams
# ✓ Updated 2 policy applicability rules
# ✓ Change propagation complete
```

### 6. Domain Ontology Integration

**Integration Points**:

#### Point 1: Domain Object Registration
```bash
# Register domain object type
zqk domain register-object \
  --domain "organizational" \
  --object-type "division" \
  --spec-file "division.yaml"

# Output:
# Domain Object: registered
# Domain: organizational
# Object Type: division
# Namespace: domain:organizational:division:*
# Status: active
```

#### Point 2: Cross-Layer Reference Validation
```bash
# Validate cross-layer references
zqk domain validate-references

# Output:
# Reference Validation:
#   ✓ All zqk:kernel:* references valid
#   ✓ All domain:organizational:* references valid
#   ✓ All cross-layer references valid
#   ✗ 2 broken references detected:
#     - domain:organizational:division:DIV-005 (deleted)
#     - domain:organizational:team:TEAM-010 (moved)
```

#### Point 3: Domain Object Lifecycle
```bash
# Handle domain object lifecycle
zqk domain lifecycle --object DIV-001 --transition restructure

# Output:
# Domain Object Lifecycle:
#   Object: DIV-001 (division)
#   Transition: restructure
#   Impact: 5 workstreams, 3 goals, 12 backlog items
#   Actions: Reassigning to new divisions...
#   ✓ Lifecycle transition complete
```

### 7. Political Ecosystem Awareness

**Political Context Objects**:

#### Political Context
```yaml
id: POL-CTX-001
kind: political_context
domain: organizational
namespace: domain:organizational:political_context:POL-CTX-001
title: "Engineering Division Political Dynamics"
organizational_units:
  - division: DIV-001
    political_dynamics:
      - dynamic: "Infrastructure team prioritizes stability"
        impact: "Slower feature delivery"
        alignment: "Conflicts with product team's rapid iteration"
      
      - dynamic: "Product team prioritizes speed"
        impact: "Technical debt accumulation"
        alignment: "Conflicts with infrastructure team's stability focus"
  
  - division: DIV-002
    political_dynamics:
      - dynamic: "Data science team needs experimentation freedom"
        impact: "Requires flexible policies"
        alignment: "Aligned with research goals, conflicts with production stability"

conflict_areas:
  - area: "Feature delivery speed vs. stability"
    parties: [DIV-001, DIV-002]
    resolution: "Partnership constraints create alignment"
  
  - area: "Experimentation vs. production standards"
    parties: [DIV-002, DIV-003]
    resolution: "Separate environments with clear boundaries"

alignment_mechanisms:
  - mechanism: "Partnership constraints"
    description: "SLA and quality standards create alignment"
    effectiveness: high
  
  - mechanism: "Shared goals"
    description: "Common goals create strategic alignment"
    effectiveness: medium
```

**Political Awareness System**:

**Command**: `zqk political analyze`

Analyzes political dynamics and their impact:

```bash
# Analyze political dynamics
zqk political analyze

# Output:
# Political Dynamics Analysis:
#   Conflict Areas: 2
#     - Feature delivery speed vs. stability (DIV-001 vs DIV-002)
#     - Experimentation vs. production standards (DIV-002 vs DIV-003)
#   
#   Alignment Mechanisms: 2
#     - Partnership constraints (effectiveness: high)
#     - Shared goals (effectiveness: medium)
#   
#   Recommendations:
#     - Maintain partnership constraints to ensure alignment
#     - Create shared goals for strategic alignment
#     - Separate environments for experimentation vs. production
```

### 8. Domain Ontology Evolution

**Evolution Support**:

#### Versioning
```yaml
# Domain ontology versioning
domain:
  id: organizational
  version: "1.2.0"
  previous_version: "1.1.0"
  changes:
    - type: object_added
      object: "sub_division"
      description: "Added sub-division object type"
    
    - type: field_added
      object: "division"
      field: "budget_allocation"
      description: "Added budget allocation field"
    
    - type: relationship_added
      from: "division"
      to: "partnership"
      relationship: "manages"
      description: "Added division-manages-partnership relationship"
```

#### Migration Support
```bash
# Migrate domain ontology to new version
zqk domain migrate --domain organizational --to-version 1.2.0

# Output:
# Domain Migration: starting
# Domain: organizational
# From Version: 1.1.0
# To Version: 1.2.0
# 
# Migration Steps:
#   1. Add sub_division object type
#   2. Add budget_allocation field to divisions
#   3. Add division-manages-partnership relationships
# 
# ✓ Migration complete
```

### 9. Organizational Structure Sync

**Command**: `zqk organizational sync`

Syncs organizational structure from external sources:

**Sync Process**:

#### Step 1: External Source Detection
```bash
# Detect external organizational sources
zqk organizational sync --detect-sources

# Output:
# External Sources Detected:
#   1. HR System (LDAP/Active Directory)
#      - Organizations: 1
#      - Divisions: 5
#      - Teams: 20
#      - Last Sync: 2025-12-29
#   
#   2. Project Management System (Jira)
#      - Teams: 15
#      - Projects: 30
#      - Last Sync: 2025-12-30
```

#### Step 2: Sync Execution
```bash
# Sync organizational structure
zqk organizational sync --source hr_system --confirm

# Output:
# Syncing organizational structure...
# ✓ Synced 1 organization
# ✓ Synced 5 divisions
# ✓ Synced 20 teams
# ✓ Detected 2 changes:
#   - Division DIV-003 renamed: "Infrastructure" → "Platform Engineering"
#   - Team TEAM-005 moved: DIV-002 → DIV-003
# ✓ Impact analysis triggered
# ✓ Sync complete
```

### 10. Domain Ontology Templates

**Template System**:

#### Template 1: Organizational Domain
```yaml
# .zqk/domains/organizational/template.yaml
domain:
  id: organizational
  namespace: domain:organizational:*
  objects:
    - organization
    - division
    - department
    - team
    - partnership
    - contract
```

#### Template 2: Financial Services Domain
```yaml
# .zqk/domains/financial/template.yaml
domain:
  id: financial
  namespace: domain:financial:*
  objects:
    - regulatory_object
    - compliance_requirement
    - audit_trail
    - certification
```

#### Template 3: Healthcare Domain
```yaml
# .zqk/domains/healthcare/template.yaml
domain:
  id: healthcare
  namespace: domain:healthcare:*
  objects:
    - compliance_object
    - certification
    - patient_data_policy
    - hipaa_requirement
```

## Implementation

### Phase 1: Domain Registry

```go
// pkg/domain/registry.go
type DomainRegistry struct {
    domains map[string]*Domain
    mu sync.RWMutex
}

func (dr *DomainRegistry) Register(domain *Domain) error {
    // 1. Validate domain specification
    // 2. Register domain in registry
    // 3. Load domain object types
    // 4. Initialize domain namespace
}
```

### Phase 2: Organizational Modeling

```go
// pkg/domain/organizational/model.go
type OrganizationalModel struct {
    organizations map[string]*Organization
    divisions map[string]*Division
    teams map[string]*Team
}

func (om *OrganizationalModel) SyncFromExternal(source ExternalSource) error {
    // 1. Fetch organizational data from external source
    // 2. Detect changes
    // 3. Update organizational objects
    // 4. Trigger impact analysis
}
```

### Phase 3: Impact Analysis

```go
// pkg/domain/organizational/impact.go
type ImpactAnalyzer struct {
    storage ObjectStorageProvider
    organizationalModel *OrganizationalModel
}

func (ia *ImpactAnalyzer) AnalyzeChange(change *OrganizationalChange) (*ImpactAnalysis, error) {
    // 1. Identify affected zqk objects
    // 2. Categorize impacts
    // 3. Generate recommendations
    // 4. Create impact analysis object
}
```

## Use Cases

### Use Case 1: Organizational Restructuring

**Scenario**: Company reorganizes divisions

**Process**:
```bash
# 1. Detect organizational change
zqk organizational detect-changes
# Output: Division restructure detected

# 2. Analyze impact
zqk organizational analyze-impact --change CHANGE-001
# Output: 5 workstreams, 3 goals, 12 backlog items affected

# 3. Propagate changes
zqk organizational propagate --change CHANGE-001 --confirm
# Output: Changes propagated to zqk objects
```

### Use Case 2: Domain Ontology Discovery

**Scenario**: Financial services domain needs regulatory objects

**Process**:
```bash
# 1. Discover domain ontologies
zqk domain discover --scan
# Output: Financial domain discovered

# 2. Register domain
zqk domain register --domain-id financial --spec-file financial_ontology.yaml
# Output: Domain registered

# 3. Create domain objects
zqk object create regulatory_object --domain financial --file reg-object.yaml
# Output: Domain object created
```

### Use Case 3: Political Ecosystem Integration

**Scenario**: Different teams have different priorities

**Process**:
```bash
# 1. Analyze political dynamics
zqk political analyze
# Output: Conflict areas and alignment mechanisms identified

# 2. Create political context
zqk object create political_context --domain organizational --file political.yaml
# Output: Political context created

# 3. Configure alignment mechanisms
zqk align (PRUNED)ment configure --political-context POL-CTX-001
# Output: Alignment mechanisms configured
```

## Benefits

1. **Layered Ontology**: Clear separation between zqk kernel and domain objects
2. **Organizational Modeling**: Organizational structures modeled as external domain objects
3. **Change Accommodation**: Organizational changes propagate with impact analysis
4. **Domain Discovery**: Domain ontologies discoverable and integrable
5. **Political Awareness**: System understands and accommodates political dynamics
6. **Evolution Support**: System accommodates organizational shifts without barriers

## Related Documentation

- [System Ontology v1.0](./ontology/system-ontology-v1.0.md)
- [Knowledge Kernel Separation](./knowledge-kernel-separation-v1.0.md)
- [Extensible Objects](../_internal/object_specs/extensible_object.yaml)

---

*This multi-layered ontology system ensures zqk can accommodate evolving organizational structures, integrate domain-specific ontologies, and operate smoothly in challenging political ecosystems without creating barriers for evolution.*

