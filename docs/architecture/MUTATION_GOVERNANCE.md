# Membrane Mutation Governance & Integrity Specification

## Overview
This document defines the membrane mutation governance architecture, provenance safeguards, value-restricted spec field constraints, and lifecycle transition enforcement within the ZQK Knowledge Operating System (KOS).

## 1. System Provenance Restriction
System provenance fields are strictly computed and maintained by the storage layer and cannot be manually overridden through ZQL mutations or CLI update commands:
- `created_at`: Set upon object creation.
- `created_by`: Identity of the initial author/account.
- `updated_at`: Automatically refreshed upon every mutation.
- `updated_by`: Identity of the mutating actor.
- `cas_address`: Content-addressable storage locator.
- `hash`: Cryptographic checksum of object contents.

Attempts to manually specify or mutate these fields fail closed unless authorized via an authenticated break-glass context.

## 2. Value-Restricted Spec Fields
Objects admitted through the membrane are validated against their respective schemas:
- **Enums & Categories**: Fields with defined discrete values (such as `category`, `status`, `tier`) reject non-conforming strings.
- **Regex Patterns**: Formatted identifiers and identifiers matching formal naming standards are verified.
- **Numeric Ranges**: Boundary constraints on numeric fields (e.g., effort estimates, metrics) are verified for upper and lower limits.

## 3. Directed Lifecycle Transitions
State transitions across object lifecycles are strictly directed:
- Objects can only transition along edges defined in the lifecycle graph.
- Out-of-order, unearned, or skipped states are rejected at preflight and admission.
- Preconditions (such as criterion validation or test verification) must be satisfied prior to forward promotion.

## 4. Emergency Governance Bypass & Override Friction (`--override`)

In emergency break-glass scenarios where manual state repair or lifecycle precondition bypass is required, the CLI provides strictly audited friction controls via `--override` and `--reason-code`:

### A. Lifecycle Precondition & Gate Bypass (`zqk object update`)
- `--override`: Activates emergency bypass mode, allowing transitions across lifecycle stages even when automated gates (e.g. test verification or criteria validation) are unfulfilled.
- `--reason-code "<justification>"`: Mandatory audited justification explaining why the bypass is necessary.
  - **Friction Gate**: Must be at least 30 characters and at least 5 words.
  - **Non-TTY Block**: Non-interactive shells, background scripts, and autonomous agents are strictly blocked from using `--override`. It must be executed by a human in an authenticated interactive TTY.
  - **Interactive Confirmation**: The operator must type the exact confirmation phrase: `"I acknowledge this bypass introduces process debt"`.
  - **CI Block**: Completely blocked in CI environments (`CI=true`).

### B. Core Object Deletions (`zqk object delete`)
- Hard deletion of core kernel objects is guarded fail-closed and requires `--reason-code "<justification>"` (minimum 30 characters, 5 words).
- Core entities (e.g. `requirement`, `criteria`, `test_case`, `priority_plan`) must be archived (`status: archived`) rather than permanently deleted whenever possible to maintain audit lineage.

### C. Process Debt Minting & Audit Trails
Whenever an `--override` is executed:
1. A durable `process_debt` kernel object is automatically minted recording the bypass reason, target ID, and actor.
2. A structured `audit_event` is appended to `.zqk/streams/` with the security context and timestamp.
3. System check (`zqk system check`) flags active process debt until resolved or validated.


