# Role Entitlements, Capability Matrix & Object Elevation Operational Architecture Guide

## 1. Executive Architectural Summary

The **Role Entitlements and Capability Matrix Subsystem** (`pkg/entitlements`, `pkg/license`, and `cmd/zqk/object/elevated.go`) provides the authoritative tier governance, feature gating, and privilege elevation boundary for the **ZQK Knowledge Kernel**. In a multi-agent autonomous environment where human operators, background daemons, and autonomous AI swarms interact with kernel graphs, Content-Addressed Storage (CAS), and Model Context Protocol (MCP) services, a clear separation between identity, role-based authorization, and cryptographic platform entitlements is critical.

The subsystem enforces five core architectural invariants:

1. **Dual-Tier Entitlement Enforcement:** Distinguishes between the free **Community Tier** (unbounded core knowledge operations with conservative safety caps on autonomous refactoring loops) and the licensed **Enterprise Tier** (unbounded concurrency, autonomous mesh federation, and privileged object elevation).
2. **Offline Cryptographic Verification:** Evaluates platform licenses completely offline using Ed25519 asymmetric signatures and JSON Web Tokens (JWT), preventing external network telemetry requirements and ensuring air-gapped sovereign execution.
3. **Fail-Closed Fallback Invariant:** If a license key or file is missing, expired, syntactically malformed, or signed by an untrusted public key, the runtime immediately and safely defaults to the strict `CommunityChecker` implementation without crashing.
4. **Privileged Object Elevation Boundary:** Enforces that internal kernel schema kinds (marked with `visibility: internal`) are shielded from standard discovery and mutation unless explicitly requested via `--internal` and authorized by the `elevated_object` entitlement bundle.
5. **Strict Planner vs. Doer Lane Segregation:** Complements role-based access control (RBAC) by ensuring execution worker seats are strictly separated from strategic orchestrator seats, preventing runaway self-delegation.

---

## 2. Component Topology & Subsystem Architecture

The entitlements subsystem acts as an operational gate between incoming user/agent CLI and MCP commands and high-privilege kernel operations:

```mermaid
flowchart TD
    subgraph Callers["Subsystem Invocations"]
        CLI["CLI Commands\n(zqk object, zqk system evolve)"]
        Mesh["Mesh Networking\n(zqk mesh federation)"]
        Swarm["Autonomous Swarm Runner\n(zqk do, zqk agent orchestrate)"]
    end

    subgraph EntitlementsSubsystem["Entitlements Engine (pkg/entitlements)"]
        Facade["Entitlement Facade\n(CheckEvolveEntitlement, CheckMeshEntitlement, CheckEntitlementBundle)"]
        GlobalChecker["Active EntitlementChecker\n(globalChecker interface)"]
        CommunityImpl["CommunityChecker\n- Evolve Limit <= 3\n- Mesh Denied\n- Elevated Bundle Denied"]
        EnterpriseImpl["EnterpriseChecker\n- Unbounded Evolve\n- Mesh Permitted\n- All Bundles Unlocked"]
        Loader["License Loader\n(LoadLicense)"]
    end

    subgraph LicenseSubsystem["License Cryptography (pkg/license)"]
        Validator["Ed25519 License Validator\n(NewValidator, Verify, RequireFeature)"]
        Claims["Claims Model\n(Features, RegisteredClaims)"]
    end

    subgraph StorageAndEnv["License Resolution Sources"]
        EnvKey["Environment Variable\n(ZQK_LICENSE_KEY)"]
        StateDir["State Directory Token\n(~/.zqk-state/license.jwt)"]
        DataDir["Fallback Data Directory Token\n(~/.zqk/license.jwt)"]
        EmbeddedKey["Embedded Ed25519 Public Key\n(DefaultPublicKeyHex)"]
    end

    Callers -->|"Check Limit / Feature"| Facade
    Facade -->|"Delegate"| GlobalChecker
    GlobalChecker -.->|"Free Tier Default"| CommunityImpl
    GlobalChecker -.->|"License Active"| EnterpriseImpl
    Loader -->|"Read Key"| EnvKey
    Loader -->|"Read File"| StateDir
    Loader -->|"Read File (Fallback)"| DataDir
    Loader -->|"Verify Signature"| Validator
    EmbeddedKey -->|"Public Key"| Validator
    Validator -->|"Extract Claims"| Claims
    Claims -->|"Populate Features"| EnterpriseImpl
    Loader -->|"Register"| GlobalChecker
```

### Component Responsibilities

