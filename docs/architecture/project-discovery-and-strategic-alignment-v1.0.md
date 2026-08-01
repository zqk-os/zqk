> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# Project Discovery and Strategic Alignment v1.0

**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Design  
**Related**: Legacy Codebase Migration v1.0, BLI-125

## Overview

This document defines a comprehensive strategy for discovering, configuring, and aligning brand new projects with zqk. It addresses projects at the very beginning (ideas, prototypes, concepts) through to enterprise-scale deployments, ensuring strategic alignment and stakeholder satisfaction.

## Problem Statement

When initializing a brand new project with zqk:

1. **No Existing Structure**: No policies, conventions, or established patterns
2. **Unclear Goals**: Short-term and long-term objectives not yet defined
3. **Missing Context**: Important dates, market conditions, stakeholder expectations unknown
4. **Diverse Customer Needs**: Individual developers vs. large enterprises have vastly different requirements
5. **Strategic Alignment**: Need to ensure all work aligns with organizational objectives
6. **Stakeholder Expectations**: Multiple stakeholders with different priorities and concerns
7. **Policy Limitations**: Policies alone may not provide sufficient context, especially during transitions

## Solution: Discovery-Driven Project Initialization

### Core Principles

1. **Discover Before Prescribe**: Understand project context before applying defaults
2. **Strategic Context First**: Goals, vision, and strategic context inform all decisions
3. **Stakeholder-Centric**: Capture and align with stakeholder expectations
4. **Configurable by Scale**: Different configurations for individuals vs. enterprises
5. **Context Beyond Policies**: Strategic context, market conditions, important dates supplement policies
6. **Continuous Alignment**: Ongoing validation that work aligns with strategic objectives

## Architecture

### 1. Project Discovery Wizard

**Command**: `zqk system init --discover`

Interactive wizard that discovers project context:

**Discovery Phases**:

#### Phase 1: Project Foundation
- **Project Type**: Idea, prototype, concept, new product, enterprise initiative
- **Project Stage**: Ideation, validation, development, production, scaling
- **Customer Type**: Individual, small team, startup, mid-size, enterprise
- **Domain**: Technology, business domain, industry

#### Phase 2: Strategic Context
- **Vision**: What is the desired future state?
- **Mission**: Why does this project exist?
- **Problem Statement**: What problem are we solving?
- **Success Criteria**: How do we know we've succeeded?

#### Phase 3: Goals & Objectives
- **Short-Term Goals** (0-3 months): Immediate objectives
- **Medium-Term Goals** (3-12 months): Near-term objectives
- **Long-Term Goals** (1-5 years): Strategic objectives
- **Goal Metrics**: How will we measure success?

#### Phase 4: Stakeholder Discovery
- **Primary Stakeholders**: Who are the key decision-makers?
- **Stakeholder Expectations**: What do they expect from this project?
- **Stakeholder Priorities**: What matters most to each stakeholder?
- **Communication Preferences**: How do stakeholders want to be informed?

#### Phase 5: Context & Constraints
- **Important Dates**: Deadlines, milestones, market windows
- **Market Conditions**: Competitive landscape, market timing
- **Technical Constraints**: Technology requirements, platform constraints
- **Business Constraints**: Budget, resources, regulatory requirements
- **Risk Factors**: Known risks, dependencies, blockers

#### Phase 6: Configuration Selection
- **Complexity Level**: Simple, moderate, complex, enterprise
- **Policy Strictness**: Flexible, balanced, strict
- **Automation Level**: Manual, semi-automated, fully automated
- **Template Selection**: Choose from project templates

### 2. Strategic Context Objects

**New Object Types**:

#### Strategic Context (`strategic_context`)

Captures strategic information beyond policies:

