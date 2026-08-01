# Managing Kernel State (Git-Efficient System Object Data)

**Status:** Solution documented for future implementation  
**Related:** docs/architecture/ (scheduler_jobs, metrics, caches, etc.), pre-commit, snapshot system, TRAIT_HARNESS.md

## Goal

Minimize git-monitored system object data while keeping all data that lives under `docs/architecture/**` committed to git. Avoid duplicate historical tracking and reduce churn from background scheduler/automation that creates and modifies data. Provide flexible, consistent, maintainable, and performant kernel state handling.

## Problem

- Lots of system object data under `docs/architecture/` (e.g. scheduler_jobs, content-addressed YAMLs) is created and updated by the scheduler and other automation.
- Every change produces new or modified files → noisy git history, large diffs, and merge friction.
- We still need the data in git for reproducibility and audit, but we don’t need to track every small change as separate file revisions.

## Solution (to implement)

### 1. Semantically compressed bundles

- Represent kernel state as **semantically compressed bundles** (e.g. a canonical serialization that groups related data and compresses it).
- Two snapshots of these bundles are enough to understand “what changed” without storing every intermediate revision in git.

### 2. Diff on stage

- **On every stage operation:** Show a diff between two snapshots of the semantically compressed bundles.
- The **diff report** knows how to read the compressed format and render a human- or tool-friendly diff (e.g. what jobs changed, what metrics changed), rather than raw binary or opaque blobs.

### 3. Pre-commit: single snapshot + manifest

- **On pre-commit:** Trigger a snapshot capture and stage:
  - **`kernel-state.csnap`** (or similar): the current semantically compressed bundle of kernel state.
  - A **manifest** with metadata (build, version, timing, etc.) that can be interrogated when necessary to extract version and timing info.
- Git then tracks one (or a few) snapshot file(s) per commit instead of hundreds of small YAMLs. As the scheduler runs in the background and modifies/creates data, those changes are folded into the next snapshot when the user runs pre-commit (or manually captures). Git is not constantly churned by background updates.

### 4. Opt-in / opt-out

- People can **opt in or out** of saving kernel state as part of their commit, or handle it some other way (e.g. commit only code, snapshot only on release, or snapshot on a schedule). This gives flexible workflows and keeps kernel state management under explicit control.

### 5. Segments (optional)

- Add an **option to create segments** so that a single snapshot does not exceed git or filesystem limits (e.g. `kernel-state.csnap.001`, `kernel-state.csnap.002`, … with a segment manifest). The diff report would understand segmented bundles.

## Benefits

- **Efficient:** One (or a few) compressed snapshot(s) per commit instead of many small files.
- **Flexible:** Opt-in/out, manual or pre-commit capture, segment when needed.
- **Consistent:** Same semantics for “what is kernel state” and “how we diff it.”
- **Maintainable:** Diff report and snapshot format are first-class; no duplicate history in git.
- **Performant:** Background scheduler/automation can write freely; git sees updates only when a snapshot is captured and staged.

## Requirement object

A **requirement** (e.g. “Managing Kernel State”) can be created via the CLI to track this work:

- **Title:** Managing Kernel State  
- **Description:** Use this doc as the canonical solution description.  
- Create with: `zqk object create requirement --file <yaml>` (ensure `criteria_refs` and other required fields are set; link to this doc in the requirement description).

## Out of scope for this doc

- Exact format of `.csnap` and manifest (to be designed during implementation).
- Integration points with existing snapshot or backup tooling (if any).
- Policy for when to run snapshot capture (pre-commit hook vs manual vs CI).
