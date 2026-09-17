# Env privilege override inventory (`ZQK_ALLOW_*` / `BYPASS_*`)

**Last Verified:** 2026-08-31


**Policy / decision:** `GLS-ENV-BREAKGLASS-ANTIPATTERN-001`, `DEC-ENV-BREAKGLASS-TO-SIGNED-LOGIN-001`  
**Tech-debt CVS:** `CVS-REDACTED`  
**BLI:** `BLI-ENV-BREAKGLASS-INVENTORY-001` → removal `BLI-ENV-BREAKGLASS-REMOVE-001`  
**Target:** signed per-ACC login — `BLI-SIGNED-ACC-LOGIN-NO-BLEED-001`  
**Snapshot date:** 2026-08-11 (repo scan of `pkg/zqkenv` + call sites)

Env flips that neuter security/policy gates are **privilege-bleeding holes**. They must be inventoried, then removed — not replaced with seat-string snowflakes.

## Removed (do not reintroduce)

| Accessor | Removed |
|----------|---------|
| `AllowDraftSweep` / `ZQK_ALLOW_DRAFT_SWEEP` | 2026-08-11 — draft sweep RBAC-only |
| `BypassVerificationOutcomeAuthority` / `BYPASS_VERIFICATION_OUTCOME_AUTHORITY` | 2026-08-11 — removed bypass in storage; use system ACC |
| `BypassSkillVerification` / `BYPASS_SKILL_VERIFICATION` | 2026-08-11 — removed bypass in skills |
| `AllowCIOverrides` / `ALLOW_CI_OVERRIDES` | 2026-08-11 — completely blocked manual overrides in CI |
| `AllowCIOverridesAdmin` | 2026-08-11 — removed subprocess auto-inject |
| `AllowForegroundGoTest` / `ALLOW_FOREGROUND_GO_TEST` | 2026-08-11 — removed env opt-in; Make/CI/scheduler detection remains |
| `BypassAgentGuard` / `BYPASS_AGENT_GUARD` | 2026-08-11 — removed alias |

## Live privilege overrides (`pkg/zqkenv`)

| Accessor | Env suffix | Gate neutered | Production call sites | Test-only call sites | Blast | Removal notes |
|----------|------------|---------------|----------------------|----------------------|-------|---------------|
| (None)   |            |               |                      |                      |       |               |

## Ranked removal order (for `BLI-ENV-BREAKGLASS-REMOVE-001`)

1. **`BypassVerificationOutcomeAuthority`** — storage integrity / outcome authority.
2. **`BypassSkillVerification`** — skill trust boundary.
3. **`AllowCIOverrides` / `AllowCIOverridesAdmin`** (+ stop `subprocess_environ` auto-`=1`).
4. **`AllowForegroundGoTest` / `BypassAgentGuard`** — keep Make/CI/scheduler detection; remove env opt-in.

## Replacement contract (not optional)

Privileged actions require **signed per-individual ACC login** (issued key / challenge proof bound to `ACC-*`). No ambient `SystemSecurityContext` elevation for unbound shells. See `DEC-ENV-BREAKGLASS-TO-SIGNED-LOGIN-001` and `BLI-SIGNED-ACC-LOGIN-NO-BLEED-001`.

## How to refresh this inventory

```bash
rg -n 'func (Allow|Bypass)[A-Za-z]+\(' pkg/zqkenv/vars.go
rg -n 'zqkenv\.(Allow|Bypass)[A-Za-z]+\(\)' --glob '*.go'
```