```yaml
id: SC-001
kind: strategic_context
title: "Q1 2026 Market Window"
context_type: market_condition
content: |
  Market analysis indicates Q1 2026 is optimal launch window due to:
  - Competitor product lifecycle gaps
  - Regulatory changes taking effect
  - Seasonal demand patterns
  - Technology maturity milestones
important_dates:
  - date: "2026-01-15"
    event: "Regulatory deadline"
    impact: "high"
  - date: "2026-03-31"
    event: "Q1 market window closes"
    impact: "critical"
stakeholder_refs:
  - account:executive-team
  - account:product-owner
affects_goals: [GOAL-001, GOAL-002]
affects_policies: [POL-WORKFLOW-001]
```

#### Stakeholder Profile (`stakeholder_profile`)

Captures stakeholder expectations and priorities:

```yaml
id: STK-001
kind: stakeholder_profile
title: "Executive Team"
stakeholder_type: decision_maker
expectations:
  - "Quarterly progress reports"
  - "Budget adherence within 10%"
  - "Risk mitigation for critical path items"
priorities:
  - priority: 1
    concern: "Time to market"
    weight: 0.4
  - priority: 2
    concern: "Quality and reliability"
    weight: 0.3
  - priority: 3
    concern: "Cost efficiency"
    weight: 0.3
communication_preferences:
  frequency: "weekly"
  format: ["executive_summary", "dashboard"]
  channels: ["email", "slack"]
alignment_metrics:
  - metric: "Goal achievement rate"
    target: ">80%"
  - metric: "On-time delivery"
    target: ">90%"
```

#### Important Date (`important_date`)

Tracks critical dates and their impact:

```yaml
id: DATE-001
kind: important_date
title: "Product Launch Deadline"
date: "2026-03-31"
date_type: deadline
importance: critical
impact_scope: project_wide
related_goals: [GOAL-001, GOAL-002]
related_milestones: [MIL-010]
dependencies:
  - "MIL-010 must complete by 2026-03-15"
  - "Regulatory approval required by 2026-03-01"
stakeholder_notifications:
  - stakeholder: STK-001
    notification_days_before: [30, 14, 7, 1]
```

### 3. Goal Discovery & Alignment

**Command**: `zqk system discover-goals` (PRUNED)

Interactive goal discovery:

**Goal Discovery Process**:
1. **Vision Extraction**: Extract goals from vision statement
2. **Problem Analysis**: Derive goals from problem statements
3. **Stakeholder Input**: Capture goals from stakeholder interviews
4. **Market Analysis**: Identify goals from market conditions
5. **Timeline Analysis**: Extract goals from important dates

**Goal Hierarchy**:
```
Vision
  └─> Long-Term Goals (1-5 years)
      └─> Medium-Term Goals (3-12 months)
          └─> Short-Term Goals (0-3 months)
              └─> Milestones
                  └─> Backlog Items
```

**Alignment Validation**:
- **Goal-Goal Alignment**: Ensure goals don't conflict
- **Goal-Policy Alignment**: Policies support goal achievement
- **Goal-Stakeholder Alignment**: Goals address stakeholder expectations
- **Goal-Context Alignment**: Goals account for market conditions and dates

### 4. Customer Type Configurations

**Configuration Profiles**:

#### Individual Developer Profile

**Characteristics**:
- Single person project
- Minimal overhead
- Fast iteration
- Flexible policies

**Configuration**:
```yaml
customer_type: individual
complexity: simple
policy_strictness: flexible
automation_level: semi_automated
features:
  - minimal_policies: true
  - quick_setup: true
  - lightweight_reporting: true
  - self_service: true
```

#### Small Team Profile

**Characteristics**:
- 2-10 people
- Some structure needed
- Collaboration important
- Moderate policies

**Configuration**:
```yaml
customer_type: small_team
complexity: moderate
policy_strictness: balanced
automation_level: semi_automated
features:
  - team_collaboration: true
  - shared_context: true
  - moderate_policies: true
  - basic_reporting: true
```

#### Enterprise Profile

**Characteristics**:
- Large organization
- Multiple teams/projects
- Strict compliance
- Comprehensive reporting
- Strategic alignment critical

**Configuration**:
```yaml
customer_type: enterprise
complexity: enterprise
policy_strictness: strict
automation_level: fully_automated
features:
  - multi_project_management: true
  - portfolio_visibility: true
  - strict_policies: true
  - comprehensive_reporting: true
  - strategic_alignment: true
  - stakeholder_management: true
  - audit_compliance: true
```

