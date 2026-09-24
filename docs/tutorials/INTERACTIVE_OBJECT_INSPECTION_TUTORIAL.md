# Tutorial: Interactive Object Inspection & Policy Authoring

In this hands-on tutorial, you will explore the ZQK Knowledge Kernel using the **Object Inspector**, drill down into object lineage, write a live policy validation rule using the **Policy Studio**, and verify release readiness in **Mission Control**.

---

## Prerequisites
Ensure the ZQK binary is built and present on your path:
```bash
go build -o ./bin/zqk ./cmd/zqk
./bin/zqk system check
```

---

## Step 1: Launch the Object Inspector

Run the inspector for backlog items:
```bash
./bin/zqk object inspect backlog_item
```

Observe the interface:
- **Header**: Shows current kind, total object count, active filter pill (`[ALL]`), and sort mode (`updated_at`).
- **Table**: Formats essential columns (`ID`, `STATUS`, `PRIORITY`, `OWNER`, `TITLE`) with TDS alignment.
- **Property Card**: Displays CAS storage details, ontology metadata, and lineage links for the currently highlighted row.
- **Footer**: Displays keyboard navigation hints.

Try cycling your experience profile:
- Press `[z]` to switch to **PRO** mode (compact single-line headers).
- Press `[z]` again to switch to **JEDI (Zen)** mode (maximum terminal data space).
- Press `[z]` once more to return to **NEWB** mode.

---

## Step 2: Search, Filter, and Drill Down

1. Press `[/]` to activate inline search.
2. Type `inspect` and press `[Enter]`.
3. Notice only rows matching the query are visible.
4. Use `[j]` and `[k]` to navigate to a row of interest.
5. Press `[Enter]` to open the **Deep Object Inspection Modal**:
   - Note the **Lineage & Traceability Radar** showing upstream requirements, milestones, and priority plans.
   - Note the **CAS Storage Profile** displaying byte size, storage plane (`cas`), and content hash.
6. Press `[Esc]` to close the modal.
7. Press `[Esc]` again to clear the search filter.

---

## Step 3: Open the Live Policy Rule Studio

1. Press `[p]` to open the **Policy Rule Studio**.
2. Notice the default rules loaded for `backlog_item`:
   - `POL-INTEGRITY-LINEAGE-001`: Asserts requirements and milestones are linked.
   - `POL-EFFORT-VALIDATION-001`: Validates effort estimates.
3. Press `[c]` to enter **DSL Edit Mode**.
4. Type `status == "in_progress" && `
5. Press `[Tab]` to view autocomplete suggestions from the schema field registry.
6. Select a field (e.g. `claimed_by`) and finish the expression:
   `claimed_by != ""`
7. Press `[Enter]` to commit the rule.
8. Press `[t]` to trigger a dry-run evaluation across all repository objects.
9. Press `[Esc]` to exit Policy Studio.

---

## Step 4: Verify Definition of Done in Mission Control

Finally, verify that your changes satisfy all criteria in Mission Control:
```bash
./bin/zqk ui --tab qa
```

1. Tab 7 (`🧪 QA`) displays the test suites table and Definition of Done card.
2. Press `[t]` to trigger an on-demand re-scan of the test matrix.
3. Observe the dynamic notification bar:
   `🧪 QA test matrix rescanned (14 test suites, 27 criteria)`
4. Press `[q]` to exit.

---

## Summary
You have successfully:
- Navigated the terminal Object Inspector using Vim keys and filter pills.
- Deep-inspected object lineage and CAS storage metadata.
- Authored and dry-run evaluated a custom validation DSL rule with autocomplete.
- Verified test matrix traceability and Definition of Done compliance.
