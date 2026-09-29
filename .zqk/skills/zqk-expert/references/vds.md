# Verifiable Decomposition Spine (VDS) — skill reference

**Policy:** `POL-WORKFLOW-VDS`  
**Glossary (titles, not ids for portability):** “Verifiable Decomposition Spine (VDS)”, acronym “VDS”  
**CLI:** `zqk workflow vds` (`checklist`, `evaluate`, `project`, `init`)

## What “done” means

A stage claim is done only when each in-scope chunk has:

1. `chunk_id` + `stage`
2. `claim` + `rubric_ref`
3. Non-empty `dsl_checks` that pass under `evaluate`
4. `evidence_refs` another agent can re-check
5. `independent_verify: yes` (or human `waiver_ref`) — use `evaluate --apply-verify` after PASS

## Files (this repo)

| Path | Role |
|------|------|
| `docs/quality/verifiable_decomposition_spine_profile.yaml` | Portable spine |
| `docs/quality/verifiable_decomposition_customization.yaml` | Project pins + `vendor_providers` |
| `docs/quality/vds_chunks.yaml` | Working chunks |
| `docs/architecture/VERIFIABLE_DECOMPOSITION_SPINE.md` | Contract + DSL catalog |
| `.cursor/rules/verifiable-decomposition-spine.mdc` | Projected Cursor surface (export) |

## Common predicates

- `object_exists:{id}`
- `field_nonempty:{id}:{field}`
- `path_exists:{relpath}`
- Test/lint modes follow customization (`test_execution.mode`, often `scheduler`)

## Swarm habit

```bash
zqk workflow vds evaluate --format json
zqk workflow vds project --all --check
# on PASS:
zqk workflow vds evaluate --apply-verify
```

`--apply-verify` surgically rewrites `independent_verify` lines (preserves comments). Full YAML rewrite is fallback only.