| Component | Primary API / Types | Architectural Responsibility |
| :--- | :--- | :--- |
| **Entitlement Facade** | `CheckEvolveEntitlement`, `CheckMeshEntitlement`, `CheckEntitlementBundle` | Package-level entrypoints delegating checks to the active global checker singleton. |
| **EntitlementChecker** | `EntitlementChecker` interface | Interface defining capability verification contracts (`CheckEvolveLimit`, `CheckMeshEntitlement`, `CheckBundle`). |
| **CommunityChecker** | `CommunityChecker` struct | Free-tier implementation enforcing conservative guardrails (e.g., evolution limit $\le 3$) and denying enterprise bundles. |
| **EnterpriseChecker** | `EnterpriseChecker` struct | Commercial-tier implementation providing unbounded autonomous limits and granting access to registered features. |
| **License Loader** | `LoadLicense`, `RegisterChecker`, `CurrentCheckerForTest` | Initializes runtime tiers, inspects environment variables and filesystem JWT tokens, and installs the active checker. |
| **License Validator** | `license.Validator`, `license.Claims` | Cryptographic engine verifying Ed25519-signed JWT tokens without external dependencies or phone-home requests. |
| **Elevated Object Guard** | `RequireElevatedInternal`, `ShouldSkipInternalKind` | Enforces license-or-admin gating when CLI commands specify `--internal` access to restricted object kinds. |

---

## 3. Entitlement Resolution & License Verification Pipeline

The entitlement resolution lifecycle runs during runtime bootstrap and on-demand whenever privileged operations are requested:

```mermaid
sequenceDiagram
    autonumber
    actor Caller as CLI / Agent / Swarm
    participant Facade as Entitlements Facade
    participant Checker as Active Checker
    participant Loader as License Loader (LoadLicense)
    participant Validator as Ed25519 Validator (pkg/license)

    Note over Loader: Initialized during package init()
    Loader->>Loader: Check Environment (ZQK_LICENSE_KEY)
    alt ZQK_LICENSE_KEY is present
        Loader->>Checker: Install EnterpriseChecker (Unbounded)
    else No Environment Key
        Loader->>Loader: Read ~/.zqk-state/license.jwt
        opt File not found in state directory
            Loader->>Loader: Read ~/.zqk/license.jwt (Fallback)
        end
        alt License File Found
            Loader->>Validator: Verify(tokenBytes, DefaultPublicKeyHex)
            alt Signature Valid & Not Expired
                Validator-->>Loader: claims.Features
                Loader->>Checker: Install EnterpriseChecker(Features)
            else Invalid Signature / Expired / Corrupted
                Loader->>Checker: Keep CommunityChecker (Fail Closed)
            end
        else No License File Found
            Loader->>Checker: Keep CommunityChecker (Default)
        end
    end

    Caller->>Facade: CheckEntitlementBundle(ctx, "elevated_object")
    Facade->>Checker: CheckBundle(ctx, "elevated_object")
    alt Enterprise Tier
        Checker-->>Caller: Allow (nil error)
    else Community Tier
        Checker-->>Caller: Deny (Errfmt: requires Enterprise license)
    end
```

### Resolution Logic Steps

1. **Environment Override:** If `ZQK_LICENSE_KEY` is present and non-empty in the runtime environment, the loader immediately upgrades the global checker to `EnterpriseChecker`.
2. **Filesystem Discovery:** When no environment key is supplied, the loader queries the primary state path (`$HOME/.zqk-state/license.jwt`). If missing, it inspects the backward-compatible fallback location (`$HOME/.zqk/license.jwt`).
3. **Cryptographic Validation:** If a license file is discovered, the embedded Ed25519 public key (`DefaultPublicKeyHex`) verifies the token's cryptographic signature and validates expiration timestamps.
4. **Claim Extraction:** Upon successful signature verification, granted feature strings (e.g., `mesh`, `elevated_object`, `unbounded_evolve`) are extracted into `EnterpriseChecker.Features`.
5. **Fail-Closed Fallback:** If any verification step fails (file unreadable, invalid Ed25519 hex, signature mismatch, or expired token), the system silently preserves the default `CommunityChecker`.

---

## 4. Role-Based Access Controls (RBAC) & Capability Matrix

The ZQK platform combines role-based authorization (who is allowed to perform an action) with tier entitlements (what capabilities the runtime environment unlocks).

### 4.1 Platform Tier Capability Matrix

