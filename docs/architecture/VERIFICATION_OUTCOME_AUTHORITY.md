# Verification outcome authority (criteria and convergence_session)

## Purpose

- **`criteria`:** Lifecycle statuses **`validated`** and **`complete`** are verification outcomes. They must reflect **durable pipeline or system-automated** results, not ad hoc manual edits under a normal user account.
- **`convergence_session`:** Lifecycle status **`error`** is marked **system-managed** in the lifecycle spec. Only the system account should set this status (validation failures, tick errors, etc.).

Manual closure of convergence work (**`completed`**, **`abandoned`**, **`escalated`**) remains available to normal accounts per lifecycle; this document does **not** restrict those transitions.

## Enforcement

`FileObjectStorage.Update` and `GraphObjectStorage.Update` reject updates that:

1. Set **`criteria.status`** to **`validated`** or **`complete`**, or  
2. Set **`convergence_session.status`** to **`error`**,  

unless the security context is the **system account** (`pkg/context.SystemAccountID`).

This mirrors the pattern used for sensitive **`keystore_entry`** fields (`credential_hash`, `salt`): only system may write high-trust fields.

## Break-glass and tests

Set the brand-prefixed environment variable from **`zqkenv.BypassVerificationOutcomeAuthority()`** to **`1`**, **`true`**, or **`yes`** to allow non-system writes of those statuses. This is intended for **tests and exceptional automation**, not for routine CLI use.

## Agent guidance

- Prefer **verification pipelines**, lifecycle hooks, and **system-context** updates when closing criteria.
- Do not instruct users to “fix” verification by hand-editing **`criteria`** status in ways that bypass measurement.
- Do not set **`convergence_session`** to **`error`** manually; that status is for system error paths.
- See also **[AGENT_GUIDELINES.md](../process/enforcement/AGENT_GUIDELINES.md)** (verification outcomes note).
