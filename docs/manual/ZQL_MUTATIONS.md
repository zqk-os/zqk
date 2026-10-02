# ZQL Declarative Mutations Developer Manual

## Overview

**ZQL** (ZQK Query Language - Mutation Engine) is the declarative mutation language for executing deterministic, ACID-compliant modifications across the Knowledge Kernel graph. ZQL statements guarantee referential integrity, trigger automatic shockwave propagation, and log all state changes to the Write-Ahead Log (WAL).

---

## 1. Declarative Mutation Anatomy

ZQL mutations can be expressed via high-level declarative syntax or structured documents. A mutation directive specifies target match criteria, transformations to apply, and pre- or post-flight validation rules:

```zql
MUTATE backlog_item
WHERE id = "BLI-SAMPLE-001"
SET {
    title: "Updated Task Title",
    priority_tier: "P1",
    estimated_effort: "4h"
}
ASSERT criteria_refs IS NOT EMPTY
```

---

## 2. The `apply` Operation

To execute mutations safely, ZQL provides an `apply` execution pathway. The engine evaluates preconditions in memory, validates the proposed object state against the schema and lifecycle definitions, and then writes the result atomically through the CAS membrane.

### CLI Execution

```bash
# Apply a declarative mutation payload from a file
zqk mutate --file mutations/update_task.zql.yaml

# Apply mutations with strict pre-condition enforcement
zqk mutate --dry-run --file mutations/batch_realign.zql.yaml
```

### Mutation Payload Example (`update_task.zql.yaml`)

```yaml
version: "1.0.0"
operation: "apply"
target:
  kind: "backlog_item"
  id: "BLI-DOC-LANGUAGES"
mutations:
  - set:
      status: "testing"
      artifacts:
        - "docs/manual/ZPARQL_QUERY_LANGUAGE.md"
        - "docs/manual/ZQL_MUTATIONS.md"
preconditions:
  - assert: "status == 'planned'"
```

---

## 3. Atomic CAS Commit and Write-Ahead Log (WAL)

Every ZQL mutation follows the two-phase lifecycle commit pipeline:
1. **Validation & Preconditions**: Schema fields, required types, and lifecycle transition constraints are checked against current state.
2. **CAS Ingestion**: The serialized object payload is hashed (`sha256`), written to `.zqk/process/<kind>/<hash>.yaml`, and the object ID index is updated.
3. **WAL Append**: An entry is appended to `.zqk/logs/audit/wal.jsonl` containing operation metadata, actor attribution, and cryptographic signatures.
4. **Shockwave Emission**: Dependent objects (e.g. parent priority plans) receive reactive notification signals to recalculate rolled-up progress metrics.

---

## 4. Rollback and Conflict Resolution

If any assertion fails during a multi-object mutation batch:
- **Transaction Abortion**: No CAS references are committed.
- **Draft Plane Isolation**: Incomplete or draft objects remain sequestered in `.zqk/object_drafts/` without polluting the live graph index.
- **Fail-Closed Guarantees**: A descriptive error diagnostic is returned, identifying the exact invariant violation.

---

## Canonical References
- [ZPARQL Query Language](ZPARQL_QUERY_LANGUAGE.md)
- [Lifecycle State Machine Specification](../architecture/LIFECYCLE_STATE_MACHINE.md)
- [Architecture Index](../INDEX.md)
