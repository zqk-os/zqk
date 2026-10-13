# CI Coverage Delta Caching and TempDir Race Remediation

## Executive Summary

As the ZQK codebase expands across modular kernel packages, CLI command suites, and background event daemons, comprehensive test coverage execution times approach workflow timeouts (e.g. GitHub Actions 30-minute threshold). Concurrently, asynchronous background workers in test suites—such as agent task hourglass heartbeats and ambient nervous system audit logging—can outlive test contexts, causing non-deterministic filesystem races and `unlinkat ...: directory not empty` failures during temporary directory (`t.TempDir()`) teardown.

This architecture specification formalizes two structural engineering patterns to maintain fast, deterministic CI pipelines:
1. **Delta Coverage Caching:** Content-addressed caching of test coverage profiles for unchanged Go package boundaries, combining cached coverage fragments with freshly executed tests into a unified coverage report.
2. **Deterministic Teardown Synchronization:** Rigorous lifecycle boundaries for background goroutines (`agent_sync_hourglass`, `ambient_audit`), replacing arbitrary sleeps with explicit `sync.WaitGroup` coordination (`EventHub.WaitForAudits()`, `sync_loop` hourglass waitgroups) and CAS write-behind queue draining before temporary directory cleanup.

---

## Part I: CI Coverage Delta Caching

### 1. Problem Statement & Motivation
A monolithic `go test -coverprofile=coverage.out ./...` re-executes tests across all packages regardless of whether their source or dependency tree changed. In pull request evaluation where only a small subset of packages (e.g., `cmd/zqk/callback` or `pkg/mcp`) is modified, running the entire suite with coverage instrumentation adds 15–20 minutes of redundant CPU cycles.

### 2. Delta Caching Algorithm
The CI coverage workflow partitions package graphs and leverages Git commit trees and Go module dependency graphs:
1. **Change Boundary Detection:** Compute the affected package set $P_{\Delta}$ by inspecting changed files between the base branch (`origin/main`) and the PR head:
   ```bash
   CHANGED_PKGS=$(go list -f '{{.ImportPath}}' ./... | while read pkg; do
     if git diff --name-only origin/main...HEAD | grep -q "^${pkg#github.com/zqk-os/zqk/}/"; then
       echo "$pkg"
     fi
   done)
   ```
2. **Cache Key Formulation:** For unchanged packages $P_{\text{base}} \setminus P_{\Delta}$, the coverage fragment is identified by a content hash:
   $$\text{Key} = \text{SHA256}(\text{PackageAST} \cup \text{GoVersion} \cup \text{DirectDeps})$$
3. **Execution Partitioning:**
   - Packages in $P_{\Delta}$ are executed under `go test -coverprofile=... -covermode=atomic`.
   - Unmodified packages retrieve pre-computed coverage blocks from the CI cache store.
4. **Coverage Matrix Stitching:** A post-processing tool merges coverage profile fragments into the canonical unified `coverage.out`, ensuring accurate overall delta coverage metrics without re-running unchanged test suites.

### 3. Fail-Closed Invariants
- **Full Run Trigger on Shared Packages:** Any modification to foundational packages (`pkg/objects`, `pkg/storage`, `pkg/context`) invalidates the entire cache and triggers a full, non-delta coverage run.
- **Scheduled Convergence Gate:** Nightly builds and release tags bypass delta caching and execute the full test suite with `-race` and non-cached coverage to detect subtle cross-package regression.

---

## Part II: Deterministic Teardown Synchronization

### 1. Problem Statement & Directory Teardown Races
In Go test suites using `t.TempDir()` or custom project test roots, tests instantiate mock projects containing CAS structures, listing indexes, and storage write-behind queues. Several background routines interact with these directories:
- **`agent_sync_hourglass`:** Refreshes task lease timestamps in CAS storage every minute while `sync-loop` executes.
- **`ambient_audit`:** Asynchronously persists audit events to the project data directory upon `EventHub.Publish()`.

