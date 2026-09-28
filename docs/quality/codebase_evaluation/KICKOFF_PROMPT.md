# CEF kickoff prompt (copy-paste)

**cef_version:** 0.1.0  
Use this verbatim after a reboot. Change only the **FILL-INS** (agent id + output home). Keep everything else identical when running A/B comparisons.

---

## Prompt (reuse)

```text
You are executing the Codebase Evaluation Framework (CEF) v0.1.0 as a SOLO OPERATOR on this repository.

## Identity (FILL-IN — only field that may differ between A/B runs)
- agent_run_id: AGENT_A   # or AGENT_B
- output_home: docs/quality/cef-runs/2026-08-13-AGENT_A
  # AGENT_B must use a different directory, e.g. docs/quality/cef-runs/2026-08-13-AGENT_B

## Mandatory first actions
1. Read, in order:
   - docs/quality/codebase_evaluation/CONSTITUTION.md
   - docs/quality/codebase_evaluation/DIAMOND_SCALE.md
   - docs/quality/codebase_evaluation/OPERATOR.md
   - docs/quality/codebase_evaluation/WAVE_PLAN.md
   - docs/quality/codebase_evaluation/DIAGRAM_CONTRACT.md
   - docs/quality/codebase_evaluation/HANDOFF_SCHEMA.md
   - docs/quality/codebase_evaluation/prompts/_SHARED_PREAMBLE.md
2. Create output_home. Copy
   docs/quality/codebase_evaluation/templates/run_scope.example.yaml
   → <output_home>/run_scope.yaml
3. Freeze run_scope.yaml EXACTLY as follows (do not widen/narrow without writing a waive in run_log.md):

cef_version: "0.1.0"
mode: truth_map
repo_root: "."
include_globs:
  - "**/*"
exclude_globs:
  - ".git/**"
  - "vendor/**"
  - "node_modules/**"
  - "dist/**"
  - "build/**"
  - "**/*.min.js"
  - "**/*.pb.go"
  - "docs/quality/cef-runs/**"
  - "docs/quality/codebase_evaluation/**"
languages_detected: []
adapters_enabled: ["go"]
top_n_default: 10
output_home: "<your output_home from Identity>"
notes: "A/B CEF compare run; truth map only; no launch triage."

## Mission
Produce a cold truth-map package for this codebase using ALL waves in WAVE_PLAN.md through Wave 4 (Integrator).

## Hard rules
- Do NOT modify application source code.
- Do NOT mint process/backlog/tracker objects.
- Do NOT bias toward product launch / open-core / near-term roadmaps.
- Prefer existing tooling; record missing tools in tooling_gaps.md.
- Tests only if absolutely necessary: narrow package + named test + timeout ≤60s; never full suites.
- Evidence grades E0–E3; accepted handoff findings must be E3 (or E2 with explicit waive in run_log.md).
- Density: D-HIGH exhaustive within scope; D-LOW Top N=10; D-MED per rubric.
- Every analysis lens: specialist pass THEN a separate adversarial pass attacking the specialist artifact (you may play both seats, but keep distinct artifacts).
- Write ALL artifacts under output_home only — never edit CEF framework files under docs/quality/codebase_evaluation/.

## Required package layout under output_home
run_scope.yaml
preflight.json
tooling_gaps.md
run_log.md
diagrams/**
findings/<lens_id>.jsonl
adversarial/<lens_id>.jsonl
adversarial_resolutions.jsonl
scorecard.json
findings.jsonl
handoff_manifest.json
EXECUTIVE_NARRATIVE.md   # ≤200 lines, cold

## Wave order (do not skip)
Wave 0 L-PREFLIGHT
Wave 1 L-ARCHITECTURE, L-SECURITY, L-SUPPLY-RELEASE
Wave 2 L-RELIABILITY, L-OBSERVABILITY, L-CONCURRENCY, L-PERFORMANCE
Wave 3 L-CODE-QUALITY, L-TESTING, L-USABILITY, L-DOCS-MODEL
Wave 4 Integrator (prompts/L-INTEGRATOR/specialist.md)

For each lens use:
- rubrics/<lens>.md
- prompts/<lens>/specialist.md
- prompts/<lens>/adversarial.md
Findings must validate against schemas/finding.schema.json.
Scorecard against schemas/scorecard.schema.json.
Diagrams against DIAGRAM_CONTRACT.md + schemas/diagram.schema.json.

## Done criteria
1. scorecard.json has all eight required axes (RDB MNT TST REL OBS RCV SEC ROB) with grade/confidence/drivers.
2. findings.jsonl has no E0; every accepted finding has adversarial resolution.
3. run_log.md lists commands run, timeouts, skips, and any waives.
4. Reply with: path to output_home, axis grade vector, count of findings by severity, and top 10 standing findings (id + title).

Begin with Wave 0 now.
```

---

## A/B compare protocol

1. Paste the prompt into Agent A with `AGENT_A` / its `output_home`.
2. After reboot (or in a fresh chat), paste the **same** prompt into Agent B with only Identity changed.
3. Do not share Agent A’s findings with Agent B before B finishes.
4. Compare: axis grades, finding overlap by theme/path, severity deltas, and evidence-grade honesty.

## Optional: freeze git SHA for fairness

Before either run, note `git rev-parse HEAD` in both `run_scope.yaml` notes (or `run_log.md`). If the tree moves between A and B, the comparison is contaminated.
