# Technical Specification: ZQL ACID Transaction Execution, Staged Isolation, and Atomic Rollback

**Document ID:** `SPEC-ZQL-TRANSACTION-EXECUTION`  
**Governing Goal:** `GOAL-ZQL-DECLARATIVE-MUTATIONS-AND-TRANSACTIONS` (Declarative ZQL Mutations and ACID Storage Transactions)  
**Governing Requirement:** `REQ-ZQL-TRANSACTION-EXECUTION-MODEL` (ACID Transaction Execution, Staged Isolation, and Atomic Rollback)  
**Governing Priority Plan:** `PRI-ZQL-ZPARQL-DECLARATIVE-ENGINES` (Language-Agnostic ZQL and ZPARQL Declarative Engines)  
**Governing Backlog Item:** `BLI-1790382668716148000-6fa28424` (`Implement: ACID Transaction Execution, Staged Isolation, and Atomic Rollback`)  
**Governing Test Case:** `TST-1790382668716148001-dc96e357` (`Test Suite: ACID Transaction Execution, Staged Isolation, and Atomic Rollback`)  
**Criteria Traceability (3 Facets):**  
1. `CRIT-ZQL-TXN-ISOLATION-SPEC` (Static Floor: Transaction Execution Lifecycle and Isolation Mode Specification)  
2. `CRIT-ZQL-ALL-OR-NOTHING-ROLLBACK-PROOF` (Dynamic Behavior: Complete Atomic Rollback on Downstream Constraint Failure)  
3. `CRIT-ZQL-DIRTY-READ-CONCURRENCY-NEGATIVE` (Negative Invariant: Zero Dirty Read Leakage and Isolation Invariant Enforcement)  
**Authoring Personas:** `PER-COMMUNITY-SYSTEM-ARCHITECT`, `PER-COMMUNITY-SOFTWARE-ENGINEER`, `PER-COMMUNITY-QA-AUDITOR`  
**Status:** Approved Architectural Specification  

---

## 1. Overview & Transactional Scope

In the ZQK Knowledge Kernel, multi-object mutations and graph dependency modifications must execute with strict ACID guarantees. Uncontrolled partial updates result in orphaned graph nodes, broken foreign keys, and corrupt state graphs.

This specification formalizes:
1. **The Transaction Execution State Machine**: Explicit states (`BEGIN`, `STAGE`, `PRE_CHECK`, `COMMIT`, `ROLLBACK`) and valid state transition paths.
2. **Three Transaction Isolation Modes**:
   - `all_or_nothing` (Default): Atomic abort and complete rollback on any record failure.
   - `partial_commit`: Quarantines invalid records with diagnostic receipts while committing valid records.
   - `dry_run`: Simulation mode executing in-memory preflight validation without persisting to storage.
3. **Write Isolation & Zero Dirty-Read Leakage**: Staging mutation accumulation in a private transaction buffer, ensuring concurrent readers cannot observe uncommitted writes.
4. **Deterministic Rollback Invariant**: Verification that an error on record $N$ during an `all_or_nothing` transaction cleanly rolls back mutations $1 \dots N-1$, leaving zero uncommitted state in underlying storage.

---

## 2. Transaction State Machine Lifecycle

A transaction advances deterministically through a finite set of discrete states:

```
                  +-----------------------------------+
                  |                                   |
                  v                                   |
[INITIAL] ---> [BEGIN] ---> [STAGE] ---> [PRE_CHECK] -+---> [COMMIT] (Terminal)
                 |            |              |
                 |            |              |
                 +------------+--------------+------------> [ROLLBACK] (Terminal)
```

### 2.1 State Definitions
- `INITIAL`: Transaction object instantiated; unstarted.
- `BEGIN`: Transaction boundary opened; snapshot timestamp recorded; staging buffer allocated.
- `STAGE`: In-flight mutations are accumulated into the private transaction staging buffer. Multiple `STAGE` calls are permitted.
- `PRE_CHECK`: Graph invariant validation, schema compliance, precondition verification, and constraint checks are evaluated across all staged mutations.
- `COMMIT`: Terminal success state. All staged mutations are atomically persisted to underlying storage.
- `ROLLBACK`: Terminal failure state. Any partially applied changes or staging buffers are discarded, and compensations are executed.

