# Onboarding Roadmap and Certification

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Status**: Design  
**Purpose**: Use system infrastructure (workstream, priority plan, backlog items) as the onboarding curriculum, with per-account completion tracking and a certification/credential model that can support elevated privileges and optional X.509 credentials.

## Overview

- **Onboarding as objects**: The onboarding curriculum is a first-class workstream, priority plan, and set of backlog items. New agents and users are assigned to complete these items before engaging in project work.
- **Per-account completion**: Completion is tracked per account (not one global "done") so each new agent proves understanding.
- **Certification and privileges**: A certification (or attestation) object records completion and can drive role/privilege elevation; optionally, X.509 certificates can later function as credentials for identification and elevated access.

## 1. Onboarding Roadmap (Existing Infrastructure)

### 1.1 Components

| Component | Purpose |
|-----------|--------|
| **Workstream** | Single workstream (e.g. "Agent & User Onboarding") that groups all onboarding work. |
| **Priority plan** | One plan (e.g. "PRI-ONBOARD" or system-assigned) dedicated to onboarding; all onboarding backlog items reference this plan. |
| **Backlog items** | Curriculum steps: e.g. "Read Operational Philosophy policy," "Complete Start Here tutorial," "List mission/vision/goals," "Run retention-status and system check," "Complete N checklist items." Items have clear acceptance criteria and are ordered by priority_tier (P0, P1, …). |

### 1.2 Assignment Model

- **Single curriculum**: One set of onboarding backlog items. Every new agent (or user) is expected to complete the same items.
- **Execution**: Agents execute the items (read policies, run commands, optionally report or attest). Completion is tracked **per account** (see §2), not by marking the backlog item "complete" globally (that would only allow one completer).
- **Discovery**: Start Here tutorial policy and Operational Philosophy policy point to the onboarding workstream/plan; agents can run e.g. `zqk object list backlog_item --filter priority_plan_ref=<ONBOARD_PLAN_ID>` to see the curriculum.

### 1.3 Creation Flow (CLI Only)

All objects are created via the zqk CLI (no direct edits under `.zqk/process/`).

**Canonical steps** (including **milestone before backlog items** so `planned` backlog items satisfy milestone preconditions) are in [scripts/onboarding_roadmap/README.md](../../scripts/onboarding_roadmap/README.md): priority plan → workstream → milestone → backlog items (with `milestone_refs` on create) → goal and cross-links. The same sequence is implemented by [scripts/scheduler_jobs/onboarding_roadmap_seed.yaml](../../scripts/scheduler_jobs/onboarding_roadmap_seed.yaml) for one-shot seeding after init.

**Advanced tutorials**: The README also describes how to reuse this **curriculum-as-data** pattern for other teachable tracks (new YAML, optional `run_wrapper` job) without forking the CLI—see *Reference pattern for advanced tutorials* in that file.

- Optionally link **agent_onboarding_preparation** to this workstream and set `preparation_tasks` to the list of onboarding backlog item IDs for discoverability.

## 2. Per-Account Completion and Certification

### 2.1 Problem

Backlog items have a single lifecycle status (e.g. `complete`). If we mark an onboarding item "complete," we cannot represent "agent A completed it, agent B has not." So we need **per-account** completion tracking.

### 2.2 Proposed: Certification (or Attestation) Object

Introduce a first-class object kind, e.g. **certification**, with at least:

- **account_ref**: Account that earned the certification.
- **certification_type**: e.g. `onboarding`, `advanced_ops`, `security_reviewer`.
- **status**: e.g. `in_progress`, `awarded`, `revoked`.
- **completed_at**: Timestamp when all requirements were met (optional until awarded).
- **criteria_refs** or **backlog_item_refs**: The set of curriculum items (or criteria) that must be completed; can be used for verification.
- **evidence** (optional): Links or hashes proving completion (e.g. audit event IDs, or attestation payload).

**Lifecycle**: `in_progress` → `awarded` when all referenced items are satisfied for this account; optionally `revoked` by admin.

**Verification**: Either manual (admin marks awarded after review) or automated (system checks that for each backlog_item_ref there exists a completion record for this account—see §2.3).

### 2.3 Completion Records (Optional Granularity)

To support automated verification, we may need **completion** (or **onboarding_completion**) records: one per (account, backlog_item) with `completed_at` and optional evidence. Then a certification is "awarded" when every required completion exists for that account. This can be a separate object kind or a nested structure; design TBD.

