# Developer & Agent Guide: Custom Validation Policy Creation & Rule DSL

## 1. Executive Summary

In a multi-agent autonomous engineering swarm, preventing regressions and architectural violations requires fail-closed governance. Rather than relying on fuzzy prompts or unverified code reviews, the **ZQK Knowledge Kernel** uses declarative, mathematically provable **Validation & Policy Rules**.

Policies are written in the **Validation Rule DSL** (formalized under the specification [`SPEC-VALIDATION-RULE-DSL-GRAMMAR`](../specs/SPEC-VALIDATION-RULE-DSL-GRAMMAR.md)), stored as first-class kernel objects in `.zqk/process/policy/`, and evaluated at:
1. **Preflight**: In-memory checks during interactive authoring and ZQL mutation planning.
2. **Pre-Commit**: Armed git pre-commit check-valves blocking non-compliant local commits.
3. **Continuous Execution**: `zqk do` and scheduler loops guarding state transitions.

This guide walks human engineers and autonomous agents through creating, dry-running, grandfathering, and deploying custom policies.

---

## 2. Validation Rule DSL Syntax & Semantics

The DSL is an ISO/IEC 14977 compliant declarative language restricted to pure, side-effect-free, bounded boolean expressions.

### 2.1 Grammar Overview

```ebnf
rule_file        = rule_declaration , { rule_declaration } ;
rule_declaration = "RULE" , identifier , "FOR" , identifier , "{" ,
                   "SEVERITY" , severity_level , ";" ,
                   "MESSAGE" , string_literal , ";" ,
                   "WHEN" , boolean_expression , ";" ,
                   "ENSURE" , boolean_expression , ";" ,
                   "}" ;
```

### 2.2 Operators & Precedence

| Precedence | Operator | Semantic Meaning | Example |
| :--- | :--- | :--- | :--- |
| 1 (Highest) | `.` | Member attribute access | `item.priority_tier` |
| 2 | `==`, `!=`, `<`, `<=`, `>`, `>=` | Binary comparisons | `status == "in_progress"` |
| 3 | `in`, `contains`, `=~` | Membership & regex | `category in ["feature", "refactor"]` |
| 4 | `!` | Logical negation | `!is_blocked` |
| 5 | `&&` | Logical conjunction | `claimed_by != "" && criteria_linked` |
| 6 | `\|\|` | Logical disjunction | `is_admin \|\| is_owner` |
| 7 (Lowest) | `==>` | Logical implication | `status == "complete" ==> tests_passed` |

> [!IMPORTANT]
> **Material Implication (`==>`)**: The expression `P ==> Q` is semantically equivalent to `(!P) || Q`. If antecedent `P` is false, the rule evaluates to `true` (pass). If `P` is true, consequent `Q` must hold true for the rule to pass.

---

## 3. Standard Predicate Library

The rule evaluator provides built-in, pure helper predicates that safely traverse graph relationships without requiring complex sub-queries:

```
┌───────────────────────────────────────────────┬─────────────────────────────────────────────────────────┐
│ Predicate Signature                           │ Purpose & Behavioral Guarantee                          │
├───────────────────────────────────────────────┼─────────────────────────────────────────────────────────┤
│ criteria_linked_or_acceptance_present(obj)    │ Confirms entity has >= 1 valid DoD criteria node in CAS │
│ tests_ok_per_customization(obj)               │ Confirms all bound test cases pass assertions           │
│ security_gate_ok_or_na(obj, ctx)              │ Verifies caller role meets required security scopes     │
│ len(collection)                               │ Returns integer element count of array or string        │
│ is_empty(val)                                 │ Returns true if string, slice, or map is empty or nil   │
└───────────────────────────────────────────────┴─────────────────────────────────────────────────────────┘
```

---

## 4. End-to-End Walkthrough: Creating a Custom Policy

