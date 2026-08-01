# Data Management Optimization - Progress Report

**Date:** 2026-01-19  
**Status:** In Progress

## Completed

### ✅ 1. Created Unified Generic Metrics Cleanup Handler
- **File**: `pkg/scheduler/handlers.go`
- **Handler**: `GenericMetricsCleanupHandler`
- **Features**:
  - Works with any metric type (specified via `METRIC_KIND` env var)
  - Default retention: 14 days (reduced from 30 for better data management)
  - Aggressive cleanup when over 3k limit (reduces retention by 50%)
  - Uses optimized bulk delete for efficiency
  - Integrated into handler factory

### ✅ 2. Fixed WaitGroup Double-Counting Issue
- **Issue**: Workers were calling `wg.Done()` even though `WithWaitGroup()` already calls it
- **Fixed in**:
  - `cas_index_write_queue.go`
  - `hash_registry.go`
  - `io_queue.go`
- **Result**: Prevents "negative WaitGroup counter" panics

### ✅ 3. Removed I/O Queue Routing Feature Flag
- **Removed**: `io_queue_routing` feature flag
- **Result**: I/O queue routing is now always enabled (default behavior)
- **Files Modified**:
  - `pkg/storage/object_storage_file.go`
  - `pkg/storage/io_queue.go`
  - `pkg/featureflags/feature_flags.go`

## In Progress

### 🔄 4. Create Cleanup Jobs for Missing Metric Types
**Status**: Ready to create

**Missing Jobs** (6 metric types):
1. `base_metric` - SCH-018
2. `code_quality_metric` - SCH-019
3. `file_lock_metric` - SCH-020
4. `kind_mapping_metric` - SCH-021
5. `scheduler_health_metric` - SCH-022
6. `test_audit_aggregation_metric` - SCH-023

**Job Configuration**:
- **Job Type**: `generic_metrics_cleanup`
- **Schedule**: Every 6 hours (`0 */6 * * *`)
- **Retention**: 14 days (default)
- **Environment Variables**:
  - `METRIC_KIND`: [metric type name]
  - `RETENTION_DAYS`: "14"

## Next Steps

1. **Create 6 cleanup job YAML files** (Priority 1)
2. **Fix SCH-015 job_type** (should be `aggregation_metrics_cleanup`, not `context_refresh`)
3. **Verify existing cleanup jobs are running** (SCH-015, SCH-016)
4. **Increase aggregation frequency** if needed (SCH-002, SCH-003)
5. **Test cleanup jobs manually** to verify functionality
6. **Monitor object counts** after cleanup jobs run

## Notes

- Scheduler jobs are stored as YAML files in `docs/architecture/scheduler_jobs/`
- Job IDs are hashed based on content (SHA-256)
- Jobs need to be created via `zqk object create` or similar command
- After creation, scheduler will automatically load and schedule them
