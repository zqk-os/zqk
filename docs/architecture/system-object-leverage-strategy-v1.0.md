# System Object Leverage Strategy v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Design  
**Related**: Strategic Plan, Roadmap Objects, Knowledge Kernel

## Problem Statement

Currently, we have a mix of markdown documentation and system objects, but we're not fully leveraging the distributed knowledge kernel's capabilities to provide:

1. **Self-Documenting System**: The system should be able to answer questions about itself
2. **Discoverable Relationships**: New agents/humans should be able to explore relationships between objects
3. **Consistent Views**: Multiple views of the same information should be generated from system objects
4. **Adaptive Documentation**: Documentation should be generated from system objects, not maintained separately
5. **Real-Time Awareness**: The system should be aware of changes and their impacts

## Solution: System-First Documentation and Discovery

### Core Principles

1. **System Objects as Source of Truth**: All planning, roadmaps, and strategic information should be system objects
2. **Markdown as Generated Views**: Markdown documents should be generated from system objects, not maintained separately
3. **Relationship Traversal**: Use the knowledge kernel to traverse relationships and answer questions
4. **Query-Based Discovery**: Enable queries like "What workstreams support GOAL-6372?" or "What agents are needed for WS-011?"
5. **Self-Awareness**: System objects should reference each other, creating a web of discoverable information

### Architecture

#### 1. Roadmap Objects as Primary Artifacts

**Current State**: Markdown roadmaps exist separately from system objects  
**Target State**: Roadmap objects (`roadmap`) are the source of truth, markdown is generated

**Roadmap Object Structure**:
```yaml
id: ROADMAP-PHASE2-001
kind: roadmap
title: Phase 2 Implementation Roadmap
workstream_refs: [WS-011, WS-012, ...]
goal_refs: [GOAL-6372, GOAL-6373, ...]
milestone_refs: [MIL-041, MIL-042, ...]
strategic_plan_ref: STRAT-PLAN-001
agent_onboarding_refs: [AGENT-PREP-002, AGENT-PREP-003]
timeline_start: "2026-04-01"
timeline_end: "2026-06-30"
```

**Benefits**:
- Queryable: `zqk object list roadmap --filter 'strategic_plan_ref=STRAT-PLAN-001'`
- Traversable: Can follow `workstream_refs` to get all related backlog items
- Consistent: Single source of truth for roadmap information
- Adaptive: Updates to workstreams automatically reflected in roadmap queries

#### 2. Strategic Plan as Central Hub

**Current State**: Strategic plan exists but relationships are not fully leveraged  
**Target State**: Strategic plan is the central query point for all strategic information

**Query Patterns**:
```bash
# Get all workstreams for a strategic plan
zqk object list workstream --filter 'goal_refs=GOAL-6372' --filter 'goal_refs=GOAL-6373'

# Get all goals for a strategic plan phase
zqk object get STRAT-PLAN-001 --format yaml | grep -A10 "phase: Foundation"

# Get all agent onboarding for a strategic plan
zqk object list agent_onboarding_preparation --filter 'target_workstream_ref=WS-008'
```

**Strategic Plan Relationships**:
- `workstream_refs`: All workstreams in the plan
- `goal_refs`: All goals in the plan
- `phases[].workstreams`: Workstreams by phase
- `phases[].goals`: Goals by phase
- `agent_onboarding_timeline[]`: Agent onboarding schedule

#### 3. Workstream as Execution Unit

**Current State**: Workstreams exist but relationships are not fully explored  
**Target State**: Workstreams are queryable execution units with full context

**Workstream Query Patterns**:
```bash
# Get all backlog items for a workstream
zqk object list backlog_item --filter 'workstream_refs=WS-011'

# Get all goals supported by a workstream
zqk object get WS-011 --format yaml | grep goal_refs

# Get milestones for a workstream
zqk object get WS-011 --format yaml | grep milestone_refs

# Get agent onboarding for a workstream
zqk object list agent_onboarding_preparation --filter 'target_workstream_ref=WS-011'

# Get workstream transitions involving this workstream
zqk object list workstream_transition --filter 'from_workstream_ref=WS-011'
zqk object list workstream_transition --filter 'to_workstream_ref=WS-011'
```

**Workstream Relationships**:
- `goal_refs`: Goals this workstream supports
- `milestone_refs`: Milestones this workstream contributes to
- `backlog_item.workstream_refs`: All backlog items (reverse lookup)
- `agent_onboarding_preparation.target_workstream_ref`: Agent preparations (reverse lookup)
- `workstream_transition.from_workstream_ref/to_workstream_ref`: Transitions (reverse lookup)

#### 4. Goal as Alignment Anchor

**Current State**: Goals exist but alignment is not easily discoverable  
**Target State**: Goals are queryable alignment anchors

**Goal Query Patterns**:
```bash
# Get all workstreams supporting a goal
zqk object list workstream --filter 'goal_refs=GOAL-6372'

# Get all milestones for a goal
zqk object list milestone --filter 'goal_refs=GOAL-6372'

# Get all backlog items for a goal (via workstreams)
zqk object list backlog_item --filter 'goal_refs=GOAL-6372'

# Get strategic plan for a goal
zqk object list strategic_plan --filter 'goal_refs=GOAL-6372'
```

#### 5. Agent Onboarding as Resource Planning

**Current State**: Agent preparations exist but integration is not clear  
**Target State**: Agent onboarding is fully integrated with workstreams and strategic plans

