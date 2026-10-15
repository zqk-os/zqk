# CLI Mutation & Transaction Rollback Engine Guide

## Executive Summary

The ZQK CLI Mutation and Transaction Rollback Engines provide ACID-compliant transactional state modifications and automated state recovery across the persistent Knowledge Kernel:

- **`zqk mutate` (alias `zqk zql`)**: Evaluates declarative ZQL scripts containing multi-node object creation, updates, references, and deletions. Employs Kahn topological sorting for variable dependency resolution (`LET $x = ...`), in-memory schema and relational preflight validation, and dry-run execution (`--dry-run`). Supports audited emergency break-glass elevation (`--break-glass`).
- **`zqk rollback`**: Manages point-in-time state snapshots and reconstructive recovery. Includes snapshot listing with time/count filters (`list`), fast single-point snapshot restoration (`apply`), change-journal timestamp reconstruction for pruned cycles (`reconstruct`), and store retention pruning (`retain`).

---

## 1. Declarative ZQL Mutations (`zqk mutate`)

### Overview & Transaction Semantics
The `mutate` command executes atomic transactions against kernel object storage. A ZQL script must be wrapped in `BEGIN; ... COMMIT;` blocks:

```sql
BEGIN;
LET $p = UPSERT priority_plan {
    title: 'Authentication Hardening',
    priority_tier: 'P1',
    status: 'originated'
};
UPSERT backlog_item {
    title: 'Implement OAuth Token Refresh',
    priority_plan_ref: $p.id,
    priority_tier: 'P1',
    status: 'planned'
};
COMMIT;
```

### Command Invocations & Input Channels
Scripts can be provided directly via command-line arguments, file paths (`-f`), or piped via standard input (`-`):

```bash
# Inline ZQL string
zqk mutate "BEGIN; UPSERT backlog_item { id: 'BLI-001', status: 'in_progress' }; COMMIT;"

# Script file
zqk mutate -f scripts/cycle_init.zql

# Piped from standard input
cat script.zql | zqk mutate -f -
echo "BEGIN; DELETE priority_plan 'PRI-001'; COMMIT;" | zqk mutate -
```

### Dry-Run Preflight Validation (`--dry-run`)
Dry-run executes the entire topological resolution and schema validation in an isolated in-memory buffer without writing changes to the CAS or filesystem:

```bash
zqk mutate -f batch.zql --dry-run --format json
```

Response payload includes execution status, isolation level, and individual mutation receipts:
```json
{
  "transaction_id": "tx-1792021950000000000-a1b2c3d4",
  "isolation_level": "SERIALIZABLE",
  "dry_run": true,
  "committed": false,
  "receipts": [
    {
      "index": 0,
      "action": "CREATE_NODE",
      "target_kind": "priority_plan",
      "target_id": "PRI-1792021950000000000-e5f6g7h8",
      "status": "VALIDATED"
    }
  ]
}
```

### Break-Glass Override Protection (`--break-glass`)
System-managed provenance fields (such as `created_at`, `created_by`, `updated_at`, `updated_by`, `cas_address`, `hash`) are fail-closed protected against manual tampering. Attempting to override system fields requires explicit elevation:

```bash
# Mandatory flag and minimum 10-character justification reason
zqk mutate -f repair.zql \
  --break-glass \
  --break-glass-reason "emergency recovery of corrupted timestamps authorized by lead architect"
```

---

## 2. Transaction Rollback & State Restoration (`zqk rollback`)

### Overview & Architecture
Every high-impact lifecycle operation, batch migration, and automated maintenance job captures a rollback point in `.zqk/rollback/`. Each snapshot stores pre-mutation object states and metadata.

### Listing Rollback Points (`zqk rollback list`)
Rollback points are indexed chronologically (most recent last):

```bash
# List all captured rollback points
zqk rollback list

# Limit output to the last N points
zqk rollback list --last 5

# Filter points within a sliding time window
zqk rollback list --within 24h --format json
```

### Applying Rollback Snapshots (`zqk rollback apply`)
Restores all objects in the snapshot to their pre-mutation states in the storage provider:

```bash
# Quick path restoration using a known point ID
zqk rollback apply rbp-20261014-230000.123456789 --format json
```

### Beyond-Threshold Reconstruction (`zqk rollback reconstruct`)
If a rollback point has aged out or been pruned by retention policies, state can be deterministically reconstructed from the change journal at a given RFC3339 timestamp:

```bash
zqk rollback reconstruct \
  --scope-type priority_plan \
  --scope-id PRI-STARTER-001 \
  --timestamp 2026-10-14T12:00:00Z
```

### Store Retention & Pruning (`zqk rollback retain`)
Prunes expired rollback points exceeding configured retention thresholds while preserving recent recovery anchors:

```bash
zqk rollback retain --format json
```

---

## 3. Flag & Option Reference

### `zqk mutate` Flags

| Flag | Short | Default | Description |
| :--- | :---: | :---: | :--- |
| `--file` | `-f` | `""` | Path to ZQL file (or `-` for stdin) |
| `--dry-run` | | `false` | Preflight schema validation without persistence |
| `--format` | | `table` | Output format (`table`, `json`) |
| `--break-glass` | | `false` | Elevated override for system fields |
| `--break-glass-reason` | | `""` | Justification reason for break-glass (min 10 chars) |

### `zqk rollback` Subcommands & Flags

| Subcommand | Key Flags | Description |
| :--- | :--- | :--- |
| `list` | `--last <N>`, `--within <dur>`, `--format` | Lists recorded rollback points |
| `apply` | `<point-id>`, `--format` | Restores object states from snapshot |
| `reconstruct` | `--scope-type`, `--scope-id`, `--timestamp` | Reconstructs state from change journal |
| `retain` | `--format` | Trims expired rollback points |
