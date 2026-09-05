# Legacy Codebase Migration Strategy v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Design  
**Related**: BLI-125, POL-WORKFLOW-001, POL-WORKFLOW-002

## Overview

This document defines a comprehensive strategy for safely migrating legacy codebases into zqk compliance. The strategy emphasizes **incremental adoption**, **risk mitigation**, **observable progress**, and **stakeholder communication** to ensure migrations happen predictably without being too disruptive or too conservative.

## Problem Statement

When onboarding a legacy codebase to zqk:

1. **No Structure**: Codebases may lack organized documentation, clear branching conventions, or structured project management
2. **Existing Conventions**: Legacy codebases have established patterns that may conflict with zqk best practices
3. **Risk of Disruption**: Enforcing zqk policies immediately could break existing workflows
4. **Stakeholder Concerns**: Teams need to understand how and why the codebase is evolving
5. **Incremental Adoption**: Need a path that allows gradual migration without requiring "big bang" changes

## Solution: Adaptive Migration Framework

### Core Principles

1. **Assess Before Enforce**: Understand the current state before applying policies
2. **Dynamic Policy Evolution**: Policies adapt to codebase maturity level
3. **Incremental Adoption**: Gradual migration with clear milestones
4. **Observable Progress**: Clear metrics and roadmaps for stakeholders
5. **Risk-Aware**: Identify and mitigate risks before they become blockers
6. **Stakeholder Communication**: Generate explanations and architectural documentation

## Architecture

### 1. Codebase Assessment Engine

**Command**: `zqk system assess`

Analyzes the codebase to understand:
- **Git Structure**: Branching conventions, commit patterns, merge strategies
- **Documentation**: Existing docs, README files, scattered information
- **Project Structure**: Directory organization, file patterns, conventions
- **Compliance Gaps**: What zqk policies would conflict with current state
- **Risk Factors**: Areas that need special attention during migration
- **Migration Complexity**: Estimated effort and timeline
- **Existing Conventions**: Discovered patterns, rules, and established practices
- **Policy Discovery**: Extract implicit policies from codebase patterns

**Output**: Assessment report with:
- Current state analysis
- Compliance gap analysis
- Risk assessment
- Recommended migration phases
- Temporary policy recommendations
- **Baseline Policy Discovery**: Extracted policies from existing conventions
- **Policy Lineage**: Traceability from discovered policies to zqk standards

### 2. Dynamic Policy System

**Concept**: Policies that evolve based on codebase maturity, starting from discovered baseline

**Policy Discovery & Baseline Generation**:

**Command**: `zqk system assess --discover-policies`

Discovers existing conventions and generates baseline policies:
- **Pattern Analysis**: Analyzes git history, code patterns, documentation
- **Convention Extraction**: Identifies implicit rules and practices
- **Baseline Policy Generation**: Creates policies that codify existing conventions
- **zqk Mapping**: Maps discovered policies to zqk standard policies

**Example Discovery Process**:
```bash
# Discover existing conventions
zqk system assess --discover-policies

# Output:
# - Discovered: Branch naming uses "bugfix/", "hotfix/", "feature/"
# - Discovered: Commits use conventional commits format
# - Discovered: Code review required for all PRs
# - Generated: POL-BASELINE-001 (Branch Naming - Legacy)
# - Generated: POL-BASELINE-002 (Commit Format - Legacy)
# - Generated: POL-BASELINE-003 (Code Review - Legacy)
```

**Policy States**:
- **`baseline`**: Policies discovered from existing conventions (starting point)
- **`legacy_compatible`**: Temporary policies that accommodate legacy patterns
- **`transitional`**: Policies that bridge legacy and zqk patterns
- **`zqk_standard`**: Full zqk compliance policies

**Policy Lifecycle with Lineage**:
1. **Discovery Phase**: Extract baseline policies from existing conventions
2. **Assessment Phase**: Generate `legacy_compatible` policies (based on baseline)
3. **Migration Phase**: Transition to `transitional` policies
4. **Compliance Phase**: Enforce `zqk_standard` policies

**Policy Lineage Tracking**:
```yaml
# Baseline policy (discovered)
id: POL-BASELINE-001
kind: policy
policy_state: baseline
discovered_from: "git branch analysis"
discovered_pattern: "bugfix/*, hotfix/*, feature/*"
lineage:
  evolves_to: POL-WORKFLOW-001-LEGACY
  target_standard: POL-WORKFLOW-001

# Legacy compatible policy (generated from baseline)
id: POL-WORKFLOW-001-LEGACY
kind: policy
policy_state: legacy_compatible
lineage:
  evolved_from: POL-BASELINE-001
  evolves_to: POL-WORKFLOW-001-TRANSITIONAL
  target_standard: POL-WORKFLOW-001

# Transitional policy
id: POL-WORKFLOW-001-TRANSITIONAL
kind: policy
policy_state: transitional
lineage:
  evolved_from: POL-WORKFLOW-001-LEGACY
  evolves_to: POL-WORKFLOW-001
  target_standard: POL-WORKFLOW-001

# zqk standard policy
id: POL-WORKFLOW-001
kind: policy
policy_state: zqk_standard
lineage:
  evolved_from: POL-WORKFLOW-001-TRANSITIONAL
  origin_baseline: POL-BASELINE-001
```

