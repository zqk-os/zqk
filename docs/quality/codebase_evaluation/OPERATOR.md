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

### Step 1: Initialize Evaluation Package (Native Swarm or Manual Skeleton)

You can orchestrate the evaluation either natively through the ZQK Knowledge Kernel swarm engine, or manually via the template skeleton:

#### Option A: Native Kernel Swarm Dispatch (Recommended)
The framework includes a fully declared swarm package under [`packs/code-eval/`](../../packs/code-eval/swarm.yaml) with 12 pipeline tasks and pre-rendered prompt templates across all lenses:

```bash
# Validate the evaluation swarm package and inspect task topology
zqk swarm run packs/code-eval --dry-run

# Stage the evaluation pipeline and prompt templates into the kernel without launching
zqk swarm run packs/code-eval --stage-only

# Or launch the automated multi-agent swarm in background
zqk swarm run packs/code-eval -y
```

#### Option B: Manual Directory Skeleton & Prompt Dispatch
To run an evaluation with an external or solo AI operator without orchestrating through the background swarm engine:

```bash
# Copy template skeleton to target evaluation run directory
mkdir -p <output_home>
cp -R docs/quality/codebase_evaluation/templates/package_skeleton/* <output_home>/
cp docs/quality/codebase_evaluation/templates/run_scope.example.yaml <output_home>/run_scope.yaml
```

Feed [`KICKOFF_PROMPT.md`](./KICKOFF_PROMPT.md) to initialize the operator session, and dispatch individual prompts from [`prompts/`](./prompts/) for each wave.

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
