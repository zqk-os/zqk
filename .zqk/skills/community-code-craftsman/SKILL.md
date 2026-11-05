---
name: community-code-craftsman
description: TDD, idiomatic implementation, and clean PRs on the community kernel.
---

# Software Engineering and Code Craftsmanship

## Objective
Execute implementation tasks with high structural rigor, complete test coverage, and strict resource hygiene.

## Workflow Protocol
1. **Pre-Implementation Verification**:
   - Run existing test suites (`go test -v ./...`) before writing new code.
   - Inspect active policies (`zqk object list policy`).
2. **Test-Driven Development (TDD)**:
   - Write unit tests demonstrating the desired behavior or defect reproduction before writing production code.
   - Verify tests fail as expected, then write minimal code to achieve a green state.
3. **Resource Hygiene**:
   - Ensure all acquired files, sockets, and channels are explicitly closed using `defer`.
   - Avoid unbounded goroutines; verify leak freedom.
4. **Code Quality Gates**:
   - Run `golangci-lint run ./...` and resolve any flagged issues.
   - Stage changes on plan-scoped integration branches.
5. **Declarative Kernel Operations**:
   - Leverage `zqk query` (ZPARQL) for cycle-safe topological queries across kernel dependencies rather than implementing custom graph search in Go scripts.
   - Leverage `zqk mutate` (ZQL) for batch object creation, ensuring all-or-nothing transactional atomicity and clean rollback.

## Before every `git push` (fail closed)

Do **not** push to burn CI minutes. CI failures on this repo are expensive and slow the release tag.

1. Identify packages you changed (`git diff --name-only origin/main...HEAD` → map to `go list` packages).
2. Run **narrow** probes with explicit timeouts:
   - Fast: `go test <pkg> -timeout 60s -run '<TouchedTest>' -count=1`
   - Heavy packages (`./pkg/storage`, full scheduler): raise timeout (e.g. `-timeout 180s`) or use the matching CI job package list — do not assume 60s is enough for `TestUpdateAllFields`.
3. If CI already failed: reproduce the **exact** failing `-run` locally and fix **before** the next push.
4. Prefer one green push over N red retries. Pushing untested commits is a protocol violation for agent seats.
