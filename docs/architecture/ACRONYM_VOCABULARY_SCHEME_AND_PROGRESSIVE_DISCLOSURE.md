# Acronym Vocabulary Scheme & Progressive Disclosure

## Overview
This specification establishes the single source of truth for all domain acronyms in the ZQK Knowledge Kernel, resolving cognitive load and onboarding hurdles (`TDE-F-USB-JARGON-ACRONYM-BURDEN-006`) under requirement `REQ-ACRONYM-VOCABULARY-SCHEME`.

The vocabulary scheme provides:
1. **Single Source of Truth (`pkg/acronyms`)**: Centralized Go definitions, categories, context, and cross-references.
2. **Interactive CLI (`zqk explain` / `zqk glossary`)**: Discoverable terminal commands with table and JSON formatting plus fuzzy suggestions.
3. **Web Studio Progressive Disclosure**: Hover tooltips across the DAG visualizer, Timeline/Gantt view, and Object Inspector.

---

## Canonical Kernel Acronyms

| Acronym | Full Name | Category | Definition & Context |
| :--- | :--- | :--- | :--- |
| **BLI** | Backlog Item | work | A discrete, tracked work package or deliverable scheduled within a Priority Plan. Decomposes requirements into engineering tasks. |
| **PRI** | Priority Plan | work | A time-bounded execution plan defining prioritized work packages and delivery horizons. Corresponds to integration branches. |
| **REQ** | Requirement | specification | A formal system invariant, capability, or architectural specification. Proven by at least three orthogonal criteria. |
| **CRIT** | Criterion | verification | An objective, verifiable acceptance condition proving a requirement (Static Floor, Operational Proof, Negative Boundary). |
| **VDS** | Verification Definition of Done | verification | Automated gate proving that all criteria and test lineages are green before pull requests merge. |
| **TCFG** | Team Configuration | organization | Approved team composition, agent roster, and operational permissions. |
| **CVS** | Convergence Session | coordination | A structured cybernetic feedback session that closes deltas between projected and actual system state. |
| **ATK** | Agent Task | execution | A granular, autonomous unit of work assigned to and executed by an agent persona. |
| **PPLAN** | Priority Plan (CLI) | cli | CLI command shortcut group for inspecting active priority plans and current workloads. |
| **ZPARQL** | ZQK Pattern Query Language | graph | Declarative graph pattern matching and query engine for traversing kernel object relationships. |
| **ZQL** | ZQK Query Language | query | Declarative object query, mutation, and filtering expression language. |
| **CAS** | Content-Addressable Storage | storage | Immutable, cryptographic hash-indexed object storage layer forming the kernel membrane. |
| **WAL** | Write-Ahead Log | storage | Append-only sequential ledger guaranteeing atomic state mutations and crash recovery. |
| **CAP** | Continuous Autonomous Protocol | governance | The self-driving cybernetic feedback loop steering agents without human intervention. |
| **CEF** | Community Evaluation Framework | evaluation | Comprehensive quality scorecard, testing pyramid, and Diamond Scale grading rubric. |
| **TDE** | Technical Debt Entry | hygiene | An objectified defect, architectural smell, or maintainability liability tracked for resolution. |

---

## CLI Usage

### Look up a specific acronym
```bash
zqk explain BLI
zqk glossary VDS
```

Output:
```
ACRONYM    : BLI
FULL NAME  : Backlog Item
CATEGORY   : work
DEFINITION : A discrete, tracked work package or deliverable scheduled within a Priority Plan.
CONTEXT    : BLIs decompose requirements into tangible engineering tasks with verifiable criteria.
RELATED    : PRI, REQ, CRIT, ATK
```

### List all acronyms
```bash
zqk explain
```

### Structured JSON Output
```bash
zqk explain CAP --format json
```

### Fuzzy Match Suggestions
```bash
zqk explain BL
```
```
Error: unknown kernel acronym "BL".
Did you mean: BLI?
Run 'zqk explain' without arguments to see all registered acronyms
```

---

## Verification & Traceability

- **Requirement**: `REQ-ACRONYM-VOCABULARY-SCHEME`
- **Criteria**:
  - `CRIT-ACRONYM-VOCABULARY-ACCEPTANCE`: Functional Acceptance (verified by `pkg/acronyms/acronyms_test.go` and `cmd/zqk/explain/explain_test.go`).
  - `CRIT-ACRONYM-VOCABULARY-BOUNDARY`: Boundary & Error Handling (fuzzy matching and unknown input rejection).
  - `CRIT-ACRONYM-VOCABULARY-DOCS`: Documentation & Knowledge Base Entry (this specification).
