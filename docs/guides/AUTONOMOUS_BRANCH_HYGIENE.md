# Autonomous Git Integration Branch Hygiene & Pruning

## Overview

The Autonomous Branch Hygiene Engine (`pkg/gitplumbing`) automates git branch lifecycle validation, ahead/behind divergence measurement, and merged integration branch pruning. It enforces the Continuous Autonomous Loop (CAP Loop) branch hygiene invariant (**POL-WORKFLOW-003**) to prevent stale branches, reduce CI queue bloat, and maintain an authoritative linear history.

## Architectural Contracts

### Integration Branch Naming Convention
All time-bounded execution cycles operate on dedicated integration branches matching the canonical prefix:
```
integration/PRI-<nanos>-<hash>
```

### 3-Phase Lifecycle
1. **Creation & Tracking:** Branched directly from `origin/main` (`git checkout -b integration/<pri-id> origin/main`). Never branched off stale local branches.
2. **Execution & Promotion:** Backlog items commit atomic mutations referencing their respective `BLI-*` and `PRI-*` IDs. Auto-merge (`gh pr merge <pr-number> --auto --squash`) is enabled upon PR creation.
3. **Automated Convergence & Pruning:**
   - Once squash-merged into `main`, the local engine identifies the branch as fully merged via `git merge-base --is-ancestor`.
   - Pruning refuses to delete the currently active branch or `main`.
   - Dry-run validation is supported before live deletion (`git branch -D`).

## Programmatic API

```go
engine, err := gitplumbing.NewBranchHygieneEngine(repoRoot, "origin", "main")
if err != nil {
    return err
}

// Evaluate branch convergence
result, err := engine.EvaluateConvergence(ctx, "integration/PRI-1793571658840339000-3901b8b5")
if result.CanPrune {
    pruned, err := engine.PruneMergedBranches(ctx, false)
}
```
