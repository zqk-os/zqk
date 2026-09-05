# On-Demand Pattern - Consistency Analysis

**Last Verified:** 2026-08-31


**Date:** 2026-01-19  
**Status:** Analysis  
**Purpose:** Evaluate whether batch operations should use on-demand pattern for consistency

## Question

Should we apply the on-demand pattern to batch operations (BulkDeleteOptimized, CommitAnalyzer) for consistency, even if they're already "efficient"?

## Analysis

### Current Batch Operation Pattern

**Components:**
1. **BulkDeleteOptimized** - Creates workers, processes batch, cleans up
2. **CommitAnalyzer.AnalyzeCommits** - Creates workers, processes batch, cleans up

**Current Behavior:**
- All workers start immediately when function is called
- Workers process all items in batch
- Function returns, workers are cleaned up
- No idle periods (operation is bounded)

### On-Demand Pattern for Batch Operations

**Proposed Behavior:**
- Workers wake on-demand as jobs are enqueued
- Workers shut down after idle timeout
- Same pattern as long-running services

## Trade-offs Analysis

### Efficiency Considerations

**Current Pattern (Batch):**
- ✅ **Pros:**
  - All workers start immediately - no wake-up delay
  - Simple: create workers, process, cleanup
  - No idle timeout logic needed (operation is bounded)
  - Lower overhead (no atomic counters, wake-up logic)
  
- ❌ **Cons:**
  - Inconsistent with long-running services
  - If jobs are enqueued gradually, workers might wait idle
  - No graceful shutdown coordination (though not needed for bounded operations)

**On-Demand Pattern (Batch):**
- ✅ **Pros:**
  - Consistent with long-running services
  - Workers only exist when needed
  - If jobs arrive gradually, workers wake as needed
  - Unified pattern across codebase
  
- ❌ **Cons:**
  - Wake-up overhead (minimal, but exists)
  - More complex (atomic counters, wake-up logic, idle timeout)
  - Idle timeout might trigger during batch if there are gaps
  - Overhead might exceed benefit for small batches

### Real-World Scenarios

**Scenario 1: Small Batch (10 items, 5 workers)**
- **Current:** 5 workers start, process 10 items, done (~2 items/worker)
- **On-demand:** Workers wake, process 2 items each, shut down
- **Verdict:** Current is simpler, on-demand adds unnecessary complexity

**Scenario 2: Large Batch with Gaps (1000 items, 10 workers, gaps in enqueue)**
- **Current:** 10 workers start, might wait for jobs to be enqueued
- **On-demand:** Workers wake as jobs arrive, no idle waiting
- **Verdict:** On-demand could be slightly more efficient

**Scenario 3: Very Large Batch (10000 items, 20 workers)**
- **Current:** 20 workers start, process continuously
- **On-demand:** Workers wake, process continuously
- **Verdict:** Similar efficiency, but current is simpler

## Key Distinction

### Long-Running Services vs Batch Operations

**Long-Running Services** (AsyncValidator, AsyncRouter, FixCommandExecutor):
- Workers exist for extended periods (hours/days)
- Long idle periods between operations
- **Benefit:** Eliminates idle worker overhead
- **Pattern:** On-demand is clearly beneficial

**Batch Operations** (BulkDeleteOptimized, AnalyzeCommits):
- Workers exist only for duration of batch (seconds/minutes)
- Operation is bounded - no idle periods after completion
- **Benefit:** Minimal (workers already cleaned up quickly)
- **Pattern:** Current pattern is already efficient

## Recommendation

### Option 1: Keep Current Pattern (Recommended)

**Rationale:**
- Batch operations are already efficient (workers cleaned up quickly)
- On-demand adds complexity without significant benefit
- Different patterns for different use cases is acceptable
- Simpler code is easier to maintain

**When to Use:**
- Bounded operations (function returns when complete)
- All work is known upfront
- Operation duration is short (seconds to minutes)

### Option 2: Apply On-Demand for Consistency

**Rationale:**
- Unified pattern across codebase
- Consistent behavior and code structure
- Easier to reason about (one pattern)
- Future-proof if batch operations evolve

**When to Use:**
- If we want strict consistency
- If batch operations might evolve to support streaming
- If we want unified shutdown coordination

## Hybrid Approach

**Best of Both Worlds:**
- Keep current pattern for simple batch operations
- Use on-demand pattern for:
  - Long-running services (already done)
  - Batch operations that might have gaps
  - Operations that could benefit from graceful shutdown

## Conclusion

**For Consistency:**
- We could apply on-demand pattern, but it adds complexity without clear benefit
- Different patterns for different use cases is acceptable architecture

**For Efficiency:**
- Current batch pattern is already efficient
- On-demand would add overhead without significant benefit
- Long-running services benefit more from on-demand

**Recommendation:**
- **Keep current pattern** for batch operations (BulkDeleteOptimized, AnalyzeCommits)
- **Use on-demand pattern** for long-running services (already implemented)
- **Document the distinction** so future developers understand when to use each pattern

## Context Pipeline

**Current:** Already uses on-demand goroutines (one per async listener)
**Assessment:** ✅ Already efficient - different pattern (not a worker pool)
**Recommendation:** No changes needed
