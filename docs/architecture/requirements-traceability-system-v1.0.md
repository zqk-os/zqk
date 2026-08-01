# Requirements Traceability System v1.0

**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Design  
**Related**: POL-PLAN-002, Requirement Objects, Test Cases, Criteria

## Purpose

This document defines the requirements traceability system that ensures all requirements can be traced upward to strategic objectives (mission, vision, goals, milestones) and downward to tactical execution (backlog items, test cases, criteria). It enables validation of progress and achievement of stated objectives.

## Traceability Hierarchy

### Complete Traceability Chain

```
Mission (MIS-###)
  └─→ Vision (VIS-###)
        └─→ Goal (GOAL-####)
              ├─→ Milestone (MIL-###)
              │     ├─→ Requirement (REQ-###)
              │     │     ├─→ Test Case (TEST-###)
              │     │     └─→ Backlog Item (BLI-####)
              │     └─→ Criteria (CRIT-####)
              └─→ Workstream (WS-###)
                    └─→ Priority Plan (PRI-###)
                          └─→ Backlog Item (BLI-####)
                                └─→ Requirement (REQ-###)
                                      └─→ Test Case (TEST-###)
```

## Requirement Object Structure

### Core Fields

```yaml
id: REQ-###
kind: requirement
title: "Requirement Title"
context: "Requirement description and context"
status: "active" | "completed" | "blocked" | "deprecated"
priority_tier: "P0" | "P1" | "P2" | "P3"

# Upward Traceability (Strategic)
milestone_refs: [MIL-###, ...]
goal_refs: [GOAL-####, ...]
workstream_refs: [WS-###, ...]

# Downward Traceability (Tactical)
backlog_item_refs: [BLI-####, ...]
test_case_refs: [TEST-###, ...]

# Validation
acceptance_criteria:
  - "Criterion 1"
  - "Criterion 2"
```

### Traceability Fields

**Upward Traceability** (Links to Strategic Objects):
- `milestone_refs`: Milestones this requirement implements
- `goal_refs`: Goals this requirement supports
- `workstream_refs`: Workstreams this requirement belongs to

**Downward Traceability** (Links to Tactical Objects):
- `backlog_item_refs`: Backlog items that implement this requirement
- `test_case_refs`: Test cases that validate this requirement

**Cross-Linking**:
- Requirements can link to multiple milestones (if requirement spans milestones)
- Requirements can link to multiple backlog items (if requirement spans items)
- Requirements can link to multiple test cases (if requirement has multiple validation paths)

## Traceability Validation

### Validation Gates

#### Gate 1: Strategic Alignment

**Check**: All requirements trace to at least one goal
```bash
zqk object list requirement --filter 'goal_refs=GOAL-####'
```

**Validation**: Every requirement must have at least one `goal_refs` entry
**Blocking**: Yes - Requirements without goal links block planning

#### Gate 2: Milestone Coverage

**Check**: All requirements trace to at least one milestone
```bash
zqk object list requirement --filter 'milestone_refs=MIL-###'
```

**Validation**: Every requirement should have at least one `milestone_refs` entry
**Blocking**: Warning - Requirements without milestone links indicate incomplete planning

#### Gate 3: Test Coverage

**Check**: All requirements have test cases
```bash
zqk object list test_case --filter 'requirement_refs=REQ-###'
```

**Validation**: Every requirement should have at least one `test_case_refs` entry
**Blocking**: Warning - Requirements without test cases indicate incomplete validation

#### Gate 4: Backlog Item Coverage

**Check**: All requirements have backlog items
```bash
zqk object list backlog_item --filter 'requirement_refs=REQ-###'
```

