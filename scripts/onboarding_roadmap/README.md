# Onboarding Roadmap

This directory contains YAML templates for the **Agent & User Onboarding** workstream, priority plan, and backlog items. The onboarding curriculum is first-class system data (workstream, priority plan, backlog items) so it is indexed, discoverable, and auditable.

**Design**: [docs/architecture/ONBOARDING_ROADMAP_AND_CERTIFICATION.md](../../docs/architecture/ONBOARDING_ROADMAP_AND_CERTIFICATION.md)

**Automated seed**: [scripts/scheduler_jobs/onboarding_roadmap_seed.yaml](../scheduler_jobs/onboarding_roadmap_seed.yaml) runs the same steps as below (milestone before backlog items with `milestone_refs`). After `zqk system init --with-onboarding-roadmap`, start the scheduler once to execute the job.

## Reference pattern for advanced tutorials

This roadmap is a **template for curriculum-as-data**: a `priority_plan`, `workstream`, `milestone`, `goal`, and linked `backlog_item` rows, all discoverable via `object list` / filters. Advanced tutorials can reuse the same shape with different YAML under `scripts/` (or another path) and a dedicated `scheduler_job` (`run_wrapper`) or manual CLI steps. That showcases **customizability** without forking the CLI: swap templates, add backlog items, tie items to additional milestones or policies, and keep everything auditable in `.zqk/process/`. Point authors at this directory and the seed job as the **minimal working example**; link longer narrative docs from `doc_entries` or policy bodies as needed.

## Creation order (CLI only)

All objects must be created via the zqk CLI. Do not edit instance YAML under `.zqk/process/` directly.

Backlog items in these templates use **`status: planned`**, which requires **at least one `milestone_ref`** (see `backlog_item` lifecycle). Create the **milestone** before the backlog items, then pass `milestone_refs` on create (and link `workstream_refs` afterward).

### 1. Create the priority plan

```bash
zqk object create priority_plan --file scripts/onboarding_roadmap/priority_plan_onboarding.yaml
```

Result: Plan with id `PRIO-onboarding` (fixed in template). Note the plan ID for filters.

### 2. Create the workstream

```bash
zqk object create workstream --file scripts/onboarding_roadmap/workstream_onboarding.yaml --relaxed
```

Result: Workstream created with system-assigned ID (e.g. `WS-XXX`). **Capture this ID** for later steps.

### 3. Create the milestone and link the workstream

```bash
zqk object create milestone --file scripts/onboarding_roadmap/milestone_onboarding.yaml
# Capture MIL-ID from output, then:
zqk object update <MIL-ID> --field "workstream_refs=[<WS-ID>]"
```

### 4. Create backlog items (with `milestone_refs`) and link the workstream

```bash
zqk object create backlog_item --file scripts/onboarding_roadmap/backlog_item_01_read_philosophy.yaml --field "milestone_refs=[<MIL-ID>]"
zqk object create backlog_item --file scripts/onboarding_roadmap/backlog_item_02_start_here.yaml --field "milestone_refs=[<MIL-ID>]"
zqk object create backlog_item --file scripts/onboarding_roadmap/backlog_item_03_system_health.yaml --field "milestone_refs=[<MIL-ID>]"
```

Then set the workstream on each item:

```bash
zqk object update <BLI-ID-1> --field "workstream_refs=[<WS-ID>]"
zqk object update <BLI-ID-2> --field "workstream_refs=[<WS-ID>]"
zqk object update <BLI-ID-3> --field "workstream_refs=[<WS-ID>]"
```

### 5. Create the goal and link milestone + backlog

```bash
zqk object create goal --file scripts/onboarding_roadmap/goal_onboarding.yaml
# Capture GOAL-ID, then link milestone and goal per your IDs:
zqk object update <MIL-ID> --field "workstream_refs=[<WS-ID>]" --field "backlog_item_refs=[<BLI-1>,<BLI-2>,<BLI-3>]"
zqk object update <GOAL-ID> --field "workstream_refs=[<WS-ID>]" --field "backlog_item_refs=[<BLI-1>,<BLI-2>,<BLI-3>]"
```

### 6. Optional: agent_onboarding_preparation

Link an `agent_onboarding_preparation` to this workstream and set `preparation_tasks` to the list of onboarding backlog item IDs for discoverability. See object spec `agent_onboarding_preparation.yaml`.

## Discovery

- List onboarding backlog items:  
  `zqk object list backlog_item --filter priority_plan_ref=PRIO-onboarding`
- Get the workstream:  
  `zqk object list workstream --filter "title=Agent & User Onboarding"`
- Start Here and Operational Philosophy policies point users/agents to this curriculum.

## Per-account completion and certification

Completion of these items is tracked **per account** (not by marking backlog items "complete" once). See the design doc for the proposed **certification** object and optional X.509 credential. Until certification is implemented, completion can be tracked manually or via audit/attestation.
