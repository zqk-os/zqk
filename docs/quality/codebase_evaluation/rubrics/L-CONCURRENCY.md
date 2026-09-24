# Rubric: L-CONCURRENCY

**Lens:** `L-CONCURRENCY` · **Density:** D-MED · **Axes:** ROB, REL, RCV

## Criteria

1. Shared mutable state without synchronization.  
2. Lock ordering / deadlock risk.  
3. Goroutine/thread leak patterns.  
4. Cancellation propagation (contexts, lifetimes).  
5. Race-detector / sanitizer usage in CI (or tooling gap).

## Citations

| Work | Point |
|------|-------|
| Herlihy & Shavit — *The Art of Multiprocessor Programming* | Correctness under concurrency (conceptual) |
| Go memory model / Effective Go concurrency | When language is Go |
| Java Memory Model / Bloch concurrency items | When language is Java |
| Rustonomicon (fearless concurrency limits) | When language is Rust |

## Pros / cons

| Pros | Cons |
|------|------|
| Language-model adapters keep citations honest | Static detection of races is incomplete — note limits |
| Cancellation is often missing in reviews | Over-flagging channel usage style — need failure evidence |
