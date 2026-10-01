# How-To: Inspect, Link, and Validate Knowledge Kernel Objects

<!-- tags: howto, inspect, validate, ontology, references, criteria, test-cases, vds -->

This practical guide provides step-by-step recipes for inspecting objects, establishing semantic graph relationships, advancing lifecycle states, authoring validation policy rules, and verifying Definition of Done (DoD) compliance.

> [!TIP]
> **End-to-End Orchestration Walkthrough**:
> For a complete, step-by-step narrative showing how human intent (*"Build X with expectations Y and constraints Z"*) transforms into a full roadmap, goals, requirements, criteria, test cases, and autonomous multi-agent swarm execution, see:  
> **[From Vision to Code: Complete Multi-Agent Orchestration Walkthrough](../guides/VISION_TO_EXECUTION_ORCHESTRATION_GUIDE.md)**.


---

## Recipe 1: Link and Unlink Relationships (`object ref add` / `object ref remove`)

Connecting objects across the ontology graph is a first-class operation. Always use schema-aware reference commands rather than low-level scalar `--field` modifications.

### Preferred Modern Pattern:
```bash
# 1. Link a requirement to a backlog item (automatically maps to requirement_refs):
zqk object ref add BLI-001 REQ-001

# 2. Link multiple criteria to a requirement atomically:
zqk object ref add REQ-001 CRIT-001 CRIT-002

# 3. Explicitly target a specific reference field when needed:
zqk object ref add BLI-001 PRI-001 --field priority_plan_ref

# 4. Remove a reference relationship cleanly:
zqk object ref remove BLI-001 REQ-001
```

> [!TIP]
> `zqk object ref add` automatically checks that the target object exists in CAS, prevents duplicate entries in array slices, and guards against cyclic dependencies on `related_object_refs`.

---

## Recipe 2: Advance Object Lifecycle States (`object promote` / `demote` / `park`)

Every kernel object kind is governed by an authoritative finite state machine (e.g. `originated` ➔ `planned` ➔ `in_progress` ➔ `validated` ➔ `complete`). Never use `object update --field status=<status>` for lifecycle progression.

### Preferred Modern Pattern:
```bash
# 1. Promote an object to its next valid lifecycle state:
zqk object promote BLI-001

# 2. Promote multiple objects concurrently:
zqk object promote BLI-001,BLI-002,BLI-003

# 3. Demote an object if QA verification uncovers rework:
zqk object demote BLI-001

# 4. Park an object along an intentional lifecycle exit:
zqk object park BLI-001
```

> [!IMPORTANT]
> `object promote` evaluates all entry preconditions and required acceptance criteria before advancing. If preconditions fail, it prints diagnostic receipts rather than leaving state corrupted.

---

## Recipe 3: Inspect an Object as an Autonomous Agent

When an AI agent needs to inspect a task, backlog item, or requirement without consuming unnecessary LLM context window tokens:

```bash
# Request agent-optimized semantic JSON projection
zqk object inspect backlog_item BLI-001 -f json
```

**Expected Result:**
```json
{
  "kind": "backlog_item",
  "id": "BLI-001",
  "status": "in_progress",
  "priority": "P0",
  "title": "Implement pure-Go CAS storage backend",
  "storage_profile": {
    "storage_plane": "cas",
    "cas_hash": "e3b0c442...",
    "byte_size": 1420
  },
  "lineage": {
    "requirement": "REQ-001",
    "priority_plan": "PRI-001",
    "is_intact": true
  },
  "criteria_summary": {
    "total": 3,
    "satisfied": 2,
    "pending": 1
  }
}
```

---

## Recipe 4: Launch the Full Interactive Object Inspector

For human developers exploring the graph interactively:

```bash
zqk object inspect
```

1. **Cycle Kinds**: Press `[Tab]` to cycle across `backlog_item`, `requirement`, `criteria`, `test_case`, `milestone`, `priority_plan`, and `policy`.
2. **Filter**: Press `[f]` to filter to `[ACTIVE]`, `[MINE]`, or `[BLOCKED]`.
3. **Search**: Press `[/]`, type a query (e.g. `auth`), and press `[Enter]`. Use `[n]` / `[N]` to navigate matches.
4. **Drill Down**: Press `[Enter]` on any row to open the deep inspection modal.
5. **Adjust Display Density**: Press `[z]` to switch between `newb` (full headers & hints), `pro` (compact single-line), and `jedi` (zen mode with maximum rows).

---

## Recipe 5: Write and Test a Policy Rule Live

To test new validation rules against the current repository before committing:

1. Launch directly into the Policy Studio:
   ```bash
   zqk object inspect backlog_item --policy-studio
   ```
2. Navigate to an existing rule with `[j]/[k]`, or create a custom rule.
3. Press `[c]` to enter **DSL Edit Mode**.
4. Type your condition:
   ```
   status != "" && len(requirement_refs) > 0
   ```
   - Press `[Tab]` to trigger dynamic autocompletion for schema field tokens.
5. Press `[Enter]` to apply.
6. Press `[t]` to run a live dry-run evaluation across all active objects. The status line will report pass/violation counts immediately.

---

## Recipe 6: Verify Definition of Done in Mission Control & Test Dashboard

To verify downward traceability and test coverage across the entire project:

```bash
# 1. Launch Mission Control on QA Tab
zqk ui --tab qa

# 2. Or run the terminal Test & DoD Verification Dashboard
zqk test dashboard --check-dod
```

- Review the **DoD Vitals Card**:
  - `Traceability DoD`: Must show `100% Intact [PASS]`.
  - `Intact Chains`: Confirms test case lineage to root objects.
  - `Unbound Criteria`: Must be `0 [OK]`.
- Press `[t]` at any time to re-scan the test matrix and refresh test case execution states.
- Press `[Enter]` on any test suite row to inspect its exact bound criteria, test target file, and lineage chain.

---

## Modern vs. Legacy Mutation Cheatsheet

| Intent | 🌟 Preferred Modern Method | ⚠️ Legacy / Unstructured Fallback |
| :--- | :--- | :--- |
| **Link objects** | `zqk object ref add <SRC> <TARG>` | `zqk object update <SRC> --field "<k>_refs=<ID>"` |
| **Unlink objects** | `zqk object ref remove <SRC> <TARG>` | `zqk object update <SRC> --field "<k>_refs=..."` |
| **Advance status** | `zqk object promote <ID>` | `zqk object update <ID> --field status=<ST>` |
| **Demote status** | `zqk object demote <ID>` | `zqk object update <ID> --field status=<ST>` |
| **Edit attributes**| `zqk object update <ID> --field title="..."` | Direct file editing in `.zqk/process/` |

---

## Related References
- [Mission Control UI & Diagnostics Visual Guide](../manual/MISSION_CONTROL_UI_VISUAL_GUIDE.md) — Comprehensive visual walkthrough of all 8 TUI tabs and Web Studio.
- [Object Inspector & Policy Studio Manual](../manual/OBJECT_INSPECTOR_AND_POLICY_STUDIO.md) — Complete keybinding reference and modal architecture.
- [First-Run Object Tutorial](../onboarding/FIRST_RUN_OBJECT_TUTORIAL.md) — Step-by-step object lifecycle walkthrough.