| Operational Capability | Community Tier (Free Core) | Enterprise Tier (Licensed) | Architectural Enforcement |
| :--- | :--- | :--- | :--- |
| **Core Knowledge Graph CRUD** | Unbounded | Unbounded | CAS Storage Plane (`pkg/kernel`) |
| **Object Promotion & DoD Verification** | Unbounded | Unbounded | VDS Engine (`pkg/vds`) |
| **Autonomous Refactoring Batch Limit** | Maximum 3 candidates per cycle | Unbounded | `CheckEvolveLimit(ctx, limit)` |
| **Autonomous Mesh Networking** | Denied | Permitted | `CheckMeshEntitlement(ctx)` |
| **Elevated Object Access (`--internal`)** | Denied (Requires Enterprise) | Permitted | `CheckBundle(ctx, "elevated_object")` |
| **Custom Enterprise Bundles** | Denied | Permitted based on Claims | `CheckBundle(ctx, bundleName)` |
| **License Check Phone-Home** | Zero (100% Offline) | Zero (100% Offline) | Cryptographic Ed25519 Signature |

### 4.2 Account Role & Privilege Matrix

Within an authenticated workspace, roles define behavioral boundaries across operations:

| Persona / Role | Core Graph CRUD | Autonomous Refactoring | Mesh Federation | Elevated Object Mutation (`--internal`) | Strategic Peer Orchestration |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **System Administrator** | Allowed | Allowed (Tier-bounded) | Allowed (Tier-bounded) | Allowed (Requires License or admin carrier) | Allowed |
| **Lead Orchestrator (Planner)** | Allowed | Allowed (Tier-bounded) | Allowed (Tier-bounded) | Denied (Read-only review) | Allowed |
| **Craftsman / Worker (Doer)** | Allowed | Allowed (Tier-bounded) | Denied | Denied | **Denied (POL-AGENT-PLANNER-DOER-001)** |
| **Auditor / Observer** | Read-Only | Denied | Read-Only | Read-Only (Masked) | Denied |

---

## 5. Object Elevation Lifecycle

Internal schema objects represent foundational kernel mechanics (e.g., system configuration tables, internal state ledgers, and engine manifests) that must not be exposed to casual listing or unauthorized modification.

```mermaid
sequenceDiagram
    autonumber
    actor User as Engineer / Agent
    participant CLI as CLI Object Command (zqk object list/get)
    participant Guard as RequireElevatedInternal
    participant Ent as pkg/entitlements
    participant Spec as Kind Spec Registry

    User->>CLI: zqk object list --internal
    CLI->>Guard: RequireElevatedInternal(cmd)
    alt Admin Binary Carrier (zqk-admin)
        Guard-->>CLI: Pass (Admin Bypass)
    else Standard Binary
        Guard->>Ent: CheckEntitlementBundle(ctx, "elevated_object")
        alt Community Checker (No License)
            Ent-->>Guard: Error: elevated object access requires Enterprise
            Guard-->>User: Command Terminated with Exit Code 1
        else Enterprise Checker (Licensed)
            Ent-->>Guard: Success
            Guard-->>CLI: Proceed
        end
    end
    CLI->>Spec: Filter Kinds (ShouldSkipInternalKind)
    Spec-->>CLI: Include visibility: internal kinds
    CLI-->>User: Render Complete Elevated Object Table
```

### Elevation Lifecycle Rules

1. **Flag Detection:** When an operator or agent passes `--internal` (defined as `FlagElevatedInternal`), the CLI triggers `ElevatedInternalRequested(cmd)`.
2. **Carrier Exemption:** For transitional administrative maintenance, executing under the binary name `zqk-admin` passes without license checks.
3. **Entitlement Verification:** In standard builds, `RequireElevatedInternal` queries `CheckEntitlementBundle(ctx, BundleElevatedObject)`.
4. **Visibility Filtering:** When `--internal` is active and entitled, `ShouldSkipInternalKind(kind, elevated=true)` returns `false`, ensuring internal objects are included in listings and inspect outputs. When omitted or denied, internal kinds are hidden to prevent accidental mutation.

---

## 6. Security Invariants & Policy Guarantees

The entitlements subsystem strictly adheres to five core security invariants:

1. **Fail-Closed Default Invariant:**
   The package initialization sets `globalChecker = &CommunityChecker{}` prior to invoking `LoadLicense()`. Any failure to parse tokens, decode keys, or verify signatures leaves the runtime in `CommunityChecker` mode.
2. **Algorithm Downgrade Defense:**
   The cryptographic license validator enforces `jwt.WithValidMethods([]string{"EdDSA"})` and explicitly verifies that the parsed algorithm is `*jwt.SigningMethodEd25519`. Any attempt to present tokens signed with symmetric HMAC (`HS256`) or RSA algorithms is rejected immediately.
