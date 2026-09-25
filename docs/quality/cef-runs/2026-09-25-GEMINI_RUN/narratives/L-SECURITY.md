# L-SECURITY Narrative: Security & Threat Vector Evaluation

**Run Package:** `2026-09-25-GEMINI_RUN`  
**Lens:** `L-SECURITY`  
**Density Class:** `D-MED` (Top-N Budget: 10; Emitted: 9)  
**Primary Axes:** `SEC` (Security), `ROB` (Robustness)  
**Evidence Baseline:** Git SHA `392fb153b506c0ecee2047a3ddb05a327a161f4e` (8,452 tracked files)

---

## 1. Executive Assessment

The ZQK codebase implements architectural security mechanisms intended to enforce privilege separation, cellular data protection (CPCP-MEMBRANE-001), and tamper-resistant content-addressable storage (Privileged Writer IPC daemon). However, rigorous static and dynamic evaluation reveals critical architectural and mechanical bypasses that puncture these membranes:

1. **Authentication Bypass (`F-SEC-AUTH-BYPASS-TEST-ARG`, Critical):** `AuthMiddleware` checks `os.Args` for any flag starting with `-test.` or checks if `ZQK_TEST_BYPASS_AUTH=1`. In the production binary, passing any `-test.*` argument (e.g. `./bin/zqk system whoami -test.dummy`) immediately bypasses all RBAC credential checks and assigns the process full superuser test permissions (`read:*`, `write:*`, `delete:*`, `access:*` as `ACC-TEST-HARNESS`).
2. **Fail-Open Default Authorization (`F-SEC-UNBOUND-SYSTEM-FALLBACK`, High):** When `cli.Processor` initializes without an explicit `SecurityContext` attached to `cmd.Context()` (the default for subcommands creating processors outside `AuthMiddleware`), it defaults to `NewSystemSecurityContext()` (`ACC-SYSTEM` with role `admin` and `read:*`, `write:*`, `delete:*`).
3. **Agent Sandbox Confinement Escapes (`F-SEC-SANDBOX-ALLOWLIST-ESCAPE`, High):** The Quantum Sandbox bash execution allowlist permits `go`, `make`, and `find`. Because `checkSandboxAllowlist` only matches base executable names, agents can execute arbitrary host binaries and untrusted code via `go run`, `go test -exec`, `make -f`, or `find -exec`.
4. **Agentic Membrane Incomplete Coverage (`F-SEC-CPCP-MEMBRANE-COVERAGE-GAPS`, High):** The CPCP membrane interceptor only intercepts tools whose names contain specific substrings (`write`, `create`, `edit`, `replace`, `delete`, `modify`, `patch`). Command-execution tools (`execute_bash`) and directory moves/copies are unmonitored. Crucially, `.zqk/run` (housing the privileged writer Unix domain socket) and `.zqk/bin` (housing executable binaries) are omitted from the protected paths list.
5. **Asymmetric Key Inversion in Tray Verification (`F-SEC-TRAY-VERIFICATION-KEY-INVERSION`, High):** Verification of restricted tray shortcuts attempts to load the private key (`.zqk/keystore/auditor.priv`) using `NewAuditorSigner`, generating a new private key on disk if missing. Furthermore, runtime verification evaluates only `e.Argv` from the manifest rather than the concatenated runtime `fullArgv`, and default tray entries bypass signature checking entirely.
6. **Unauthenticated HTTP & Loopback Surfaces (`F-SEC-HTTP-UNAUTH-UNBOUNDED-INPUTS`, Moderate):** `CallbackListenerHandler` has `authenticateRequest` annotated as unused dead code, allowing unauthenticated job triggers. Ambient ingest reads request bodies via `io.ReadAll` without `MaxBytesReader`. Telemetry WebSocket hijacks connections without validating the `Origin` header (Cross-Origin WebSocket Hijacking).
7. **Clean Credential Hygiene Baseline (`F-SEC-CLEAN-SECRETS-HYGIENE`, Info):** Zero committed secrets, private keys, or prohibited paths were found across all 8,452 tracked repository files.

---

## 2. Findings Summary Table

