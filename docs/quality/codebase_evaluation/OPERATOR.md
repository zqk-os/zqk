# Operator quickstart (one run)

1. Materialize the full package skeleton (preferred):  
   `sh ./scripts/cef/materialize-package.sh <output_home> <AGENT_ID> <freeze_sha>`  
   Stub markers (`REPLACE_ME` / `UNGRADED` / `F-STUB-000`) must be overwritten before a stage is done.  
   Legacy: copy only `templates/run_scope.example.yaml` → `<output_home>/run_scope.yaml`.
2. Point agents at CEF root (`CONSTITUTION.md` sibling). Output artifacts go under `output_home`, not into CEF source.
3. Prefer bite-sized execution via kernel pipeline `PIP-CEF-DIAMOND-REMEASURE-001` (one ATK per stage; specialist ≠ adversarial). Waves in [`WAVE_PLAN.md`](./WAVE_PLAN.md) remain the conceptual order.
4. Each analysis lens: specialist → different-seat adversarial → resolutions JSONL.
5. Integrator (`prompts/L-INTEGRATOR/specialist.md`) produces scorecard + handoff package per [`HANDOFF_SCHEMA.md`](./HANDOFF_SCHEMA.md).
6. Downstream process engineers objectify from handoff — CEF agents do not mint tracker objects. Scores between remesures: `zqk matrix report --name cef_diamond_scorecard` + `CVS-CEF-UNTIL-45-001`.

**Success:** `scorecard.json` with eight required axes + consolidated `findings.jsonl` with no E0 and adversarial resolution on every accepted finding.
