# Run log — 2026-09-25-GEMINI_RUN

| UTC | Action | Notes |
|-----|--------|-------|
| 2026-09-25T09:14:00Z | Materialize output home | Initialized `docs/quality/cef-runs/2026-09-25-GEMINI_RUN` |
| 2026-09-25T09:15:00Z | Commit SHA locked | Frozen at `392fb153b506c0ecee2047a3ddb05a327a161f4e` |
| 2026-09-25T09:16:00Z | Codebase inventory | 7,341 Go files (1,100,614 LOC); 8,452 total tracked files in scope |
| 2026-09-25T09:17:00Z | Tool availability probe | Probed go, golangci-lint, zqk-vet, zqk, police scripts, docker, jq |
| 2026-09-25T09:17:30Z | Tool execution: go vet | `go vet ./cmd/... ./internal/...` returned exit 0 (clean) |
| 2026-09-25T09:17:40Z | Tool execution: zqk-vet hygiene | `./bin/zqk-vet -suite hygiene` returned exit 1 (4 findings: hardcoded "zqk" invocations) |
| 2026-09-25T09:17:45Z | Tool execution: zqk-vet tree | `./bin/zqk-vet -suite tree` returned exit 0 (0 findings) |
| 2026-09-25T09:17:50Z | Tool execution: zqk-vet payload | `./bin/zqk-vet -suite payload` returned exit 0 (0 findings) |
| 2026-09-25T09:17:55Z | Tool execution: police script | `./scripts/open-core/police-community-tree.sh .` returned exit 0 (POLICE: PASS) |
| 2026-09-25T09:18:00Z | Tool execution: export gate | `./bin/zqk system export-gate` returned exit 0 (PASS: clean) |
| 2026-09-25T09:18:30Z | Wave 0 L-PREFLIGHT completed | Scope frozen, preflight.json, tooling_gaps.md, draft diagrams emitted |
| 2026-09-25T09:28:00Z | Tool execution: scan-secrets | `./scripts/scan-secrets.sh .` returned exit 0 (Zero secrets detected) |
| 2026-09-25T09:28:30Z | Tool execution: path & perm lint | `./scripts/check-hardcoded-paths-and-perms-repo.sh .` executed (detected 54 path and 199 permission violations) |
| 2026-09-25T09:30:15Z | Dynamic probe: AuthMiddleware | `./bin/zqk system whoami -test.dummy` confirmed authentication bypass returning ACC-TEST-HARNESS |
| 2026-09-25T09:32:00Z | L-SECURITY evaluation completed | Emitted findings/L-SECURITY.jsonl (9 findings), narratives/L-SECURITY.md, diagrams/D-SECURITY-01.md |
| 2026-09-25T09:33:00Z | Architecture inventory | Inventoried 709 Go packages across 137 pkg/ dirs; measured 2,308 .(string) type assertions; audited pkg/storage, pkg/mcp, pkg/scheduler |
| 2026-09-25T09:34:00Z | Dead artifact scan | Identified 211 unimported files in pkg/cli/command_builders/bldr_cli_cmd_v1 with conflicting codegen default in generate_command_builders.go |
| 2026-09-25T09:36:00Z | L-ARCHITECTURE evaluation completed | Emitted findings/L-ARCHITECTURE.jsonl (10 findings), narratives/L-ARCHITECTURE.md, diagrams/D-ARCH-STORAGE-01.md, diagrams/D-ARCH-MCP-BRIDGE-01.md |
| 2026-09-25T09:37:00Z | Tool execution: go mod verify | `go mod verify` returned exit 0 (all modules verified) |
| 2026-09-25T09:37:30Z | Tool execution: scan-secrets | `./scripts/scan-secrets.sh .` returned exit 0 (Zero secrets detected) |
| 2026-09-25T09:38:00Z | Codebase license census | Scanned 7,547 Go files; 9 with license headers, 0 with SPDX-License-Identifier, 7,538 without headers |
| 2026-09-25T09:38:30Z | Probe: install.sh source target | `make -n zqk` returned exit 0 with "Nothing to be done for 'zqk'." |
| 2026-09-25T09:39:00Z | Targeted test: processhygiene | `go test -short ./pkg/processhygiene/...` failed (missing .github/dependabot.yml and secret-scan.yml) |
| 2026-09-25T09:40:00Z | L-SUPPLY-RELEASE completed | Emitted findings/L-SUPPLY-RELEASE.jsonl (9 findings), narratives/L-SUPPLY-RELEASE.md, diagrams/D-SUPPLY-01.md |
| 2026-09-25T09:42:00Z | Wave 2 Adversarial L-SECURITY completed | Emitted adversarial/L-SECURITY.jsonl (9 audited findings: 9 stand, 0 downgrade, 0 retract, 0 reframe) |
| 2026-09-25T09:44:00Z | Wave 2 Adversarial L-SUPPLY-RELEASE completed | Emitted adversarial/L-SUPPLY-RELEASE.jsonl (9 audited findings: 7 stand, 1 downgrade, 0 retract, 1 reframe) |
| 2026-09-25T09:46:00Z | Wave 2 Adversarial L-ARCHITECTURE completed | Emitted adversarial/L-ARCHITECTURE.jsonl (10 audited findings: 8 stand, 1 downgrade, 0 retract, 1 reframe) |
| 2026-09-25T09:51:00Z | Wave 3 L-OBSERVABILITY completed | Emitted findings/L-OBSERVABILITY.jsonl (7 findings), narratives/L-OBSERVABILITY.md, diagrams/D-OBS-PIPELINE-01.md |
| 2026-09-25T09:55:00Z | Wave 3 Adversarial L-OBSERVABILITY completed | Emitted adversarial/L-OBSERVABILITY.jsonl (7 audited findings: 5 stand, 1 downgrade, 0 retract, 1 reframe) |
| 2026-09-25T09:56:00Z | Test codebase inventory | Scanned 2,912 test files (428,470 test LOC) across 710 packages (689 tested, 21 untested) |
| 2026-09-25T09:57:00Z | Concurrency & flake probe | Identified 475 time.Sleep calls across 216 test files; 82 test files combining t.Parallel() and time.Sleep |
| 2026-09-25T09:58:00Z | Targeted probe: go test -race | `go test -race -short -timeout 30s ./pkg/bufferpool/... ./pkg/goroutinelabels/...` failed with exit 1 (DATA RACE in Pool.Stop/Submit) |
| 2026-09-25T09:59:00Z | CI & Makefile test audit | Audited .github/workflows/ci.yml and Makefile; confirmed total exclusion of -race and orphaned //go:build integration tests |
| 2026-09-25T10:00:00Z | Invariant & TDD census | Identified 230 zero-assertion tests, 160+ extra_coverage_test.go files, 0 native fuzz tests, and SkipDeleteAudit backdoor in pkg/storage |
| 2026-09-25T10:01:00Z | L-TESTING evaluation completed | Emitted findings/L-TESTING.jsonl (10 findings), narratives/L-TESTING.md, diagrams/D-TEST-PYRAMID-01.md |
| 2026-09-25T10:02:00Z | Reliability audit | Audited transaction commit rollback, Darwin fsync queue, IPCWriter net/rpc calls, WAL compaction, and JobLock stale lock semantics |
| 2026-09-25T10:03:00Z | Dynamic probe: JobLock & WAL | Verified commitStageApplyPerOpMixed rollback no-op, darwinSyncQueue exit-drop, and applied_seq advancement on queue full |
| 2026-09-25T10:04:00Z | L-RELIABILITY evaluation completed | Emitted findings/L-RELIABILITY.jsonl (9 findings), narratives/L-RELIABILITY.md, diagrams/D-RELIABILITY-01.md |
| 2026-09-25T10:05:00Z | Code quality inventory & audit | Scanned 7,341 Go files; detected 20,709 duplicate literals, 199 magic permissions, 54 raw paths, and 2,894 blank identifier error discards |
| 2026-09-25T10:06:00Z | Tool execution: nilerr & errorlint | `golangci-lint run --enable-only nilerr ./cmd/...` detected 50 nilerr returns; identified 50 %v/%s error chain breaks |
| 2026-09-25T10:07:00Z | Clone & hygiene scan | `dupl` and filecmp identified 168 identical file clones in command builders; zqk-vet hygiene raised 4 CLI literal errors |
| 2026-09-25T10:09:00Z | L-CODE-QUALITY evaluation completed | Emitted findings/L-CODE-QUALITY.jsonl (11 findings), narratives/L-CODE-QUALITY.md, diagrams/D-CODE-QUALITY-01.md |
| 2026-09-25T10:10:00Z | Wave 3 Adversarial L-TESTING completed | Emitted adversarial/L-TESTING.jsonl (10 audited findings: 8 stand, 1 downgrade, 0 retract, 1 reframe) |
| 2026-09-25T10:12:00Z | Wave 3 Adversarial L-CODE-QUALITY completed | Emitted adversarial/L-CODE-QUALITY.jsonl (11 audited findings: 9 stand, 0 downgrade, 0 retract, 2 reframe) |
| 2026-09-25T10:15:00Z | Wave 4 Integrator synthesis completed | Emitted scorecard.json (all 8 axes), findings.jsonl (65 accepted, 0 E0), adversarial_resolutions.jsonl, handoff_manifest.json, and EXECUTIVE_NARRATIVE.md |


