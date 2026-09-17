# System-check maladaptation remediation map

Tracks recurring `zqk system check` clusters for **BLI-REDACTED**.

## Reproduce clusters

```bash
./bin/zqk system check all --fast --format json --timeout 3600s \
  -o .zqk/logs/health/system-check-$(date +%Y%m%d-%H%M%S).json --allow-degraded
# or summarize an existing log/JSON:
./scripts/summarize-system-check-clusters.py .zqk/logs/health/system-check-*.log
```

## Anti-pattern → remediation

| Cluster | Symptom | Action |
|--------|---------|--------|
| **orphan_integrity** | YAML under `.zqk/process/` not in object ID / DB index (ATK/TDE churn, snap-remedy leftovers) | Prefer `zqk system check --refresh-cache` / `--clean-cache`; if still orphaned, `zqk object bulk delete --file ids.yaml --unlink-references` (CLI only — never hand-delete CAS YAML) |
| **qa_success_error** | `qa_success` with illegal `status=error` (lifecycle allows `success`/`failure`/`archived` only) | Cannot promote/update from invalid source status — **bulk delete** illegal rows (`object bulk delete --file … --unlink-references`) or recreate with a valid status |
| **dangling_account** | Refs to `account:agent:swarm-worker` / `account:agent:default` / `account:swarm_worker` | Retarget to **`ACC-*`** (e.g. swarm worker `ACC-1785920548450214011-dabd3692` via `scripts/acc-migrate-map.json` / `authcred.CanonicalAccountID`). Do not invent `account:agent:*` or keep `account:username` as primary ids (POL-AGENT-ACCOUNT-LOGIN-001). |
| **lifecycle_status** | Other kinds with statuses outside their lifecycle | `promote`/`demote` to a valid status, or archive/delete if terminal junk |

## Policy interrupt surface (QA auditor)

Pending critical interrupts: `zqk system policy-interrupts pending`.

Common shapes from the auditor:

- **AST / structural integrity** — `pkg/validation/qa` ASTAuditor disparities (often noisy across many BLIs)
- **Missing verified test assets** — traceability / DoD linkage gaps

**Easy button (bulk ack):**

```bash
./bin/zqk system policy-interrupts ack --all
./bin/zqk system policy-interrupts ack --all --prefix qa-disparity-
```

After a **clean** QA auditor replay (`qa_success` path), matching `qa-disparity-<id>` interrupts are **auto-acked** so humans are not stuck re-acking one key at a time.

Snapshot for drift review:

```bash
./bin/zqk system policy-interrupts pending --format json > .zqk/logs/policy/interrupts-pending-$(date +%Y%m%d-%H%M%S).json
./scripts/run-bg.sh policy-gates -- bash -c '…'   # field-key / env / logging / architecture / drift hotspots
```

## Audit trail (this tranche)

- Summarizer: `scripts/summarize-system-check-clusters.py`
- Deleted 25 illegal `qa_success` (`status=error`) via bulk delete + unlink (log under `.zqk/logs/policy/qa_success-delete-*.log`)
