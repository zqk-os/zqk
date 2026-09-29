---
name: zqk-expert
description: >-
  Expert guidance for ZQK Knowledge Kernel operations: CLI-only process data, VDS done-gates,
  scheduler test bundles, agent feed/MCP mesh, TPM Gantt matrix (roadmap/workstream/plan),
  plan-scoped branch collaboration, CLI command DNA, and convergence discipline.
  Use when operating zqk CLI, claiming work done, orchestrating swarm seats, touching .zqk/process,
  or aligning agent workflows with current system standards.
seal_hash: 53d963ff3c8a861178125d60a75a428378c0b59951d265cc8877b1c65b5674d9
seal_version: 1.0.0
seal_issuer: system:cli
seal_date: 2026-09-29T00:42:51-07:00
---

# ZQK Expert

Mandatory operating skill for swarm and IDE agents. Prefer **re-runnable evidence** over narrative.
When this file disagrees with live CLI help or an active `POL-*` object, **trust the kernel + CLI**.

**Canonical path:** `.zqk/skills/zqk-expert/` (keep `skills/zqk-expert/` in sync).


**Kernel registration:** orchestration loads `agent_skill` (`ASK-*`) objects, not filesystem packs alone. Current twins (approved):
- `ASK-1785642072074959000-a1dfbde9` — ZQK Expert Operating Protocol (`[orchestration-boot]`)
- `ASK-1786081148616665000-c78bdd4d` — ZQK Orchestration — Plan Primary Protocol (`.zqk/skills/zqk-orchestration/`)
- `ASK-1785642073632982000-e7c81062` — Scheduler Expert Protocol
- `ASK-1785642074896778000-83af9fd2` — Go Architect Protocol
- `ASK-1785642076100778000-b4459584` — DevOps Expert Protocol
- `ASK-1785642077281504000-9cda2d52` — ZQK QA Traceability Auditor Protocol
- `ASK-1785642078724330000-9a788725` — Adversarial Auditor Protocol

Personas (e.g. `PER-1781486429759398000-a79939a1` ZQK Expert) link these via `related_object_refs`. Keep SKILL.md and ASK `instructions` in sync when refreshing.

**Trust / seal (planned):** skills should be cryptographically sealed (hash + version/date + issuer) before swarm load — tracked `[REDACTED-ID]` (REQ-SYM-021). Periodic skill refresh under maint policy: `[REDACTED-ID]`. Until seals enforce fail-closed, treat skill text as advisory against live CLI/POL objects.


## 1. Orchestration Hygiene & Critical Sections

You must execute all operations within a strict critical section to prevent race conditions.
Structure your execution as a `try-catch-finally` block guaranteeing lock release on all paths.

- **TRY:**
  - Execute: `./bin/zqk agent claim <task_id>`
  - IF exit_code != 0 THEN HALT and do not proceed.
  - Append `--atomic --skip-parallel` to EVERY state-altering command you execute.
- **CATCH (Exception/Failure):**
  - IF any step fails, force lock release AND propagate failure.
- **FINALLY:**
  - ALWAYS execute lock release: `./bin/zqk agent release <task_id>`

## 2. Fail-Closed Compliance

You must enforce strict fail-closed operations.
- IF any command returns a non-zero exit code, "command not found", "timeout", or "network unreachable", THEN emit an explicit JSON failure payload to stderr AND HALT.
- Standardize the error payload across all failure branches: `{"status": "FAIL_CLOSED", "cause": "<specific_cause>", "context": "<command_executed>"}`.
- Narrative fallbacks are strictly FORBIDDEN. Never guess, assume completion, or hallucinate recovery logic for schema/validation failures.

## 3. Kernel Traceability & Deterministic Checks

- **Deterministic Evaluation**: Before claiming success on any kernel object mutation, you MUST run deterministic checks. For example, execute `make verify` and ensure the exit code is 0.
- **Event-Driven Traceability**: Shift from in-place mutation to event-driven logging where applicable. Update kernel objects via the CLI and ALWAYS use `--version <current>+1` to prevent race conditions. Ensure any changes trigger the appropriate CAS updates (`.zqk/cas-index.json`).
## Session boot (do this first)

