# Data Management Optimization Plan

**Last Verified:** 2026-08-31


**Date:** 2026-01-19  
**Status:** In Progress  
**Goal:** Reduce total object counts from current levels to target ~3,000

## Current Situation

### Object Count Analysis (from 2026-01-17 snapshot)
- **Total Objects**: 84,825
- **Target**: ~3,000
- **Over Target**: **28x**

### Breakdown by Category

#### 1. Raw Events (32,606 objects - 38% of total)
- **audit_event**: 29,102
- **change_journal_entry**: 3,504
- **Status**: Jobs exist (SCH-002, SCH-003) but may not be running frequently enough

#### 2. Metric Objects (50,216 objects - 59% of total) ⚠️ **CRITICAL**

**8 metric types, each with ~6,277 objects:**

| Metric Type | Count | Cleanup Job | Status |
|------------|-------|-------------|--------|
| audit_aggregation_metric | 6,277 | SCH-015 | ⚠️ Job exists but may not be running |
| base_metric | 6,277 | ❌ None | **NO CLEANUP** |
| code_quality_metric | 6,277 | ❌ None | **NO CLEANUP** |
| command_metric | 6,277 | SCH-016 | ⚠️ Job exists but may not be running |
| file_lock_metric | 6,277 | ❌ None | **NO CLEANUP** |
| kind_mapping_metric | 6,277 | ❌ None | **NO CLEANUP** |
| scheduler_health_metric | 6,277 | ❌ None | **NO CLEANUP** |
| test_audit_aggregation_metric | 6,277 | ❌ None | **NO CLEANUP** |

**Total**: 50,216 metric objects

#### 3. Other Objects (~2,000 objects - 3% of total)
- scheduler_job: 405
- backlog_item: 376
- criteria: 333
- doc_entry: 274
- mcp_session: 196
- ... (various other types)

## Root Causes

### 1. Missing Cleanup Jobs ⚠️ **HIGH PRIORITY**
- **6 of 8 metric types have NO cleanup jobs**:
  - base_metric
  - code_quality_metric
  - file_lock_metric
  - kind_mapping_metric
  - scheduler_health_metric
  - test_audit_aggregation_metric

**Impact**: These metric types accumulate indefinitely with no cleanup mechanism.

### 2. Aggregation Frequency May Be Insufficient
- **SCH-002** (audit aggregation): May need more frequent runs
- **SCH-003** (change journal aggregation): May need more frequent runs
- **Window size**: Current 15m window may only process recent events, not backlog

### 3. Retention Periods May Be Too Long
- Metrics may be retained longer than necessary
- Need to balance historical data vs. system health

## Optimization Strategy

### Phase 1: Immediate Actions (High Priority)

#### 1.1 Create Missing Cleanup Jobs
**Goal**: Create cleanup handlers and jobs for 6 missing metric types

**Metric Types Needing Cleanup Jobs:**
1. `base_metric` - Generic metrics
2. `code_quality_metric` - Code quality metrics
3. `file_lock_metric` - File lock metrics
4. `kind_mapping_metric` - Kind mapping metrics
5. `scheduler_health_metric` - Scheduler health metrics
6. `test_audit_aggregation_metric` - Test aggregation metrics

**Approach:**
- Create unified metric cleanup handler (similar to `AggregationMetricsCleanupHandler`)
- Create scheduler jobs for each metric type
- Use consistent retention period (e.g., 30 days)
- Schedule jobs to run frequently (e.g., every 6 hours)

#### 1.2 Verify Existing Cleanup Jobs Are Running
**Goal**: Ensure SCH-015 and SCH-016 are executing properly

**Actions:**
- Check scheduler logs for job execution
- Verify job schedules are correct
- Test manual job execution
- Monitor job execution frequency

#### 1.3 Increase Aggregation Frequency
**Goal**: Process events more frequently to prevent accumulation

**Actions:**
- Reduce SCH-002 window from 15m to 5m (process more frequently)
- Reduce SCH-003 window from 15m to 5m
- Ensure jobs run at least every 15 minutes
- Consider running aggregation jobs every 10 minutes during high-activity periods

### Phase 2: Short-term Improvements

