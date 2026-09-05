# Project Root Wipe Hazards

This document ranks and inventories the hazards associated with unguarded `RemoveAll` operations within the ZQK project root.

## Hazard Inventory

1. **`os.RemoveAll(projectRoot)`**: The highest severity hazard. Agents or idle cleanup routines attempting to wipe what they perceive as a "temporary worktree" may inadvertently wipe the primary repository if path resolution fails or defaults to the primary project root.
2. **`os.RemoveAll(".zqk")`**: Wipes the kernel's state, scheduler data, agent chat channels, and mesh seating. Results in complete loss of agent context and scheduler continuity.
3. **`os.RemoveAll(".git")`**: Destroys the repository's history and configuration.

## Prevention

All operations that recursively remove directories must pass through `paths.MustNotDestroyProjectRoot(projectRoot, targetPath)`.
This guard function will return an error (`PROJECT_ROOT_WIPE_HAZARD`) if the target path evaluates to the project root, `.git`, or `.zqk`.

## Agent Rule

Agents must NEVER use `rm -rf /`, `rm -rf ./`, or unguarded `os.RemoveAll` on paths that could resolve to the project root. Always use `paths.MustNotDestroyProjectRoot` before programmatically deleting worktrees or directories.