**Example Evolution**: Branch naming policy
- **Baseline**: Discovered pattern `bugfix/123`, `hotfix/prod`, `feature/new-thing`
- **Legacy Compatible**: Accept existing branch names (codifies baseline)
- **Transitional**: Accept both legacy and zqk patterns
- **zqk Standard**: Enforce zqk patterns (e.g., `feature/pri-210`)

### 3. Migration Roadmap Generator

**Command**: `zqk system roadmap`

Generates a phased migration plan:
- **Phase 1: Assessment & Baseline** (Week 1)
  - Run assessment
  - Establish baseline metrics
  - Generate temporary policies
  - Create initial backlog items

- **Phase 2: Structure Establishment** (Weeks 2-3)
  - Create `docs/process` structure
  - Migrate existing documentation
  - Establish initial priority plans
  - Set up basic zqk structure

- **Phase 3: Workflow Integration** (Weeks 4-6)
  - Introduce zqk branching conventions (parallel with legacy)
  - Set up pre-commit hooks (warnings only)
  - Begin using zqk object management
  - Maintain backward compatibility

- **Phase 4: Policy Enforcement** (Weeks 7-8)
  - Transition policies from `legacy_compatible` to `transitional`
  - Enforce critical policies (with grace periods)
  - Monitor compliance metrics
  - Address blockers

- **Phase 5: Full Compliance** (Weeks 9+)
  - Enforce all zqk standards
  - Remove legacy compatibility
  - Optimize workflows
  - Continuous improvement

### 4. Risk Assessment System

**Command**: `zqk system assess --risk`

Identifies and categorizes risks:

**Risk Categories**:
- **High Risk**: Could break existing workflows (e.g., blocking commits to main)
- **Medium Risk**: May cause confusion or require training (e.g., new branching conventions)
- **Low Risk**: Minor adjustments needed (e.g., documentation structure)

**Risk Mitigation**:
- **High Risk**: Implement with grace periods, warnings before enforcement
- **Medium Risk**: Provide training, documentation, gradual rollout
- **Low Risk**: Direct implementation with clear communication

### 5. Stakeholder Communication Tools

**Command**: `zqk system explain`

Generates human-readable explanations:
- **Migration Rationale**: Why changes are being made
- **Impact Analysis**: What will change and what won't
- **Timeline**: When changes will happen
- **Benefits**: What the team gains from migration
- **Architectural Documentation**: How the new structure works

**Output Formats**:
- Markdown documentation
- Presentation slides
- Email templates
- Team meeting agendas

## Implementation

### Phase 1: Assessment Command

```bash
# Run full assessment
zqk system assess

# Discover existing conventions and generate baseline policies
zqk system assess --discover-conventions
zqk system assess --discover-policies

# Generate baseline policies from discovered conventions
zqk policy generate --from-baseline

# Generate risk report
zqk system assess --risk

# Generate migration roadmap with evolutionary path
zqk system assess --roadmap --evolutionary

# Generate stakeholder communication
zqk system assess --explain

# View policy lineage
zqk policy lineage POL-WORKFLOW-001
zqk policy trace POL-BASELINE-001

# View migration traceability
zqk migration trace MIG-001
zqk migration roadmap MIG-001 --visualize
```

### Phase 2: Dynamic Policy System

**Policy Metadata**:
```yaml
policy_state: legacy_compatible | transitional | zqk_standard
migration_phase: 1-5
effective_date: "2025-01-15"
sunset_date: "2025-03-01"  # When legacy_compatible policies expire
```

**Policy Evaluation**:
- System checks current migration phase
- Applies appropriate policy state
- Logs policy transitions for audit

### Phase 3: Migration Tracking

**Migration Object**:
```yaml
id: MIG-001
kind: migration
status: in_progress
current_phase: 3
assessed_at: "2025-01-01"
started_at: "2025-01-02"
target_completion: "2025-03-01"
risks:
  - category: high
    description: "Branch naming conflicts"
    mitigation: "Parallel naming conventions for 2 weeks"
metrics:
  compliance_score: 65
  policy_enforcement: 40%
  structure_complete: 80%
```

## Safety Mechanisms

### 1. Grace Periods

Critical policies (e.g., no commits to main) start with:
- **Week 1-2**: Warnings only
- **Week 3-4**: Warnings + notifications
- **Week 5+**: Full enforcement

### 2. Rollback Capability

- All policy changes are reversible
- Migration state can be rolled back
- Legacy workflows remain functional during transition

### 3. Parallel Operation