### Step 1: Define Intent & Invariant
Suppose your team wants to enforce the following engineering standard:
> *"Every Backlog Item marked as `P0` or `P1` that enters `in_progress` must have an assigned claimant and an unbroken criteria chain."*

### Step 2: Formulate the DSL Expression
```text
(status == "in_progress" && priority_tier in ["P0", "P1"])
  ==> (claimed_by != "" && criteria_linked_or_acceptance_present() == true)
```

### Step 3: Launch Interactive Policy Rule Studio
Instead of guessing YAML formatting, launch the **Policy Rule Studio**:

```bash
zqk object inspect --policy-studio
```

Within the Studio:
1. Select Target Kind: Press <kbd>Tab</kbd> until `[backlog_item]` is active.
2. Enter Rule ID: Set to `POL-SAFETY-OWNERSHIP-001`.
3. Set Severity: Choose `ERROR (Reject Mutation)`.
4. Compose Expression: Use real-time autocompletion to insert field tokens and predicates.

---

## 5. Live Population Dry-Run Evaluation

Before saving, press <kbd>t</kbd> to trigger a live dry-run evaluation across the repository.

```
┌─ EVALUATION RESULTS ────────────────────────────────────────────────────────┐
│  ✓ 194 / 196 objects COMPLIANT (99.0%)                                      │
│  ✗ 2 objects VIOLATE RULE:                                                  │
│    • BLI-AUTH-004: status is 'in_progress' but 'claimed_by' is empty        │
│    • BLI-UI-012: status is 'in_progress' but 'claimed_by' is empty          │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Grandfathering & Transition Check-Valves

When applying a new policy to an existing repository with non-compliant historical records, the Policy Studio configures enforcement as **`CHECK_VALVE_ON_TRANSITION`**:
- **Existing entities** (`BLI-AUTH-004`) remain untouched and can be read or queried.
- **Future state mutations** (e.g., updating status, modifying titles) will be rejected by the check-valve until the violation is remedied.

---

## 6. Committing Policy to the Kernel

Press <kbd>s</kbd> to commit the policy:
1. **Serialization**: The policy is written to `.zqk/process/policy/POL-SAFETY-OWNERSHIP-001.yaml`.
2. **CAS Ingestion**: Generates an authoritative SHA-256 fingerprint.
3. **Hook Binding**: Automatically binds the rule to `scripts/git-hooks/pre-commit` and `zqk do` mutation preflight checks.

```yaml
schema_version: 2.0.0
id: POL-SAFETY-OWNERSHIP-001
kind: policy
title: "Strict Owner Assignment for High-Priority Workstreams"
status: active
policy_type: standard
category: workflow
description: "Enforce claimant assignment and criteria DoD linkage for high-priority backlog items entering in_progress state."
body: |
  Every Backlog Item marked as P0 or P1 that transitions into `in_progress` must have an assigned claimant and an unbroken criteria chain before work commences.
applicability:
  object_types:
    - backlog_item
enforcement:
  automated: true
  severity: error
  reminder_enabled: true
validation_overlays:
  - "(status == 'in_progress' && priority_tier in ['P0', 'P1']) ==> (claimed_by != '' && criteria_linked_or_acceptance_present() == true)"
created_at: "2026-09-29T16:00:00Z"
created_by: ACC-SYSTEM
updated_at: "2026-09-29T16:00:00Z"
updated_by: ACC-SYSTEM
```

> [!NOTE]
> **Kernel Storage vs. In-Memory Studio Projection**:
> - In authoritative CAS storage (`.zqk/process/policies/*.yaml`), policies adhere strictly to the Kernel Policy Schema (`schema_version: 2.0.0`), utilizing `applicability.object_types` for target binding, `enforcement` for check-valve severity, and standard ISO-8601 UTC timestamps (`created_at`, `updated_at`).
> - When evaluated interactively inside the Policy Rule Studio console, rules are mapped into the in-memory `PolicyRule` evaluator projection (`id`, `name`, `target_kind`, `expression`, `severity`).

