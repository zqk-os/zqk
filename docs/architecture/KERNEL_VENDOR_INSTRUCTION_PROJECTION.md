# Kernel → vendor instruction projection

**Last Verified:** 2026-08-31


**Status:** Active debt / MVP in progress  
**Tracked:** `[REDACTED-ID]`  
**Related:** `POL-ARCH-AGENT-STATE-EXPORT`, `POL-AGENT-001` (Kernel-First), `POL-WORKFLOW-VDS`

## Problem

Updating Cursor `.mdc` (or other IDE rule files) by hand when kernel intent changes makes the **vendor surface** look like the authoring plane. That violates Knowledge Kernel primacy: ephemeral agents are clients; durable rules live as kernel objects (`policy`, `glossary_term`, etc.).

## Contract

| Role | What |
|------|------|
| **SSOT** | Kernel objects (+ portable profiles such as VDS spine YAML) |
| **Export** | Vendor instruction packs (`.cursor/rules/*.mdc`, future Claude/Agy packs) |
| **Drift gate** | Regenerated content must match on-disk export, or CI/agent check fails |
| **Forbidden** | Inventing policy prose only in `.mdc` without a kernel object |

### Inputs (examples)

- `POL-WORKFLOW-VDS` (and peer policies)
- Glossary by **durable title** (e.g. `Verifiable Decomposition Spine (VDS)`), not hardcoded `GLS-*`
- Spine / customization profiles under `docs/quality/`

### Outputs

- Generated files with an explicit header: do not hand-edit body; re-run projector
- Optional fingerprint / check mode for pre-commit or agents

## MVP (first slice)

**CLI (vendor-agnostic):** `zqk workflow vds project --provider <id> --write|--check`  
(`--all`, `--list`; default provider from config)

**Config:** `docs/quality/verifiable_decomposition_customization.yaml` → `vendor_providers:`

```yaml
vendor_providers:
  default: cursor
  providers:
    - id: cursor
      format: cursor_mdc
      out: .cursor/rules/verifiable-decomposition-spine.mdc
    # - id: claude
    #   format: claude_md   # when renderer exists
    #   out: .claude/rules/vds.md
```

Do **not** add `project-cursor` / `project-claude` subcommands — only new `format` renderers in `pkg/vds` plus config rows.

**Drift gate:** `scripts/check-vds-vendor-project.sh` (`zqk workflow vds project --all --check`), wired into `scripts/pre-commit-lint.sh`. Skip: `ZQK_SKIP_VDS_VENDOR_PROJECT=1`.

Later slices: more formats, multi-policy packs, integrate with `validate-agent-rules` / context refresh.

## Partial existing pieces

- `scripts/agent-protocol-context-refresh.sh` → `AGENT_CONTEXT_REFRESH.md` (checklist, not full `.mdc` bodies)
- `zqk system validate-agent-rules` (manifest file list, not content from POL)

## Agent habit until complete

Prefer updating kernel objects + re-running the projector. If you must touch `.mdc` for non-projected rules, leave a `TRACK:` pointing at this BLI or a child item.
