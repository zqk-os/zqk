# ZQK CLI Shim (`zqk-shim`)

## Overview

`zqk-shim` is a transparent, low-latency binary interceptor for developer and agent tools—specifically `git` and GitHub CLI (`gh`). 

When installed (typically symlinked in `~/.zqk/shims` which is prepended to `$PATH`), invocations to `git` or `gh` route through `zqk-shim` first. The shim inspects mutating actions, enforces Knowledge Kernel traceability constraints, injects cryptographic attestation stamps, and proxies execution to the genuine system binary without interfering with interactive terminal streams.

---

## Core Capabilities

### 1. Transparent Binary Proxying
- Inspects `os.Args[0]` to determine the invoked tool name (e.g. `git`, `gh`).
- Resolves the real system binary by traversing `$PATH`, actively skipping the shim directory and dereferencing symlinks to prevent infinite recursive interception loops (`findRealBinary`).
- Attaches standard I/O streams and executes the underlying command via the ZQK CLI proxy pipeline.

### 2. Traceability Enforcement Gate (POL-CODE-009)
Whenever a mutating command is executed:
- `git commit`
- `gh pr create`

`zqk-shim` intercepts the operation and validates adherence to **POL-CODE-009** (`validatePOLCODE009`):
1. Detects the current project root and active git branch.
2. Queries the Knowledge Kernel storage for active `workstream` objects.
3. Asserts bidirectional traceability: the active git branch name must contain a reference to an active workstream ID (e.g. `integration/pri-...` or `feat/WS-...`).
4. If no active workstream link is established, the operation **fails closed** and blocks the commit or PR creation.

### 3. Cryptographic Seating & Attestation Stamps
When creating pull requests (`gh pr create`):
1. Resolves the current agent's identity and seating credential via `authcred.LoadSeatCredential(projectRoot, agentID)`.
2. Computes an Ed25519 cryptographic signature or cryptographic SHA-256 stamp over `commit:agentID`.
3. Injects a verifiable provenance stamp (`[ZQK_AGENT_STAMP: <hex>]`) into the PR body text before passing it to GitHub CLI.

---

## Why Do Bypass Environment Variables Exist?

In the codebase and test suites, you will encounter:
```bash
ZQK_SHIM_BYPASS_POLCODE009=1
GIT_ZQK_SHIM_BYPASS_POLCODE009=1
GH_ZQK_SHIM_BYPASS_POLCODE009=1
```

### Purpose & Rationale
1. **Automated Testing & Greenfield Setup**: Unit tests and E2E harnesses (such as `cmd/zqk-shim/e2e_test.go`) often need to run `git commit` or mock `gh pr create` in isolated temporary directories that do not yet have an initialized `.zqk/process` Knowledge Kernel or active workstream.
2. **Emergency Maintenance / Bootstrapping**: During early repo bootstrapping or out-of-band operational emergencies, engineers may need to commit without kernel objects present.

### Fail-Closed Protection: The Human Break-Glass Rule
To prevent rogue agents or scripts from silently disabling POL-CODE-009 by exporting `ZQK_SHIM_BYPASS_POLCODE009=1`, the shim requires a human break-glass reason:

> **A naked bypass variable is strictly rejected.** Setting `ZQK_SHIM_BYPASS_POLCODE009=1` alone will abort with:
> `unvalidated POL-CODE-009 bypass rejected: human break-glass requires explicit justification in ZQK_BREAK_GLASS_REASON (min 30 chars)`

To execute a valid bypass, the caller **must** provide an audited break-glass reason of at least 30 characters:
```bash
export ZQK_SHIM_BYPASS_POLCODE009=1
export ZQK_BREAK_GLASS_REASON="Emergency hotfix for auth outage approved by lead engineer"
git commit -m "fix: resolve critical auth lockup"
```
This guarantees that any bypass leaves an explicit, auditable, and intentional trail.
