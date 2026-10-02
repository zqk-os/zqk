# First-Run Object Tutorial (Mint ➔ Promote ➔ Reference Linking ➔ Lifecycle Transitions)

**Audience:** Developers and autonomous AI agents getting started with ZQK Knowledge Kernel mutations.  
**CLI:** Canonical binary `zqk` (or `./bin/zqk`).  
**Core Principle:** Always prefer typed, schema-aware kernel commands (`zqk object ref add`, `zqk object promote`, `zqk new object`) over unstructured legacy field mutations (`--field status=...` or `--field *_ref=...`).

---

## 0. Orient: Discover Active Priorities

Before creating or modifying objects in the kernel, query the lead priority plan and shovel-ready backlog:

```bash
zqk workflow whats-next --format json
```

This returns active priority plan details, runway depth, and pending backlog items (`BLI-*`).

---

## 1. Step 1: Mint onto the Draft Plane (`new object`)

The standard, fail-safe way to create an object is by minting it onto the **Draft Plane**:

```bash
zqk new object question --title "First-run sanity question"
```

This creates a new object in `.zqk/object_drafts/` with a deterministic ID (e.g. `QUE-178...`). 
Draft-plane objects remain safely isolated in working memory without polluting Content-Addressed Storage (CAS) or affecting audit metrics until they pass schema validation and Definition of Done (DoD).

---

## 2. Step 2: Enrich Schema Attributes

Enrich scalar attributes (such as body text, questions, or descriptions) on the draft object:

```bash
zqk object update <QUESTION_ID> --field question_text="What is the canonical object creation flow?"
```

> [!NOTE]
> `object update --field <key>=<value>` is intended strictly for scalar attribute updates (e.g., `title`, `description`, `body`, `question_text`). 
> **Never** use `--field` to mutate relationship references or lifecycle status directly.

---

## 3. Step 3: Promote to Authoritative CAS (`object promote`)

Once the object satisfies schema requirements and definition of done, promote it from the draft plane into authoritative CAS:

```bash
zqk object promote <QUESTION_ID>
```

`object promote` checks preconditions, computes the cryptographic CAS content address, logs a state journal mutation, and promotes the object into the master knowledge graph.

---

## 4. Step 4: Link Relationships Semantically (`object ref add` / `object ref remove`)

Connecting objects across the ontology graph is a first-class operation. 

### Preferred Modern Mechanism: `zqk object ref`
Do **not** use `object update --field <kind>_ref=<ID>` or edit slice arrays manually. Use schema-aware reference commands:

```bash
# Link a requirement to a backlog item (automatically resolves to requirement_refs):
zqk object ref add BLI-001 REQ-001

# Link multiple acceptance criteria to a requirement in a single atomic command:
zqk object ref add REQ-001 CRIT-001 CRIT-002

# Explicitly target a specific reference field when disambiguation is needed:
zqk object ref add BLI-001 PRI-001 --field priority_plan_ref

# Remove references cleanly:
zqk object ref remove BLI-001 REQ-001
```

**Why `object ref add` is preferred:**
- **Schema-Aware Resolution:** Automatically maps target IDs to scalar fields (`*_ref`) or slice arrays (`*_refs`).
- **Target Existence Verification:** Fails closed if the referenced object does not exist in CAS.
- **Cycle Detection:** Automatically prevents cycles in `related_object_refs`.
- **Scope-Lock Enforcement:** Enforces sealed priority plan boundaries.

*(Legacy Fallback: Manual edits via `zqk object update <ID> --field "requirement_refs=..."` exist for raw migration scripts, but bypass kernel graph consistency validation).*

---

## 5. Step 5: Advance Lifecycle States (`object promote` / `demote` / `park`)

Every object kind in ZQK follows an authoritative finite state machine (e.g. `originated` ➔ `planned` ➔ `in_progress` ➔ `validated` ➔ `complete`).