### 2.2 Valid State Transitions
| Source State | Destination State | Description / Trigger |
| :--- | :--- | :--- |
| `INITIAL` | `BEGIN` | `Begin()` invoked with configured isolation mode |
| `BEGIN` | `STAGE` | Initial mutation appended via `Stage(mutation)` |
| `BEGIN` | `ROLLBACK` | Immediate transaction abort before any staging |
| `STAGE` | `STAGE` | Subsequent mutations appended via `Stage(mutation)` |
| `STAGE` | `PRE_CHECK` | Staging closed; preflight validation invoked via `PreCheck()` |
| `STAGE` | `ROLLBACK` | Premature transaction abort during staging |
| `PRE_CHECK` | `COMMIT` | All validations succeed; `Commit()` executed |
| `PRE_CHECK` | `ROLLBACK` | Validation failure or abort in `all_or_nothing` mode |
| `PRE_CHECK` | `STAGE` | In interactive/resilient workflows, returning to stage after partial check |

Any other transition (such as `COMMIT` $\to$ `STAGE`, `ROLLBACK` $\to$ `COMMIT`, or `BEGIN` $\to$ `COMMIT` bypassing `PRE_CHECK`) violates the state machine and returns `ERR_ZQL_INVALID_TXN_STATE`.

---

## 3. Isolation Modes

### 3.1 `all_or_nothing` Mode (Atomic Abort)
- **Semantics**: Strict ACID atomicity. Either all operations in the mutation batch succeed and become visible simultaneously, or the entire transaction aborts.
- **Rollback Invariant**: When an error occurs on record $N$ in a batch of $M$ records ($1 \le N \le M$), all mutations $1 \dots N-1$ previously staged are completely unwound.
- **Zero Orphan Guarantee**: No intermediate nodes, edges, or metadata files are left in CAS or graph indexes.

### 3.2 `partial_commit` Mode (Quarantine & Diagnostic Receipt)
- **Semantics**: Resilient batch processing for multi-agent ingestion.
- **Behavior**: Each mutation in the batch is evaluated independently during `PRE_CHECK`.
  - Mutations passing validation are scheduled for commit.
  - Mutations failing validation are quarantined into a diagnostic receipt collection with structured error codes.
- **Commit**: Valid mutations commit; quarantined mutations generate diagnostic feedback receipts.

### 3.3 `dry_run` Mode (Preflight Simulation)
- **Semantics**: Read-only validation pass.
- **Behavior**: Executes `BEGIN` $\to$ `STAGE` $\to$ `PRE_CHECK`. Calculates generated identifiers, checks schema constraints, and constructs proposed graph deltas.
- **Invariant**: Strictly zero write operations are issued to CAS or underlying storage. Transitions directly to terminal state with a simulation receipt.

---

## 4. Write Isolation & Zero Dirty-Read Invariant

### 4.1 Staged Buffer Isolation
In-flight mutations are accumulated exclusively inside an uncommitted in-memory `StagedBuffer` held within the `Transaction` context:
1. Write operations (`create_node`, `update_node`, `add_edge`, `remove_edge`) write only to the private `StagedBuffer`.
2. The shared `ObjectStorageProvider` remains untouched during `STAGE` and `PRE_CHECK`.

### 4.2 Concurrency Negative Invariant
- **Rule**: Concurrent readers executing queries or lookups against the storage provider during an in-flight transaction MUST NOT observe uncommitted staged mutations.
- **Commit Barrier**: Staged modifications only become visible to external readers upon the successful completion of the atomic `Commit()` barrier under synchronization locks.
- **Rollback Barrier**: If the transaction aborts or encounters an error, the `StagedBuffer` is discarded, and external readers never observe the failed mutations.

---

## 5. Diagnostic Receipts & Error Protocol

Each executed mutation produces a `DiagnosticReceipt`:

```json
{
  "index": 1,
  "action": "create_node",
  "target_kind": "backlog_item",
  "target_id": "BLI-1001",
  "status": "committed",
  "error": ""
}
```

Receipt statuses include:
- `staged`: Accumulated in private buffer.
- `committed`: Persisted to storage.
- `quarantined`: Rejected during `partial_commit` with error details.
- `rolled_back`: Unwound due to transaction abort.
- `dry_run_validated`: Simulated successfully without writes.
