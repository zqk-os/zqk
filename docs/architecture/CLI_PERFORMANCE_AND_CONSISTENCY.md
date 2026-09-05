# CLI performance and consistency

**Last Verified:** 2026-08-31


**Version:** 1.1  
**Status:** Active  
**Purpose:** Authoritative policy for CLI performance contracts, caching rules, and definition of "works." Use in design and review. If something is unclear, prompt human input to establish it.

This version adds guidance for the spec index cache and CLI completions.

---

## 1. Performance contracts

**All CLI operations must be:** accurate, efficient, observable, and consistent.

**Response time:** In most cases, a CLI command should return some type of response within **1 second max**.

**Design approach:**

- Decide **what operations need to happen immediately** vs **what can happen in the background** for eventually consistent results.
- Decide **what to respond with** so the CLI user knows what is going on.
- If a long-running process must be waited on, it should be **async with progress notifications** at least every **1 second** (or on some other reasonable interval) so the user is not left wondering if something is hung.

---

## 2. Architecture rules: caching (including spec index)

**Everything that can be cached, should be cached.** Caching must be **efficient** and **deterministic**.

**When to re-populate caches:**

- **Full re-population** only when:
  - explicitly **requested** by the user, or
  - a **catastrophic failure** has happened where the overall state of the cache is questionable or unknown.
- Otherwise, caches must be **incrementally maintained** (update/invalidate on create/update/delete; no full clear on every operation).

**Full cache population:** At most, a full cache population should happen **once during scheduler initialization** — or when explicitly requested by the user. Not on every CLI create, list, or check.

**Spec index cache (`spec_index.json`):**

- **Source of truth:** The spec index is a generated, immutable snapshot of **fields by kind**, including traits (groupable/filterable/sortable), enums, and key validation hints. It is built from `docs/process/_internal/object_specs` via `zqk system generate-spec-index` (PRUNED) and written under `docs/process/_internal/spec_index.json` (resolved via path-cache).
- **Usage:**
  - CLI helpers (e.g. `object fields --list-kinds`, future completion backends) **MUST prefer the spec index** when present (via `TryLoadSpecIndexForProjectRoot`) instead of re-walking specs or calling `FieldRegistry.LoadFields()` on hot paths.
  - Validators and ID-cache builders **SHOULD** use the index for field metadata, reference detection (`*_ref`), and trait-based capabilities (groupable/filterable/sortable) rather than repeating spec traversal logic.
  - Shell completions **SHOULD** use the index to decide which fields to offer for flags such as `--group-by`, `--filter`, and `--sort`, and to surface constraints (enum values, numeric ranges, time formats) in the suggestion description.
- **Rebuild policy:**
  - Generating the spec index is a **background / maintenance** operation (`system generate-spec-index`) or part of bootstrap; CLI hot paths **must not block** on building it.
  - When the index is missing or stale, commands fall back to existing behavior (e.g. `FieldRegistry`), and may **trigger generation asynchronously** but still return quickly.

**Quick I/O:** For things that are quick single-file or single-value reads, caching may not be required; but if the cost is small, cache it.

---

## 3. CLI completions and role-aware views (high level)

**Completions are subject to the same performance and safety rules as other CLI features.**

- **Spec-driven:** Completion metadata comes from the spec index (kinds, fields, traits, constraints), not from ad-hoc logic.
- **Role-aware:** Suggested kinds and fields must already be visible/allowed for the current account/role; the completion layer filters the spec index through existing permission checks before presenting options.
- **Constraint hints:** When suggesting fields or values, completions should include concise constraint hints derived from the spec index:
  - Enum values (`status (enum: pending, active, complete)`),
  - Numeric ranges (`estimate_days (numeric, 0–365)`),
  - Length constraints where canonical storage requires them (`operation (string, max 500 chars)`),
  - Time/date formats (`created_at (timestamp, RFC3339)`), etc.
- **Value completion:** For enum-like and tightly constrained fields, filter completions SHOULD suggest valid values (e.g. `status=pending`, `status=active`) in addition to field names, using the same spec-index metadata (enum values, ranges, formats).
- **Parity:** Internal commands (e.g. `zqk internal list/count`) MUST eventually offer the same spec-driven completion semantics as user-facing object commands for shared kinds and traits.
- **No heavy work on TAB:** Completion handlers must not trigger expensive spec walks or cache rebuilds; they should read the spec index (or fall back to in-memory registries) and return quickly.

---

## 4. Definition of "works"

**"Works" means:**

- Logic is **correct**.
- **Coding best practices** are followed.
- **Established architecture policies** are considered and followed (including this document and related architecture docs).
- Both **functional** and **non-functional** requirements are met.

If non-functional requirements (e.g. latency, throughput, observability) are not clearly articulated, prompt the human to establish them before treating a feature as complete.