### Preferred Modern Mechanism: Typed Lifecycle Transitions
Do **not** use `object update --field status=<status>`. Mutating status directly bypasses state machine check-valves and preconditions. Instead, use the dedicated lifecycle verbs:

```bash
# Advance object to the next valid lifecycle state (evaluates all entry gates & criteria):
zqk object promote BLI-001

# Promote multiple objects concurrently:
zqk object promote BLI-001,BLI-002,BLI-003

# Demote an object if verification fails or work needs rework:
zqk object demote BLI-001

# Intentionally park an object along a valid lifecycle exit:
zqk object park BLI-001
```

**Why `object promote` is preferred:**
- Validates all prerequisite acceptance criteria (`CRIT-*`) and tests (`TST-*`).
- Enforces strict unidirectional transition rules defined in `.zqk/specs/lifecycles/`.
- Emits real-time WAL lifecycle shockwave events for listeners and dashboards.

*(Legacy Fallback: Direct field modification `zqk object update <ID> --field status=<status>` should only be used by administrative operators recovering from manual corruption).*

---

## 6. Step 6: Read & Inspect the Object

Inspect objects using machine-readable JSON or human-centric TUI inspection:

```bash
# Retrieve full YAML representation
zqk object get <QUESTION_ID> --format yaml

# Autonomous agent semantic projection (token-efficient JSON)
zqk object get <QUESTION_ID> --format json

# Interactive visual inspector with lineage radar and CAS profile
zqk object inspect question <QUESTION_ID>
```

---

## 7. Step 7: Graph Traversal (`neighbors` & `path`)

Once relationships are linked, traverse the Knowledge Kernel graph:

```bash
# Discover immediate 1-hop dependencies and parent objects:
zqk object neighbors BLI-001

# Trace the ontological path between a Goal and an Acceptance Criterion:
zqk object path GOAL-001 CRIT-001

# Query related objects via reference edges:
zqk object related BLI-001
```

---

## 8. Clean Up (Optional)

When an experimental object is no longer needed, remove it cleanly:

```bash
zqk object delete <QUESTION_ID>
```

---

## Summary of Modern vs. Legacy Mutation Patterns

| Operation | 🌟 Preferred Modern Command | ⚠️ Legacy / Low-Level Fallback | Rationale |
| :--- | :--- | :--- | :--- |
| **Object Creation** | `zqk new object <kind> --title "..."` | `zqk object template` + `object create --file` | Safe draft plane isolation; zero CAS pollution. |
| **Lifecycle Advance** | `zqk object promote <id>` | `zqk object update <id> --field status=<st>` | Evaluates VDS done-gates, criteria latches, and FSM rules. |
| **Lifecycle Demote** | `zqk object demote <id>` | `zqk object update <id> --field status=<st>` | Enforces valid backwards state-machine transitions. |
| **Add References** | `zqk object ref add <src> <targ>` | `zqk object update <id> --field "*_ref=<id>"` | Validates target existence, deduplicates, and avoids cycles. |
| **Remove References** | `zqk object ref remove <src> <targ>` | `zqk object update <id> --field "*_refs=..."` | Safely prunes edges without slice parsing errors. |
| **Scalar Field Edits** | `zqk object update <id> --field k=v` | Raw YAML file edits on disk | Best suited for titles, bodies, and descriptions. |

> [!WARNING]
> Do not treat `--allow-degraded` as the default fix — that flag means partial or degraded results are intentionally accepted.


---

## Related Guides & References

- [AI Agent Onboarding Guide](./AI_AGENT_ONBOARDING.md) — Agent directives and continuous loop discipline.
- [Visual Lifecycle State Machines](../architecture/LIFECYCLE_STATE_MACHINE.md) — State machine check-valves and transitions.
- [Object Inspector & Policy Studio Manual](../manual/OBJECT_INSPECTOR_AND_POLICY_STUDIO.md) — TUI inspector and live policy dry-runs.
- [Community First-Run Guide](./COMMUNITY_FIRST_RUN.md) — Complete onboarding and verification.
