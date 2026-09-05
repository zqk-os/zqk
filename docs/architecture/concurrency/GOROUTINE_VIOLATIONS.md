# Goroutines Not Using Builder Pattern

**Last Verified:** 2026-08-31


This document lists all goroutines in production code that are not using the `goroutinelabels.NewGoroutine()` builder pattern.

## Production Code Violations

### cmd/zqk/system/async_check.go
- **Line 451**: `g.Go(func() error {` - Uses errgroup, acceptable to use `SetGoroutineLabel()` directly
  - Note: This is in an errgroup context, so builder pattern cannot be used

### pkg/scheduler/handlers.go
- **Line 143**: `go func() {` - Simple wait group closure goroutine
  - Purpose: Wait for tier 2 cache completion
  - Should use: `NewGoroutine().WithWaitGroup().StartSimple()`
  
- **Line 201**: `go func() {` - Simple wait group closure goroutine
  - Purpose: Wait for tier 3 cache completion
  - Should use: `NewGoroutine().WithWaitGroup().StartSimple()`

### pkg/storage/cas_orphan_cleanup_queue.go
- **Line 525**: `go func() {` - Simple wait group closure goroutine
  - Purpose: Wait for worker to finish processing
  - Should use: `NewGoroutine().WithWaitGroup().StartSimple()`

### pkg/storage/hash_registry_manager.go
- **Line 71**: `go func(hr *HashRegistry) {` - Worker goroutine with wait group
  - Purpose: Drain hash registry
  - Should use: `NewGoroutine().WithWaitGroup().StartSimple()`
  
- **Line 80**: `go func() {` - Simple wait group closure goroutine
  - Purpose: Wait for drain operations to complete
  - Should use: `NewGoroutine().WithWaitGroup().StartSimple()`

### pkg/storage/id_generation/queue.go
- **Line 409**: `go func() {` - Simple wait group closure goroutine
  - Purpose: Wait for worker to finish
  - Should use: `NewGoroutine().WithWaitGroup().StartSimple()`

## Excluded (Acceptable)

### pkg/goroutinelabels/labels.go
- **Line 43**: `go func() {` - Part of `StartNamedGoroutine()` convenience wrapper
  - This is the implementation of the goroutine labeling infrastructure itself
  - Acceptable to not use builder here

### cmd/zqk/system/async_check.go
- **Line 451**: `g.Go(func() error {` - Uses errgroup
  - Note: errgroup manages goroutine lifecycle, so builder pattern cannot be used
  - Currently uses `SetGoroutineLabel()` directly, which is acceptable

## Summary

**Total Production Violations: 6 goroutines** (excluding errgroup and infrastructure code)

All of these are simple wait group closure goroutines or worker goroutines that should be converted to use the builder pattern for:
- Automatic goroutine labeling
- Panic recovery
- Consistent lifecycle management
- Better debugging and profiling

## Test Files

Test files are excluded from this list as they may have different requirements. However, if you want test files to also use the builder pattern, there are many more violations in test code (40+ files with `go func()` calls).