#### 2.1 Unified Metric Cleanup Handler
**Goal**: Single handler that can clean up all metric types

**Benefits:**
- Consistent cleanup logic across all metric types
- Easier to maintain and update
- Can apply same retention policies

**Implementation:**
- Create `GenericMetricsCleanupHandler` in `pkg/scheduler/handlers.go`
- Accept metric kind as parameter
- Use same retention logic as `AggregationMetricsCleanupHandler`
- Create scheduler jobs that call this handler with different metric kinds

#### 2.2 Aggressive Cleanup When Over Limit
**Goal**: Automatically trigger more aggressive cleanup when object counts exceed limits

**Current Implementation:**
- `AuditAggregationHandler` already has aggressive cleanup logic (reduces retention to 50% when over limit)
- Apply same pattern to all cleanup handlers

**Actions:**
- Add object count checks to all cleanup handlers
- Trigger aggressive cleanup (reduce retention by 50%) when over 3k limit
- Log cleanup actions for monitoring

#### 2.3 Reduce Retention Periods
**Goal**: Keep metrics for shorter periods to reduce accumulation

**Current Retention:**
- Audit aggregation metrics: 30 days (configurable)
- Command metrics: 30 days (configurable)

**Proposed Retention:**
- Metrics: 7-14 days (reduce from 30 days)
- Aggregated events: 14 days (reduce from 30 days)
- Keep only recent metrics for trending

### Phase 3: Long-term Optimizations

#### 3.1 In-Memory Metric Buffering
**Goal**: Buffer metrics in memory before writing to reduce object creation

**Benefits:**
- Fewer individual metric objects
- Batch writes reduce I/O
- Can aggregate before persistence

**Implementation:**
- Similar to `AuditEventBuffer` for audit events
- Buffer metrics in memory
- Flush periodically (e.g., every 15 minutes)
- Aggregate similar metrics before writing

#### 3.2 Metric Sampling
**Goal**: Sample high-frequency metrics to reduce volume

**Approach:**
- Sample metrics at configurable rate (e.g., 1 in 10)
- Keep all metrics for low-frequency events
- Aggregate sampled metrics

#### 3.3 Automatic Cleanup Triggers
**Goal**: Automatically trigger cleanup when thresholds are exceeded

**Implementation:**
- Monitor object counts continuously
- Trigger cleanup jobs automatically when counts exceed thresholds
- No need to wait for scheduled runs

## Implementation Plan

### Step 1: Create Unified Metric Cleanup Handler
1. Create `GenericMetricsCleanupHandler` in `pkg/scheduler/handlers.go`
2. Support all metric types via parameter
3. Use same retention logic as existing handlers
4. Add aggressive cleanup when over limit

### Step 2: Create Cleanup Jobs for Missing Metric Types
1. Create scheduler job definitions for 6 missing metric types
2. Use unified handler with appropriate metric kind
3. Schedule jobs to run every 6 hours
4. Set retention period to 14 days

### Step 3: Verify and Optimize Existing Jobs
1. Check SCH-002 and SCH-003 execution frequency
2. Reduce aggregation windows if needed
3. Verify SCH-015 and SCH-016 are running
4. Monitor job execution logs

### Step 4: Test and Monitor
1. Run cleanup jobs manually to verify functionality
2. Monitor object counts after cleanup
3. Adjust retention periods based on results
4. Verify system health after cleanup

## Success Criteria

### Immediate (Week 1)
- ✅ All 8 metric types have cleanup jobs
- ✅ Cleanup jobs are executing properly
- ✅ Object counts start decreasing

### Short-term (Month 1)
- ✅ Total object count below 10,000
- ✅ Metric objects below 3,000 per type
- ✅ Raw events below 1,500 per type

### Long-term (Month 3)
- ✅ Total object count at or below target (~3,000)
- ✅ Automatic cleanup working
- ✅ System maintaining healthy object counts

## Next Steps

1. **Create unified metric cleanup handler** (Priority 1)
2. **Create cleanup jobs for 6 missing metric types** (Priority 1)
3. **Verify existing cleanup jobs are running** (Priority 2)
4. **Increase aggregation frequency** (Priority 2)
5. **Test and monitor** (Priority 3)