**Validation**: Every requirement should have at least one `backlog_item_refs` entry (or requirement should be in backlog item's `requirement_refs`)
**Blocking**: Warning - Requirements without backlog items indicate incomplete execution planning

## Query Patterns

### "What requirements support GOAL-6370?"

```bash
zqk object list requirement --filter 'goal_refs=GOAL-6370' --format table
```

### "What test cases validate REQ-036?"

```bash
zqk object list test_case --filter 'requirement_refs=REQ-036' --format table
```

### "What backlog items implement REQ-036?"

```bash
zqk object list backlog_item --filter 'requirement_refs=REQ-036' --format table
```

### "What is the complete traceability chain for REQ-036?"

```bash
# Get requirement
zqk object get REQ-036 --format yaml

# Get goals
zqk object get REQ-036 --format yaml | grep goal_refs

# Get milestones
zqk object get REQ-036 --format yaml | grep milestone_refs

# Get test cases
zqk object get REQ-036 --format yaml | grep test_case_refs

# Get backlog items
zqk object get REQ-036 --format yaml | grep backlog_item_refs
```

### "What requirements are in MIL-038?"

```bash
zqk object list requirement --filter 'milestone_refs=MIL-038' --format table
```

### "What is the test coverage for MIL-038?"

```bash
# Get requirements for milestone
zqk object list requirement --filter 'milestone_refs=MIL-038' --format yaml | grep id:

# Get test cases for each requirement
zqk object list test_case --filter 'requirement_refs=REQ-036' --format table
```

## Progress Validation

### Requirement Completion

A requirement is considered complete when:
1. All `acceptance_criteria` are met
2. All linked `test_case_refs` are passing
3. All linked `backlog_item_refs` are complete
4. Requirement `status` is set to `completed`

### Milestone Completion

A milestone is considered complete when:
1. All linked `requirement_refs` are complete
2. All linked `criteria_refs` are met
3. Milestone `status` is set to `complete`

### Goal Achievement

A goal is considered achieved when:
1. All linked `milestone_refs` are complete
2. All linked `workstream_refs` have completed their objectives
3. Goal `status` is set to `achieved`

## Best Practices

1. **Requirement-Driven Development**: All development should trace to requirements
2. **Test-Driven Validation**: All requirements should have test cases
3. **Continuous Traceability**: Maintain traceability throughout lifecycle
4. **Regular Validation**: Run validation gates regularly
5. **Clear Acceptance Criteria**: Requirements must have clear, measurable acceptance criteria
6. **Bidirectional Links**: Maintain links in both directions (requirement → test case, test case → requirement)

## Integration with Planning Lifecycle

The requirements traceability system integrates with POL-PLAN-002:

1. **Phase 3: Tactical Planning**: Requirements are defined with full traceability
2. **Phase 4: Validation Planning**: Test cases are linked to requirements
3. **Validation Gates**: Traceability is validated at each gate
4. **Progress Tracking**: Progress is tracked through requirement completion

## Agent orchestration and knowledge loops (traceability extension)

Cross-cutting work on **multi-agent prompt delivery**, **auditability**, and **policy/knowledge self-assessment** is modeled using the same hierarchy (goal → requirement → criteria → backlog). The **persistent traceability bundle** at `test-scenarios/agent-orchestration-traceability-bundle/agent-orchestration-traceability-bundle.yaml` defines **GOAL-AO-001**, **REQ-AO-001**, criteria **CRIT-AO-001**–**006**, and **doc_entry** links back to this document and to [SCENARIO_BUNDLES_AND_TRACEABILITY.md](../../architecture/SCENARIO_BUNDLES_AND_TRACEABILITY.md). Apply the bundle with `zqk-scenario bundle apply` or `zqk bundle apply` (see bundle header comments); then link **REQ-AO-001** to milestones and priority plans via `zqk object update` when scheduling execution.

## Related Objects

- **POL-PLAN-002**: Planning Lifecycle Policy (defines requirement creation process)
- **Test Cases**: Test cases validate requirements
- **Criteria**: Criteria define milestone completion (may reference requirements)
- **Backlog Items**: Backlog items implement requirements
- **Milestones**: Milestones group requirements
- **Goals**: Goals are supported by requirements

