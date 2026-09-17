# CEF S7 — Pre-launch polish (doc honesty)

**Last Verified:** 2026-08-31


**TRACK:** `BLI-CEF-POLISH-PRELAUNCH` / `REQ-CEF-DOC-001` / `CRIT-CEF-DOC-001A`  
**Date:** 2026-08-17

## Intent

Catch-all polish for remaining CEF doc-honesty / thin friction after S1–S6: keep the five-minute path aligned with Community first-run, and stop training agents that `--allow-degraded` is the primary recovery for a down scheduler.

## Changes

| Surface | Before | After |
|---------|--------|--------|
| `README.md` golden path | Led with `system check --fast --allow-degraded` | Scheduler start + full `system check` as health bar; degraded called out as optional partial smoke |
| `docs/onboarding/AI_AGENT_ONBOARDING.md` | “MUST pass `--allow-degraded`” when scheduler down | Prefer `scheduler start` / `recycle-stable-daemons.sh`; degraded = intentional partial only |
| `docs/onboarding/COMMUNITY_FIRST_RUN.md` | `zqk mcp serve` | `zqk mcp ensure --tcp 127.0.0.1:8443` + `feed doctor` |

## Acceptance vs CRIT-CEF-DOC-001A

- README five-minute / golden path no longer presents degraded check as kernel health.
- AI agent onboarding no longer mandates degraded bypass over starting the daemon.
- Community first-run MCP step matches the README / stable promote cycle.

Sibling `BLI-CEF-DOC-FIRSTRUN` already completed the primary first-run alignment; this BLI closes residual honesty gaps called out in CEF A/B (README degraded theater, agent “MUST --allow-degraded”).

## Related

- `docs/architecture/SCHEDULER_DEGRADED_MODE_GUARDRAILS.md`
- `docs/architecture/check-fast-mode.md`
- `docs/onboarding/COMMUNITY_FIRST_RUN.md`
