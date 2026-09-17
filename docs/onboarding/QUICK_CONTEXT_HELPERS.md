# Quick Context Helpers

This document provides quick reference commands for common context discovery tasks in zqk.

## Current Priorities

### Get Current Active Priority Plan
```bash
zqk object list priority_plan --filter 'status=active' --sort-by active_order --format table
```

### Get Current Priority Plan Details
```bash
# Get the ID first
PRI_ID=$(zqk object list priority_plan --filter 'status=active' --sort-by active_order --format json | jq -r '.[0].id')

# Get full details
zqk object get $PRI_ID --format yaml
```

### Get Backlog Items for Current Priority Plan
```bash
PRI_ID=$(zqk object list priority_plan --filter 'status=active' --sort-by active_order --format json | jq -r '.[0].id')
zqk object list backlog_item --filter "priority_plan_ref=$PRI_ID" --format table
```

## Strategic Plan Alignment

### Get Strategic Plan Overview
```bash
zqk object get STRAT-PLAN-001 --format yaml
```

### Get Phase 1 Workstreams
```bash
zqk object list workstream --filter 'id=WS-008|WS-009|WS-010' --format table
```

### Get Phase 1 Goals
```bash
zqk object list goal --filter 'id=GOAL-6369|GOAL-6370|GOAL-6371|GOAL-6372' --format table
```

### Get Phase 1 Milestones
```bash
zqk object list milestone --filter 'id=MIL-038|MIL-039|MIL-040' --format table
```

## Workflow and Policies

### Get Workflow Policies
```bash
zqk object list policy --filter 'category=workflow' --format table
```

### Get All Active Policies
```bash
zqk object list policy --filter 'status=active' --format table
```

### Get Policies by Type
```bash
# Standards (mandatory)
zqk object list policy --filter 'policy_type=standard' --format table

# Requirements
zqk object list policy --filter 'policy_type=requirement' --format table

# Guidelines
zqk object list policy --filter 'policy_type=guideline' --format table
```

## System Health

### Quick System Check
```bash
zqk system check --fast
```

### Full System Check
```bash
zqk system check
```

### System Status
```bash
zqk system status
```

## Workstream Discovery

### List Active Workstreams
```bash
zqk object list workstream --filter 'status=active' --format table
```

### Get Workstream Details
```bash
zqk object get WS-008 --format yaml
```

### Get Workstream Backlog Items
```bash
zqk object list backlog_item --filter 'workstream_refs=WS-008' --format table
```

## MCP Integration

All CLI commands are automatically available via MCP tools with the `cli_` prefix:

- `zqk object list` → `cli_object_list`
- `zqk system check` → `cli_system_check`
- `zqk object get` → `cli_object_get`

The MCP server automatically discovers and exposes all CLI commands with proper security filtering.
