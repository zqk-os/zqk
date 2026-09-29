# Go Language Adapter Specification

> **Purpose:** Reference language adapter specializing the Codebase Evaluation Framework for the Go ecosystem, providing canonical idiom citations, automated tool mappings, and AST scan criteria.

| Specification Metadata | Value |
| :--- | :--- |
| **Framework Version** | CEF v0.1.0 |
| **Adapter Type** | Language Adapter (`adapters_enabled: ["go"]`) |
| **Target Runtime** | Go 1.22+ Toolchain |
| **Governance Status** | Reference Implementation (Optional Extension) |

---

## 1. Authoritative Idiom Citations

When evaluating Go codebases, findings must cite canonical Go engineering standards:

| Authoritative Work | Focus Area & Citation Scope |
| :--- | :--- |
| *Effective Go* | Variable naming, package boundaries, control flow, and error handling idioms. |
| *Go Code Review Comments* | Common code style conventions, documentation comments, and receiver types. |
| *The Go Memory Model* | Goroutine synchronization invariants, atomic operations, and memory visibility. |
| *Google Go Style Guide* | API interface design, package naming, error wrapping, and concurrency patterns. |

---

## 2. Recommended Automated Analysis Tooling

Specialists and preflight operators should prioritize the following ecosystem tools:

| Automated Tool | Diagnostic Signal & Purpose |
| :--- | :--- |
| `go vet`, `staticcheck`, `golangci-lint` | Code correctness, dead code detection, and common defect patterns. |
| `go test -race` (targeted packages) | Concurrency race detection and data synchronization verification. |
| AST Scanners (e.g., `zqk-vet`, custom AST visitors) | Exhaustive mechanical pattern enforcement (literals, forbidden APIs, magic numbers). |
| `govulncheck` | Supply chain vulnerability scanning and dependency CVE analysis. |

---

## 3. High-Density (`D-HIGH`) Mechanical Criteria

The following patterns must be scanned exhaustively across the declared codebase scope:

- **Untyped Literals:** Magic strings and raw numbers that should be declared as typed constants or typed enums.
- **Unhandled Errors:** Ignored error returns (`_ =`) on I/O operations, network calls, or close methods.
- **Uncontrolled Panics:** Invocations of `panic()` within shared libraries or packages outside CLI application entrypoints.
- **Resource Leaks:** Missing `defer resp.Body.Close()` or unreleased mutex locks across error exit paths.

---

## 4. Architectural Analysis & Tradeoffs

| Advantage | Operational Consideration |
| :--- | :--- |
| Significantly improves diagnostic precision and automated tooling coverage for Go systems. | Adapter rules must remain isolated and never leak into evaluations of multi-language repositories. |
| Directly leverages native Go toolchain features (`-race`, `go/ast`, `govulncheck`). | Avoids overfitting to proprietary organizational linter rules by sticking to standard Go idioms. |