### 2.4 Privilege Elevation

- **Today**: Accounts have `roles` (e.g. observer, coder_agent). A new role (e.g. `onboarding_certified` or `certified_agent`) can be granted when a certification of type `onboarding` is awarded. Auth/MCP checks `account.roles` (and optionally presence of certification object) to allow elevated actions.
- **Implementation**: On award, either (a) update `account.roles` to include the new role, or (b) have the auth layer resolve certifications for the account and derive effective permissions. (a) is simpler; (b) allows revocation by revoking the certification without editing the account.)

## 3. Certificate of Completion as Credential (X.509)

### 3.1 Role of the Certificate

- **Identification**: The certificate can be part of the agent’s/user’s credential (e.g. client certificate in TLS for MCP).
- **Elevated privileges**: Permissions or roles can be encoded in extensions or mapped from subject/SAN so that completing onboarding (and possibly other certifications) results in a cert that grants higher access.

### 3.2 Design Options

| Option | Description | Pros / Cons |
|-------|-------------|-------------|
| **A. Object-only (no X.509)** | Certification object + role upgrade only. No PKI. | Simple, works with current auth. No cross-system verifiable credential. |
| **B. X.509 issued on award** | When certification is awarded, system (as CA) issues an X.509 client cert; subject/SAN or extensions encode account and certification type. | Strong identity, TLS client auth, possible cross-system use. Requires CA, key storage, revocation, renewal. |
| **C. Hybrid** | Certification object is source of truth; X.509 is optional. When X.509 is enabled, issuance is triggered by award; auth can accept either cert or session bound to account + certification lookup. | Flexibility; can adopt X.509 later. |

**Recommendation**: Start with **A** (certification object + role); design **B/C** so that (1) certification object remains the authority for "who has completed what," and (2) X.509 is a derived credential that can be issued/revoked when the certification is awarded/revoked.

### 3.3 X.509 Details (Future)

- **Issuer**: Project or org CA (e.g. `.zqk/certs/ca.crt`).
- **Subject / SAN**: Account ID or stable agent identifier; optional OIDs or SAN attributes for certification type and level.
- **Storage**: Per-account keystore or secure storage; never commit private keys to repo.
- **Revocation**: CRL or OCSP; when certification is revoked, revoke the cert.
- **MCP/auth**: TLS client auth; server verifies cert chain and maps cert → account + permissions.

## 4. Implementation Order

1. **Roadmap objects**: Create onboarding priority plan, workstream, and backlog items via CLI (see scripts/onboarding_roadmap).
2. **Docs and discovery**: Update Start Here / Operational Philosophy policies to reference the onboarding workstream and plan; document "new agents complete onboarding backlog before project work."
3. **Certification object**: Add object spec for `certification` (and optionally `completion`), implement storage and CLI (create, list, get, update). No X.509 yet.
4. **Award flow**: Manual or automated: when an account has satisfied all onboarding criteria, create/update certification to `awarded` and optionally add role to account.
5. **Auth integration**: Auth layer considers certification (and/or role `onboarding_certified`) for elevated privileges.
6. **X.509 (later)**: CA setup, issuance on award, revocation, MCP client cert auth.

## 5. Alpha: Init, Gating, and Progressive Access

For alpha release, bootstrap/init clarity, gating (no manipulation until certified), per-account duplication and archival, "my desktop," and progressive CLI access are designed in:

- **[ALPHA_ONBOARDING_AND_GATING.md](ALPHA_ONBOARDING_AND_GATING.md)** – Init welcome and start-here; command tiers (0–3) and certification-based gating; duplicating onboarding per account; archival and certificate on completion; "my desktop" (user-scoped objectives); layered tutorials; implementation order.

## 6. Related

- [Object-First Alignment](../enforcement/OBJECT_FIRST_ALIGNMENT.md) – philosophy and Start Here tutorial
- [Project Policies](../policies/README.md) – Operational Philosophy and Start Here policy objects
- [System Object Discovery Guide](system-object-discovery-guide-v1.0.md)
- [Agent Onboarding Preparation](../_internal/object_specs/agent_onboarding_preparation.yaml) – preparation_tasks as backlog refs
- [MCP Account Identification](MCP_ACCOUNT_IDENTIFICATION.md) – account_id and roles
