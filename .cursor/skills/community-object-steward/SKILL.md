---
name: community-object-steward
description: CLI-only CAS: mint/update/promote via zqk, never edit .zqk/process YAML.
---

# Kernel Object Stewardship (CLI-only CAS)

## Objective
Keep the community knowledge kernel coherent. Process data under `.zqk/process/` is content-addressed. Hand-edits break filenames, caches, and `object get`.

## Protocol
1. **Mint**: `zqk new object <kind> --title "..."` (draft plane). Requirements/goals/milestones auto-run `zqk workflow gen-trace-pipeline` unless `--skip-trace-pipeline`. Do not hand-mint a 1:1 REQ→CRIT.
2. **Enrich**: `zqk object update <id> --field key=value` or `--file`. Link `goal_refs`, `workstream_refs`, `priority_plan_ref`, `persona_refs`.
3. **Promote**: `zqk object promote <id>` one hop when preconditions pass. Do not claim complete without VDS (`zqk workflow vds evaluate`).
4. **Forbidden**: editor save, `sed`, or rewrite of `.zqk/process/**/*.yaml`. Untracked hash YAML is kernel state — never `git clean` it.
5. **Roadmap Gantt**: Roadmaps frame products; workstreams are lanes; priority_plans are columns. A PRI stays `originated` until it has a workstream lane, written identity, and persona/team refs; grooming then active requires a shovel-ready BLI.
6. **Binary**: live CLI is `brand.executable_name`. Do not point at a studio kernel.
