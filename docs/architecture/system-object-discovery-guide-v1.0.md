# System Object Discovery Guide v1.0

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Active  
**Related**: System Object Leverage Strategy v1.0, Strategic Plan, Roadmaps

## Purpose

This guide demonstrates how to use the zqk knowledge kernel to discover, explore, and understand the system through queries and relationship traversal. It shows how system objects are the source of truth and how to leverage them for onboarding, planning, and execution.

## Core Principle

**System Objects First**: All planning, roadmaps, and strategic information are system objects. Markdown documents are generated views or linked references, not the source of truth.

## Discovery Patterns

### 1. Starting from Strategic Plan

**Query**: "What is our strategic plan and what does it include?"

```bash
# Get the strategic plan
zqk object get STRAT-PLAN-001 --format yaml

# Get all workstreams in the strategic plan
zqk object list workstream --filter 'goal_refs=GOAL-6369' --filter 'goal_refs=GOAL-6370' --filter 'goal_refs=GOAL-6371' --format table

# Get all goals in the strategic plan
zqk object get STRAT-PLAN-001 --format yaml | grep goal_refs

# Get agent onboarding timeline
zqk object get STRAT-PLAN-001 --format yaml | grep -A30 agent_onboarding_timeline
```

**Result**: Complete strategic context with workstreams, goals, and agent onboarding schedule.

### 2. Exploring a Workstream

**Query**: "What is WS-011 and what work does it contain?"

```bash
# Get workstream details
zqk object get WS-011 --format yaml

# Get all backlog items for this workstream
zqk object list backlog_item --filter 'workstream_refs=WS-011' --format table

# Get goals this workstream supports
zqk object get WS-011 --format yaml | grep goal_refs

# Get milestones for this workstream
zqk object get WS-011 --format yaml | grep milestone_refs

# Get agent onboarding for this workstream
zqk object list agent_onboarding_preparation --filter 'target_workstream_ref=WS-011' --format table

# Get workstream transitions involving this workstream
zqk object list workstream_transition --filter 'from_workstream_ref=WS-011' --format table
zqk object list workstream_transition --filter 'to_workstream_ref=WS-011' --format table
```

**Result**: Complete workstream context including backlog items, goals, milestones, agents, and transitions.

### 3. Understanding Goal Alignment

**Query**: "What workstreams support GOAL-6372?"

```bash
# Get all workstreams supporting this goal
zqk object list workstream --filter 'goal_refs=GOAL-6372' --format table

# Get all milestones for this goal
zqk object list milestone --filter 'goal_refs=GOAL-6372' --format table

# Get all backlog items for this goal (via workstreams)
zqk object list backlog_item --filter 'goal_refs=GOAL-6372' --format table

# Get strategic plan for this goal
zqk object list strategic_plan --filter 'goal_refs=GOAL-6372' --format table
```

**Result**: Complete goal alignment showing all workstreams, milestones, and backlog items contributing to the goal.

### 4. Roadmap Exploration

**Query**: "What is the Phase 2 roadmap and what does it include?"

```bash
# Get roadmap details
zqk object get ROAD-006 --format yaml

# Get all workstreams in this roadmap
zqk object get ROAD-006 --format yaml | grep workstream_refs

# Get all goals in this roadmap
zqk object get ROAD-006 --format yaml | grep goal_refs

# Get all milestones in this roadmap
zqk object get ROAD-006 --format yaml | grep milestone_refs

# Get timeline
zqk object get ROAD-006 --format yaml | grep timeline
```

**Result**: Complete roadmap context with workstreams, goals, milestones, and timeline.

### 5. Agent Onboarding Discovery

**Query**: "What agents are being onboarded and when?"

```bash
# Get all agent onboarding preparations
zqk object list agent_onboarding_preparation --format table

# Get agent onboarding for a specific workstream
zqk object list agent_onboarding_preparation --filter 'target_workstream_ref=WS-009' --format table

# Get readiness criteria for an agent
zqk object get AGENT-PREP-002 --format yaml | grep -A20 readiness_criteria

# Get all agents for a strategic plan
zqk object get STRAT-PLAN-001 --format yaml | grep -A30 agent_onboarding_timeline
```

**Result**: Complete agent onboarding schedule with readiness criteria and workstream assignments.

### 6. Workstream Transition Discovery