```bash
./bin/zqk workflow whats-next --format json          # mission / PRI / cues; add --agent-id <seat>
./bin/zqk system policy-interrupts list              # before high-stakes work
# Kernel health (do not stop at summary.blocking / error_status_objects):
#   ./bin/zqk system check --details --verbose --layer 1   # Layer 1 + draft plane; do not --clear-cache as routine
#   Spot-check residual fuel: system check <BLI-id> on holding-column items
# Optional refresh snapshot:
#   docs/architecture/AGENT_CONTEXT_REFRESH.md
```

Infer next work from **whats-next JSON**, not from chat memory. Active `convergence_session` (`CVS-*`) contracts beat improvised priorities.

**Pristine ≠ summary green.** Layer 1 membership mismatches (e.g. `validated` BLI on `active` plan), draft-plane piles, and empty-`description` CAS objects can hide while `error_status_objects=0`. TRACK: `BLI-KERNEL-CHECK-UNBLIND-MEMBERSHIP-001`.

## Hard rules (non-negotiable)

| Rule | Do | Do not |
|------|----|--------|
| Process data | `zqk object create\|update\|promote\|bulk …` | Edit instance YAML under `.zqk/process/` by hand |
| Kind formalization | Use `StageCrossMembrane` and cite snag catalog `GLS-1786413953213934000-1c0fb3ff` | Hand-edit `id_prefixes` / advertise kinds missing a storage membrane |
| Done claims | `zqk workflow vds evaluate` (+ evidence) | Mark BLI/CVS/CAP done from chat alone |
| Tests | `zqk test run --all` or `make test-cases` | Foreground `go test ./…` / multi-minute waits |
| CLI surface | Edit `.zqk/cli/specs/…` then regenerate builders | Hand-edit `pkg/cli/bldr_cli_cmd_v1/*_command_builder.go` |
| Kernel vs IDE | Author intent in POL/GLS/BLI; project vendors | Treat `.cursor/rules/*.mdc` as SSOT |
| Three planes (checkout / kernel / correspondence) | Exec cwd may be the worktree; feed + daemons stay on the seated kernel (`POL-AGENT-KERNEL-ROOT-BINDING-001`) | Collapse git cwd, CAS, and feed home into one env; guess CAS epoch from cwd |
| 3-plan Gantt runway | 1 execution lead + ≥2 grooming columns (`POL-AGENT-THREE-PLAN-RUNWAY-001`) | Poach another PRI; remint orch on a sealed lead; align-cache as a substitute for objectify |
| Code search | `zqk grep` / `zgrep` (in-process AST & trigram queries) | External slow shell binaries / brittle regex scans |
| Public publish | Human ACK + `scripts/check-public-release-payload.sh` | Push unpublished trees to public orgs |

**Foreground `go test`:** only narrow probes (`-timeout` ≤ 60s, tight `-run`). Universal verification = **`zqk test run --all`** (legacy `scan-tests` deprecated under `REQ-TEST-BUNDLE-DEPRECATION-001`).
**Native code search:** use **`zqk grep`** (or `zgrep`). Sub-15ms Go AST (`--ast --kind struct|func|interface`, `--ast --recv <Type>`) and token-budgeted AI payloads (`--max-tokens 2000 -f json`).

## Verifiable Decomposition Spine (VDS)

Policy: **POL-WORKFLOW-VDS**. Culture + DSL gate for “done.”

```bash
./bin/zqk workflow vds checklist --format json
./bin/zqk workflow vds evaluate --format json          # exit non-zero on FAIL
./bin/zqk workflow vds evaluate --format agent-prompt
./bin/zqk workflow vds evaluate --apply-verify         # persist independent_verify=yes (surgical)
./bin/zqk workflow vds project --all --check           # vendor projection drift
```

Working chunks: `docs/quality/vds_chunks.yaml`. Profiles under `docs/quality/verifiable_decomposition_*.yaml`.  
Architecture: `docs/architecture/VERIFIABLE_DECOMPOSITION_SPINE.md`.  
Before claiming stage progress: name `stage` + `chunk_id`, show `rubric_ref` / `dsl_checks` / `evidence_refs` / `independent_verify`.

