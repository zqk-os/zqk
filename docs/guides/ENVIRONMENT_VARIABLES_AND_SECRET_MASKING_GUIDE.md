# Environment Variables, Secret Masking & Agent Execution Guards Subsystem Operational Architecture Guide (`pkg/zqkenv`)

## Executive Summary

The `pkg/zqkenv` package implements the foundational environment configuration, cryptographic secret redaction, brand prefix resolution, and agent execution guardrails for the **ZQK Knowledge Kernel**. In a distributed multi-agent swarm ecosystem executing across interactive developer shells, background worker daemons, Model Context Protocol (MCP) bridges, and autonomous headless agents, strict control over environment variable access, credential visibility, and process sandboxing is paramount.

This subsystem provides:
1. **Strongly-Typed Environment Descriptors (`EnvVar`):** Zero stringly-typed environment accesses. All kernel variables are defined as strongly typed constants with fluent parsing (`OrDefault`, `IntOrDefault`, `BoolOrDefault`, `Required`).
2. **Dynamic Brand Key Resolution:** Multi-tenant and white-label isolation allowing dynamic brand key switching (`DefaultBrandKey`, `AdminBrandKey`) and brand-prefixed environment mapping without code changes.
3. **Secret Masking & Sanitization Membrane:** Deterministic redaction of sensitive credentials, API keys, tokens, and authorization headers (`MaskSensitiveValue`, `SanitizeFields`, `SanitizeEnvironment`).
4. **Agent Execution Guards (`agentguard`):** Fail-closed electrocution sentinels preventing autonomous agents from running unauthorized foreground commands (such as raw, ungoverned `go test` invocations outside of `zqk test run`).
5. **Subprocess Environment Sandboxing:** Pristine process isolation (`SubprocessEnvironWithTestRoot`, `SubprocessEnvironWithTestRootAndExtras`) stripping inherited host roots and enforcing hermetic temporary project roots.

---

## Architectural Principles & Invariants

```
                        ┌────────────────────────────────────────────────┐
                        │              Host Environment (OS)             │
                        └───────────────────────┬────────────────────────┘
                                                │
                                                ▼
                        ┌────────────────────────────────────────────────┐
                        │          pkg/zqkenv Membrane & Registry        │
                        ├───────────────────────┬────────────────────────┤
                        │  EnvVar Accessors     │  Brand Key Resolution  │
                        │  (Typed Defaults)     │  (Default/Admin Keys)  │
                        ├───────────────────────┼────────────────────────┤
                        │  Secret Redaction     │  Agent Guard Sentinels │
                        │  (Masking Pipeline)   │  (Electrocution Trap)  │
                        └───────────────────────┴────────────────────────┘
                                                │
                      ┌─────────────────────────┴─────────────────────────┐
                      ▼                                                   ▼
        ┌───────────────────────────┐                       ┌───────────────────────────┐
        │  Subprocess Sandboxing    │                       │  Privileged Writer Daemon │
        │  (SubprocessEnviron)      │                       │  (Socket Launch Guards)   │
        └───────────────────────────┘                       └───────────────────────────┘
```

### 1. Zero Stringly-Typed Env Invariant
Raw calls to `os.Getenv` across business logic packages are prohibited by `zqk-vet`. Every configuration variable must be registered in `pkg/zqkenv/vars.go` as an `EnvVar` descriptor, guaranteeing consistent documentation, default resolution, and audit traceability.

### 2. Secret Redaction Membrane Invariant
Any environment variable or key-value dictionary passing through logging, diagnostics, telemetry, or agent prompts must pass through `SanitizeFields` or `SanitizeEnvironment`. Keys matching token, secret, credential, password, or key patterns are masked to `***MASKED***` (or partial redaction preserving trailing characters for debugging).

### 3. Fail-Closed Agent Electrocution Invariant
Direct execution of developer test commands by autonomous agents outside the kernel's verifiable graph framework triggers `EnforceForegroundGoTestGuard`. The process panics with a high-salience warning directing the agent back to `zqk test run <tst-id>` and kernel objectification.

---

## Core Component Reference

### 1. Strongly-Typed Environment Descriptors (`EnvVar`)

`EnvVar` represents a strongly-typed handle to an environment key:

```go
type EnvVar struct {
    Key string
}

func (e EnvVar) Name() string
func (e EnvVar) Get() string
func (e EnvVar) Set(value string) error
func (e EnvVar) Unset() error
func (e EnvVar) OrDefault(defaultValue string) string
func (e EnvVar) IntOrDefault(defaultValue int) int
func (e EnvVar) BoolOrDefault(defaultValue bool) bool
func (e EnvVar) Required() string
```

Common kernel variables include:
- `zqkenv.Root()`: Path to the active ZQK project root.
- `zqkenv.TestRoot()`: Hermetic isolated test directory root.
- `zqkenv.SchedulerDaemonMode()`: Boolean flag controlling background daemon execution.
- `zqkenv.LogLevel()`: Operational log verbosity (`debug`, `info`, `warn`, `error`).

### 2. Brand Key Resolution & Prefix Isolation

The subsystem decouples product branding from internal code semantics:
- `DefaultBrandKey(name)`: Produces the canonical brand-prefixed variable name (e.g., `ZQK_PROJECT_ROOT`).
- `AdminBrandKey(name)`: Produces administrative override keys.
- `IsProductPrefixed(envVar)`: Fast-path check verifying whether an environment variable belongs to the current product namespace.

### 3. Secret Masking & Redaction

Security functions ensure zero secret leakage in logs, error traces, or transcripts:
- `IsSensitiveKey(key)`: Case-insensitive substring matching against known credential patterns (`TOKEN`, `SECRET`, `PASSWORD`, `API_KEY`, `AUTH`, `PRIVATE_KEY`).
- `MaskSensitiveValue(key, value)`: Returns `******` for sensitive keys, `Bearer ******` for bearer tokens, or original safe values.
- `SanitizeFields(fields)`: Returns a copy of key-value maps with all sensitive keys safely redacted.
- `SanitizeEnvironment(environ)`: Sanitizes a slice of `KEY=VALUE` strings for safe diagnostics dump.

### 4. Agent Execution Guardrails (`agentguard`)

The `agentguard` package provides proactive drift prevention:
- `EnforceForegroundGoTestGuard()`: Inspects running binaries and process arguments. If an autonomous agent attempts foreground `go test` without `MAKEFLAGS` or `ZQK_ALLOW_FOREGROUND_GO_TEST=1`, it immediately terminates the process with an informative electrocution banner.
- `IsGoTestBinary()`: Identifies whether the current process is executing under a Go test harness.

### 5. Subprocess Sandboxing

Spawning child processes from within tests or worker seats requires clean environment propagation:
- `SubprocessEnvironWithTestRoot(testRoot)`: Copies `os.Environ()`, strips host `PROJECT_ROOT`, `TEST_ROOT`, and sensitive API keys, then injects the isolated `testRoot`.
- `SubprocessEnvironWithTestRootAndExtras(testRoot, extras...)`: Injects additional verified configuration flags while maintaining isolation guarantees.

---

## Operational Troubleshooting & Runbooks

### Issue 1: Agent Electrocution Banner Triggered
**Symptom:**
```
⚡️ BZZZT! AGENT ELECTROCUTED: FOREGROUND TEST EXECUTION DETECTED
    User Mandate Violation: All tests MUST run via zqk test run (kernel test_case objects).
```
**Resolution:**
Autonomous agents must execute tests via the Knowledge Kernel CLI:
```bash
./bin/zqk test run <TST-ID>
```
For manual developer runs or CI pipelines, bypass by setting:
```bash
ZQK_ALLOW_FOREGROUND_GO_TEST=1 go test ./...
```

### Issue 2: Credential Exposure in Transcripts
**Symptom:** API keys or tokens appearing in event logs or telemetry chunks.
**Resolution:**
Ensure logging calls wrap variable maps with `zqkenv.SanitizeFields`:
```go
safeFields := zqkenv.SanitizeFields(rawFields)
logger.Info("Dispatching request", safeFields)
```

---

## Verification & Conformance Invariants

| Proof Dimension | Target Invariant | Verification Mechanism |
| :--- | :--- | :--- |
| **Static Floor Invariant** | `pkg/zqkenv` statement coverage $\ge 85.0\%$ | `go test -cover ./pkg/zqkenv` |
| **Operational Dynamic Proof** | Brand key synthesis, accessor type conversion, masking, subprocess isolation | `go test -v ./pkg/zqkenv -run 'TestCycle245.*'` |
| **Negative Adversarial Boundary** | Zero secret leakage, fail-closed agent electrocution guard | Hermetic adversarial injection test cases |