**Agent Onboarding Query Patterns**:
```bash
# Get all agent preparations for a workstream
zqk object list agent_onboarding_preparation --filter 'target_workstream_ref=WS-009'

# Get readiness criteria for an agent
zqk object get AGENT-PREP-002 --format yaml | grep -A20 readiness_criteria

# Get all agents for a strategic plan
zqk object get STRAT-PLAN-001 --format yaml | grep -A30 agent_onboarding_timeline

# Get workstreams requiring an agent
zqk object list agent_onboarding_preparation --filter 'agent_type=test_agent' | grep target_workstream_ref
```

#### 6. Documentation as Generated Views

**Current State**: Markdown documents are maintained separately  
**Target State**: Markdown documents are generated from system objects and linked via `doc_entry`

**Documentation Strategy**:
1. **System Objects First**: Create roadmap, strategic plan, workstream objects
2. **Generate Markdown**: Use CLI commands to generate markdown views
3. **Link via doc_entry**: `doc_entry` objects link markdown to system objects via `related_objects`
4. **Keep in Sync**: Markdown is regenerated when system objects change

**Example**:
```yaml
id: DOC-152
kind: doc_entry
title: Phase 2 Implementation Roadmap
path: docs/process/planning/PHASE_2_ROADMAP_v1.0.md
related_objects:
  - roadmap:ROADMAP-PHASE2-001
  - strategic_plan:STRAT-PLAN-001
  - workstream:WS-011
  - workstream:WS-012
```

**Query Pattern**:
```bash
# Get all documentation for a strategic plan
zqk object list doc_entry --filter 'related_objects=strategic_plan:STRAT-PLAN-001'

# Get system objects referenced by documentation
zqk object get DOC-152 --format yaml | grep related_objects
```

### Implementation Strategy

#### Phase 1: Create System Objects for All Planning Artifacts

1. **Roadmap Objects**: Create `roadmap` objects for Phase 1 and Phase 2
2. **Link Relationships**: Ensure all `workstream_refs`, `goal_refs`, `milestone_refs` are populated
3. **Agent Onboarding Links**: Link agent preparations to workstreams and strategic plans
4. **Documentation Links**: Create `doc_entry` objects linking markdown to system objects

#### Phase 2: Generate Views from System Objects

1. **Roadmap Generation**: `zqk roadmap generate ROADMAP-PHASE2-001 --format markdown`
2. **Strategic Plan View**: `zqk strategic plan view STRAT-PLAN-001 --format markdown`
3. **Workstream Summary**: `zqk workstream summary WS-011 --format markdown`
4. **Agent Onboarding Report**: `zqk agent onboarding report AGENT-PREP-002 --format markdown`

#### Phase 3: Enable Discovery Queries

1. **Relationship Traversal**: `zqk object traverse STRAT-PLAN-001 --depth 3`
2. **Impact Analysis**: `zqk object impact GOAL-6372 --show workstreams,backlog_items,agents`
3. **Alignment Check**: `zqk align (PRUNED)ment check --strategic_plan STRAT-PLAN-001`
4. **Resource Planning**: `zqk resource plan --workstream WS-011 --show agents,humans,timeline`

#### Phase 4: Self-Documenting System

1. **System Introspection**: `zqk system introspect --object-type workstream`
2. **Relationship Map**: `zqk system map --from STRAT-PLAN-001 --to workstream,goal,milestone`
3. **Onboarding Guide**: `zqk system onboarding-guide --generate`
4. **Status Dashboard**: `zqk system dashboard --strategic-plan STRAT-PLAN-001`

### Query Examples

#### "What workstreams support GOAL-6372?"
```bash
zqk object list workstream --filter 'goal_refs=GOAL-6372'
# Returns: WS-010, WS-011, WS-012
```

#### "What agents are needed for WS-011?"
```bash
zqk object list agent_onboarding_preparation --filter 'target_workstream_ref=WS-011'
# Returns: (none currently, but can be added)
```

#### "What is the timeline for Phase 2?"
```bash
zqk object get ROADMAP-PHASE2-001 --format yaml | grep timeline
# Returns: timeline_start: "2026-04-01", timeline_end: "2026-06-30"
```

#### "What backlog items are in WS-011?"
```bash
zqk object list backlog_item --filter 'workstream_refs=WS-011'
# Returns: BLI-730, BLI-731, BLI-732, BLI-733, BLI-734
```

#### "What milestones must complete before WS-011 can start?"
```bash
zqk object list workstream_transition --filter 'to_workstream_ref=WS-011'
# Returns: WST-003 (triggered by MIL-040)
```

#### "What is the current status of all Phase 2 workstreams?"
```bash
zqk object list workstream --filter 'goal_refs=GOAL-6372' --filter 'goal_refs=GOAL-6373' --filter 'goal_refs=GOAL-6374' --format table
# Returns: All Phase 2 workstreams with their status
```

### Benefits

1. **Single Source of Truth**: System objects are the authoritative source
2. **Discoverable**: Relationships enable exploration and discovery
3. **Consistent**: Multiple views generated from same source
4. **Adaptive**: Changes to objects automatically reflected in queries
5. **Self-Documenting**: System can answer questions about itself
6. **Onboarding**: New agents/humans can explore and understand the system
7. **Awareness**: System is aware of changes and their impacts

### Next Steps

1. Create roadmap objects for all phases
2. Link all documentation to system objects via `doc_entry`
3. Implement view generation commands
4. Create discovery and introspection commands
5. Generate markdown from system objects
6. Enable relationship traversal queries

