# Declarative product launch as a matrix-shaped pipeline

**Last Verified:** 2026-08-31


**Status:** Design note (pre-final URLs)  
**Purpose:** Keep **branding and public surfaces flexible** while still operating launch like the **vetting matrix loop**: every transition is **measurable**, continuation **routed** by outcomes, and execution bounded to a **finite stage set**.

**Related:** [data-pipeline-lifecycle.md](./data-pipeline-lifecycle.md), [MEASUREMENT_OUTCOME_TAXONOMY.md](./MEASUREMENT_OUTCOME_TAXONOMY.md), [MATRIX_CLI_STRATEGY.md](./MATRIX_CLI_STRATEGY.md), `docs/quality/vetting_matrix_profile.yaml`, `docs/quality/matrix_registry.yaml`, [alpha-launch-parallel-tracks.md](../marketing/alpha-launch-parallel-tracks.md)

---

## Why this exists

Public **URLs**, **forum products**, and **utility entrypoints** may not be final until late in alpha. Hard-coding them in prose, code, or runbooks creates churn. We still want **repeatable** launch behavior: the same structure as **infrastructure-as-code**—inputs declared once, **apply** produces concrete outputs—without pretending the DNS or vendor choice is fixed on day one.

---

## Terraform analogy (conceptual, not a required toolchain)

| IaC idea | Launch / GTM analogue |
|----------|----------------------|
| **Variables** | Placeholders for forum base URL, status page, docs site, support email, social handles. **No canonical public URL is authoritative until promoted to an output.** |
| **Module** | One **launch module**: ordered stages, shared locals (product name, legal entity, support tier). |
| **Plan / Apply** | **Plan** = dry-run checklist + missing inputs; **Apply** = stamp outputs (e.g. “primary discussion URL = X”) into a small **state file** or object, not scattered README edits. |
| **Outputs** | After apply: exported **named outputs** (e.g. `primary_forum_url`, `issue_template_path`) consumed by docs generators or onboarding—**single source of truth**. |

Implementation can be **Terraform**, **OpenTofu**, a **YAML profile + script**, or a **convergence_session** field set—what matters is the **contract**: inputs → stages → measurable results → emitted outputs.

---

## Alignment with the matrix vetting loop

The **codebase vetting matrix** (and siblings in `docs/quality/matrix_registry.yaml`) already encodes:

- A **profile** (`vetting_matrix_profile.yaml`): column set, **gate columns**, **done_values**, **column_semantics** (identity, status_gate, session_ref, evidence).
- **Rows**: discrete units of work with **measurable** gate cells.
- **Completion**: row migrates when gates satisfy the profile—**not** when someone “feels done.”
- Optional **`cvs_id`**: ties rows to a **`convergence_session`** for traceability.

**Launch GTM** can use the **same shape**:

| Matrix concept | Launch concept |
|----------------|----------------|
| Profile YAML | Launch profile: stage names, required evidence fields, allowed “done” values per gate |
| CSV row | One launch work item (e.g. “enable public forum,” “publish utility story U3”) |
| Gate column | Measurable condition (e.g. `moderation_policy_published=yes`, `pinned_topics_set=yes`) |
| `cvs_id` / session | Optional **convergence_session** tracking the initiative (“alpha GTM”) |

Scripts that already understand **profile + registry** (`zqk matrix report`, vetting generators) are the **native** path to avoid one-off spreadsheets; see [MATRIX_CLI_STRATEGY.md](./MATRIX_CLI_STRATEGY.md).

---

## Finite stages and routing (pipeline contract)

Treat launch execution as a **pipeline** with the same discipline as `pkg/pipeline` and [data-pipeline-lifecycle.md](./data-pipeline-lifecycle.md):

- **Stages are finite and named** (you may use a subset: e.g. INGEST → DECIDE → COMMIT → FINALIZE for a lightweight GTM flow).
- Each stage produces an **Outcome**; **no silent partial success**—align with [MEASUREMENT_OUTCOME_TAXONOMY.md](./MEASUREMENT_OUTCOME_TAXONOMY.md) (`measurement_yields_convergence` | `divergence` | `halt_or_error` | `ambiguous_outcome`).
- **Routing** = which stage runs next given the outcome:
  - **Convergence** → advance to next stage or emit **output** (e.g. register forum URL).
  - **Divergence** → remediation branch (fix content, fix permissions) before retry.
  - **Halt/error** → stop the automated path; ticket or human gate.
  - **Ambiguous** → explicit human decision recorded; do not auto-promote URLs.

This is the same **“every condition has a measurable outcome and defines paths for continuation”** property as a **vetting loop**: the **condition** is the gate; the **measurement** is the rubric + evidence; the **path** is the router.

---

## Practical split: what stays stable vs what swaps

| Stable (slow to change) | Flexible (swap per environment / phase) |
|-------------------------|------------------------------------------|
| Stage names and outcome taxonomy | Concrete URLs and vendor choice |
| Rubric for “forum ready” | Which forum product |
| Requirement for audit / CoC | Where the CoC is hosted |
| Matrix profile schema version | Row contents and completion dates |

---

## Next steps (non-blocking)

1. Add a **`launch_profile.yaml`** (or extend an existing registry) when you want automation to **validate** “forum checklist complete” without editing markdown tables by hand.
2. Keep [alpha-launch-parallel-tracks.md](../marketing/alpha-launch-parallel-tracks.md) as the **human-readable** scenario list; derive or link **matrix rows** from it when ready.
3. When URLs freeze, **emit outputs** once and point branding docs at those outputs instead of duplicating strings.