- Legacy and zqk workflows can coexist
- Gradual migration of features
- No "big bang" cutover required

### 4. Monitoring & Alerts

- Track compliance metrics
- Alert on policy violations
- Monitor migration progress
- Identify blockers early

## Example Migration Scenario

### Legacy Codebase Profile

- **Branching**: Uses `bugfix/`, `hotfix/`, `feature/` (no priority plan alignment)
- **Documentation**: Scattered README files, no structured docs
- **Project Management**: GitHub Issues, no backlog structure
- **Compliance**: No policies, no structure checks

### Convention Discovery

```bash
zqk system assess --discover-conventions
```

**Discovered Conventions**:
```yaml
discovered_conventions:
  git:
    branch_patterns:
      - pattern: "bugfix/*"
        frequency: 45%
        confidence: 0.95
      - pattern: "hotfix/*"
        frequency: 15%
        confidence: 0.90
      - pattern: "feature/*"
        frequency: 40%
        confidence: 0.95
    commit_patterns:
      - pattern: "^(feat|fix|chore):"
        frequency: 78%
        convention: "conventional commits"
        confidence: 0.85
  
  process:
    code_review:
      - pattern: "PR requires 1 approval"
        frequency: 100%
        confidence: 1.0
```

### Baseline Policy Generation

```bash
zqk policy generate --from-baseline
```

**Generated Baseline Policies**:
```yaml
# POL-BASELINE-001: Branch Naming (discovered)
lineage:
  evolves_to: POL-WORKFLOW-001-LEGACY
  target_standard: POL-WORKFLOW-001

# POL-BASELINE-002: Commit Format (discovered)
lineage:
  evolves_to: POL-CODE-009-LEGACY
  target_standard: POL-CODE-009

# POL-BASELINE-003: Code Review (discovered)
lineage:
  evolves_to: POL-WORKFLOW-003-LEGACY
  target_standard: POL-WORKFLOW-003
```

### Assessment Results

```yaml
assessment:
  git_structure:
    branch_patterns: ["bugfix/*", "hotfix/*", "feature/*"]
    commit_patterns: ["fix:", "feat:", "chore:"]
    merge_strategy: "merge commits"
  
  documentation:
    readme_files: 12
    scattered_docs: true
    structured_docs: false
  
  compliance_gaps:
    - policy: POL-WORKFLOW-001
      gap: "Branch naming doesn't align with priority plans"
      risk: medium
    - policy: POL-DOC-001
      gap: "No structured documentation"
      risk: low
    - policy: POL-CODE-004
      gap: "No system integrity checks"
      risk: high
  
  migration_complexity: medium
  estimated_timeline: "8-10 weeks"
```

### Generated Policies with Lineage

```yaml
# Baseline policy (discovered from conventions)
id: POL-BASELINE-001
policy_state: baseline
discovered_from: "git branch analysis"
lineage:
  evolves_to: POL-WORKFLOW-001-LEGACY
  target_standard: POL-WORKFLOW-001

# Legacy compatible policy (generated from baseline)
id: POL-WORKFLOW-001-LEGACY
policy_state: legacy_compatible
migration_phase: 1-3
effective_date: "2025-01-15"
sunset_date: "2025-03-01"
lineage:
  evolved_from: POL-BASELINE-001
  evolves_to: POL-WORKFLOW-001-TRANSITIONAL
  target_standard: POL-WORKFLOW-001

body: |
  **Temporary Policy: Legacy Branch Compatibility**
  
  Evolved from baseline policy POL-BASELINE-001 which codified existing
  branch naming convention (bugfix/*, hotfix/*, feature/*).
  
  During migration phase 1-3, both legacy and zqk branch naming are accepted:
  - Legacy: bugfix/*, hotfix/*, feature/* (from baseline)
  - zqk: feature/pri-*, chore/pri-*, fix/pri-*
  
  After phase 3, will evolve to POL-WORKFLOW-001-TRANSITIONAL.
  Final target: POL-WORKFLOW-001 (zqk standard).
```

### Evolutionary Roadmap

```bash
zqk migration roadmap MIG-001 --visualize
```

**Evolutionary Path**:
```
Baseline (Week 1)
  └─> Discovered 8 conventions
  └─> Generated 8 baseline policies
       │
       ├─> POL-BASELINE-001 (Branch Naming)
       │   └─> POL-WORKFLOW-001-LEGACY (Phase 1-3)
       │       └─> POL-WORKFLOW-001-TRANSITIONAL (Phase 4)
       │           └─> POL-WORKFLOW-001 (Phase 5+)
       │
       ├─> POL-BASELINE-002 (Commit Format)
       │   └─> POL-CODE-009-LEGACY (Phase 1-3)
       │       └─> POL-CODE-009-TRANSITIONAL (Phase 4)
       │           └─> POL-CODE-009 (Phase 5+)
       │
       └─> ... (6 more policies)

Current Position: Phase 3
Evolutionary State: 60% toward zqk Standard
Lineage Completeness: 85% (policies have complete evolution paths)
```