3. **Zero Secret Leakage:**
   License keys and tokens are never printed to terminal logs, error messages, or telemetry streams. Error messages explicitly instruct the operator on upgrade remediation without disclosing cryptographic tokens.
4. **No Hot-Path Disk Thrashing:**
   `LoadLicense()` executes once during runtime initialization. Repeated checks to `CheckEvolveEntitlement`, `CheckMeshEntitlement`, or `CheckEntitlementBundle` evaluate in-memory state in under $10\mu\text{s}$.
5. **Open-Core Payload Integrity:**
   All public entitlement interfaces, error messages, and documentation avoid hardcoding studio internal nanosecond IDs or proprietary tracking comments.

---

## 7. Best Practices & Operational Patterns

### 7.1 Automated Workstation Provisioning

For continuous integration (CI) runners or developer workstations entitled to Enterprise features:

```bash
# Option A: Inject via environment variable (ideal for containerized CI/CD)
export ZQK_LICENSE_KEY="<cryptographic_jwt_token>"

# Option B: Install license file to user state directory (ideal for developer machines)
mkdir -p "$HOME/.zqk-state"
chmod 700 "$HOME/.zqk-state"
cp license.jwt "$HOME/.zqk-state/license.jwt"
chmod 600 "$HOME/.zqk-state/license.jwt"
```

### 7.2 Programmatic Entitlement Checking in Custom Commands

When implementing new CLI commands or kernel extensions that require enterprise capabilities:

```go
package mycommand

import (
    "github.com/spf13/cobra"
    "github.com/zqk-os/zqk/pkg/entitlements"
    "github.com/zqk-os/zqk/pkg/errfmt"
)

func runCustomFederation(cmd *cobra.Command, args []string) error {
    ctx := cmd.Context()

    // Enforce platform tier requirement
    if err := entitlements.CheckMeshEntitlement(ctx); err != nil {
        return errfmt.Newf("mesh federation is an Enterprise capability").Wrap(err)
    }

    // Proceed with privileged operation
    return executeFederation()
}
```

---

## 8. Troubleshooting & Diagnostics

| Symptom | Probable Cause | Remediation |
| :--- | :--- | :--- |
| `community tier is limited to 3 evolution candidates per cycle (requested N)` | A cycle requested more than 3 autonomous refactoring candidates without an active Enterprise license. | Keep evolution candidate batches $\le 3$ for Community use, or provide a valid Enterprise license via `ZQK_LICENSE_KEY` or `~/.zqk-state/license.jwt`. |
| `autonomous mesh networking is an Enterprise feature. Please upgrade your license.` | An agent or CLI command attempted to invoke peer mesh federation under the Community tier. | Activate an Enterprise license containing the `mesh` feature. |
| `elevated object access (--internal) requires an active subscription or an Enterprise license.` | An operator ran `zqk object ... --internal` without an active license. | Ensure a valid license is present in `$HOME/.zqk-state/license.jwt` or execute using the administrative carrier. |
| `bundle '<bundle_name>' requires an active subscription or an Enterprise license.` | A requested feature pack or bundle is not entitled in the active runtime. | Verify that the presented JWT claims list the requested bundle in its `features` array. |
| License file present in `~/.zqk-state/license.jwt` but system remains in Community tier. | The license file contains an invalid signature, is expired, or permissions prevent reading. | Run `zqk system info` or verify the token expiration date. Ensure file permissions allow read access (`0600`). |

---

## 9. Verification & Continuous Quality Compliance

The `pkg/entitlements` package maintains continuous compliance across all standard verification metrics:

| Verification Dimension | Standard Target | Current Status | Evidence & Verification Command |
| :--- | :--- | :--- | :--- |
| **Statement Test Coverage** | $\ge 85.0\%$ (Target 100.0%) | **100.0%** | `go test -v -coverprofile=coverage.out ./pkg/entitlements` |
| **Go Vet Invariants** | Zero warnings | **PASS** | `go vet ./pkg/entitlements` |
| **Code Formatting** | Clean gofmt | **PASS** | `gofmt -l ./pkg/entitlements` |
| **Data Race Freedom** | Zero data races | **PASS** | `go test -race ./pkg/entitlements` |
| **Open-Core Payload Integrity** | Zero literal nanosecond IDs | **PASS** | `sh scripts/open-core/check-public-release-payload.sh .` |
