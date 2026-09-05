# Git Workflow Enforcement via Pre-commit Hook

**Last Verified:** 2026-08-31


**Version**: 1.0  
**Status**: Implementation  
**Date**: 2025-12-29  
**Related**: REQ-9006, AI_AGENT_ONBOARDING.md

## Overview

This document describes the pre-commit hook implementation that enforces the git workflow policy, preventing direct commits to the main branch.

## Problem Statement

The workflow policy states:
> "There will never be a time when an AI agent commits code directly to the main branch unless prior authorization has been granted/established."

However, AI agents were committing directly to main, violating this policy. A technical enforcement mechanism was needed.

## Solution

A pre-commit hook was added that:
1. Checks the current branch name before allowing commits
2. Blocks commits to `main` branch
3. Provides clear error message with instructions
4. Allows bypass via `ZQK_ALLOW_MAIN_COMMIT` environment variable for authorized operations

## Implementation

### Hook Location
- **Path**: `tools/git-hooks/pre-commit`
- **Installation**: Configured via `git config core.hooksPath tools/git-hooks`

### Hook Logic

```bash
# Get current branch
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)

# Check if committing to main
if [ "$CURRENT_BRANCH" = "main" ]; then
    # Block unless authorized
    if [ -z "$ZQK_ALLOW_MAIN_COMMIT" ]; then
        echo "❌ ERROR: Direct commits to main branch are not allowed!"
        # ... error message with instructions ...
        exit 1
    fi
fi
```

### Installation

The hook is installed by setting:
```bash
git config core.hooksPath tools/git-hooks
```

This should be done:
- During project setup
- Documented in onboarding
- Verified in CI/CD

### Bypass Mechanism

For authorized operations (e.g., emergency fixes, human operations):
```bash
export ZQK_ALLOW_MAIN_COMMIT=1
git commit -m "Authorized commit to main"
```

## Workflow Enforcement

The hook enforces:
- ✅ All development in feature branches (feature/, chore/, fix/, release/)
- ✅ Feature branch established per priority plan
- ✅ Feature branch accumulates all changes for that priority plan
- ✅ Sub-branches allowed but must merge back into feature branch
- ✅ PR opened when priority plan is complete (not per backlog item)
- ✅ Changes merged via pull request
- ✅ AI agents cannot commit directly to main unless authorized

### Priority Plan-Based Workflow

The desired workflow is:
1. **Establish Feature Branch**: Create feature branch per priority plan
   ```bash
   git checkout -b feature/priority-plan-name
   ```

2. **Accumulate Changes**: All backlog items for the priority plan are worked on this branch
   - Sub-branches can be created for parallel work
   - Sub-branches must merge back into the feature branch
   - All changes accumulate in the feature branch

3. **Complete Priority Plan**: When all backlog items are complete
   - Open pull request from the feature branch
   - All changes must be in the feature branch used for the PR

4. **Post-Merge**: After PR is merged by human
   - Agent syncs local main: `git checkout main && git pull`
   - Agent creates new feature branch for next priority plan

## Testing

### Test 1: Block Commits to Main
```bash
git checkout main
echo "test" > test.txt
git add test.txt
git commit -m "test"  # Should be blocked
```

### Test 2: Allow Commits to Feature Branches
```bash
git checkout -b feature/test
echo "test" > test.txt
git add test.txt
git commit -m "test"  # Should succeed
```

### Test 3: Bypass for Authorized Operations
```bash
git checkout main
export ZQK_ALLOW_MAIN_COMMIT=1
echo "test" > test.txt
git add test.txt
git commit -m "test"  # Should succeed with bypass
```

## Integration with Existing Hooks

The branch check runs **before** linting, ensuring:
1. Workflow compliance is checked first
2. Linting only runs if workflow is compliant
3. Clear error messages guide users to fix workflow issues

## Future Enhancements

1. **Server-side Hook**: Add server-side hook (pre-receive) for additional enforcement
2. **Branch Pattern Validation**: Validate branch naming conventions
3. **Commit Message Validation**: Ensure commit messages follow conventions
4. **Work Item References**: Validate that commits reference work items when appropriate

## References

- `docs/onboarding/AI_AGENT_ONBOARDING.md` - Workflow policy
- `docs/process/requirements/REQ-9006.yaml` - Requirement
- `tools/git-hooks/pre-commit` - Implementation