### 5. Strategic Alignment System

**Command**: `zqk system align` (PRUNED)

Validates strategic alignment:

**Alignment Checks**:
1. **Goal Alignment**: All work items trace to goals
2. **Stakeholder Alignment**: Work addresses stakeholder expectations
3. **Policy Alignment**: Policies support goal achievement
4. **Context Alignment**: Work accounts for market conditions and dates
5. **Timeline Alignment**: Work fits within important date constraints

**Alignment Reporting**:
```bash
# Check alignment
zqk system align (PRUNED)

# Show alignment gaps
zqk system align --gaps (PRUNED)

# Show alignment for specific goal
zqk system align --goal GOAL-001 (PRUNED)

# Show alignment for stakeholder
zqk system align --stakeholder STK-001 (PRUNED)
```

**Alignment Metrics**:
- **Goal Coverage**: % of goals with active work
- **Stakeholder Satisfaction**: Alignment with stakeholder expectations
- **Policy Compliance**: % of policies being followed
- **Context Adherence**: % of important dates being met
- **Strategic Fit**: Overall alignment score

### 6. Context-Aware Policy Generation

**Command**: `zqk policy generate --from-context`

Generates policies informed by strategic context:

**Context Sources**:
- **Goals**: Policies support goal achievement
- **Stakeholder Expectations**: Policies address stakeholder concerns
- **Market Conditions**: Policies account for market timing
- **Important Dates**: Policies respect deadline constraints
- **Risk Factors**: Policies mitigate identified risks

**Example Context-Aware Policy**:
```yaml
id: POL-WORKFLOW-001
kind: policy
title: "Git Branch Workflow"
policy_state: zqk_standard

# Context-informed policy
strategic_context:
  goal_refs: [GOAL-001]  # Supports rapid iteration goal
  stakeholder_refs: [STK-001]  # Addresses time-to-market concern
  important_date_refs: [DATE-001]  # Respects launch deadline
  market_condition_refs: [SC-001]  # Accounts for market window

body: |
  **Git Branch Workflow**
  
  This policy supports GOAL-001 (Rapid Iteration) and addresses STK-001's
  time-to-market priority. The workflow is optimized for fast delivery
  while maintaining quality, respecting the Q1 2026 market window (SC-001)
  and launch deadline (DATE-001).
  
  [Policy details...]
```

### 7. Stakeholder Expectation Management

**Command**: `zqk stakeholder manage`

Manages stakeholder expectations and alignment:

**Features**:
- **Expectation Capture**: Record stakeholder expectations
- **Priority Weighting**: Weight stakeholder priorities
- **Alignment Tracking**: Track how work aligns with expectations
- **Communication**: Generate stakeholder communications
- **Satisfaction Monitoring**: Monitor stakeholder satisfaction

**Stakeholder Dashboard**:
```bash
zqk stakeholder dashboard STK-001
```

Shows:
- **Expectations**: What stakeholder expects
- **Current Alignment**: How current work aligns
- **Gap Analysis**: What's missing or misaligned
- **Communication History**: Past communications
- **Satisfaction Metrics**: Current satisfaction level

### 8. Strategic Context Integration

**Context Layers**:

1. **Vision & Mission Layer**: Highest-level strategic direction
2. **Goal Layer**: Measurable objectives
3. **Stakeholder Layer**: Expectations and priorities
4. **Context Layer**: Market conditions, important dates
5. **Policy Layer**: Rules and standards
6. **Work Layer**: Backlog items, priority plans

**Context Propagation**:
- **Top-Down**: Vision → Goals → Milestones → Backlog Items
- **Bottom-Up**: Work items validate against goals and vision
- **Lateral**: Policies align with goals, stakeholders, context

**Context Validation**:
```bash
# Validate context consistency
zqk system validate-context

# Show context relationships
zqk system context-graph --visualize

# Check context gaps
zqk system context-gaps
```

## Implementation

