# Verifiable Decomposition Spine (VDS)

**Last Verified:** 2026-08-31


**Status:** Binding when `POL-WORKFLOW-VDS` is active  
**Glossary (durable titles):** `Verifiable Decomposition Spine (VDS)` · acronym `VDS` · CLI `CLI Command: workflow vds`  
**Glossary ids:** project-local `GLS-*` — pin in `verifiable_decomposition_customization.yaml` `glossary:` or resolve at runtime by title (do not hardcode CAS ids in Go).  
**Policy:** `POL-WORKFLOW-VDS` (durable)  
**Spine profile:** [`docs/quality/verifiable_decomposition_spine_profile.yaml`](../quality/verifiable_decomposition_spine_profile.yaml)  
**Customization (per project):** [`docs/quality/verifiable_decomposition_customization.yaml`](../quality/verifiable_decomposition_customization.yaml)

## Purpose

Agents can claim “done” with no consequences. VDS makes **every collaborative activity** advance only through **independently, objectively verifiable chunks**. Chat is never evidence.

This contract is **project-agnostic**: the **spine** (stages, chunk contract, hard gates, forbidden theater) is rigid. **Customization** (lint commands, scheduler vs terminal tests, style, CI names) lives in a separate file and **may not remove gates**.

## Relationship to CAP and convergence sessions

| Mechanism | Role |
|-----------|------|
| **VDS** | Universal work hygiene for intent → design → implement → integrate → operate |
| **CAP** | Priority / whats-next progression; must consume VDS evidence to advance stages |
| **`convergence_session`** | Hypothesis / desired-end-state loops; chunks still need VDS gates |
| **`verification_matrix`** | Preferred accountability surface (see POL-AGENT-003 / matrix accountability) |

CAP and CVS are **consumers** of VDS, not substitutes for it.

## Rigid stages

1. `intent_capture` — vision, mission, goals, constraints  
2. `design` — requirements, criteria, tests, backlog, agent tasks/instructions  
3. `implement` — code/docs with quality and unit evidence  
4. `integrate_verify` — integration/smoke, environment, CI  
5. `operate_release` — DevOps, security, performance, publish/ops  

Do not add, remove, or rename stages without a **policy version bump** and human approval.

## Chunk contract (minimum)

Every unit of work that may be called “done” MUST record:

| Field | Meaning |
|-------|---------|
| `chunk_id` | Stable id for the chunk |
| `stage` | One of the five stage ids |
| `claim` | One verifiable claim (short) |
| `rubric_ref` | Criteria / matrix row / rubric anchor |
| `dsl_checks` | Machine predicates (see catalog) |
| `evidence_refs` | Re-runnable footprints (job id, log path, object id, commit SHA) |
| `independent_verify` | `yes` only after re-run of `dsl_checks` by automation or a second party |

**Rules:**

- **VDS-R1:** One chunk = one claim; later phases may not “prove” earlier ones.  
- **VDS-R2:** Narrative ≠ evidence.  
- **VDS-R3:** Stage advance only when all chunks in that stage have `independent_verify=yes` or a human `waiver_ref`.  
- **VDS-R4:** Customization changes *how* checks run, never *whether* gates exist.

## Customization (allowed)

Projects MAY set in `verifiable_decomposition_customization.yaml`:

- Code style / lint / formatter commands  
- Test execution mode (`scheduler` | `foreground` | `ci` | `hybrid`) and timeouts  
- Process-data mutation mode (`cli_only` vs `allow_direct`)  
- CI check names, security scanners, performance budget docs  
- Auto-merge conditions and waiver authority  

Projects MUST NOT:

- Delete spine stages or hard gates  
- Treat empty `dsl_checks` as green  
- Use customization to redefine “done” as agent self-report  

## DSL catalog (portable predicates)

Evaluators (human script, CI, future engine) resolve these predicates. Implementations may map names to local commands via customization.

| Predicate | Pass criterion |
|-----------|----------------|
| `object_exists:{id}` | Object load succeeds |
| `path_exists:{relpath}` | Path exists under project root (file or directory) |
| `field_nonempty:{id}:{field}` | Field present and non-empty |
| `criteria_linked_or_acceptance_present:{id}` | Linked CRIT or structured acceptance present |
| `git_diff_nonempty_or_waiver` | Diff or waiver for no-op claims |
| `lint_ok_per_customization` | All `code_style.lint_commands` exit 0 |
| `tests_ok_per_customization` | Per `test_execution.mode`: job+log green, or CI green, or allowed foreground probe green |
| `smoke_or_integration_evidence_present` | Named smoke/integration evidence in `evidence_refs` |
| `ci_required_checks_green_or_na` | Listed CI checks green, or list empty (`na`) |
| `security_gate_ok_or_na` | Security commands green, or none configured |
| `performance_gate_ok_or_na` | Perf evidence present, or none configured |
| `publish_ack_present_if_public` | Fail-closed ACK when publishing public surfaces |