### Migration Roadmap

```yaml
phases:
  - phase: 1
    name: "Assessment & Baseline"
    duration: "1 week"
    activities:
      - Run assessment
      - Generate temporary policies
      - Create initial backlog
      - Establish baseline metrics
  
  - phase: 2
    name: "Structure Establishment"
    duration: "2 weeks"
    activities:
      - Create docs/process structure
      - Migrate documentation
      - Set up basic zqk objects
  
  - phase: 3
    name: "Workflow Integration"
    duration: "3 weeks"
    activities:
      - Introduce zqk branching (parallel)
      - Set up pre-commit hooks (warnings)
      - Begin object management
  
  - phase: 4
    name: "Policy Enforcement"
    duration: "2 weeks"
    activities:
      - Transition to transitional policies
      - Enforce critical policies
      - Monitor compliance
  
  - phase: 5
    name: "Full Compliance"
    duration: "Ongoing"
    activities:
      - Enforce all standards
      - Remove legacy compatibility
      - Optimize workflows
```

## Success Metrics

### Compliance Score

```yaml
metrics:
  compliance_score: 0-100
  calculation:
    - policy_enforcement: 40%
    - structure_completeness: 30%
    - workflow_adoption: 20%
    - documentation_quality: 10%
```

### Evolutionary Progress Metrics

```yaml
evolutionary_metrics:
  baseline_establishment: 100%  # Baseline policies discovered
  policy_evolution: 60%         # Policies evolved from baseline
  lineage_completeness: 85%      # Policies with complete lineage
  zqk_alignment: 45%          # Alignment with zqk standards
  roadmap_progress: 60%         # Progress through evolutionary phases
```

### Progress Tracking

- **Phase Completion**: Track completion of each migration phase
- **Policy Adoption**: Monitor policy enforcement levels
- **Risk Resolution**: Track risk mitigation progress
- **Stakeholder Satisfaction**: Survey team on migration experience
- **Evolutionary Position**: Track position on evolutionary roadmap
- **Lineage Completeness**: Track policy lineage establishment
- **Baseline Coverage**: Track how many conventions have baseline policies

## Policy Discovery & Baseline Generation

### Convention Discovery

**Command**: `zqk system assess --discover-conventions`

Analyzes codebase to discover existing patterns:

**Git Convention Discovery**:
- **Branch Patterns**: Analyzes branch names to identify patterns
- **Commit Patterns**: Analyzes commit messages for conventions
- **Merge Patterns**: Analyzes merge strategies (merge commits, squash, rebase)
- **Tag Patterns**: Analyzes version tagging conventions

**Code Convention Discovery**:
- **File Organization**: Directory structure patterns
- **Naming Conventions**: File, function, class naming patterns
- **Code Style**: Linting rules, formatting conventions
- **Documentation Patterns**: Comment styles, README patterns

**Process Convention Discovery**:
- **Review Requirements**: PR requirements, approval processes
- **Testing Patterns**: Test organization, coverage requirements
- **Deployment Patterns**: CI/CD conventions, release processes

**Example Discovery Output**:
```yaml
discovered_conventions:
  git:
    branch_patterns:
      - pattern: "bugfix/*"
        frequency: 45%
        examples: ["bugfix/123", "bugfix/auth-fix"]
      - pattern: "hotfix/*"
        frequency: 15%
        examples: ["hotfix/prod-issue"]
      - pattern: "feature/*"
        frequency: 40%
        examples: ["feature/new-ui", "feature/api-v2"]
    commit_patterns:
      - pattern: "^(feat|fix|chore|docs):"
        frequency: 78%
        convention: "conventional commits"
    merge_strategy: "merge commits"
  
  code:
    file_organization:
      - pattern: "src/**, tests/**, docs/**"
        structure: "standard"
    naming_conventions:
      - pattern: "PascalCase for classes"
        frequency: 95%
  
  process:
    code_review:
      - pattern: "PR requires 2 approvals"
      - pattern: "All CI checks must pass"
```

### Baseline Policy Generation

**Command**: `zqk policy generate --from-baseline`

Generates baseline policies from discovered conventions:

**Generation Process**:
1. **Convention Analysis**: Analyzes discovered conventions
2. **Policy Template Matching**: Matches conventions to policy templates
3. **Baseline Policy Creation**: Creates policies that codify existing patterns
4. **zqk Mapping**: Maps baseline policies to target zqk standards
5. **Lineage Establishment**: Creates lineage relationships

