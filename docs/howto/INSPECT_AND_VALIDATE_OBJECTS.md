# How-To: Inspect and Validate Knowledge Kernel Objects

This guide provides step-by-step recipes for inspecting kernel objects, authoring validation policy rules, and verifying Definition of Done compliance.

---

## Recipe 1: Inspect an Object as an Autonomous Agent

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
  "priority": "high",
  "title": "Core Implementation Task",
  "storage_profile": {
    "storage_plane": "cas",
    "cas_hash": "a1b2c3d4...",
    "byte_size": 1240
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

## Recipe 2: Launch the Full Interactive Object Inspector

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

## Recipe 3: Write and Test a Policy Rule Live

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

## Recipe 4: Verify Definition of Done in Mission Control

To verify downward traceability and test coverage across the entire project:

```bash
# Launch Mission Control on QA Tab
zqk ui --tab qa
```

- Review the **DoD Vitals Card**:
  - `Traceability DoD`: Must show `100% Intact [PASS]`.
  - `Intact Chains`: Confirms test case lineage to root objects.
  - `Unbound Criteria`: Must be `0 [OK]`.
- Press `[t]` at any time to re-scan the test matrix and refresh test case execution states.
- Press `[Enter]` on any test suite row to inspect its exact bound criteria, test target file, and lineage chain.