| Finding ID | Title | Severity | Grade | Axes | Density |
|------------|-------|----------|-------|------|---------|
| `F-SEC-AUTH-BYPASS-TEST-ARG` | Authentication bypass in AuthMiddleware via unauthenticated test flag injection | critical | E2 | SEC, ROB | D-MED |
| `F-SEC-UNBOUND-SYSTEM-FALLBACK` | Processor defaults to full SystemSecurityContext when unbound by AuthMiddleware | high | E2 | SEC, ROB | D-MED |
| `F-SEC-SANDBOX-ALLOWLIST-ESCAPE` | Quantum Sandbox allowlist permits compilers and utilities susceptible to arbitrary execution | high | E2 | SEC, ROB | D-MED |
| `F-SEC-CPCP-MEMBRANE-COVERAGE-GAPS` | CPCP boundary interceptor relies on naive tool name substrings and omits critical daemon directories | high | E2 | SEC, ROB | D-MED |
| `F-SEC-TRAY-VERIFICATION-KEY-INVERSION` | Tray entry signature verification requires private key and omits appended arguments | high | E2 | SEC, ROB | D-MED |
| `F-SEC-HTTP-UNAUTH-UNBOUNDED-INPUTS` | Unauthenticated HTTP endpoints with unbounded request bodies and missing Origin validation | moderate | E2 | SEC, ROB | D-MED |
| `F-SEC-SCHEDULER-SHELL-INTERPOLATION` | Scheduler run wrapper interpolates direct executable commands through /bin/sh | moderate | E2 | SEC, ROB | D-MED |
| `F-SEC-PW-AFFINITY-BYPASS` | Privileged writer IPC server checkAffinity bypasses validation on empty project root | moderate | E2 | SEC, ROB | D-MED |
| `F-SEC-CLEAN-SECRETS-HYGIENE` | Zero detected secrets, credentials, or prohibited paths committed across candidate tree | info | E2 | SEC, CMP | D-MED |

---

## 3. Structural & Architectural Hotspots

### 3.1 Trust Boundaries & Privilege Membranes
Refer to diagram `diagrams/D-SECURITY-01.md` ("ZQK Trust Boundaries, Privilege Membranes, and Attack Surfaces") for structural illumination.

The architecture relies on four concentric membranes:
1. **CLI Authentication Interceptor (`AuthMiddleware`):** Intended to enforce ACC identity resolution, credentials matching, and session validation before command execution. Compromised by `-test.*` argument inspection and unauthenticated environment variables (`ZQK_TEST_BYPASS_AUTH`, `ZQK_DEV_CODEGEN`).
2. **Execution Sandbox & CPCP Membrane (`CPCPInterceptor` & `tools_sandbox_allowlist.go`):** Intended to confine autonomous subagents within a secure quantum sandbox. Compromised by including meta-compilers (`go`, `make`) in the execution allowlist, and relying on naive tool-name substring matching that fails to protect `.zqk/run` and `.zqk/bin`.
3. **Privileged Writer IPC Boundary (`PrivilegedWriterDaemon`):** Intended to isolate CAS file mutations to a single daemon process communicating over a Unix domain socket (`.zqk/run/zqk-privileged-writer.sock`). Bypassed if `args.ProjectRoot` is left blank during RPC dispatch.
4. **Network & HTTP Ingest Boundaries (`CallbackListenerHandler`, `DocServer`, `agentfeed.httpapi`):** Intended for node-local IPC and browser telemetry. Weakened by dead authentication logic, missing `MaxBytesReader` resource bounds, and unverified WebSocket `Origin` headers.

---

## 4. Key Recommendations

1. **Eliminate Test Flag Inspection in Production CLI (`F-SEC-AUTH-BYPASS-TEST-ARG`):** Remove naive `strings.HasPrefix(arg, "-test.")` parsing from `cmd/zqk/app/auth_middleware.go`. Gate test-mode authentication strictly behind compilation tags (`//go:build test`) or Go `testing.Testing()` runtime verification.
2. **Fail-Closed Processor Security Context (`F-SEC-UNBOUND-SYSTEM-FALLBACK`):** Replace fallback to `NewSystemSecurityContext()` in `internal/cli/processor.go` with a restricted guest context or fail-closed error return when an unauthenticated invocation is encountered.
3. **Harden Sandbox Executable Allowlist (`F-SEC-SANDBOX-ALLOWLIST-ESCAPE`):** Disallow unrestricted `"go"` and `"make"` invocations in `sandboxAllowedExecutableList`. If Go tooling is required, restrict strictly to `"go vet"` or subcommands with verified flag allowlists that forbid `-exec`, `-run`, or arbitrary binary building. Universally strip `-exec` from `find`.
4. **Broaden CPCP Membrane Invariants (`F-SEC-CPCP-MEMBRANE-COVERAGE-GAPS`):** Update `CPCPInterceptor` to inspect all tool calls, intercept command executions, and protect all directories under `.zqk/` (`.zqk/run`, `.zqk/bin`, `.zqk/keystore`, `.zqk/object_drafts`).
5. **Decouple Key Verification from Key Storage (`F-SEC-TRAY-VERIFICATION-KEY-INVERSION`):** Refactor `tray` verification to evaluate ECDSA signatures strictly against a public key or certificate; never require reading or generating `auditor.priv` during verification. Verify the full runtime argument vector rather than static manifest definitions.
6. **Enforce HTTP Input Limits and Authentication (`F-SEC-HTTP-UNAUTH-UNBOUNDED-INPUTS`):** Wire `authenticateRequest` into `handleCallback` in `handlers_callback_listener.go`. Apply `http.MaxBytesReader` across all HTTP ingest handlers (`cmd/zqk/ambient/ingest.go`). Enforce `Origin` header checking before WebSocket connection hijacking.
