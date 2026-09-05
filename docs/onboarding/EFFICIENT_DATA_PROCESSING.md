# Efficient Data Processing and Pipeline Optimization

## Overview

This document outlines architectural and coding best practices for building efficient data processing pipelines in the ZQK codebase. These principles help ensure scalability, maintainability, and optimal resource utilization when working with large datasets or high-frequency operations.

## Core Principles

### 1. Understand State Transitions

Before optimizing any process, understand the state transitions required to achieve the desired end state:

1. **Identify Current State**: What is the initial condition?
2. **Identify Desired State**: What is the target condition?
3. **Map Transitions**: What steps are necessary to move from current to desired?
4. **Analyze Dependencies**: Which transitions depend on prior conditions?

**Example**: When auto-fixing validation issues:
- **Current State**: Object with validation violations
- **Desired State**: Object with all auto-fixable issues resolved
- **Transitions**: Each fix updates the object (state change)
- **Dependencies**: Higher-level fixes (goals) must complete before lower-level fixes (backlog items)

### 2. Identify Sequential vs. Parallel Operations

Not all operations can be parallelized. Identify which operations are:

- **Sequential (Dependent)**: Operations that must execute in order because later operations depend on earlier results
  - Example: Multiple fixes to the same object must be sequential (they update the same state)
  - Pattern: `O(n)` sequential processing where `n` = number of operations

- **Parallel (Independent)**: Operations that can execute concurrently because they operate on independent data
  - Example: Fixing different objects can be parallel (they update different states)
  - Pattern: Process in dependency layers (sequential) with parallel execution within each layer

**Rule of Thumb**: 
- Operations on the same resource/state → Sequential
- Operations on different resources/states → Parallel (if no cross-dependencies)

### 3. Target Optimal Big O Complexity

Always analyze algorithmic complexity and target optimal patterns:

- **Prefer O(1) or O(log n)**: Constant or logarithmic complexity
  - Example: Hash map lookups, binary search
  
- **Accept O(n) when necessary**: Linear complexity is acceptable for single-pass operations
  - Example: Reading all files once, processing all objects once
  
- **Avoid O(n²) or worse**: Quadratic or higher complexity should be refactored
  - Solution: Use indexing, caching, or divide-and-conquer strategies

- **For large n, split into smaller groups**: When `n` is large, split into batches and process in parallel
  - Example: Process objects in dependency-ordered layers (sequential layers, parallel within layer)
  - Configurable batch size (e.g., 1000 objects per batch)

### 4. Eliminate Redundant Resource Creation

Resource creation (factories, loaders, clients) should happen at the highest appropriate scope:

- **Per-operation scope**: ❌ Creates resources repeatedly
  ```go
  // BAD: Creates factory for each issue
  for _, issue := range issues {
      factory := NewFactory()  // O(n) creations
      process(issue, factory)
  }
  ```

- **Per-entity scope**: ✅ Creates resources once per entity
  ```go
  // GOOD: Creates factory once per object
  factory := NewFactory()  // O(1) creation
  for _, issue := range issues {
      process(issue, factory)  // Reuse factory
  }
  ```

**Common Redundancies to Eliminate**:
- Storage factories (create once per object, not per fix command)
- Spec loaders (create once per object, not per issue)
- Validators (create once, reuse)
- HTTP clients (create once, reuse connection pool)
- Database connections (use connection pool)

### 5. Batch Operations When Possible

When multiple operations target the same resource, batch them to reduce overhead:

- **Before**: Multiple Read + Update cycles
  ```
  Operation 1: Read → Update
  Operation 2: Read → Update
  Operation 3: Read → Update
  Total: 6 storage operations
  ```

- **After**: Single Read + Batch Update
  ```
  Read once → Apply all changes → Update once
  Total: 2 storage operations
  ```

**Constraints**: 
- Must respect dependency ordering (cannot batch dependent operations)
- Must ensure atomicity (either all changes succeed or all fail)
- Consider transaction boundaries and rollback scenarios

### 6. Cache and Reuse Expensive Operations

