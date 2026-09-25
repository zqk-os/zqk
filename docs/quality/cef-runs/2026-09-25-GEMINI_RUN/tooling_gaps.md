# Tooling gaps — 2026-09-25-GEMINI_RUN

| Gap | Needed for | Workaround used |
|-----|------------|-----------------|
| `govulncheck` | Automated Go vulnerability and known CVE scanning in `L-SECURITY` and `L-SUPPLY-RELEASE` | Manual dependency review of `go.mod` / `go.sum` and commit history |
| `staticcheck` | Advanced static analysis and dead-code detection in `L-CODE-QUALITY` | `golangci-lint` and `zqk-vet` suite rules |
| `goimports` | Automated formatting, import sorting, and unused import validation in `L-CODE-QUALITY` | Standard `gofmt` and `zqk-vet` verification rules |
| Automated diagram generator | Generating referentially verified C4 architecture diagrams | Manual authoring of anchored Mermaid C4 diagrams per `DIAGRAM_CONTRACT.md` |
| `gosec` | Automated Go security AST linter (CWE/OWASP scanning, command injection, crypto weakness) in `L-SECURITY` | Manual code path audits and targeted AST inspection |
| HTTP surface / API boundary fuzzer | Automated validation of HTTP endpoints for unbounded bodies, missing auth, and CSWSH/CORS flaws in `L-SECURITY` | Manual endpoint review and source tracing |
| `go-cleanarch` / Package Layering Linter | Enforcing 3-tier package layering rules, quarantine boundaries, and detecting cyclical package dependencies in `L-ARCHITECTURE` | Custom tests in `pkg/kernel/import_ban_test.go` and manual AST tracing |
| `deadcode` / Package Reachability Analyzer | Automated detection of orphaned, unimported Go packages and dead codegen trees (e.g. `pkg/cli/command_builders`) in `L-ARCHITECTURE` | Shell scripts and manual `go list` / `grep` cross-referencing |
| `cosign` / Sigstore CLI | Automated cryptographic keyless signing and verification of release binaries, checksums, and container/blob attestations in `L-SUPPLY-RELEASE` | Manual inspection of `.goreleaser.yaml` and `.github/workflows/release.yml` |
| `addlicense` / SPDX Header Linter | Automated verification and batch injection of SPDX license identifiers and copyright notices across source files in `L-SUPPLY-RELEASE` | Custom Python census script |
| `gitleaks` / CI Secret Scanner Action | Automated pull request and commit gating for credential, token, and private key leakage in `L-SUPPLY-RELEASE` | Ad-hoc execution of `scripts/scan-secrets.sh` |
| `otel-collector` / OpenTelemetry test harness | Verifying trace span propagation, W3C traceparent headers, and metric export sinks in `L-OBSERVABILITY` | Manual static AST audit of `pkg/telemetry` and unit test tracing |
| Prometheus metric validator / scraper | Automated validation of OpenMetrics / Prometheus exposition format and bucket distribution correctness in `L-OBSERVABILITY` | Unit testing with `pkg/metrics/prometheus_histogram_test.go` |
| `go test -race` CI runner partition | Automated execution of Go data race detector across concurrent packages in `L-TESTING` | Targeted probe running `go test -race -short` on sample packages (`pkg/bufferpool`, `pkg/goroutinelabels`) |
| `testifylint` / Zero-assertion test linter | Detecting and rejecting tests with zero assertions or vacuous unasserted accessor loops in `L-TESTING` | Custom Python AST census script |
| Continuous Go fuzzing harness (`go test -fuzz`) | Automated mutation fuzzing of CAS serialization, WAL frame decoding, and MCP protocol parsers in `L-TESTING` | Manual code path inspection of static test fixtures |
| Goroutine leak detector (`uber-go/goleak`) | Automated detection of orphaned goroutines and unclosed channels across parallel test packages in `L-TESTING` | Manual static code audit of `pkg/goroutinelabels` and `pkg/scheduler` |
| `chaos-mesh` / Disk Fault Injection Harness | Simulating kernel panics, sudden process kills (`SIGKILL`), and `fsync` stalls to evaluate crash consistency and WAL durability in `L-RELIABILITY` | Static code analysis and targeted crash test inspection (`wal_crash_recovery_integration_test.go`) |
| Distributed Concurrency Model Checker / Jepsen-style flock validator | Automated fuzzing and race-condition detection in multi-process file locking (`JobLock`) and stale lock recovery in `L-RELIABILITY` | Static source tracing and manual lock order verification |
| Power-cut / Torn-write File System Emulator (CrashMonkey / ALICE) | Verifying atomic rename and WAL recovery against torn writes and mid-transaction fsync cuts in `L-RELIABILITY` | Synthetic torn-line unit tests in `object_wal_test.go` |
| `dupl` / Code Clone Detector in CI | Automated detection of copy-paste duplicate logic and redundant directory trees in `L-CODE-QUALITY` | Manual execution of `golangci-lint run --enable-only dupl` and `filecmp` analysis |
| `nilerr` CI Gating Analyzer | Automated enforcement preventing functions from returning nil when an error is checked non-nil in `L-CODE-QUALITY` | Ad-hoc execution of `golangci-lint run --no-config --enable-only nilerr` |
| `errorlint` Error Chain Validator | Automated detection of `%v` and `%s` broken error chains in `fmt.Errorf` in `L-CODE-QUALITY` | Custom Python AST and regex scanner |
| Fast Compile-Only Test Gate (`go test -run=^$ ./...`) | Preventing uncompilable test source (like `pkg/testing/package_timeouts_test.go`) from entering repository in `L-CODE-QUALITY` | Manual `go test` compilation probe |