### Phase 1: Discovery Wizard

```bash
# Interactive discovery
zqk system init --discover

# Non-interactive (from file)
zqk system init --discover --file discovery.yaml

# Quick start (minimal questions)
zqk system init --quick-start
```

### Phase 2: Strategic Context Setup

```bash
# Create strategic context
zqk object create strategic_context --file strategic-context.yaml

# Create stakeholder profiles
zqk object create stakeholder_profile --file stakeholder.yaml

# Create important dates
zqk object create important_date --file important-dates.yaml
```

### Phase 3: Goal Discovery

```bash
# Discover goals from vision/context
zqk system discover-goals (PRUNED)

# Validate goal alignment
zqk system align --goals (PRUNED)

# Generate policies from context
zqk policy generate --from-context
```

### Phase 4: Ongoing Alignment

```bash
# Check alignment
zqk system align (PRUNED)

# Update stakeholder expectations
zqk stakeholder update STK-001 --expectation "..."

# Validate context
zqk system validate-context
```

## Customer Type Examples

### Individual Developer: Prototype Project

**Discovery**:
```yaml
project_type: prototype
customer_type: individual
stage: ideation
domain: "AI tooling"

goals:
  short_term:
    - "Validate core concept (2 weeks)"
    - "Build MVP (4 weeks)"
  long_term:
    - "Launch product (6 months)"
    - "1000 users (12 months)"

stakeholders:
  - name: "Self"
    type: "developer"
    expectations: ["Fast iteration", "Minimal overhead"]
```

**Configuration**:
- **Policies**: Minimal, flexible
- **Automation**: Basic
- **Reporting**: Lightweight
- **Structure**: Simple

### Enterprise: Multi-Product Initiative

**Discovery**:
```yaml
project_type: enterprise_initiative
customer_type: enterprise
stage: production
domain: "Financial services"

goals:
  short_term:
    - "Compliance with new regulations (Q1)"
    - "Security audit completion (Q2)"
  medium_term:
    - "Portfolio modernization (12 months)"
  long_term:
    - "Market leadership (5 years)"

stakeholders:
  - name: "Executive Team"
    expectations: ["Regulatory compliance", "Risk mitigation"]
  - name: "Product Owners"
    expectations: ["Feature delivery", "Quality"]
  - name: "Security Team"
    expectations: ["Security standards", "Audit readiness"]
```

**Configuration**:
- **Policies**: Comprehensive, strict
- **Automation**: Full
- **Reporting**: Enterprise dashboards
- **Structure**: Multi-project, portfolio view
- **Alignment**: Strategic alignment required

## Strategic Alignment Validation

### Alignment Checks

**1. Goal-Work Alignment**:
```bash
# Check if all backlog items trace to goals
zqk system align --check goal-work (PRUNED)

# Show items without goal alignment
zqk system align --gaps goal-work (PRUNED)
```

**2. Stakeholder-Work Alignment**:
```bash
# Check if work addresses stakeholder expectations
zqk system align --check stakeholder-work (PRUNED)

# Show stakeholder satisfaction
zqk stakeholder satisfaction STK-001
```

**3. Context-Work Alignment**:
```bash
# Check if work respects important dates
zqk system align --check context-dates (PRUNED)

# Check if work accounts for market conditions
zqk system align --check context-market (PRUNED)
```

**4. Policy-Goal Alignment**:
```bash
# Check if policies support goals
zqk system align --check policy-goal (PRUNED)

# Show policies that don't support goals
zqk system align --gaps policy-goal (PRUNED)
```

### Alignment Dashboard

**Command**: `zqk system align --dashboard` (PRUNED)

Shows comprehensive alignment view:
- **Overall Alignment Score**: 0-100%
- **Goal Coverage**: % of goals with active work
- **Stakeholder Satisfaction**: Per-stakeholder scores
- **Context Adherence**: % of important dates met
- **Policy Compliance**: % of policies followed
- **Strategic Fit**: Alignment with vision/mission

## Context Beyond Policies

### Strategic Context Objects

**When Policies Aren't Enough**:

