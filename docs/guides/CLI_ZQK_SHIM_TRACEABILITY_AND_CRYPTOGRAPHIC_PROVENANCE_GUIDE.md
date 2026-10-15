# CLI ZQK-Shim Traceability & Cryptographic Provenance Guide

## Overview

The ZQK Shim (`cmd/zqk-shim`) provides a transparent, low-latency binary interceptor for developer workflows involving `git` and `gh`. When positioned ahead of genuine system executables on `PATH` (such as `~/.zqk/shims` or project-local shim directories), `zqk-shim` provides four core operational guarantees:

1. **Infinite-Loop-Free System Binary Resolution**: Locates the underlying Genuine system binary while safely skipping the shim directory itself.
2. **Bidirectional Workstream Traceability Enforcement**: Intercepts mutating operations (`git commit`, `gh pr create`) to ensure active branches are strictly tied to active workstream contracts in the Knowledge Kernel.
3. **Cryptographic Agent Provenance Attestation**: Signs pull requests using Ed25519 project seating credentials (or SHA-256 fallbacks) and injects verifiable agent provenance stamps into PR bodies.
4. **Interactive CLI Proxying with Break-Glass Telemetry**: Seamlessly proxies interactive and automated terminal sessions via `pkg/specbuilder/cli_builders`, providing fail-closed break-glass override mechanics.

---

## Architectural Taxonomy & Core Components

```
cmd/zqk-shim/
├── main.go                          # Interception engine, traceability gates, & crypto stamping
├── main_test.go                     # Unit tests for bypass, mutations, and break-glass checks
├── shim_coverage_elevation_test.go  # Isolated git repository, CAS workstream, & edge-case tests
├── e2e_test.go                      # End-to-end process execution & stdout/stderr interception
└── README.md                        # Quickstart documentation and PATH configuration
```

---

## 1. Binary Interception & Execution Lifecycle

```
[Developer / Agent Command: git commit / gh pr create]
                         │
                         ▼
             ┌───────────────────────┐
             │       runShim         │
             └───────────┬───────────┘
                         │
         ┌───────────────┴───────────────┐
         ▼                               ▼
┌──────────────────┐           ┌──────────────────┐
│  findRealBinary  │           │isMutatingOp(cmd) │
└────────┬─────────┘           └─────────┬────────┘
         │ (found real path)             │ (true for commit/pr)
         ▼                               ▼
┌──────────────────┐           ┌────────────────────────────────────┐
│ Check Bypass Env │◄──────────┤ validateCommitTraceabilityWithEnv  │
└────────┬─────────┘           └────────────────────────────────────┘
         │                               │
         │ (if bypass, require >=30 char)│ (must match active workstream)
         ▼                               ▼
┌──────────────────┐           ┌────────────────────────────────────┐
│ Inject Crypto PR │           │   Proceed with Executable Spec     │
│   Body Stamp     │           │   via pkg/specbuilder/cli_builders │
└────────┬─────────┘           └─────────────────┬──────────────────┘
         │                                       │
         └───────────────────────────────────────┘
                         │
                         ▼
             [Genuine Binary Execution]
```

### Path Resolution (`findRealBinary`)
`findRealBinary` inspects `os.Getenv("PATH")` across OS-specific path separators. It determines the location of the current shim using `fileutil.Executable()` and deliberately ignores any matching binary located within that directory. If no genuine executable exists elsewhere on `PATH`, the shim logs a diagnostic error and exits with code 1, preventing runaway execution loops.

---

## 2. Commit & PR Traceability Gate

### The Bidirectional Traceability Contract
To maintain pristine graph integrity within the ZQK Knowledge Kernel, mutating source control actions must be explicitly traceable to authorized workstream objects:

- **Target Commands**: Mutating operations include `git commit` and `gh pr create`. Non-mutating commands (`git status`, `git diff`, `git log`, `gh pr list`, `gh pr view`) pass through unhindered.
- **Branch Matching**: The active Git branch (retrieved via `git rev-parse --abbrev-ref HEAD`) must match an active workstream identifier (e.g. `WS-STARTER-COMMUNITY-001` or `integration/ws-starter-community-001`).
- **Storage Verification**: The shim reads from the project's CAS object storage via `pkg/storage`, querying `KindWorkstream` records whose status is strictly `active`.

### Fail-Closed Break-Glass Override (`hasBreakGlassOverride`)
In emergency production scenarios or critical air-gapped hotfixes, traceability checks can be bypassed:
- Environment variable: `SHIM_BYPASS_TRACEABILITY=1` or `ZQK_SHIM_BYPASS_TRACEABILITY=1`.
- **Mandatory Justification Constraint**: The bypass will fail closed with code 1 unless accompanied by an explicit, auditable break-glass reason via `BREAK_GLASS_REASON` or `ZQK_BREAK_GLASS_REASON`.
- **Minimum Length Invariant**: The justification must contain at least 30 characters (`minBreakGlassReasonLen = 30`), ensuring sufficient context is recorded for compliance audit trails.

---

## 3. Cryptographic Provenance & Agent Stamping

### Provenance Stamp Architecture
When creating pull requests via `gh pr create`, `zqk-shim` generates a cryptographic stamp proving the identity of the executing agent or contributor:

1. **Seating Key Resolution (`resolveTrustedAgentPrivateKey`)**:
   - Discovers project-level seating credentials stored in `.zqk/credentials/agent_seating.json`.
   - Reads private signing keys via `pkg/authcred.AgentSeatingCredential`.
   - Validates that the active agent identity possesses valid RBAC permissions.
2. **Ed25519 Cryptographic Signatures**:
   - Hashes project commit metadata and current branch state.
   - Signs the state digest using Ed25519 private keys.
   - Formats the signature as a verifiable hex payload:
     `[ZQK_AGENT_STAMP: <hex_encoded_signature>]`
3. **Fallback & Degradation Handling**:
   - If no private key is present (e.g., untrusted ambient environment), the shim computes a deterministic SHA-256 fingerprint from the repository state.
   - If agent identity is not declared, it falls back to `anonymous-agent` provenance.

### Body Injection (`injectPRBodyStamp`)
`injectPRBodyStamp` parses the argument stream of `gh pr create`:
- Appends the stamp to existing `--body <text>` or `-b <text>` arguments.
- If no body flag exists, injects `--body "[ZQK_AGENT_STAMP: <hex>]"`.
- Preserves all other command flags and interactive arguments untouched.

---

## 4. Verification & Test Suite

The `cmd/zqk-shim` package is verified to **94.2% statement coverage** across three dedicated test suites:

| Test File | Focus & Verification Domain |
|:---|:---|
| `main_test.go` | Unit verification of mutation flags, break-glass length checks, and private key resolution. |
| `shim_coverage_elevation_test.go` | Isolated Git repository creation, CAS workstream matching, path traversal, and edge cases. |
| `e2e_test.go` | End-to-end binary execution, stdout/stderr streaming, and simulated command interception. |

All tests comply with fail-closed kernel constraints and pass automated verification under `TST-1792064673995605001-67d85bd6`.
