# System Sync Command Architecture

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Active  
**Purpose**: Document the `zqk system sync` command implementation and behavior

## Overview

The `zqk system sync` command provides a simple wrapper around Git operations to synchronize the local zqk project state with remote repositories. It is designed to be a convenient way to pull and push changes while maintaining awareness of the zqk project context.

## Command Structure

```bash
zqk system sync [--pull] [--push] [--verify]
```

### Flags

- `--pull`: Pull changes from the remote repository (default if no flags specified)
- `--push`: Push local changes to the remote repository
- `--verify`: Enable GPG signature verification during sync operations

### Default Behavior

If neither `--pull` nor `--push` is specified, the command defaults to `--pull` to fetch the latest changes from remote.

## Implementation Details

### Architecture

The sync command is a **thin wrapper** around Git's native `git pull` and `git push` commands. It:

1. **Validates Context**: Ensures the current directory is a zqk project (has `.zqk/` or `.zqk/process/` directory)
2. **Checks Git Repository**: Verifies that the project root is a Git repository (has `.git/` directory)
3. **Determines Active Branch**: Uses `git rev-parse --abbrev-ref HEAD` to get the currently checked-out branch
4. **Executes Git Commands**: Runs `git pull` and/or `git push` using Go's `exec.Command`
5. **Handles Errors**: Captures and formats Git command output for user-friendly error messages

### Branch Detection

The sync command determines which branch to sync using Git's default behavior:

1. **Current Branch**: Uses the branch that is currently checked out (HEAD)
   ```bash
   git rev-parse --abbrev-ref HEAD  # Returns: feature/pri-210-phase-4
   ```

2. **Tracking Configuration**: Git uses the branch's upstream tracking configuration:
   - If branch has upstream set: pulls/pushes from/to that remote/branch
   - If no upstream: uses default remote (usually 'origin') and current branch name

3. **No Explicit Branch Selection**: The command does NOT:
   - Allow specifying a different branch via flags
   - Automatically switch branches
   - Sync multiple branches at once

**Example**:
```bash
# Current branch: feature/pri-210-phase-4
# Branch tracking: origin/feature/pri-210-phase-4
zqk system sync --pull
# Executes: git pull
# Git automatically pulls from origin/feature/pri-210-phase-4
```

### Code Flow

```
runSync()
  ├─> Validate zqk project context
  ├─> Check for Git repository (.git directory)
  ├─> Determine operation (pull/push/both)
  ├─> gitPull() or gitPush()
  │    ├─> exec.Command("git", "pull"/"push")
  │    ├─> Set working directory to project root
  │    ├─> Optionally set GIT_VERIFY_SIGNATURES env var
  │    └─> Execute and capture output
  └─> Return success or formatted error
```

### Git Integration

The sync command uses **direct Git command execution** rather than a Git library:

- **Pros**: Simple, reliable, uses system Git configuration
- **Cons**: Requires Git to be installed and in PATH

The command respects:
- Git configuration (`.git/config`)
- Git hooks (pre-push, post-pull, etc.)
- Git credentials and authentication
- Git remotes and branch tracking

### Signature Verification

When `--verify` is specified, the command sets the `GIT_VERIFY_SIGNATURES=true` environment variable. This enables Git's built-in GPG signature verification for commits and tags.

**Note**: The actual verification behavior depends on Git configuration and whether commits/tags are signed. This is a pass-through to Git's native verification.

## Use Cases

### 1. Pull Latest Changes

```bash
zqk system sync --pull
# or simply
zqk system sync
```

Use this to:
- Fetch the latest changes from remote
- Update local branch with remote changes
- Sync after a PR merge or team member push

### 2. Push Local Changes

```bash
zqk system sync --push
```

Use this to:
- Push local commits to remote
- Share work with team
- Update remote after completing work

### 3. Pull and Push

```bash
zqk system sync --pull --push
```

Use this to:
- First pull any remote changes
- Then push local changes
- Ensure you're up-to-date before pushing

### 4. Verify Signatures

```bash
zqk system sync --pull --verify
```

Use this to:
- Pull changes with GPG signature verification
- Ensure commits are signed and verified
- Maintain security and authenticity checks

## Relationship to Other Commands

### `zqk system git`

The `zqk system git` command provides **advanced Git integration**:
- Analyzes commits to extract work item references
- Creates `code_reference` objects for changed files
- Links commits to backlog items, milestones, and goals
- Enables queries like "What code changes support goal X?"

**Difference**: 
- `sync` is a **simple Git wrapper** for pull/push operations
- `git` is a **zqk-aware analyzer** that integrates Git commits with the knowledge graph

### Git Hooks

The sync command respects and triggers Git hooks:
- **Pre-push hook**: Runs before push (can block if issues found)
- **Post-pull hook**: Runs after pull (can trigger actions)
- **Pre-commit hook**: Not triggered by sync (runs on commit)

The zqk pre-commit hook enforces:
- No direct commits to `main` branch
- Documentation registration checks
- System integrity checks

## Error Handling

### Common Errors

1. **Not a zqk project**
   ```
   Error: not a zqk project (no project root found)
   ```
   Solution: Run from a zqk project directory

2. **Not a Git repository**
   ```
   Error: not a git repository
   ```
   Solution: Initialize Git repository with `git init`

3. **Git pull/push failures**
   ```
   Error: failed to pull: <git error message>
   ```
   Solution: Check Git configuration, network, credentials, or merge conflicts

### Error Messages

The command captures Git's `stderr` and `stdout` output and includes it in error messages, providing context about what went wrong.

## No Active Priority Plan

When there is no active or in_progress priority plan:

1. **Branch Management**: Skipped - no branch creation or switching occurs
2. **Warning Message**: A warning is logged suggesting to create a priority plan or use `--message` for commit messages
3. **Auto-Commit**: If `--auto-commit` is used without a plan, the command fails with an error requiring `--message`
4. **Other Operations**: Continue to work normally:
   - `--stage`: Still stages changes
   - `--commit --message`: Still commits with provided message
   - `--pull` / `--push`: Still syncs with remote

**Example**:
```bash
# Without priority plan - auto-commit fails
zqk system sync --auto-commit
# Error: cannot auto-generate commit message without active priority plan. Use --message to provide a commit message

# Without priority plan - manual message works
zqk system sync --stage --commit --message "Fix bug" --push
# Works: stages, commits with provided message, pushes
```

## System Health Check Enforcement

The `sync` command enforces system health checks before commit/push operations, per **POL-CODE-004** and **POL-CODE-005**:

### Automatic Health Checks

- **Enabled by default**: System check runs automatically before any commit or push operation
- **Tier 1 (Blocking) violations**: Zero tolerance - commit/push is blocked if any are detected
- **Tier 2 (Warnings) threshold**: Warns if ≥5 violations are detected (does not block)
- **Bypass option**: Use `--skip-check` to bypass (not recommended)

### Policy Compliance

- **POL-CODE-004**: "All system check violations must be resolved before committing code"
- **POL-CODE-005**: "Zero tolerance for Tier 1 violations - they block operations"

### Examples

```bash
# Normal usage - health check runs automatically
zqk system sync --stage --auto-commit --push
# ✅ If no violations: proceeds with commit/push
# ❌ If Tier 1 violations: blocks with error message

# Bypass health check (not recommended)
zqk system sync --stage --commit --message "Fix" --skip-check --push

# Dry run shows health check would run
zqk system sync --stage --auto-commit --push --dry-run
# [DRY RUN] Would run system health check (POL-CODE-004, POL-CODE-005)
```

### Error Messages

When Tier 1 violations are detected:
```
❌ Tier 1 (Blocking) violations detected: 3 total (2 public, 1 internal)
POL-CODE-004: All system check violations must be resolved before committing
POL-CODE-005: Zero tolerance for Tier 1 violations

To view details:
  zqk system check --verbose

To resolve:
  zqk system check --auto-fix --force
```

## Pull Request / Merge Request Creation

The `sync` command can automatically create Pull Requests (GitHub) or Merge Requests (GitLab) per **POL-WORKFLOW-002**:

### Automatic PR/MR Creation

- **Platform Detection**: Automatically detects GitHub or GitLab from remote URL
- **Auto-Generated Content**: PR/MR title and body generated from priority plan information
- **Backlog Item Verification**: Warns if not all backlog items are complete (per POL-WORKFLOW-002)
- **Requires CLI Tools**: GitHub (`gh`) or GitLab (`glab`) CLI must be installed

### Usage

```bash
# Create PR/MR after push (auto-detects platform)
zqk system sync --stage --auto-commit --push --create-pr

# Custom PR/MR title and body
zqk system sync --push --create-pr --pr-title "Custom Title" --pr-body "Custom body"

# Dry run to preview
zqk system sync --push --create-pr --dry-run
```

### PR/MR Content Generation

When `--pr-title` and `--pr-body` are not provided, the command generates them from the current priority plan:

**Title Format**: `{PLAN_ID}: {PLAN_TITLE}`  
**Body Includes**:
- Priority plan title and description
- Context information
- Backlog items summary (total, completed, remaining)
- Footer indicating automated creation

### Policy Compliance

- **POL-WORKFLOW-002**: "Pull requests should be opened using the `gh` CLI upon the successful completion of all backlog items linked to a priority plan"
- **Verification**: Warns (but doesn't block) if backlog items are incomplete
- **One PR per Plan**: PRs are created per priority plan, not per backlog item

### Platform Support

- **GitHub**: Uses `gh pr create` command
- **GitLab**: Uses `glab mr create` command
- **Detection**: Automatically detects platform from `git remote get-url origin`

### Examples

```bash
# Complete workflow: stage, commit, push, and create PR
zqk system sync --stage --auto-commit --push --create-pr

# Just create PR (assumes branch already pushed)
zqk system sync --create-pr

# Custom PR details
zqk system sync --push --create-pr \
  --pr-title "PRI-210: Phase 4 Complete" \
  --pr-body "All backlog items for PRI-210 are complete."
```

## Future Enhancements

Potential improvements:

1. **Conflict Resolution**: Detect and help resolve merge conflicts
2. **Status Reporting**: Show what changed during pull/push
3. **Selective Sync**: Sync specific object kinds or directories
4. **Integration with `zqk system git`**: Automatically analyze pulled commits
5. **Fallback Branch Naming**: Use current branch or generate from context when no plan exists
6. **PR Templates**: Support for PR/MR templates
7. **Reviewer Assignment**: Auto-assign reviewers based on priority plan

## Testing

Tests are located in `cmd/zqk/system/sync_test.go`:

- `TestNewSyncCmd`: Validates command structure
- `TestSyncCommandFlags`: Validates flag definitions
- `TestSyncRequiresGitRepository`: Validates Git repository detection
- `TestSyncWithGitRepository`: Integration test with actual Git repository

## Related Documentation

- [Git Workflow Enforcement](../policies/POL-WORKFLOW-001.yaml)
- [AI Agent Onboarding](../../onboarding/AI_AGENT_ONBOARDING.md)
- [System Commands Architecture](cli-ontology-v1.0.md)

