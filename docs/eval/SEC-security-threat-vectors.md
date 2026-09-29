# SEC — Security & Threat Vectors Evaluation

Domain: Security & Threat Vectors
Scope: `pkg/crypto`, `pkg/daemon/overseer`, `pkg/service`, `pkg/agent`, `pkg/validation/qa`
Method: Evidence-driven threat modeling, static inspection of authorization boundaries, IPC channels, and secret handling.

---

## 1. Architecture & Trust Boundaries

### Findings
- **Local Domain Socket Boundary (`pkg/daemon/overseer/ipc.go`)**:
  - The overseer exposes a Unix domain socket at `.zqk/state/daemon/overseer.sock` for IPC control actions (`start`, `stop`, `restart`, `add`, `remove`, `shutdown`).
  - **Vulnerability/Smell**: Socket directory `.zqk/state/daemon` is created with permissions `paths.DirPerm755` (line 64). On multi-user Unix systems, if project root permissions permit directory traverse, local unprivileged users could access the socket if umask permits read/write.
  - **Severity: Medium.**
  - **Recommendation**: Ensure the socket directory is strictly restricted to `0700` (`FilePerm700`) and the Unix domain socket is explicitly chmodded to `0600` after binding.

- **System Context vs. Agent Context (`pkg/objects/lifecycle_loader.go`)**:
  - Transitioning `criteria` to `validated` is strictly fail-closed: unprivileged agent contexts receive `permission denied: only system may set criteria status to validated (verification outcome); use the verification pipeline or system context`.
  - Positive security finding: Anti-forgery enforcement prevents autonomous agents from unilaterally stamping criteria as validated without running verifiable tests.

---

## 2. Authentication, Stamps & Cryptographic Integrity

### Findings
- **Ed25519 JWT Stamp Verification (`pkg/crypto/stamp.go`)**:
  - `GenerateStamp` and `VerifyStamp` utilize EdDSA (Ed25519) with 24-hour expiration (`ExpiresAt`) and explicit issuer validation (`Issuer: "zqk-agent"`).
  - Explicit algorithm validation in `VerifyStamp`:
    ```go
    if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
        return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
    }
    ```
    This completely precludes algorithm-confusion attacks (e.g., `none` algorithm or HMAC-as-public-key substitutions).
  - **Verdict:** Cryptographic stamp generation and verification is robust and conforms to modern defensive standards.

- **Agent Task Claim Locking (`pkg/agent/claim.go`)**:
  - Work units require exclusive claimant occupancy (`claimed_by`, `claimed_at`).
  - Attempting to modify or transition an occupiable task without an active claim fails closed:
    `claimed_by must be set when transitioning into in_progress on occupiable objects (exclusive claim required)`.
  - This prevents race conditions and task-hijacking across concurrent agent sessions.

---

## 3. Host Service & Process Supervision Security

### Findings
- **LaunchAgent Plist Permissions (`pkg/daemon/overseer/hostservice_darwin.go`)**:
  - In `InstallOverseerLaunchAgent`, `plistBody` includes environment variables such as `HOME`, `PATH`, `ZQK_PROJECT_ROOT`, `ZQK_IS_DAEMON`, and `ZQK_API_KEY`.
  - File is written to `~/Library/LaunchAgents/<label>.plist` with `paths.FilePerm644` (world-readable):
    ```go
    if err := fileutil.WriteFile(plistPath, []byte(plistBody), paths.FilePerm644); err != nil {
    ```
  - **Severity: Low-Medium.**
  - **Recommendation**: If `ZQK_API_KEY` contains non-system secrets or user credentials, `FilePerm600` must be used instead of `FilePerm644` to avoid token disclosure to other local accounts on macOS.

- **Child Process Execution Hardening (`pkg/execwrap/`)**:
  - Subprocess execution across the codebase is mediated through `execwrap.Command` and `execwrap.CommandContext`, avoiding raw shell expansion strings and preventing command-injection vectors.

---

## 4. Input Validation & Strict Object Mutation

### Findings
- **CAS Strict Tier-2 Mode**:
  - Kernel object storage enforces strict schema validation: unexpected fields (such as stray test or formula keys) reject writes fail-closed (`validation errors block save: unknown field ... (strict mode)`).
  - This eliminates deserialization pollution and parameter tampering across the graph store.

- **Git Commit Evidence Verification (`pkg/gitevidence/commit_refs.go`)**:
  - Backlog item completion gates verify that commit SHAs exist in the git object database, mutate real product files outside metadata paths, and explicitly reference the corresponding task or work item identifier.
  - This prevents phantom milestone closure without verifiable cryptographic git evidence.

---

## 5. Summary & Hardening Roadmap

| Vector / Area | Current State | Risk Severity | Planned Remediation |
| :--- | :--- | :--- | :--- |
| **Daemon IPC Socket** | `DirPerm755` directory | Medium | Restrict socket parent dir to `0700` and socket to `0600` |
| **LaunchAgent Plist** | `FilePerm644` in LaunchAgents | Low-Medium | Restrict plist file to `FilePerm600` |
| **Verification Gate** | Fail-closed system context only | **Secure** | Invariant enforced by kernel lifecycle |
| **Ed25519 Stamps** | Alg pinned to EdDSA, 24h TTL | **Secure** | Verified immune to alg-confusion |
| **Subprocess Exec** | `execwrap` typed arguments | **Secure** | Zero raw shell concatenations in core |
