# CLI Session Reuse and Auth Integration v1.0

**Last Verified:** 2026-08-31


**Status:** Design  
**Version:** 1.0  
**Last Updated:** 2026-02

## Summary

This document describes the target model for **reusable CLI sessions**: one session ID spanning many CLI operations, with lifecycle driven by login/logout and configurable idle timeout and permission-driven invalidation. It extends the current “one session per invocation” behavior and aligns with future login/logout flows.

## Current State

- **One session per CLI run:** PreRunE creates a `zqk_session` (status `active`), PostRunE sets it to `completed` or `error`. Session ID is in command context only; no persistence across invocations.
- **No login/logout:** CLI runs with system or ambient identity; no explicit “log in” or “log out” today.
- **No idle timeout or revocation:** Session ends when the process exits.

## Target Model

### 1. Reuse one session across many CLI operations

- **Persist the “current” session** for the user/environment (e.g. in `.zqk/state/session` or `ZQK_SESSION_ID` env) so that the next `zqk` invocation **reuses** the same session ID instead of creating a new one.
- **Lifecycle:** Session is **created** at login (or first use after bootstrap); **activated** when it becomes the current session; **deactivated** on logout or when superseded by another session.
- **Attribution:** Every command and audit event carries `session_id` (and thus can be tied to an account) so downstream systems know “who is responsible” for each operation.

### 2. Login / Logout flows

- **Login:** Authenticate (e.g. username/password, PAT, or SSO); on success, **create** a new session (or **activate** an existing one for that account). Set it as the current session (persist ID) and optionally set `account_id` / `created_by` on the session.
- **Logout:** **Deactivate** the current session (e.g. set status to `completed` or `logged_out`). Clear persisted session ID so the next run does not reuse it; optionally require login again for sensitive commands.
- **Session create/activate/deactivate** map naturally to `zqk_session` lifecycle (e.g. statuses or transitions like `active` ↔ `completed` / `logged_out`).

### 3. Configurable idle timeout → re-auth

- **Config:** e.g. `session.idle_timeout` (or `auth.idle_timeout`) in project/config – duration with no activity after which the session is considered idle.
- **Activity:** Any CLI command run in that session updates “last activity” (e.g. `updated_at` on the session or a dedicated `last_activity_at` field).
- **Behavior:** If the last activity is older than the configured idle timeout when a command runs, **prompt for re-auth** (or treat session as expired and require login again) instead of reusing the session. Optionally deactivate the session so it cannot be reused until re-authentication.
- **Result:** Balances convenience (reuse across invocations) with security (stale sessions must re-authenticate).

### 4. Privilege / permission changes – quick shutdown or enable

- **Revoke access:** When permissions are reduced or an account is disabled, **invalidate** that account’s sessions (e.g. set status to `revoked` or `error`, or mark “invalid after T”). CLI checks session validity at start of run; if invalid, require re-login and do not run the command until re-authenticated. Enables “shut down access quickly” without waiting for idle timeout.
- **Enable new access:** When permissions are granted, existing sessions can either (a) **pick up new permissions** on next command (e.g. re-load roles/permissions from account when using the session), or (b) require a fresh login to get a new session with updated privileges. (a) is better for “enable new access quickly” without forcing logout.

### 5. Bounds (per account)

- **One active session per account** (or “current” session per environment) to avoid ambiguity.
- **Cap total sessions per account** (e.g. 5); when creating a new session, archive or remove oldest so the set does not grow unbounded.

## Implemented (current)

- **Persistence:** Session ID is read from **ZQK_SESSION_ID** (env) first, then from `.zqk/state/session` (file). Env ties the session to a shell/process tree; file is shared across terminals. **File lock** (`.zqk/state/session.lock`) ensures consistent read/write across processes so multiple terminals don’t race.
- **Login command:** `zqk auth login` – creates or reuses a session, writes to file under lock, prints session ID and hints “export ZQK_SESSION_ID=...”.
- **Logout command:** `zqk auth logout` – ends current session (status completed), clears file under lock, hints “unset ZQK_SESSION_ID”.
- **Idle timeout:** Config key **`session.idle_timeout`** (e.g. `"30m"`, `"24h"`) in `.zqk/config/config.yaml` or `.zqk/config.yaml`. Before reusing a session, `updated_at` is compared to `now - idle_timeout`; if expired, session is closed and file cleared so the next run gets a new session (no prompt yet; full re-auth can be added later).
- **PreRunE:** Skips session start for `auth login` and `auth logout` so login owns create/reuse and logout never creates a session.

## Implementation Hooks (future)

- **Validity check:** Before using a session, ensure it is not `revoked` / invalid; if account was disabled or permissions reduced, invalidate session and require re-login.
- **Config:** `session.max_per_account`, and (if needed) `session.allow_reuse` or similar.
- **Re-auth prompt:** When idle-expired, optionally prompt for password/login instead of silently creating a new session.

## Relationship to other docs

- **BLI-952 (Auth plan):** MCP and scheduler auth (passwords, PAT, etc.). Login/logout and session reuse are the CLI-facing complement: once auth exists, login creates/activates a session and logout deactivates it; session carries account and is reused until logout or idle timeout or revocation.
- **zqk_session spec/lifecycle:** Add fields as needed (e.g. `account_id`, `last_activity_at`, or status values like `logged_out` / `revoked`) and transitions for activate/deactivate/revoke.

## Session touch storm (many invocations, one session)

When the same session is reused across **many short-lived CLI invocations** (e.g. a loop running `zqk object delete <id>` hundreds of times), each invocation touches the session in PostRunE → one storage Update per command → WAL and CAS churn.

**Options (implemented or considered):**

1. **Throttle session touch (implemented):** Before touching, check a shared state file (`.zqk/state/last_session_touch`) under the session lock; if the last touch was within a short window (e.g. 5s), skip the touch. At most one touch per window per project. Idle timeout still works.
2. **Prefer bulk operations:** Use `zqk object bulk delete --file ids.yaml` instead of looping single deletes. Documented in PRE_COMMIT_BACKGROUND_RESULTS.md and .cursor/rules (object-bulk-delete-not-loop).
3. **Storage-side debounce (not implemented):** For `zqk_session` updates that only change `updated_at`/`title`, skip persisting if the object was updated very recently.
4. **Batch / no-session-touch mode (not implemented):** A flag to disable session create/touch for scripted runs.

## Out of scope for this doc

- Concrete UI for “prompt re-auth” (e.g. interactive vs non-interactive).
- Exact storage format for persisted session ID or config keys (can be refined in implementation).
