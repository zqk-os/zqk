# CLI Orphan Verbs Retirement & Canonical Domain Mapping

**Document ID:** `DOC-ORPHAN-VERBS-RETIREMENT`  
**Requirement Reference:** `REQ-ORPHAN-VERBS-RETIREMENT`  
**Backlog Item:** `BLI-1790814952688118000-a20ee76d`  
**Status:** Canonical / Implemented  
**Date:** 2026-10-01  

---

## 1. Context & Rationale

As described in `docs/architecture/CLI_COMMAND_TAXONOMY_STANDARDS.md` (Rule 2) and CEF evaluation finding `F-DOC-ORPHAN-VERBS-RULE-VIOLATION-004`, unqualified verbs at the root level clutter the command namespace, introduce cognitive fatigue, and break clean noun-verb domain stratification.

However, completely deleting familiar root commands causes catastrophic friction for human engineers and agent muscle memory.

To resolve this conflict definitively:
1. **Canonical Domain Homes:** Every single root command has a canonical home under its architectural domain group.
2. **Approved Universal Ergonomics Shortcuts:** Root verbs are preserved as direct, documented shortcuts delegating to their canonical counterparts.
3. **Spec Coverage & Parity:** All commands are validated against `.zqk/cli/specs/`.

---

## 2. Canonical Domain Mapping Matrix

| Root Ergonomics Shortcut | Canonical Domain Command | Domain Group | Description |
| :--- | :--- | :--- | :--- |
| `zqk do` | `zqk workflow do` | `workflow` | Autonomous CAP loop execution |
| `zqk inspect` | `zqk object inspect` | `object` | Interactive TUI Object Inspector |
| `zqk mutate` | `zqk object mutate` | `object` | Declarative ZQL mutations |
| `zqk query` | `zqk graph query` | `graph` | Declarative ZPARQL graph queries |
| `zqk validate` | `zqk system validate` | `system` | Invariant gate and schema validation |
| `zqk rollback` | `zqk object rollback` | `object` | Transaction rollback restoration |
| `zqk completion` | `zqk system completion` | `system` | Shell completion script generator |
| `zqk sync` | `zqk mesh sync` | `mesh` | CAS and mesh synchronization |
| `zqk pre-commit` | `zqk system pre-commit` | `system` | Release gate and secret scan runner |
| `zqk learn` | `zqk agent learn` | `agent` | Institutional memory capture |
| `zqk new` | `zqk object new` | `object` | Scaffolding wizard |
| `zqk reports` | `zqk system reports` | `system` | Velocity and quality metric reporting |
| `zqk tray` | `zqk service tray` | `service` | Background daemon menu bar companion |
| `zqk join` | `zqk graph join` | `graph` | Relational graph join and federation |

---

## 3. Verification & Invariants

All mappings are guaranteed by automated Three-Fold Proof verification in `cmd/zqk/app/orphan_verbs_retirement_test.go`:
- **Static Floor:** `TestStaticFloor_CanonicalDomainCommandMappings` guarantees that each canonical command is present under its domain parent.
- **Operational Proof:** `TestOperationalProof_ErgonomicsShortcutsExecution` verifies that invoking commands via root vs canonical domain resolves correctly.
- **Negative Boundary:** `TestNegativeBoundary_InvalidSubcommandRejection` validates that unknown commands and invalid flags fail closed.