## Swarm / correspondence plane

```bash
./bin/zqk mcp ensure --tcp 127.0.0.1:8443
./bin/zqk feed doctor --format json                    # feed_health; peer_wake needs subscribers
./bin/zqk feed watch --agent-id <seat> --persona-ref PER-DEFAULT-OPERATOR   # long-lived; use dedicated terminal (or --timeout 24h)
./bin/zqk feed steer --agent-id <from> --to-agent-id <peer> --message "…"
./bin/zqk feed ack --in-reply-to AFE-… --agent-id <seat> --persona-ref PER-…
./bin/zqk agent orchestrate                            # route PRI backlog to seats
./bin/zqk agent status --format json
```

- Pass **`--agent-id`** on whats-next / feed for inbox/outbox. Unacked inbox → ack, then continue.
- Prefer native `zqk feed …` over legacy wake shell scripts (`scripts/mesh/README.md`).
- Vendor-host subagents (Cursor Task, Antigravity `invoke_subagent`) are allowed. Cold launch is not: hydrate the initial prompt with `zqk agent prepare-context` (`POL-AGENT-SUBAGENT-CTX-001`) before any vendor or native worker.
- Mesh wake = notify/MCP only (`POL-AGENT-MESH-WAKE-001`); directed steers use `--await-peer-ack` (`POL-AGENT-ORCH-HOURGLASS-001`).

## TPM plane: Gantt matrix + plan branch collaboration

TPM / portfolio altitude lives here. **Plan primaries** (peers orchestrating subagents on one PRI) use **`zqk-orchestration`**: `.zqk/skills/zqk-orchestration/SKILL.md` (workstream + assigned plan; debt vs feature sequencing).
Detail (TPM): [references/tpm-orchestration.md](references/tpm-orchestration.md).

| Axis | Object | Meaning |
|------|--------|---------|
| Frame | `roadmap` | Product/system viewport around the Gantt |
| Rows (lanes) | `workstream` | Like/complementary activities; long-lived reporting lanes |
| Columns | `priority_plan` | Feature/functional bundles; may span multiple workstreams |
| Time anchors | `milestone` | Make the timeline honest |
| Cells | BLI → ATK | Work under a **locked** plan |

**Scope lock:** fill the plan shovel-ready **before** `active`/`in_progress`. Kernel membership refuses draft-plane scope creep onto execution-facing plans. Mid-flight gaps → new plan (or strategic replan), not stuffing the locked column.

**Ownership:** hand each plan to a primary (`zqk agent orchestrate <PRI>` … to completion). TPM stewards roadmaps/workstreams/conflicts and PR gates — **not** ATK babysitting / conveyor minting.

**Branch collaboration** (`POL-CODE-1784784370706305000-e3e7245a`):

1. After any merge to `main`, fetch `origin/main`. TPM provisions **one collaboration branch per plan** from **that tip** (pre-PR trunk; e.g. `integration/pri-…`). Never mint from a feature HEAD or stale local `main`.
2. Agents work in isolated **git worktrees** off that branch — never `git checkout` between seats in the studio tree, never race `main` or peer tips.
3. Merge up into the plan branch after pedantic/build gates.
4. TPM opens the **single PR** to `main` and chooses auto-merge vs human oversight from criteria.

## Objects & CLI DNA