Projects may **add** predicates; they may not **skip** the chunk contract.

## Forbidden theater

- Claiming done with only chat text  
- Empty receipts / matrices with no re-runnable `evidence_refs`  
- Advancing CAP / marking BLI complete / closing CVS without VDS gates for in-scope chunks  
- Ghost health / fabricated metrics to force green  
- Waivers without a human-authored `waiver_ref` object  

## Enforcement (teeth)

1. **Policy type:** `standard` — mandatory for agent and human collaborators.  
2. **Default deny:** Unverified “done” is a policy violation.  
3. **Stage gates:** Orchestrators (CAP, priority plans, release checklists) MUST refuse advance without VDS completion for the stage.  
4. **PR / merge:** Prefer blocking checks that require matrix/chunk evidence when automation exists; until then, reviewers reject narrative-only completion.  
5. **Waivers:** Human-only; recorded; time-bounded when possible.  
6. **Portability:** New projects copy spine + empty customization; fill customization before first implement-stage work.  
7. **Automation:** `zqk workflow vds evaluate` is the DSL evaluator. Prefer exit code + JSON/`agent-prompt` over chat claims. `enforcement.automated` on `POL-WORKFLOW-VDS` is **true** when this binary is the project gate. After a green evaluate, `zqk workflow vds evaluate --apply-verify` persists `independent_verify=yes` on PASS/WAIVED chunks in the chunks YAML.

## Agent operating rule

Before reporting progress as complete for any stage:

1. Name the **stage** and **chunk_id**(s).  
2. Show **rubric_ref** + **dsl_checks**.  
3. Show **evidence_refs** (paths/ids that another agent can re-check).  
4. Confirm **independent_verify** (or `waiver_ref`).  

If any item is missing, the work is **not done**.

## CLI (fluent evaluator)

```bash
zqk workflow vds checklist --format json          # stages + culture
zqk workflow vds checklist --format agent-prompt  # markdown brief
zqk workflow vds init                             # scaffold chunks file
zqk workflow vds evaluate --format json           # exit 0 only when PASS/WAIVED
zqk workflow vds evaluate --format agent-prompt   # pasteable gate report
zqk workflow vds evaluate --run-commands          # also run lint/scan from customization
zqk workflow vds evaluate --persist               # write .zqk/state/vds/last_evaluate.json
zqk workflow vds project --provider cursor --write   # regenerate export (id from vendor_providers)
zqk workflow vds project --check                     # drift gate (default provider)
zqk workflow vds project --list                      # configured providers
```

Vendor projection contract: [`KERNEL_VENDOR_INSTRUCTION_PROJECTION.md`](./KERNEL_VENDOR_INSTRUCTION_PROJECTION.md) (`[REDACTED-ID]`).

Default chunks file: `docs/quality/vds_chunks.yaml`  
Schemas: `zqk_vds_evaluate_v1`, `zqk_vds_checklist_v1`

## Vendor instruction projection (tracked debt)

Hand-editing Cursor `.mdc` (or peer IDE rule files) when kernel intent changes inverts
**POL-ARCH-AGENT-STATE-EXPORT**: the Knowledge Kernel is SSOT; vendor files are export
targets. Today `AGENT_CONTEXT_REFRESH.md` is partially generated, but `.mdc` content is
still often authored by agents.

**Tracked:** `[REDACTED-ID]` — kernel→vendor instruction projection
(Cursor `.mdc` and peers). Until that lands, treat `.mdc` updates as temporary mirrors of
`POL-WORKFLOW-VDS` / VDS glossary / this contract, not as the authoring plane.

## Init checklist (any project)

1. Ensure `POL-WORKFLOW-VDS` (or equivalent) is active in the project kernel.  
2. Copy spine profile + customization template into the project.  
3. Fill customization (`lint_commands`, `test_execution.mode`, etc.).  
4. Bind CAP/CVS/matrices as consumers if those tools exist.  
5. Refuse agent “done” claims that omit VDS evidence.
