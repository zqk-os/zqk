# Auto-Fix Engine & Remediation DAG (`pkg/systemcheck/autofix`)

`pkg/systemcheck/autofix` provides the decoupled auto-fix rule loading, template placeholder substitution, and hierarchical remediation dependency ordering for ZQK system checks and validation pipelines.

## Responsibilities

1. **AutoFixRuleLoader**:
   - Lazily loads and caches `auto_fix_rule` objects per project root from the storage provider.
   - Evaluates rule applicability based on target object kind, category, condition tier, specific rule identifier, and error message substrings.
   - Performs template placeholder substitution (e.g. `{object_id}`, `{field}`, `{kind}`, `{message}`, `{rule}`, `{tier}`, `{category}`).
   - Deterministically selects the highest priority rule (lowest numerical value) when multiple matching rules are found.
   - Supports thread-safe cache invalidation per project root.

2. **Hierarchical Remediation DAG (`FixDependencyOrder`)**:
   - Defines the ontological dependency hierarchy for reference repairs:
     - `goal_refs` (Priority 4): Goals are root objects with fewest dependencies.
     - `priority_plan_ref` (Priority 3): Priority plans structure program delivery.
     - `milestone_refs` / `workstream_refs` (Priority 2): Milestones and workstreams depend on plans/goals.
     - `backlog_item_refs` (Priority 1): Backlog items depend on milestones, plans, and goals.
   - Sorts validation issues so that upstream root entities are remediated before downstream dependent entities, avoiding thrashing and cascade failures.
   - Secondary sorting orders by severity tier (Tier 1 blocking issues before Tier 2 warnings).

## Usage

```go
import "github.com/zqk-os/zqk/pkg/systemcheck/autofix"

// Lookup singleton rule loader
loader := autofix.GetAutoFixRuleLoader()

// Match fix command for an issue
cmd, ok := loader.GetFixCommand(
    projectRoot,
    storageProvider,
    kind,
    category,
    tier,
    rule,
    message,
    objID,
    field,
    objMap,
)

// Sort issues by dependency hierarchy
sorted := autofix.SortIssuesByDependency(issues)
```