Expensive operations should be cached and reused:

- **Spec loading**: Load specs once, reuse across validations
- **Storage factory creation**: Create once, reuse for multiple operations
- **Parser instances**: Create once, reuse for parsing multiple files
- **Hash registry instances**: Cache per directory, reuse across checks

**Pattern**: 
```go
// Create expensive resource once
loader := NewExpensiveLoader()
cache := make(map[string]Resource)

// Reuse across operations
for _, item := range items {
    key := item.Key()
    if resource, ok := cache[key]; ok {
        use(resource)  // Cache hit
    } else {
        resource := loader.Load(key)
        cache[key] = resource
        use(resource)  // Cache and use
    }
}
```

## Rules and Best Practices for Efficient Data Processing and Pipelines

### Rule 1: Analyze Dependencies First

Before designing a pipeline, identify all dependencies:

1. **Map dependency graph**: Which operations depend on which?
2. **Identify dependency layers**: Group operations by dependency level
3. **Process layers sequentially**: Higher layers (fewer dependencies) before lower layers
4. **Process within layers in parallel**: Operations in the same layer can run concurrently

**Example - Auto-Fix Dependency Hierarchy**:
```
Layer 4 (Top): Goals (no dependencies)
  ↓
Layer 3: Priority Plans (depend on goals)
  ↓
Layer 2: Milestones (depend on goals/priority plans)
  ↓
Layer 1 (Bottom): Backlog Items (depend on milestones/goals)
```

Implementation:
- Process layers sequentially (Layer 4 → Layer 3 → Layer 2 → Layer 1)
- Process objects within each layer in parallel (with configurable batch size)

### Rule 2: Minimize Resource Creation Overhead

Create resources at the highest appropriate scope:

- **Per-process scope**: Application-level resources (loggers, config)
- **Per-request scope**: Request-level resources (storage factory, validators)
- **Per-entity scope**: Entity-level resources (spec loader per object kind)
- **Per-operation scope**: ❌ Avoid (creates unnecessary overhead)

**Optimization Checklist**:
- [ ] Are factories/loaders created once per entity, not per operation?
- [ ] Are expensive objects (parsers, validators) cached and reused?
- [ ] Are storage connections pooled and reused?
- [ ] Are HTTP clients created once and reused?

### Rule 3: Reduce Storage Operations

Minimize I/O operations by batching and caching:

- **Batch updates**: Collect multiple field changes, apply in single update
- **Cache reads**: Cache object reads when multiple operations need the same data
- **Defer writes**: Buffer writes and flush in batches when safe
- **Avoid redundant reads**: Don't read after update if you already have the state

**Pattern - Batch Field Updates**:
```go
// BAD: Multiple Read+Update cycles
for _, fix := range fixes {
    obj := storage.Read(id)
    applyFix(obj, fix)
    storage.Update(id, obj)  // N Read+Update operations
}

// GOOD: Single Read, batch updates, single Update
obj := storage.Read(id)
for _, fix := range fixes {
    applyFix(obj, fix)  // Accumulate changes
}
storage.Update(id, obj)  // 1 Read+Update operation
```

### Rule 4: Use Dependency-Aware Ordering

When processing multiple items, order by dependencies:

1. **Sort by dependency level**: Higher levels (fewer dependencies) first
2. **Within same level, sort by priority**: Tier 1 (blocking) before Tier 2 (warnings)
3. **Process in batches**: Group by dependency level, process levels sequentially
4. **Parallelize within batches**: Process items in the same batch concurrently

**Implementation Pattern**:
```go
// Group by dependency level
issuesByLayer := make(map[int][]Issue)
for _, issue := range issues {
    level := getDependencyLevel(issue)
    issuesByLayer[level] = append(issuesByLayer[level], issue)
}

// Process layers sequentially (highest level first)
levels := []int{4, 3, 2, 1}
for _, level := range levels {
    batch := issuesByLayer[level]
    processBatchInParallel(batch)  // Parallel within layer
}
```

### Rule 5: Measure and Profile

