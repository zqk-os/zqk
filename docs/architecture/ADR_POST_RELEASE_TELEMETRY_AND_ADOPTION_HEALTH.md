# ADR: Post-Release Telemetry & Community Adoption Health Monitoring Protocol

## Context
Following the public release of `v0.3.4` (incorporating zero-vendor decoupling, test matrix coverage aggregation without redundancy, and hardened kernel execution), the system requires structured health monitoring. Community adoption health encompasses binary artifact distribution verification, telemetry opt-in ingestion metrics, and crash/error reporting signals.

## Decision
1. **Telemetry Pipeline & Opt-Out Invariant**:
   - The telemetry collector (`pkg/telemetry`) remains strictly non-identifying and respects user opt-out configurations (`ZQK_TELEMETRY_DISABLED=1`).
   - Memory buffers and event aggregators operate under strict bounds (5000 max events, memory cap enforcement) to prevent any resource leaks during long-lived daemon sessions.
2. **Release Distribution Verification**:
   - The distribution endpoints (GitHub releases, documentation portal) must be verified for asset checksum validity and download accessibility.
3. **Adoption Metrics Rollup**:
   - Feedback from community first-run workflows and issue tracker traffic is aggregated into Knowledge Kernel technical debt and backlog triage cycles.

## Status
Accepted and codified in Knowledge Kernel for Cycle 85 (Post-Release Telemetry & Community Adoption Health Monitoring).