**Query**: "What are the workstream transitions and when do they occur?"

```bash
# Get all workstream transitions
zqk object list workstream_transition --format table

# Get transitions from a specific workstream
zqk object list workstream_transition --filter 'from_workstream_ref=WS-010' --format table

# Get transitions to a specific workstream
zqk object list workstream_transition --filter 'to_workstream_ref=WS-011' --format table

# Get transition details
zqk object get WST-003 --format yaml
```

**Result**: Complete workstream transition map showing sequencing and triggers.

### 7. Documentation Discovery

**Query**: "What documentation exists for Phase 2?"

```bash
# Get all documentation for a strategic plan
zqk object list doc_entry --filter 'related_objects=strategic_plan:STRAT-PLAN-001' --format table

# Get all documentation for a roadmap
zqk object list doc_entry --filter 'related_objects=roadmap:ROAD-006' --format table

# Get system objects referenced by documentation
zqk object get DOC-152 --format yaml | grep related_objects
```

**Result**: All documentation linked to system objects, enabling bidirectional discovery.

### 8. Multi-Hop Relationship Traversal

**Query**: "What backlog items support GOAL-6372 through WS-011?"

```bash
# Step 1: Get workstreams for goal
zqk object list workstream --filter 'goal_refs=GOAL-6372' --format table

# Step 2: Get backlog items for workstream
zqk object list backlog_item --filter 'workstream_refs=WS-011' --format table

# Step 3: Verify goal alignment
zqk object list backlog_item --filter 'workstream_refs=WS-011' --filter 'goal_refs=GOAL-6372' --format table
```

**Result**: Complete traceability from goal to workstream to backlog items.

## Query Patterns for Common Questions

### "What is the current status of Phase 2?"

```bash
# Get Phase 2 roadmap
zqk object get ROAD-006 --format yaml

# Get all workstreams in Phase 2
zqk object list workstream --filter 'goal_refs=GOAL-6372' --filter 'goal_refs=GOAL-6373' --filter 'goal_refs=GOAL-6374' --format table

# Get status of each workstream
zqk object list workstream --filter 'goal_refs=GOAL-6372' --format yaml | grep -E "id:|status:"
```

### "What agents are needed for WS-011?"

```bash
# Get agent onboarding for workstream
zqk object list agent_onboarding_preparation --filter 'target_workstream_ref=WS-011' --format table

# If none, check if agents are needed for related workstreams
zqk object list agent_onboarding_preparation --format table
```

### "What milestones must complete before WS-011 can start?"

```bash
# Get workstream transitions to WS-011
zqk object list workstream_transition --filter 'to_workstream_ref=WS-011' --format yaml

# Get trigger milestones
zqk object list workstream_transition --filter 'to_workstream_ref=WS-011' --format yaml | grep trigger_milestone_ref
```

### "What is the timeline for Phase 2?"

```bash
# Get roadmap timeline
zqk object get ROAD-006 --format yaml | grep timeline

# Get milestone target dates
zqk object list milestone --filter 'goal_refs=GOAL-6372' --filter 'goal_refs=GOAL-6373' --filter 'goal_refs=GOAL-6374' --format yaml | grep -E "id:|target_date:"
```

### "What documentation explains Phase 2?"

```bash
# Get documentation for Phase 2 roadmap
zqk object list doc_entry --filter 'related_objects=roadmap:ROAD-006' --format table

# Get documentation for strategic plan
zqk object list doc_entry --filter 'related_objects=strategic_plan:STRAT-PLAN-001' --format table
```

## Benefits of System-First Approach

1. **Single Source of Truth**: System objects are authoritative
2. **Discoverable**: Relationships enable exploration
3. **Consistent**: Multiple views from same source
4. **Adaptive**: Changes automatically reflected
5. **Self-Documenting**: System can answer questions about itself
6. **Onboarding**: New agents/humans can explore and understand
7. **Awareness**: System is aware of changes and their impacts

## Next Steps

1. **Generate Views**: Create commands to generate markdown from system objects
2. **Relationship Traversal**: Implement multi-hop query capabilities
3. **Impact Analysis**: Show how changes affect related objects
4. **Alignment Validation**: Verify alignment across goals, workstreams, and backlog items
5. **Resource Planning**: Query agent and human resource requirements