When a test function completes, `t.Cleanup` or deferred calls execute `os.RemoveAll(tempDir)`. If background goroutines are actively writing files or creating subdirectories during `os.RemoveAll`, OS filesystem calls fail with:
```
unlinkat /tmp/.../project_data: directory not empty
```
or panic due to writing into closed storage contexts.

### 2. Synchronization Architecture

#### A. Hourglass Supervised WaitGroup Pattern
The agent sync loop launches its background hourglass refresher under an explicit cancellation context and `sync.WaitGroup`. On loop termination (clean exit, error, or context timeout), the cancel function signals the goroutine, and teardown strictly blocks on `hourglassWg.Wait()`:

```go
// cmd/zqk/agent/sync_loop.go
ctxHourglass, cancelHourglass := context.WithCancel(ctx)
var hourglassWg sync.WaitGroup
hourglassWg.Add(1)
defer func() {
    cancelHourglass()
    hourglassWg.Wait()
}()

goroutinelabels.NewGoroutine("agent_sync_hourglass", "refresh agent_task hourglass while sync-loop runs").
    StartSimple(func() {
        defer hourglassWg.Done()
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        for {
            select {
            case <-ctxHourglass.Done():
                return
            case <-ticker.C:
                flipHourglass(ctxHourglass, secCtx, sp, taskID, proc.ProjectRoot())
                _ = agentclaim.RenewCheckin(proc.ProjectRoot(), taskID)
            }
        }
    })
```

#### B. EventHub Deterministic Drain Pattern (`WaitForAudits`)
Rather than relying on non-deterministic arbitrary sleeps (`time.Sleep(1 * time.Second)`), `EventHub` provides explicit synchronization via `WaitForAudits()`:

```go
// pkg/ambient/eventhub.go
type EventHub interface {
    Subscribe(eventType EventType, handler EventHandler)
    Publish(ctx context.Context, event Event) error
    Status() string
    EnableEventSourcing(projectRoot string, secCtx *pkgctx.SecurityContext)
    WaitForAudits()
}

func (h *eventHubImpl) Publish(ctx context.Context, event Event) error {
    // ... synchronous dispatch to subscribers ...
    if root != "" && sCtx != nil {
        h.auditWg.Add(1)
        goroutinelabels.NewGoroutine("ambient_audit", "emit audit for ambient event").
            StartSimple(func() {
                defer h.auditWg.Done()
                _ = storage.CreateAuditEventWithBuilder(context.Background(), root, sCtx, nil, opts)
            })
    }
    return nil
}

func (h *eventHubImpl) WaitForAudits() {
    h.auditWg.Wait()
}
```

In test suites:
```go
// pkg/ambient/extra_coverage_test.go
hub.Publish(context.Background(), ev)
hub.WaitForAudits()
storage.ScrubProjectRootForTempCleanup(tempDir, 20, 25*time.Millisecond)
```

#### C. CAS Write Queue and Membrane Scrubbing
Before removing temporary project structures, test harnesses invoke:
1. `writeQueue.FlushAll(1 * time.Second)`: Flushes in-memory CAS index mutations.
2. `storage.RunProjectTestTeardown`: Shuts down hash registries, audit buffers, and Darwin sync queues.
3. `storage.ScrubProjectRootForTempCleanup`: Retries removing residual locked or transitional files with exponential backoff.

---

## Part III: Verification and Invariants

| Invariant Name | Verification Method | Enforcement Mechanism |
| :--- | :--- | :--- |
| **Zero Sleep Teardown** | AST / Linter Grep | Rejection of naked `time.Sleep` in event hub and storage test teardown |
| **Supervised Goroutine Drain** | `go test -race` | Goroutine leak checks and waitgroup completion prior to test return |
| **CAS Write-Behind Quiescence** | Integration Tests | Clean `unlinkat` on temporary directories across 1,000 stress cycles |
| **Coverage Profile Integrity** | CI Stitch Validator | Merged `coverage.out` matches exact AST statement counts |

---

## Conclusion

By formalizing delta coverage caching and deterministic teardown synchronization, ZQK eliminates flakiness and timeout bottlenecks in continuous integration. These patterns preserve open-core engineering rigor, ensuring tests are fast, thread-safe, and deterministic across all environments.
