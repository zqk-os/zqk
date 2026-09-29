# Solo Operator Runbook & Execution Checklist

> **Purpose:** Operational execution runbook for running single-operator and swarm-assisted Codebase Evaluation Framework passes, from workspace materialization through multi-wave analysis and final handoff.

| Specification Metadata | Value |
| :--- | :--- |
| **Framework Version** | CEF v0.1.0 |
| **Governance Tier** | Authoritative Core (Operational Runbook) |
| **Target Roles** | Solo Operators, Evaluation Orchestrators |
| **Pipeline Binding** | `PIP-CEF-DIAMOND-REMEASURE-001` |

---

## 1. Step-by-Step Operator Workflow

### Step 1: Materialize Output Package Skeleton

Materialize the complete evaluation package directory before launching analysis agents:

```bash
# Materialize package skeleton with agent identity and freeze commit SHA
sh ./scripts/cef/materialize-package.sh <output_home> <AGENT_ID> <freeze_sha>
```

> [!NOTE]
> All scaffolded stub markers (`REPLACE_ME`, `UNGRADED`, `F-STUB-000`) must be fully populated and validated before each wave is marked complete.

---

### Step 2: Configure Evaluation Scope & Pointers

Point analysis agents at the CEF framework root directory. All generated artifacts must be written to `<output_home>`, preserving CEF specification sources without modification.

---

### Step 3: Phased Execution & Wave Dispatch

Execute evaluation waves strictly in accordance with [`WAVE_PLAN.md`](./WAVE_PLAN.md):

- Run stages using discrete agent tasks (preferring pipeline `PIP-CEF-DIAMOND-REMEASURE-001`).
- Enforce role separation: lens specialist agents must not act as adversarial auditors for the same lens.

---

### Step 4: Lens Specialist & Adversarial Passes

For each active evaluation lens:

1. **Specialist Evaluation:** Dispatch specialist agent with corresponding lens prompt from `prompts/L-<NAME>/specialist.md`.
2. **Adversarial Audit:** Dispatch independent auditor with `prompts/L-<NAME>/adversarial.md` to review the specialist artifact.
3. **Resolution Emission:** Record auditor findings and resolutions in `adversarial_resolutions.jsonl`.

---

### Step 5: Lead Integrator Synthesis & Scorecard

Dispatch the lead integrator (`prompts/L-INTEGRATOR/specialist.md`) to:

- Arbitrate adversarial resolutions and deduplicate findings.
- Assign 1–5 Diamond Scale scores and confidence ratings to each axis.
- Assemble the final handoff manifest conforming to [`HANDOFF_SCHEMA.md`](./HANDOFF_SCHEMA.md).

---

### Step 6: Downstream Ingestion & Remeasurement

Downstream process engineers ingest the handoff package to objectify actionable work items. For ongoing evaluation tracking between cycles:

```bash
# Generate Diamond Scale matrix report
zqk matrix report --name cef_diamond_scorecard
```

---

## 2. Operational Success Criteria

An evaluation run is successfully completed when the following gates are satisfied:

| Success Invariant | Verification Standard |
| :--- | :--- |
| **Complete Scorecard** | `scorecard.json` contains validated scores and confidence ratings across all eight required Diamond Scale axes. |
| **Evidence Purity** | Zero `E0` (opinion-based) findings in `findings.jsonl`. |
| **Adversarial Verification** | Every accepted finding has a recorded adversarial resolution in `adversarial_resolutions.jsonl`. |
| **Architectural Anchoring** | Required C4 and sequence diagrams are referentially anchored to physical source symbols per [`DIAGRAM_CONTRACT.md`](./DIAGRAM_CONTRACT.md). |
| **Handoff Manifest** | `handoff_manifest.json` is fully populated with repository fingerprint, accepted findings, and suggested remediation structure. |
