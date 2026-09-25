# Operational Incident Runbooks

This directory contains standardized incident response and operational troubleshooting runbooks for the ZQK Knowledge Kernel platform.

## Runbook Catalog

| ID | Title | Domain | Severity |
| :--- | :--- | :--- | :--- |
| [RB-CAS-001](./RB-CAS-001-CAS-CORRUPTION-RECOVERY.md) | CAS Hash Mismatch and Quarantine Exhaustion | Storage / CAS | P0 / Critical |
| [RB-WAL-001](./RB-WAL-001-WAL-COMPACTION-FAILURES.md) | Write-Ahead Log (WAL) Compaction Failures | Durability / WAL | P1 / High |
| [RB-LCK-001](./RB-LCK-001-LOCK-CONTENTION-DEADLOCKS.md) | Concurrency Lock Contention and Deadlock Resolution | Runtime / Concurrency | P1 / High |
| [RB-SCH-001](./RB-SCH-001-SCHEDULER-DAEMON-TRIAGE.md) | Scheduler Daemon Exit Cascades and Job Triage | Scheduler / Automation | P1 / High |

## General Incident Triage Protocol
1. **Assess System State**: Run `./bin/zqk system status` and `./bin/zqk scheduler status`.
2. **Collect Diagnostics**: In emergencies, run `./bin/zqk diagnostics dump` to preserve traces before restarting daemons.
3. **Consult Relevant Runbook**: Follow the Step-by-Step Resolution and Verification gates specified in the catalog.
