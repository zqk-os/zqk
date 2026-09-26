# Technical Specification: ZQL In-Memory Preflight Validation and Diagnostic Receipt Protocol

**Document ID:** `SPEC-ZQL-PREFLIGHT-VALIDATION`  
**Governing Goal:** `GOAL-ZQL-DECLARATIVE-MUTATIONS-AND-TRANSACTIONS` (Declarative ZQL Mutations and ACID Storage Transactions)  
**Governing Requirement:** `REQ-ZQL-PREFLIGHT-VALIDATION-HOOK` (In-Memory Preflight Validation and Diagnostic Receipts)  
**Governing Priority Plan:** `PRI-ZQL-ZPARQL-DECLARATIVE-ENGINES` (Language-Agnostic ZQL and ZPARQL Declarative Engines)  
**Governing Backlog Item:** `BLI-1790382685932031000-4a7028c6` (`Implement: In-Memory Preflight Validation and Diagnostic Receipt Protocol`)  
**Governing Test Case:** `TST-1790382685932031001-e4ba68b4` (`Test Suite: In-Memory Preflight Validation and Diagnostic Receipt Protocol`)  
**Criteria Traceability (3 Facets):**  
1. `CRIT-ZQL-INMEMORY-VALIDATION-CONTRACT` (Static Floor: Pre-Persistence In-Memory Semantic Validator Contract)  
2. `CRIT-ZQL-PREFLIGHT-DIAGNOSTIC-RECEIPT` (Dynamic Behavior: Structured Pre-Flight Diagnostic Receipt Generation)  
3. `CRIT-ZQL-SCHEMA-CORRUPTION-FAILCLOSED-NEGATIVE` (Negative Invariant: Fail-Closed Boundary for Type and Enum Violations)  
**Authoring Personas:** `PER-COMMUNITY-SYSTEM-ARCHITECT`, `PER-COMMUNITY-SOFTWARE-ENGINEER`, `PER-COMMUNITY-QA-AUDITOR`  
**Status:** Approved Architectural Specification  

---

## 1. Executive Summary & Purpose

Before mutations are dispatched to persistent Content Addressable Storage (CAS) or underlying transactional write buffers, the ZQK Knowledge Kernel requires an in-memory preflight validation gate. 

Dispatching malformed entities directly to storage causes:
1. **Unnecessary Disk & CAS I/O Overhead**: Persisting and then rolling back unvalidated objects degrades throughput.
2. **Opaque Error Propagation**: Low-level storage serialization errors lack semantic context regarding schema constraints, field paths, and human- or agent-actionable remediations.
3. **Graph Corruption Vulnerabilities**: Subtle type mismatches or disallowed enums can compromise graph traversal and ontology query soundness.

This specification formalizes the **In-Memory Preflight Validation Contract** and the **Diagnostic Receipt Protocol**, ensuring fail-closed rejection of corrupted or malformed entities entirely in memory prior to storage dispatch.

---

## 2. In-Memory Validation Contract

The pre-persistence validation interface evaluates candidate mutations without initiating disk reads or writes.

### 2.1 Interface Definition
```go
type PreflightValidator interface {
    // Validate evaluates an individual mutation entirely in-memory.
    Validate(ctx context.Context, mut *Mutation) (*PreflightDiagnosticReceipt, error)

    // ValidateBatch evaluates a slice of mutations, aggregating diagnostic receipts.
    ValidateBatch(ctx context.Context, muts []Mutation) (*PreflightBatchReceipt, error)
}
```

### 2.2 Validation Invariants
1. **Zero Disk I/O Invariant**: The preflight validation pipeline operates exclusively on in-memory ASTs, entity maps, and compiled schema definitions. No temporary files or storage locks are created.
2. **Idempotence**: Evaluating `Validate` multiple times over identical inputs yields deterministic receipts with identical violation order and error codes.
3. **Comprehensive Evaluation**: The validator accumulates all schema violations within an entity rather than aborting at the first failure, giving agents complete remediation context in a single pass.

---

## 3. Structured Diagnostic Receipt Protocol

When a mutation passes or fails preflight validation, the engine produces a machine-parseable `PreflightDiagnosticReceipt`.

### 3.1 JSON Schema Contract

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://zqk.dev/schemas/mutation/preflight_diagnostic_receipt_v1.json",
  "title": "Preflight Diagnostic Receipt",
  "type": "object",
  "required": ["valid", "disposition", "violations"],
  "properties": {
    "valid": {
      "type": "boolean"
    },
    "disposition": {
      "type": "string",
      "enum": ["accepted", "rejected_failclosed", "quarantined"]
    },
    "target_kind": {
      "type": "string"
    },
    "target_id": {
      "type": "string"
    },
    "violations": {
      "type": "array",
      "items": {
        "$ref": "#/$defs/SchemaViolation"
      }
    }
  },
  "$defs": {
    "SchemaViolation": {
      "type": "object",
      "required": ["field_path", "failing_constraint", "expected", "actual", "remediation"],
      "properties": {
        "field_path": { "type": "string" },
        "failing_constraint": { "type": "string" },
        "expected": { "type": "string" },
        "actual": { "type": "string" },
        "remediation": { "type": "string" }
      }
    }
  }
}
```

### 3.2 Concrete Diagnostic Receipt Example
```json
{
  "valid": false,
  "disposition": "rejected_failclosed",
  "target_kind": "backlog_item",
  "target_id": "BLI-INVALID-EXAMPLE",
  "violations": [
    {
      "field_path": "fields.priority_tier",
      "failing_constraint": "enum_membership",
      "expected": "one of: [P0, P1, P2, P3, P4, P5]",
      "actual": "URGENT_NOW",
      "remediation": "Update priority_tier to a canonical tier: P0, P1, P2, P3, P4, or P5"
    },
    {
      "field_path": "fields.title",
      "failing_constraint": "type_mismatch",
      "expected": "string",
      "actual": "integer",
      "remediation": "Wrap title value in string quotes"
    }
  ]
}
```

---

## 4. Fail-Closed Boundary & Negative Invariants

To guarantee that corrupted state never crosses into persistent storage, the validation engine enforces strict fail-closed boundaries:

| Failure Category | Trigger Condition | Enforcement Behavior |
| :--- | :--- | :--- |
| **Missing Mandatory Fields** | Omission of required fields (`action`, `target_kind`, `title`, etc.) | Marked invalid; `disposition=rejected_failclosed`; blocks commit |
| **Field Type Mismatch** | Value runtime type differs from schema (e.g. integer instead of string) | Marked invalid; `failing_constraint=type_mismatch`; blocks commit |
| **Disallowed Enum Constant** | Value not in defined enum set (e.g. status `exploded` instead of `planned`) | Marked invalid; `failing_constraint=enum_membership`; blocks commit |
| **Identifier Pattern Violation** | Target ID violates kind prefix or formatting (e.g. `invalid_id` instead of `BLI-...`) | Marked invalid; `failing_constraint=id_pattern_mismatch`; blocks commit |
| **Unrecognized Kind** | `target_kind` not registered in Kernel Kind Catalog | Marked invalid; `failing_constraint=unrecognized_kind`; blocks commit |

**Negative Invariant Rule**: If `valid == false`, the mutation engine MUST NOT invoke underlying storage `Create`, `Update`, or CAS serialization. Storage mutation dispatch is completely aborted.
