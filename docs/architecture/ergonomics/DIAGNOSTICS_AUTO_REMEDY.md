# Diagnostics Auto-Remedy & Precondition Self-Healing Engine

## Overview
The **Diagnostics Auto-Remedy Engine** provides deterministic, non-destructive, and automated recovery recipes for developer workspace anomalies, stale synchronization locks, and orphaned filesystem artifacts in `zqk`.

It bridges system health checks (`zqk system check`) and ambient program management discovery (`zqk workflow whats-next`) directly to executable recovery paths, eliminating manual triage loops for recurring environment blockers.

---

## Architectural Principles

1. **Deterministic Diagnosis**: Every diagnosed anomaly maps to an unambiguous, typed `RemedyPlan` with a confidence score, human description, and executable action.
2. **Safe Auto-Execution Boundary**:
   - Automated remediation (`--auto-remedy`) only operates on verified safe targets within the `.zqk` project boundary (e.g. stale `.lock` files older than expiration thresholds, abandoned temporary files in `.zqk/tmp/`).
   - Invasive changes or mutations to business objects require explicit human/agent confirmation.
3. **Dual-Plane Availability**:
   - `zqk system check --auto-remedy`: Full diagnostics scan with automated remediation output.
   - `zqk workflow whats-next`: Ambient display of active remedy recipes, with `--auto-remedy` support on the zero-cost hot path.

---

## Component Layout

```
pkg/system/
├── auto_remedy.go        # DiagnosticsRemedyEngine, RemedyPlan, RemedyOutcome
└── auto_remedy_test.go   # Unit test suite verifying stale lock, temp reap, and seeding hooks

cmd/zqk/system/
├── check.go              # CLI flag: --auto-remedy
└── check_impl_output.go  # Rendering engine integrating remedy summaries and auto-fix reports

cmd/zqk/workflow/
└── whats_next.go         # Ambient whats-next enrichment and CLI flag --auto-remedy
```

---

## Supported Remedy Actions

| Issue Type | Action Type | Default Resolution | Auto-Apply Safe |
|------------|-------------|--------------------|-----------------|
| Stale Scheduler/Session Lock | `ActionRemoveFile` | Remove file matching `.zqk/**/*.lock` exceeding age threshold | Yes |
| Orphaned Temp File | `ActionRemoveFile` | Remove abandoned `.zqk/tmp/*.tmp` | Yes |
| Unseeded Default Policy Pack | `ActionSeed` | Invoke registered `PolicySeeder` (`zqk system init --force`) | Safe in greenfield |
| Unseeded Agent Seating Pack | `ActionSeed` | Invoke registered `PersonaSeeder` (`zqk system agent-onboard`) | Safe in greenfield |
| Object Validation Issue | `ActionRunCommand` | Prescriptive `zqk object fix <id>` | Manual/Confirmed |

---

## Verification
- Unit test suite: `go test -v ./pkg/system/...`
- Integration verification:
  ```bash
  zqk workflow whats-next --auto-remedy
  zqk system check --auto-remedy --fast
  ```
