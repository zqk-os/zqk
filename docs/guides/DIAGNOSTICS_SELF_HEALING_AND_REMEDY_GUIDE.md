# Developer & Agent Guide: Knowledge Kernel Diagnostics, Auto-Remedies & Self-Healing

## 1. Executive Summary

In high-concurrency environments with multiple autonomous processes and agents mutating the repository simultaneously, transient failures can occur:
- Abandoned lock files caused by abruptly killed processes.
- Orphaned temporary files from interrupted file writes.
- Content-Addressed Storage (CAS) hash mismatches or quarantine overflows.
- Write-Ahead Log (WAL) journal fragmentation.

The **ZQK Diagnostics & Self-Healing Engine** provides continuous, multi-layered telemetry and deterministic auto-remedy recipes to detect, isolate, and repair system anomalies without manual human intervention.

---

## 2. The 4-Layer Inspection Model (`zqk system check`)

Running `zqk system check` performs a 4-layer holistic audit of the repository state:

```
┌────────────────────────────────────────────────────────┐
│ Layer 0: CAS Structural Integrity & Invariant Blockers │
├────────────────────────────────────────────────────────┤
│ Layer 1: Entities Needing Fixes (Inside Membrane)      │
├────────────────────────────────────────────────────────┤
│ Layer 2: Entities Ready for Execution / Promotion      │
├────────────────────────────────────────────────────────┤
│ Layer 3: Completed & Validated Entities                │
├────────────────────────────────────────────────────────┤
│ Layer 4: I/O Resource Hygiene & Storage Telemetry      │
└────────────────────────────────────────────────────────┘
```

### Layer Telemetry Breakdown

- **Layer 0 (Integrity Blockers)**: Checks for corrupted YAML objects, malformed frontmatter, and broken cryptographic CAS digests.
- **Layer 1 (Schema & Lifecycle Deficiencies)**: Flags entities with missing required attributes, unbound criteria, or severed lineage.
- **Layer 2 (Ready Pipeline)**: Counts objects currently actionable by agents (`planned` items ready to claim, `in_progress` ready to promote).
- **Layer 3 (Historical Done Pipeline)**: Audits completed artifacts, archived items, and satisfied QA criteria.
- **Layer 4 (I/O & Storage Telemetry)**: Monitors open file descriptors, storage volume footprint, stale `.lock` files, and orphaned `.tmp` staging buffers.

---

## 3. Deterministic Auto-Remedies (`--auto-remedy`)

When anomalies are detected in Layer 4 (e.g., stale locks or orphaned temp files), the system outputs deterministic remedy recipes. Instead of manual removal, invoke:

```bash
zqk system check --auto-remedy
```

### How Auto-Remedy Works

1. **Deadlock Detection**: Examines file modification timestamps of locks in `.zqk/lock/`, `.zqk/scheduler/locks/`, and `.zqk/scheduler/state/locks/`.
2. **Process Liveness Verification**: Checks whether the PID that created the lock is still alive. If the owning process has exited, the lock is classified as abandoned.
3. **Atomic Purge**: Removes stale lock files and sweeps orphaned `.tmp` staging files.
4. **Receipt Generation**: Outputs a verification receipt detailing every removed artifact.

---

## 4. Deep System Diagnostics (`zqk system check --details` & `zqk feed doctor`)

For comprehensive environment verification across compiler versions, git configurations, and daemon sockets:

```bash
zqk system check --details
zqk feed doctor
```

### Diagnostic Check Categories
- **Toolchain & Storage Health**: Verifies CAS integrity, draft plane objects, file descriptors, and storage volume footprint.
- **Daemon Telemetry**: Tests connectivity to the background scheduler daemon (`zqk scheduler health-check`).
- **Storage Backend**: Confirms file-based storage permissions and Content-Addressable Storage (CAS) digest verification.
- **Git Hook Integrity**: Verifies that `pre-commit` and `pre-push` check-valves are executable and properly linked to `zqk-vet`.

---

## 5. Recovering from CAS Corruptions & Quarantine

If an unexpected power loss or interrupted disk write results in a CAS hash mismatch:

```bash
# 1. Detect corrupted objects and integrity blockers
zqk system check

# 2. Inspect quarantined objects
zqk system quarantine-report

# 3. Repair CAS filename and content hash mismatches
zqk system repair-cas-corruption
```

The repair engine re-parses the YAML payload, validates attributes against the schema registry, re-stamps a valid SHA-256 CAS hash, and re-admits the object into the authoritative storage plane.