- **Rosetta:** `docs/onboarding/SYSTEM_OBJECTS_GUIDE.md`
- Common kinds: `roadmap`, `workstream`, `priority_plan` (PRI), `backlog_item` (BLI), `milestone` (MIL), `goal`, `policy` (POL), `glossary_term` (GLS), `convergence_session` (CVS), `scheduler_job` (SCH), `agent_task` (ATK)
- Hierarchy: BLIs need valid refs (`goal_ref`/`requirement_ref`, often `priority_plan_ref` + milestone; optional `workstream_refs`) before promote past exploring — fix linkage; don’t snap-remedy status.
- **Traceability mint:** `requirement` / `goal` / `milestone` → `zqk workflow gen-trace-pipeline <id>` before leaving conceptual (`POL-AGENT-TPM-TRACE-PIPELINE-001`). `new object` auto-runs it. Do not hand-mint 1:1 REQ→CRIT.
- **Creates** often land in lifecycle **draft/origin**; promote when criteria allow. Don’t invent status strings.
- **CLI commands:** edit `.zqk/cli/specs/<group>/*_command.yaml` →  
  `go build -o bin/zqk-admin ./cmd/zqk && ./bin/zqk-admin system generate-command-builders --overwrite` → wire `RunE` in `cmd/zqk/…` → rebuild `bin/zqk` → `./bin/zqk system validate-command-specs`.
  Do not generate from `.zqk/process/command_specs` by default (CAS slop). See `.cursor/rules/cli-command-spec-codegen.mdc`.
- Output/logging: logger / `cli.FormatOutput` — never `fmt.Print*` for user-facing CLI (POL-CODE-007 / POL-CODE-015).

## Verification & convergence

```bash
./bin/zqk test run --all                               # spec-driven test_case verification
./bin/zqk scheduler test-failures
./bin/zqk scheduler convergence measure --session-id CVS-…
```

- Ground truth for a remediation loop = the **`convergence_session`** hypothesis / predictions / `desired_end_state` — not agent self-ratings.
- Optional promotion helpers: `scripts/check_convergence_promotion_readiness.sh` (see `scripts/README.md`).
- Pre-commit gates include VDS vendor projection (`scripts/check-vds-vendor-project.sh`).

## Kernel → vendor projection

Intent lives in kernel objects. Vendors (Cursor `.mdc`, etc.) are **exports**:

```bash
./bin/zqk workflow vds project --all --write
./bin/zqk workflow vds project --all --check
```

Doc: `docs/architecture/KERNEL_VENDOR_INSTRUCTION_PROJECTION.md`.

## Public push (fail closed)

Do not publish unpublished Open-Core trees. Push toward public orgs requires human-written `.zqk/state/public_push_ack.json` and green `scripts/check-public-release-payload.sh`. Agents prepare commits; humans publish.

## References (progressive detail)

| Topic | Path |
|-------|------|
| Native Code Search & AST | [references/code-search.md](references/code-search.md) |
| VDS (skill-local) | [references/vds.md](references/vds.md) |
| Testing / scan-tests | [references/testing.md](references/testing.md) |
| Data cells | [references/data-cell.md](references/data-cell.md) |
| Stream storage | [references/stream-storage.md](references/stream-storage.md) |
| TPM Gantt / plan branches | [references/tpm-orchestration.md](references/tpm-orchestration.md) |
| System objects | `docs/onboarding/SYSTEM_OBJECTS_GUIDE.md` |
| Agent onboarding | `docs/onboarding/AI_AGENT_ONBOARDING.md` |
| Pre-change checklist | `docs/architecture/PRE_CHANGE_CHECKLIST.md` |
| Runtime organism narrative | `docs/architecture/DATA_CELL_RUNTIME_ORGANISM.md#data-cell-narrative` |
| Mesh ops | `scripts/mesh/README.md` |
| Context refresh (generated) | `docs/architecture/AGENT_CONTEXT_REFRESH.md` |

## Anti-patterns (swarm confusion)

- Editing CAS YAML under `.zqk/process/` then “fixing” hashes
- Equating green chat text with VDS PASS
- Blocking the session on multi-minute `go test` / sleep-polling scheduler jobs
- Treating `skills/zqk-expert` and `.zqk/skills/zqk-expert` as intentionally different (keep identical)
- Claiming peer wake live without `feed doctor` evidence
- Hand-authoring policy prose only in `.cursor/rules` without kernel + `vds project`
- TPM micro-minting ATKs / babysitting seats instead of plan ownership + orchestrate-to-completion
- Expanding an `in_progress`/`active` plan with draft-plane BLIs (scope creep)
- Agents inventing integration branches or opening many PRs to `main` instead of merging into the TPM plan branch
- Plan primaries ignoring workstream balance (e.g. never scheduling tech-debt lanes) until sequencing collapses into mayhem