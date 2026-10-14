# CLI Explainability & Health Check Registry Guide (`zqk explain` & `zqk healthchk`)

## 1. Overview & System Mission

The ZQK Knowledge Kernel pairs deterministic, fail-closed operations with rich operator telemetry and explainability. Two dedicated command surfaces enable agents and humans to understand system nomenclature and monitor subsystem vitality:

1. **Kernel Explainability & Acronyms (`zqk explain`)**: Provides instantaneous disambiguation and definition lookup for core domain acronyms (`BLI`, `PRI`, `REQ`, `CRIT`, `TST`, `ATK`, `EPC`, `MIL`), dynamically discovers custom `glossary_term` objects registered in the kernel, provides Levenshtein-distance fuzzy suggestions on unrecognized terms, and synchronizes canonical taxonomies into kernel schemes (`--sync`).
2. **Health Check Registry (`zqk healthchk`)**: Manages and inspects pluggable health monitors (`autonomy_inbox`, `object_volume`, `scheduler_events`, `stream_volume`, `wal_backlog`), records historical execution journals into WAL change entries, and enables atomic runtime state updates (enable/disable/bulk).

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                              EXPLAINABILITY & HEALTH REGISTRY                          │
│                                                                                        │
│  Operator / Agent Query                 Knowledge Kernel Vocabulary                    │
│ ┌────────────────────────┐             ┌─────────────────────────────┐                 │
│ │ zqk explain BLI        │ ──────────► │ Scheme: VOC-KERNEL-ACRONYMS │                 │
│ └────────────────────────┘             │ Term: Backlog Item (BLI)    │                 │
│                                        └─────────────────────────────┘                 │
│                                                                                        │
│  Subsystem Telemetry                    Pluggable Health Registry                      │
│ ┌────────────────────────┐             ┌─────────────────────────────┐                 │
│ │ zqk healthchk run      │ ──────────► │ - autonomy_inbox            │                 │
│ └────────────────────────┘             │ - object_volume             │                 │
│                                        │ - scheduler_events          │                 │
│                                        │ - wal_backlog               │                 │
│                                        └─────────────────────────────┘                 │
└────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Kernel Acronyms & Explainability (`zqk explain`)

### 2.1 Basic Usage & Discovery

```bash
# List all registered acronyms in a clean, aligned console table
zqk explain

# Explain a specific kernel term
zqk explain BLI

# Output definition in machine-readable JSON
zqk explain BLI -f json

# Sync canonical acronyms into kernel storage scheme VOC-KERNEL-ACRONYMS
zqk explain --sync

# Query against a specific vocabulary scheme
zqk explain --scheme VOC-CUSTOM-TERMINOLOGY
```

### 2.2 Term Resolution Hierarchy

When an operator queries a term (e.g. `zqk explain <CODE>`):
1. **Kernel Storage Lookup**: Inspects active storage for `glossary_term` objects matching the code under the target scheme (`VOC-KERNEL-ACRONYMS`). Any term minted dynamically in storage is immediately queryable.
2. **Built-in Fallback**: If storage is unavailable or term is not yet synced, falls back to compiled-in static definitions.
3. **Fuzzy Did-You-Mean Hints**: If the term is unknown, calculates closest Levenshtein distance matches and emits a structured hint:
   ```text
   Error: unknown kernel acronym "BL".
   Did you mean: BLI?
   Run 'zqk explain' without arguments to see all registered acronyms
   ```

### 2.3 Output Formatting

Single item explain outputs all key facets:
```text
ACRONYM    : BLI
FULL NAME  : Backlog Item
CATEGORY   : work
DEFINITION : A discrete, tracked work package or deliverable scheduled within a Priority Plan.
CONTEXT    : operational
RELATED    : PRI, REQ, CRIT, ATK
```

---

## 3. Pluggable Health Check Registry (`zqk healthchk`)

### 3.1 Architecture & Monitor Suite

Health checks are modular `Monitor` interfaces registered in `healthcheck.DefaultRegistry`:
- `autonomy_inbox`: Audits message queues and dispatch backlogs for unattended tasks.
- `object_volume`: Monitors CAS object counts and verifies storage capacity envelopes.
- `scheduler_events`: Evaluates scheduler loop health, heartbeat freshness, and dead-letter count.
- `stream_volume`: Tracks real-time event stream throughput and ingestion lag.
- `wal_backlog`: Ensures WAL journals are draining cleanly without uncommitted transaction leaks.

### 3.2 Command Surface

#### List Registered Monitors
```bash
zqk healthchk list
```
Outputs table of monitor ID, human-readable name, and enabled state (`true`/`false`).

#### Enable or Disable Monitors
```bash
# Disable a monitor
zqk healthchk update object_volume --enabled=false

# Re-enable monitor
zqk healthchk update object_volume --enabled=true
```

#### Execute Monitors
```bash
# Run all enabled monitors
zqk healthchk run

# Run a specific monitor
zqk healthchk run wal_backlog

# Run in JSON format
zqk healthchk run wal_backlog --format json
```

Single-monitor runs automatically record an audit entry via `storage.CreateHealthCheckChangeJournalEntry`, ensuring long-term health telemetry is preserved.

#### Bulk Disable
```bash
# Disable via comma-separated IDs
zqk healthchk bulk disable --ids "object_volume,stream_volume"

# Disable via configuration file (YAML list or newline-separated IDs)
zqk healthchk bulk disable --ids-file disabled_monitors.yaml
```

---

## 4. Verification & Testing Standards

All code and test suites supporting `cmd/zqk/explain` and `cmd/zqk/healthchk` adhere to:
- **Statement Test Coverage Floor**: Must meet or exceed **>=80.0%** statement coverage across both packages (`go test -v -cover ./cmd/zqk/explain ./cmd/zqk/healthchk`). Current verification:
  - `cmd/zqk/explain`: **90.3%**
  - `cmd/zqk/healthchk`: **90.6%**
- **Test Isolation**: All tests construct isolated project fixtures via `testkit.PrepareIsolatedTempProject` to prevent repository mutation and ensure reproducible test results.
- **Fail-Closed Verification**: Validates negative boundaries, fuzzy suggestions, invalid monitor update handling, and error routing when required flags are omitted.