Always measure before and after optimizations:

- **Profile CPU usage**: Identify hotspots
- **Measure memory usage**: Check for leaks or excessive allocation
- **Count operations**: Track storage I/O, network calls, object creations
- **Benchmark critical paths**: Use Go's `testing.B` for benchmarks

**Example Metrics to Track**:
- Number of storage factory creations
- Number of storage Read/Update operations
- Number of spec loader creations
- Number of cache hits/misses
- Time spent in each dependency layer

### Rule 6: Design for Scale

Consider how the system behaves as input grows:

- **Input size n**: How does performance scale with number of objects?
- **Operations per object m**: How does performance scale with issues per object?
- **Total complexity**: O(n × m) sequential vs. O(layers × batch_time) with parallelization

**Scalability Patterns**:
- **Horizontal scaling**: Process multiple objects in parallel
- **Vertical scaling**: Optimize single-object processing (reduce m)
- **Batching**: Split large n into manageable batches
- **Streaming**: Process items as they arrive, don't buffer entire dataset

## Practical Examples

### Example 1: Auto-Fix Optimization

**Problem**: Auto-fix was creating `SpecBasedAutoFixer` and storage factory for each issue.

**Before**:
```go
for _, issue := range issues {
    fixer := NewSpecBasedAutoFixer(ctx, logger)  // O(m) creations
    factory := storage.NewStorageFactory(ctx, root)  // O(m) creations
    fixer.Fix(issue, factory)
}
// Complexity: O(m) resource creations
```

**After**:
```go
fixer := NewSpecBasedAutoFixer(ctx, logger)  // O(1) creation
factory := storage.NewStorageFactory(ctx, root)  // O(1) creation
for _, issue := range issues {
    fixer.FixWithStorage(issue, factory.GetStorage())  // Reuse
}
// Complexity: O(1) resource creations
```

**Improvement**: Reduced from O(m) to O(1) resource creations per object.

### Example 2: Dependency-Aware Batch Processing

**Problem**: Processing fixes for multiple objects without respecting dependencies.

**Before**:
```go
// Process all objects in parallel (incorrect - breaks dependencies)
for _, obj := range objects {
    go fixObject(obj)  // Object B might need Object A's fixes first
}
```

**After**:
```go
// Group by dependency level
objectsByLevel := groupByDependencyLevel(objects)
levels := []int{4, 3, 2, 1}  // Highest to lowest

// Process levels sequentially
for _, level := range levels {
    batch := objectsByLevel[level]
    // Process objects in level in parallel
    processBatchInParallel(batch)  // Safe - no cross-dependencies
}
```

**Improvement**: Respects dependencies while maximizing parallelization.

### Example 3: Eliminating Redundant Reads

**Problem**: Reading object after update when state is already known.

**Before**:
```go
success := executeFixCommand(obj, command)
if success {
    updatedObj := storage.Read(obj.ID)  // Redundant read
    return updatedObj
}
```

**After**:
```go
success := executeFixCommand(obj, command)
if success {
    return nil  // Signal: object already updated, no need to read
}
```

**Improvement**: Eliminated redundant storage operation.

## Checklist for Code Review

When reviewing code for efficiency, check:

- [ ] Resources (factories, loaders, clients) are created at appropriate scope (not per operation)
- [ ] Dependencies are analyzed and respected (sequential vs. parallel)
- [ ] Operations are batched when possible (reduce I/O)
- [ ] Redundant operations are eliminated (caching, avoiding duplicate reads)
- [ ] Complexity is analyzed (target O(1) or O(log n), accept O(n) when necessary)
- [ ] Large inputs are split into batches with configurable size
- [ ] Parallelization is used where safe (independent operations)
- [ ] Sequential processing is used where required (dependent operations)

## Related Documentation

- [AI Agent Onboarding](./AI_AGENT_ONBOARDING.md) - General agent onboarding guidelines
- [CLI Handler Patterns](../CLI_HANDLER_PATTERNS.md) - Patterns for CLI command handlers
- Architecture documentation in `docs/process/architecture/`
