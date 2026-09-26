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

