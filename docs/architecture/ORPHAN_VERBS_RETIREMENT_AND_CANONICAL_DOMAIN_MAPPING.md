# Canonical Domain Command Mapping & Ergonomic Shortcuts

<!-- tags: cli, commands, taxonomy, ergonomics, domain-mapping, shortcuts -->

This architectural specification details the relationship between ZQK's canonical domain command taxonomy (strict noun-verb hierarchy) and high-frequency root ergonomic shortcuts (`zqk do`, `zqk inspect`, `zqk mutate`).

---

## 1. Context & Architectural Rationale

As established in [`CLI Command Taxonomy Standards`](./CLI_COMMAND_TAXONOMY_STANDARDS.md), an enterprise-scale CLI requires strict noun-verb stratification to prevent namespace collisions, maintain cognitive clarity, and ensure deterministic sub-agent tool calling. Unqualified verbs directly at the root level can introduce confusion across large distributed systems.

At the same time, forcing human developers and autonomous agents to type lengthy commands for the most frequently executed operational verbs (`zqk workflow do`, `zqk object inspect`, `zqk object mutate`) introduces keystroke fatigue and breaks established workflow velocity.

To achieve both architectural purity and maximum ergonomics:
1. **Canonical Domain Homes**: Every single CLI command has a canonical home under its architectural domain group (`zqk workflow do`, `zqk object mutate`, `zqk graph query`).
2. **Approved Universal Ergonomics Shortcuts**: A curated, stable set of root verbs are preserved as direct, documented shortcuts delegating to their canonical counterparts.
3. **Spec Coverage & Parity**: All shortcuts and domain commands are fully specced in `.zqk/cli/specs/` and generated via `pkg/cli/bldr_cli_cmd_v1/`.

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
