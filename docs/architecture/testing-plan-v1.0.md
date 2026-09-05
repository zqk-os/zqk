# Testing Plan for Storage Orchestration and Async Validation

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Active  
**Purpose**: Comprehensive testing plan before integrating into production system

## Overview

This document outlines the testing strategy for:
1. Deferred hash updates
2. Storage orchestration across multiple backends
3. Async validator baseline comparison
4. Performance benchmarks

## Test Categories

### 1. Unit Tests

#### Deferred Hash Manager Tests
- ✅ `TestDeferredHashManager_BasicOperations`: Basic register/complete operations
- ✅ `TestDeferredHashManager_MultipleOperations`: Multiple operations per object
- ✅ `TestDeferredHashManager_ConcurrentOperations`: Concurrent operation handling
- ✅ `TestDeferredHashManager_HashUpdate`: Hash update after operations complete

#### Storage Orchestrator Tests
- ✅ `TestStorageOrchestrator_BasicOperations`: Basic backend registration and operation tracking
- ✅ `TestStorageOrchestrator_MultipleBackends`: Operations across multiple backends
- ✅ `TestStorageOrchestrator_ConcurrentOperations`: Concurrent operations across backends

### 2. Integration Tests

#### Async Validator Baseline Comparison
- `TestAsyncValidator_BaselineComparison`: Compare async vs sync validation results
- Verify same number of results
- Verify same issues found
- Verify async is faster (or at least not significantly slower)
- Verify all object IDs match

### 3. Performance Tests

#### Baseline Metrics Collection
- Run synchronous check command on production data
- Collect metrics:
  - Total objects checked
  - Total results
  - Duration
  - Objects/second
  - Issues by tier
  - Memory usage
  - CPU usage

#### Async Validator Performance
- Run async validator on same data
- Compare metrics:
  - Duration (should be faster)
  - Memory usage (should be similar or better)
  - CPU usage (should utilize multiple cores)
  - Throughput (objects/second)

### 4. Stress Tests

#### Concurrent Operations
- Multiple CLI processes running check simultaneously
- Multiple operations on same object
- High concurrency scenarios
- Verify no deadlocks
- Verify no data corruption
- Verify hash updates complete correctly

#### Large Scale Tests
- 10,000+ objects
- Multiple backends
- Concurrent operations
- Verify performance scales
- Verify memory usage is reasonable

## Test Execution Plan

### Phase 1: Unit Tests (Current)
- ✅ Deferred hash manager tests
- ✅ Storage orchestrator tests
- Fix compilation errors
- Run all unit tests

### Phase 2: Integration Tests
- Implement async validator baseline comparison
- Run on test data
- Verify results match
- Document performance improvements

### Phase 3: Baseline Collection
- Run synchronous check on production data
- Collect baseline metrics
- Document baseline performance
- Create baseline output file

### Phase 4: Async Validator Testing
- Run async validator on production data
- Compare results with baseline
- Verify correctness
- Measure performance improvements

### Phase 5: Stress Testing
- Concurrent operations
- Large scale tests
- Edge cases
- Error scenarios

### Phase 6: Production Integration
- Gradual rollout
- Monitor performance
- Collect metrics
- Verify correctness

## Success Criteria

### Correctness
- ✅ All unit tests pass
- Async validator produces same results as sync
- No data corruption
- Hash updates complete correctly

### Performance
- Async validator is faster than sync (or at least not significantly slower)
- Memory usage is reasonable
- CPU utilization is efficient
- Throughput is improved

### Reliability
- No deadlocks
- No race conditions
- Graceful error handling
- Proper cleanup

## Test Data

### Test Environment
- Use `ZQK_TEST_ROOT` environment variable
- Isolated test directories
- No impact on production data

### Test Objects
- Create test objects programmatically
- Various object kinds
- Various sizes
- Various complexity levels

## Metrics to Collect

### Synchronous Check Baseline
- Total objects: X
- Total results: Y
- Duration: Z seconds
- Objects/second: W
- Issues by tier: [T1: A, T2: B, T3: C, T4: D]
- Memory usage: M MB
- CPU usage: C%

### Async Validator
- Total objects: X
- Total results: Y
- Duration: Z seconds (should be < baseline)
- Objects/second: W (should be > baseline)
- Issues by tier: [T1: A, T2: B, T3: C, T4: D] (should match baseline)
- Memory usage: M MB (should be similar)
- CPU usage: C% (should utilize multiple cores)

## Next Steps

1. Fix compilation errors in test files
2. Run unit tests
3. Implement baseline collection command
4. Run baseline on production data
5. Implement async validator comparison
6. Run comparison tests
7. Document results
8. Proceed with integration