**Example Generated Baseline Policy**:
```yaml
id: POL-BASELINE-001
kind: policy
title: "Branch Naming - Discovered Convention"
policy_state: baseline
category: workflow
policy_type: standard
version: 1.0.0
effective_date: "2025-01-15"  # Assessment date

# Discovery metadata
discovery:
  discovered_at: "2025-01-15T10:00:00Z"
  discovered_from: "git branch analysis"
  confidence: 0.95
  pattern: "bugfix/*, hotfix/*, feature/*"
  frequency: 100%
  examples:
    - "bugfix/123"
    - "hotfix/prod-issue"
    - "feature/new-ui"

# Lineage tracking
lineage:
  evolves_to: POL-WORKFLOW-001-LEGACY
  target_standard: POL-WORKFLOW-001
  evolution_phase: 1

body: |
  **Baseline Policy: Branch Naming Convention**
  
  This policy codifies the existing branch naming convention discovered
  during codebase assessment.
  
  **Current Convention**:
  - `bugfix/*` - Bug fixes
  - `hotfix/*` - Production hotfixes
  - `feature/*` - New features
  
  **Evolution Path**:
  This policy will evolve through migration phases:
  1. Phase 1-3: Legacy compatible (accepts current patterns)
  2. Phase 4: Transitional (accepts both legacy and zqk)
  3. Phase 5: zqk standard (enforces `feature/pri-*` pattern)
```

### Policy Lineage & Traceability

**Lineage Tracking**:

Each policy tracks its evolution:
```yaml
lineage:
  # Where this policy came from
  evolved_from: POL-BASELINE-001  # or null if baseline
  origin_baseline: POL-BASELINE-001  # Always points to baseline
  
  # Where this policy is going
  evolves_to: POL-WORKFLOW-001-TRANSITIONAL  # Next evolution
  target_standard: POL-WORKFLOW-001  # Final zqk standard
  
  # Evolution metadata
  evolution_phase: 1  # Which migration phase this policy belongs to
  evolution_date: "2025-01-20"  # When policy should evolve
  evolution_trigger: "phase_completion"  # What triggers evolution
```

**Lineage Visualization**:

**Command**: `zqk policy lineage POL-WORKFLOW-001`

Shows complete evolution path:
```
Baseline → Legacy Compatible → Transitional → zqk Standard
POL-BASELINE-001 → POL-WORKFLOW-001-LEGACY → POL-WORKFLOW-001-TRANSITIONAL → POL-WORKFLOW-001

Timeline:
  Phase 1 (Week 1):     POL-BASELINE-001 (discovered)
  Phase 1-3 (Weeks 1-6): POL-WORKFLOW-001-LEGACY (active)
  Phase 4 (Weeks 7-8):   POL-WORKFLOW-001-TRANSITIONAL (active)
  Phase 5+ (Week 9+):    POL-WORKFLOW-001 (active)
```

**Command**: `zqk policy trace POL-BASELINE-001`

Shows forward traceability (where baseline leads):
```
POL-BASELINE-001 (baseline)
  └─> POL-WORKFLOW-001-LEGACY (legacy_compatible)
      └─> POL-WORKFLOW-001-TRANSITIONAL (transitional)
          └─> POL-WORKFLOW-001 (zqk_standard)
```

## Evolutionary Roadmap Tracking

### Migration State Object

**Object Type**: `migration`

Tracks complete migration state and evolutionary position:

```yaml
id: MIG-001
kind: migration
title: "E-Commerce Platform Migration"
status: in_progress
current_phase: 3
current_evolutionary_state: "workflow_integration"

# Evolutionary roadmap position
evolutionary_roadmap:
  baseline_established: true
  baseline_date: "2025-01-15"
  phases:
    - phase: 1
      name: "Assessment & Baseline"
      status: complete
      completed_at: "2025-01-22"
      policies_active: 5
      policies_discovered: 8
    - phase: 2
      name: "Structure Establishment"
      status: complete
      completed_at: "2025-02-05"
      policies_active: 12
    - phase: 3
      name: "Workflow Integration"
      status: in_progress
      started_at: "2025-02-06"
      policies_active: 15
      policies_transitioned: 3
    - phase: 4
      name: "Policy Enforcement"
      status: pending
      policies_planned: 20
    - phase: 5
      name: "Full Compliance"
      status: pending
      target_compliance: 95%

# Policy evolution tracking
policy_evolution:
  baseline_policies: 8
  legacy_compatible_policies: 12
  transitional_policies: 3
  zqk_standard_policies: 0
  total_evolutions: 3
  
# Lineage summary
lineage_summary:
  policies_with_lineage: 15
  complete_lineages: 8
  partial_lineages: 7
  orphaned_policies: 0
```

### Evolutionary Roadmap Visualization

**Command**: `zqk migration roadmap MIG-001 --visualize`

Generates visual roadmap showing:
- **Current Position**: Where project is on evolutionary path
- **Policy Evolution**: Which policies have evolved and which are pending
- **Compliance Progress**: Progress toward zqk standards
- **Timeline**: When each phase completes
- **Milestones**: Key evolutionary milestones

**Example Output**:
```
Evolutionary Roadmap: E-Commerce Platform Migration

Baseline ──────────> Legacy Compatible ──────> Transitional ──────> zqk Standard
(Week 1)            (Weeks 1-6)              (Weeks 7-8)          (Week 9+)
     │                     │                        │                    │
     │                     │                        │                    │
     ▼                     ▼                        ▼                    ▼
[████]                 [████████]              [██]                [    ]
8 policies           12 policies             3 policies          0 policies

Current Position: Phase 3 (Workflow Integration)
Evolutionary State: 60% toward zqk Standard
Next Milestone: Phase 4 (Policy Enforcement) - Week 7
```

### Traceability Queries

**Command**: `zqk migration trace MIG-001`

Shows complete traceability:
- **Baseline → Current**: What policies started from baseline
- **Current → Target**: What policies will become
- **Evolution History**: Complete evolution path for each policy
- **Compliance Gaps**: What still needs to evolve

**Example**:
```bash
# Show all policies that evolved from baseline
zqk migration trace MIG-001 --from-baseline

# Show all policies that will evolve to zqk standard
zqk migration trace MIG-001 --to-standard

# Show evolution history for specific policy
zqk migration trace MIG-001 --policy POL-WORKFLOW-001

# Show compliance gaps (policies not yet evolved)
zqk migration trace MIG-001 --gaps
```

## Policy Lifecycle Management

### Policy States and Transitions

Policies exist in one of three states during migration:

1. **`legacy_compatible`**: Temporary policies that accommodate existing patterns
   - **Activation**: Generated during assessment phase
   - **Duration**: Phases 1-3 (weeks 1-6)
   - **Retirement**: Automatically transitioned when migration phase advances

2. **`transitional`**: Policies that bridge legacy and zqk patterns
   - **Activation**: When migration phase 4 begins
   - **Duration**: Phases 4-5 (weeks 7-10)
   - **Retirement**: When full compliance achieved

3. **`zqk_standard`**: Full zqk compliance policies
   - **Activation**: When migration phase 5 completes
   - **Duration**: Permanent (until superseded)
   - **Retirement**: Only via policy lifecycle (deprecated/superseded)

### Policy Activation Triggers

**Automatic Activation**:
- **Phase-based**: Policies activate based on migration phase
- **Date-based**: `effective_date` reached (for scheduled policies)
- **Compliance-based**: When compliance threshold reached (e.g., 80% compliance)

**Manual Activation**:
- **Human approval**: Via `zqk policy activate` command
- **Governor pattern**: High-stakes policy changes require human sign-off
- **Decision record**: Human provides rationale and approval

### Policy Retirement Triggers

**Automatic Retirement**:
- **Sunset date**: `sunset_date` reached (for temporary policies)
- **Phase completion**: Migration phase completes, legacy policies retired
- **Supersession**: New policy supersedes old policy

**Manual Retirement**:
- **Human decision**: Via `zqk policy retire` command
- **Assessment-based**: Assessment determines policy no longer needed
- **Stakeholder feedback**: Team requests policy retirement

### Policy Orchestration

**Command**: `zqk policy orchestrate`

Manages policy lifecycle across migration:
- **Phase Detection**: Detects current migration phase
- **Policy Evaluation**: Determines which policies should be active
- **Transition Planning**: Plans policy activations/retirements
- **Compliance Checking**: Verifies policies can be safely activated
- **Human Approval**: Requests approval for high-stakes transitions

**Example**:
```bash
# Review policy transitions for next phase
zqk policy orchestrate --phase 4 --preview

# Execute policy transitions (requires approval)
zqk policy orchestrate --phase 4 --execute

# Generate policy transition report
zqk policy orchestrate --phase 4 --report
```

## Multi-Repository Coordination

### Project Group Concept

**Object Type**: `project_group`

Represents a collection of related repositories:
```yaml
id: PG-001
kind: project_group
title: "E-Commerce Platform"
repositories:
  - path: "/path/to/frontend"
    url: "https://github.com/org/frontend"
    role: "frontend"
    build_pipeline: "npm"
  - path: "/path/to/backend"
    url: "https://github.com/org/backend"
    role: "backend"
    build_pipeline: "maven"
  - path: "/path/to/mobile"
    url: "https://github.com/org/mobile"
    role: "mobile"
    build_pipeline: "gradle"
migration_state: in_progress
current_phase: 3
```

### Cross-Repository Assessment

**Command**: `zqk system assess --project-group PG-001`

Assesses all repositories in a project group:
- **Individual Assessments**: Run assessment on each repository
- **Aggregated Analysis**: Combine results across repositories
- **Dependency Mapping**: Identify cross-repo dependencies
- **Unified Roadmap**: Generate single migration roadmap for all repos
- **Coordinated Policies**: Ensure policies are consistent across repos

### Repository-Specific Policies

Policies can be scoped to specific repositories:

```yaml
# Policy applies to all repos in project group
applicability:
  project_groups: [PG-001]
  
# Policy applies to specific repository
applicability:
  repositories: ["frontend", "backend"]
  
# Policy applies to repositories with specific build pipeline
applicability:
  build_pipelines: ["npm", "yarn"]
```

### Build Pipeline Awareness

**Build Pipeline Detection**:
- **Automatic**: Detects build tools (npm, maven, gradle, etc.)
- **Configuration**: Reads build config files (package.json, pom.xml, etc.)
- **Custom**: Human provides build pipeline context

**Pipeline-Specific Policies**:
- **npm/yarn**: Node.js-specific policies (e.g., lockfile management)
- **maven/gradle**: Java-specific policies (e.g., dependency management)
- **docker**: Container-specific policies (e.g., image tagging)
- **ci/cd**: Pipeline-specific policies (e.g., test coverage requirements)

**Example**:
```yaml
# Policy for npm projects
id: POL-CODE-008-NPM
policy_state: legacy_compatible
applicability:
  build_pipelines: ["npm", "yarn"]
body: |
  **Legacy npm Policy**: During migration, both package-lock.json and yarn.lock are accepted.
  After phase 3, only package-lock.json will be accepted.
```

### Cross-Repository Visibility

**Command**: `zqk project-group status PG-001`

Provides unified view across repositories:
- **Migration Progress**: Overall migration status
- **Compliance Scores**: Aggregated compliance metrics
- **Policy Status**: Which policies are active in which repos
- **Risk Summary**: Combined risk assessment
- **Dependency Graph**: Visualize cross-repo dependencies

**Dashboard View**:
```
Project Group: E-Commerce Platform
├── Frontend (npm)
│   ├── Migration Phase: 4
│   ├── Compliance: 75%
│   ├── Active Policies: 12
│   └── Risks: 2 medium
├── Backend (maven)
│   ├── Migration Phase: 3
│   ├── Compliance: 60%
│   ├── Active Policies: 8
│   └── Risks: 1 high
└── Mobile (gradle)
    ├── Migration Phase: 2
    ├── Compliance: 40%
    ├── Active Policies: 5
    └── Risks: 0
```

## Human Context Integration

### Context Collection Points

**1. Assessment Phase** (`zqk system assess --interactive`)

Interactive prompts for human context:
- **Business Context**: Why the codebase exists, key stakeholders
- **Technical Constraints**: Legacy systems, dependencies, limitations
- **Team Context**: Team size, expertise, availability
- **Risk Tolerance**: How conservative/aggressive migration should be
- **Timeline Constraints**: Deadlines, release schedules

**2. Policy Generation** (`zqk policy generate --with-context`)

Human provides context that analysis can't detect:
- **Historical Decisions**: Why certain patterns exist
- **Business Rules**: Domain-specific constraints
- **Team Preferences**: Preferred workflows, tools
- **Compliance Requirements**: External regulations, standards

**3. Migration Planning** (`zqk system roadmap --interactive`)

Human adjusts migration plan:
- **Phase Timing**: Adjust phase durations based on team capacity
- **Risk Mitigation**: Add additional safety measures
- **Stakeholder Communication**: Customize communication materials
- **Rollback Plans**: Define rollback criteria and procedures

### Human Context Object

**Object Type**: `migration_context`

Stores human-provided context:
```yaml
id: MIG-CTX-001
kind: migration_context
migration_ref: MIG-001
context_type: business_rules
content: |
  The codebase uses legacy authentication because it integrates with
  a third-party system that only supports OAuth 1.0. This cannot be
  changed until Q2 2026 when the third-party system is upgraded.
provided_by: account:human
provided_at: "2025-01-15T10:00:00Z"
affects_policies: [POL-AUTH-001-LEGACY]
affects_phases: [1, 2, 3]
```

### Context-Aware Policy Generation

Policies generated with human context:
- **Respect Constraints**: Policies accommodate business/technical constraints
- **Timeline Awareness**: Policies account for timeline limitations
- **Risk-Adjusted**: Policies adjusted based on risk tolerance
- **Team-Aware**: Policies consider team expertise and preferences

**Example**:
```bash
# Generate policies with human context
zqk policy generate --migration MIG-001 --context MIG-CTX-001

# Review generated policies
zqk policy list --migration MIG-001 --state legacy_compatible

# Adjust policy based on additional context
zqk policy update POL-AUTH-001-LEGACY --context "Third-party system upgrade delayed to Q3"
```

### Decision Records

**Human-in-the-Loom Protocol** for policy decisions:

When high-stakes policy changes are proposed:
1. **Decision Record Generated**: System creates decision record
2. **Human Review**: Human reviews rationale, risks, impact
3. **Cryptographic Approval**: Human signs with cryptographic key
4. **Immutable Audit Trail**: Decision recorded in knowledge graph
5. **Policy Activation**: Policy activated only after approval

**Decision Record Format**:
```yaml
id: DEC-POL-001
kind: decision_record
policy_ref: POL-WORKFLOW-001-LEGACY
decision: activate
rationale: |
  Legacy branch naming policy can be retired because all repositories
  have migrated to zqk naming conventions. Assessment shows 95%
  compliance across all repos.
risks:
  - risk: "Some legacy branches may still exist"
    mitigation: "Grace period of 2 weeks for branch cleanup"
impact:
  - repository: frontend
    impact: "No impact - already using zqk conventions"
  - repository: backend
    impact: "Minor - 3 legacy branches need cleanup"
approved_by: account:human
approved_at: "2025-01-20T14:00:00Z"
signature: "cryptographic_signature_hash"
```

## Policy Orchestration Workflow

### Phase-Based Policy Management

**Phase 1-3 (Legacy Compatible)**:
```bash
# Assessment generates legacy_compatible policies
zqk system assess --generate-policies

# Policies automatically activated
zqk policy orchestrate --phase 1 --auto-activate

# Human reviews and approves
zqk policy review --phase 1
```

**Phase 4 (Transitional)**:
```bash
# System proposes policy transitions
zqk policy orchestrate --phase 4 --preview

# Human reviews transitions
zqk policy review --phase 4

# Human approves transitions
zqk policy orchestrate --phase 4 --execute --approve
```

**Phase 5 (zqk Standard)**:
```bash
# Legacy policies automatically retired
zqk policy orchestrate --phase 5 --retire-legacy

# Standard policies activated
zqk policy orchestrate --phase 5 --activate-standard
```

### Compliance-Based Activation

Policies can activate based on compliance thresholds:

```yaml
# Policy activates when compliance threshold reached
activation:
  type: compliance_based
  threshold: 80
  metric: overall_compliance_score
  
# Policy retires when compliance threshold exceeded
retirement:
  type: compliance_based
  threshold: 95
  metric: overall_compliance_score
```

### Risk-Based Activation

High-risk policies require additional safeguards:

```yaml
# High-risk policy requires human approval
activation:
  type: risk_based
  risk_level: high
  requires_approval: true
  approval_mechanism: decision_record
```

## Multi-Repository Coordination Example

### Scenario: E-Commerce Platform

**Project Group**: 3 repositories (frontend, backend, mobile)

**Assessment**:
```bash
# Assess entire project group
zqk system assess --project-group PG-001

# Results show:
# - Frontend: npm, well-structured, low risk
# - Backend: maven, legacy patterns, medium risk
# - Mobile: gradle, scattered docs, low risk
```

**Coordinated Migration**:
```bash
# Generate unified migration plan
zqk system roadmap --project-group PG-001

# Phase 1: All repos assessed simultaneously
# Phase 2: Frontend leads (lowest risk)
# Phase 3: Backend and Mobile follow
# Phase 4: All repos transition together
# Phase 5: All repos achieve compliance
```

**Policy Coordination**:
```yaml
# Project-group-wide policy
id: POL-WORKFLOW-001-PG
applicability:
  project_groups: [PG-001]
policy_state: legacy_compatible

# Repository-specific policy
id: POL-CODE-008-NPM
applicability:
  repositories: [frontend]
  build_pipelines: [npm]
policy_state: legacy_compatible
```

**Cross-Repository Visibility**:
```bash
# View unified status
zqk project-group status PG-001

# View dependency graph
zqk project-group dependencies PG-001 --visualize

# View compliance across repos
zqk project-group compliance PG-001 --aggregate
```

## Future Enhancements

1. **Automated Policy Generation**: AI-assisted policy creation based on assessment
2. **Predictive Risk Analysis**: ML models to predict migration risks
3. **Interactive Migration Wizard**: Step-by-step guided migration
4. **Integration Testing**: Automated tests for migration safety
5. **Community Templates**: Pre-built migration templates for common scenarios
6. **Multi-Repository Dependency Analysis**: Automatic detection of cross-repo dependencies
7. **Build Pipeline Templates**: Pre-configured policies for common build systems
8. **Human Context Learning**: System learns from human context to improve future assessments

## Related Documentation

- [Migration Strategy: File-Based to Graph Backend](./migration-strategy-file-to-graph-v1.0.md)
- [Project Import and Onboarding System](../backlog/BLI-125.yaml)
- [Git Workflow Enforcement](./git-workflow-enforcement-v1.0.md)
- [System Sync Command](./system-sync-command-v1.0.md)
- [Policy Lifecycle Management](./POLICY_LIFECYCLE.md)
- [Multi-Repository Project Support](../milestones/MIL-023.yaml)
- [Human-in-the-Loom Protocol](./ai-agent-communication-channels-v1.0.md#human-in-the-loom-protocol)

---

*This strategy ensures safe, predictable, and observable migration of legacy codebases into zqk compliance while maintaining stakeholder confidence and minimizing disruption. The system supports single repositories, multi-repository projects, and complex enterprise scenarios with library-specific build pipelines.*