1. **Market Conditions**: External factors affecting decisions
2. **Important Dates**: Deadlines, milestones, market windows
3. **Stakeholder Expectations**: Human expectations not codifiable in policies
4. **Business Constraints**: Budget, resources, regulatory requirements
5. **Risk Factors**: Known risks requiring special attention
6. **Strategic Priorities**: Changing priorities based on context

**Integration with Policies**:
- Policies reference strategic context
- Context informs policy interpretation
- Context provides "why" behind policies
- Context enables policy exceptions when justified

**Example**:
```yaml
# Policy with context
id: POL-WORKFLOW-001
strategic_context:
  important_date_refs: [DATE-001]  # Launch deadline
  market_condition_refs: [SC-001]  # Market window

body: |
  **Branch Workflow Policy**
  
  Standard workflow: feature branches per priority plan.
  
  **Exception**: When DATE-001 (launch deadline) is within 7 days,
  hotfix branches may be created directly from main with expedited
  review process. This exception is justified by SC-001 (market
  window closing) and stakeholder priority STK-001 (time-to-market).
```

## Multi-Stakeholder Alignment

### Stakeholder Priority Matrix

**Command**: `zqk stakeholder priorities --matrix`

Shows stakeholder priority alignment:
```
Stakeholder    | Priority 1    | Priority 2    | Priority 3    | Weight
---------------|---------------|---------------|---------------|--------
Executive      | Time-to-Market| Quality       | Cost          | 0.4
Product Owner  | Features      | User Experience| Performance | 0.3
Security       | Compliance    | Security      | Audit         | 0.2
Engineering    | Quality       | Performance   | Maintainability| 0.1
```

### Consensus Building

**Command**: `zqk stakeholder consensus`

Helps build consensus:
- **Priority Conflicts**: Identifies conflicting priorities
- **Compromise Solutions**: Suggests balanced approaches
- **Stakeholder Communication**: Generates communication materials
- **Alignment Proposals**: Proposes alignment strategies

## Enterprise Configuration

### Portfolio Management

**Command**: `zqk portfolio`** (for enterprise customers)

Manages multiple projects:
- **Portfolio View**: All projects in one view
- **Cross-Project Alignment**: Ensure projects align with enterprise goals
- **Resource Allocation**: Track resources across projects
- **Strategic Reporting**: Enterprise-level reporting

**Portfolio Configuration**:
```yaml
portfolio:
  name: "Enterprise Product Portfolio"
  projects:
    - project: "E-Commerce Platform"
      strategic_priority: P0
      resource_allocation: 40%
    - project: "Mobile App"
      strategic_priority: P1
      resource_allocation: 30%
    - project: "Analytics Platform"
      strategic_priority: P2
      resource_allocation: 30%
  
  enterprise_goals:
    - GOAL-ENTERPRISE-001: "Market leadership"
    - GOAL-ENTERPRISE-002: "Regulatory compliance"
  
  alignment_requirements:
    - "All projects must align with enterprise goals"
    - "Cross-project dependencies must be tracked"
    - "Portfolio-level reporting required"
```

## Future Enhancements

1. **AI-Assisted Discovery**: AI helps discover goals and context from conversations
2. **Predictive Alignment**: Predict alignment issues before they occur
3. **Stakeholder Sentiment Analysis**: Analyze stakeholder satisfaction trends
4. **Market Intelligence Integration**: Automatic market condition updates
5. **Strategic Scenario Planning**: Model different strategic scenarios
6. **Automated Alignment Reports**: Periodic alignment reports to stakeholders

## Related Documentation

- [Legacy Codebase Migration Strategy](./legacy-codebase-migration-v1.0.md)
- [Project Initialization Guide](./PROJECT_INITIALIZATION.md)
- [Policy Lifecycle Management](./POLICY_LIFECYCLE.md)
- [Multi-Repository Project Support](../milestones/MIL-023.yaml)

---

*This strategy ensures brand new projects are properly discovered, configured, and aligned with strategic objectives, stakeholder expectations, and market conditions, regardless of customer type or project scale.*

