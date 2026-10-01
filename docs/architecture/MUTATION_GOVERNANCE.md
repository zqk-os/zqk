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

## 4. Break-Glass Emergency Elevation & Override Flags

In break-glass scenarios where emergency state repair is required, the CLI provides strictly audited mechanisms depending on the operation:

### A. Declarative Script Mutations (`zqk mutate` / ZQL)
- `--break-glass`: Activates elevated bypass capability in ZQL mutations to allow manual provenance or transition repairs.
- `--break-glass-reason "<justification>"`: Mandatory justification string (minimum 10 characters required) explaining why emergency elevation is necessary.
- Emits structured audit events (`audit_event`) into `.zqk/streams/` with the reason and actor identity.

### B. Interactive Object Updates (`zqk object update`)
- `--override`: Bypasses strict directed lifecycle precondition gates for single object updates.
- **Human TTY Confirmation**: To prevent autonomous agents from silently bypassing governance, `--override` prompts for interactive confirmation on standard input (`tty`). Non-interactive/agent shells are blocked from using `--override` directly.

### C. Core Object Deletions (`zqk object delete`)
- `--reason-code "<justification>"`: Hard deletion of core kernel objects is guarded fail-closed and requires an explicit, audited reason (minimum 30 characters). Core objects should generally be archived rather than deleted.

