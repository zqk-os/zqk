# ADR: Dynamic Public Release Gate Invariants & Launchctl Daemon Supervision

**Status**: Accepted  
**Date**: November 2026  
**Context**: Cycle 93  
**Scope**: Open-Core Release Verification & Background Host Daemon Supervision  

---

## 1. Context & Problem Statement

Public releases and continuous integration runs previously suffered from two failure modes:
1. **Static Release Pinning & Hardcoded Object IDs in Helper Scripts**: Release gate scripts historically relied on hardcoded version paths (e.g. `docs/releases/v0.3.3.md`) and transient patch scripts (e.g. `scripts/open-core/repo-resolver/patch_ingested_issues.py`) contained static kernel object ID literals (`TDE-*`, `GOAL-*`). This violated the Knowledge Kernel CAS invariant (`POL-KERNEL-CAS-INVARIANT`), requiring manual maintenance and risking link-rot or test failures on release version bumps.
2. **Daemon Host Service Supervision Drift**: When background daemons were run manually via `./bin/zqk daemon run` or per-daemon launchd templates (`com.zqk.mcp*`, `com.zqk.privileged-writer*`), conflicts arose with macOS launchctl supervision. Multiple unmanaged units accumulated in `~/Library/LaunchAgents` with status `-15`/`-9`, preventing the unified overseer from running cleanly under host init supervision.

---

## 2. Decision & Architectural Changes

1. **Dynamic Release Note Discovery & Zero Script Pinning**:
   - `scripts/open-core/test-public-release-gates.sh` dynamically discovers release notes via directory scanning (`docs/releases/v*.md`) rather than static version string pinning.
   - `scripts/open-core/repo-resolver/patch_ingested_issues.py` was refactored into a parameterized, argument-driven CLI utility (`--goal-id` and positional `ids`), eliminating all hardcoded kernel IDs.
   - Verified that zero static release versions (`v0.3.[0-9]`) or raw kernel IDs remain hardcoded in `scripts/`.

2. **Unified Solitary Overseer LaunchAgent**:
   - Consolidated all background services (`scheduler`, `ambient`, `steward`, `privileged-writer`) under the solitary `com.zqk.overseer` LaunchAgent managed by launchctl.
   - Executed legacy cleanup (`zqk daemon service cleanup-legacy`) to remove orphaned per-daemon units.
   - Removed file lock attributes (`uchg`) and verified `com.zqk.overseer` running directly under macOS launchd supervision (`launchctl list com.zqk.overseer`).

---

## 3. Verification & Proofs

- **Static Floor Invariant**: Verified via `TestLaunchctlServiceConfiguration_StaticFloorInvariant` in `pkg/daemon/overseer/overseer_test.go` (asserting valid plist arguments, working directory, and absence of legacy fragments).
- **Operational Dynamic Proof**: Verified via `TestOverseerHealthReconciliation_OperationalProof` and live inspection (`zqk daemon service status` reports `installed: true, loaded: true, running: true`).
- **Negative Adversarial Boundary**: Verified via `TestMalformedWakeQuarantine_NegativeBoundary` and `TestInvalidDaemonActionRejection` (failing closed on unknown actions or unmanaged targets).
