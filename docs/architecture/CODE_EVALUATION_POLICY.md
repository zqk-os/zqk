# Code Evaluation and Refactoring Policy

**Last Verified:** 2026-08-31


**Version**: 1.0.0  
**Created**: 2026-01-01  
**Status**: Active  
**Purpose**: Policy for evaluating code quality, identifying refactoring opportunities, and applying design patterns to improve maintainability, readability, efficiency, accuracy, and observability

## Overview

This policy establishes a systematic approach to evaluating code for refactoring opportunities and applying common design patterns. It ensures code quality improvements are identified, prioritized, and implemented consistently.

## Evaluation Criteria

### 1. Maintainability

**Indicators**:
- Code duplication (DRY violations)
- Cyclomatic complexity
- Function/class size
- Dependency coupling
- Test coverage

**Evaluation Questions**:
- Is code duplicated across multiple locations?
- Are functions/classes too large (> 200 lines)?
- Are there tight coupling between modules?
- Is test coverage below 80% for critical paths?

**Refactoring Opportunities**:
- Extract common logic into shared functions/utilities
- Apply Builder pattern for complex object construction
- Use Context Object pattern to encapsulate scattered logic
- Implement Strategy pattern for algorithm selection
- Create Adapter pattern for interface compatibility

### 2. Readability

**Indicators**:
- Naming clarity
- Code organization
- Documentation completeness
- Comment quality
- Consistent style

**Evaluation Questions**:
- Are variable/function names descriptive and clear?
- Is code organized logically (related code grouped together)?
- Are complex algorithms documented?
- Is the code style consistent with project standards?
- Are magic numbers/strings replaced with named constants?

**Refactoring Opportunities**:
- Rename variables/functions for clarity
- Extract complex logic into well-named functions
- Add documentation comments for public APIs
- Group related functionality using Context Objects
- Use Builder pattern for fluent, readable APIs

### 3. Efficiency

**Indicators**:
- Algorithm complexity
- Resource usage (memory, CPU)
- Database/API call patterns
- Caching opportunities
- Concurrency usage

**Evaluation Questions**:
- **CRITICAL**: Are there O(n²) or worse algorithms that could be optimized?
  - **Automated Detection**: `gocyclo` linter flags high complexity (nested loops)
  - **Example**: 15k objects × 15k iterations = 225M operations causing 700% CPU spikes
  - **Solution**: Use maps for O(1) lookups instead of nested loops
- Are resources (files, connections) properly closed?
- Are database/API calls batched where possible?
- Could caching improve performance?
- Are goroutines used appropriately for concurrent operations?

**Refactoring Opportunities**:
- **CRITICAL**: Optimize algorithms (e.g., use maps for O(1) lookups)
  - **Before**: O(n²) nested loops over large datasets
  - **After**: O(n) with map-based lookups
  - **Detection**: `gocyclo` and `gocritic` (performance tag) will flag these
- Implement caching strategies
- Batch operations to reduce I/O
- Use context objects to avoid repeated computations
- Apply async patterns for non-blocking operations

**Automated Enforcement**:
- `gocyclo` linter automatically flags functions with complexity >= 15
- `gocritic` (performance tag) detects performance anti-patterns
- Pre-commit hooks validate code before commit
- CI/CD fails builds with high complexity violations

### 4. Accuracy

**Indicators**:
- Error handling completeness
- Input validation
- Edge case handling
- Type safety
- State management

**Evaluation Questions**:
- Are all error paths handled?
- Is input validated before processing?
- Are edge cases (empty, nil, boundary values) handled?
- Are types used correctly (avoiding `interface{}` where possible)?
- Is state managed consistently?

**Refactoring Opportunities**:
- Add comprehensive error handling
- Implement input validation at boundaries
- Use type-safe interfaces instead of `interface{}`
- Apply Context Object pattern for consistent state management
- Use Strategy pattern for validation rules

### 5. Observability

**Indicators**:
- Logging coverage
- Metrics collection
- Tracing support
- Error reporting
- Debug information

**Evaluation Questions**:
- Are important operations logged?
- Are metrics collected for key operations?
- Is tracing available for request flows?
- Are errors reported with sufficient context?
- Is debug information available when needed?

**Refactoring Opportunities**:
- Add structured logging using logging framework
- Implement metrics collection for key operations
- Add tracing spans for request flows
- Use Event Emitter pattern for event-based observability
- Apply Middleware pattern for cross-cutting observability

## Design Pattern Application

### When to Apply Patterns

#### Handler Pattern
**Apply when**:
- Routing method/command calls
- Need clean separation of routing and logic
- Multiple handlers for different methods

**Example**: MCP method routing (`pkg/mcp/handler.go`)

#### Middleware Pattern
**Apply when**:
- Adding cross-cutting concerns (logging, auth, metrics)
- Need composable behavior
- Don't want to modify core logic

**Example**: Request/response tracing (`TraceMiddleware`)

#### Event Emitter Pattern
**Apply when**:
- Decoupling producers from consumers
- Multiple subscribers needed
- Real-time notifications required

**Example**: MCP event subscriptions (`pkg/mcp/event_emitter.go`)

#### Context Object Pattern
**Apply when**:
- Scattered boolean checks
- Repeated logic for related operations
- Need computed/derived properties

**Example**: `QueryContext`, `ClientEventContext`, `LoggingDecisionContext`

#### Builder Pattern
**Apply when**:
- Complex object construction
- Many optional parameters
- Want fluent API

**Example**: `ContextBuilder`, `ChainBuilder`

#### Strategy Pattern
**Apply when**:
- Multiple algorithms for same problem
- Runtime algorithm selection
- Easy to add new algorithms

**Example**: `BucketingStrategy`, authentication strategies

#### Adapter Pattern
**Apply when**:
- Bridging incompatible interfaces
- Integrating existing systems
- Gradual migration needed

**Example**: `LoggerEventAdapter`, CLI-to-MCP bridge

## Evaluation Process

### Step 1: Code Review Checklist

Before refactoring, evaluate code against criteria:

```markdown
## Maintainability
- [ ] No code duplication (DRY)
- [ ] Functions/classes appropriately sized
- [ ] Loose coupling between modules
- [ ] Adequate test coverage

## Readability
- [ ] Clear, descriptive naming
- [ ] Logical code organization
- [ ] Documentation for complex logic
- [ ] Consistent code style

## Efficiency
- [ ] Optimal algorithms
- [ ] Resources properly managed
- [ ] Caching where appropriate
- [ ] Appropriate concurrency

## Accuracy
- [ ] Comprehensive error handling
- [ ] Input validation
- [ ] Edge cases handled
- [ ] Type safety

## Observability
- [ ] Structured logging
- [ ] Metrics collection
- [ ] Tracing support
- [ ] Error context
```

### Step 2: Pattern Identification

For each issue identified, determine if a design pattern applies:

1. **Query Design Patterns Library**: Check `docs/process/architecture/DESIGN_PATTERNS.md`
2. **Match Problem to Pattern**: Use pattern descriptions to find matches
3. **Review Examples**: Check existing implementations
4. **Verify Applicability**: Ensure pattern solves the specific problem

### Step 3: Refactoring Plan

Create a refactoring plan:

```markdown
## Refactoring Plan

### Issue: [Description]
**Current State**: [What exists now]
**Problem**: [What's wrong]
**Pattern**: [Which pattern to apply]
**Solution**: [How pattern solves problem]
**Implementation**: [Steps to implement]
**Testing**: [How to verify]
**Risks**: [Potential issues]
```

### Step 4: Implementation

1. **Create Branch**: `refactor/apply-[pattern-name]-to-[component]`
2. **Apply Pattern**: Implement pattern following established examples
3. **Update Tests**: Ensure tests cover new pattern usage
4. **Update Documentation**: Document pattern application
5. **Review**: Submit for code review

### Step 5: Validation

After refactoring, validate improvements:

- [ ] Code duplication reduced
- [ ] Complexity decreased
- [ ] Test coverage maintained/improved
- [ ] Performance maintained/improved
- [ ] Observability improved
- [ ] Documentation updated

## Automated Evaluation

### Static Analysis Tools

**CRITICAL: Required Linters (See POL-CODE-008)**

The following linters **MUST** remain enabled and cannot be disabled without explicit approval:

**Linters**:
- `golangci-lint`: Comprehensive Go linting
- `errcheck`: Unchecked error detection
- `staticcheck`: Advanced static analysis
- `gofmt`: Code formatting

**Required Complexity & Anti-Pattern Detection** (POL-CODE-008):
- **`gocyclo`**: **REQUIRED** - Cyclomatic complexity detection
  - **Purpose**: Detects O(n²) nested loops, high complexity functions
  - **Why Critical**: Prevents performance regressions (e.g., 15k × 15k = 225M iterations causing 700% CPU spikes)
  - **Configuration**: `min-complexity: 15` (flags functions with complexity >= 15)
  - **Cannot be disabled** without architecture review approval
  
- **`gocritic`**: **REQUIRED** - Advanced code analysis
  - **Purpose**: Detects performance anti-patterns, code smells, best practice violations
  - **Why Critical**: Catches performance issues, inefficient algorithms, code duplication
  - **Required Tags**: `performance` tag **MUST** be enabled
  - **Cannot be disabled** without architecture review approval
  
- **`gosec`**: **REQUIRED** - Security analysis
  - **Purpose**: Detects security vulnerabilities, unsafe operations
  - **Why Critical**: Prevents security issues from entering codebase
  - **Cannot be disabled** without architecture review approval

**Additional Analysis**:
- `gocognit`: Cognitive complexity (optional)
- `unparam`: Unused parameters detection (required)
- `unused`: Unused code detection (required)

**Policy Reference**: See [POL-CODE-008](../policies/POL-CODE-008.yaml) for complete requirements and exception process.

### Metrics Collection

Track code quality metrics:

```go
// Example: Code quality metrics
type CodeQualityMetrics struct {
    CyclomaticComplexity int
    TestCoverage         float64
    DuplicationRatio     float64
    MaintainabilityIndex float64
    LinesOfCode          int
}
```

### Continuous Monitoring

- **Pre-commit Hooks**: Run linters before commit
- **CI/CD Pipeline**: Run static analysis in CI
- **Periodic Audits**: Weekly/monthly code quality reviews
- **Lifecycle Reminders**: Track refactoring opportunities

## Refactoring Priorities

### High Priority

1. **Security Issues**: Vulnerabilities, unsafe operations
2. **Critical Bugs**: Data corruption, crashes
3. **Performance Bottlenecks**: O(n²) algorithms, memory leaks
4. **Maintainability Blockers**: Unmaintainable code preventing features

### Medium Priority

1. **Code Duplication**: Repeated logic across multiple locations
2. **Complexity**: High cyclomatic complexity
3. **Observability Gaps**: Missing logging/metrics
4. **Pattern Violations**: Code that could use established patterns

### Low Priority

1. **Style Improvements**: Naming, formatting
2. **Documentation**: Missing comments
3. **Test Coverage**: Gaps in non-critical paths
4. **Minor Optimizations**: Small performance improvements

## Pattern Application Guidelines

### Before Applying a Pattern

1. **Verify Pattern Exists**: Check `DESIGN_PATTERNS.md`
2. **Review Examples**: Study existing implementations
3. **Understand Trade-offs**: Consider complexity vs. benefit
4. **Plan Migration**: If refactoring existing code, plan gradual migration

### Pattern Selection Criteria

- **Problem Match**: Pattern must solve the specific problem
- **Complexity**: Pattern should reduce, not increase complexity
- **Maintainability**: Pattern should improve maintainability
- **Consistency**: Pattern should be consistent with codebase style
- **Testability**: Pattern should improve testability

### Anti-Patterns to Avoid

1. **Over-Engineering**: Don't apply patterns for simple problems
2. **Pattern Mismatch**: Don't force patterns that don't fit
3. **Premature Optimization**: Don't optimize before measuring
4. **Inconsistent Application**: Don't apply patterns inconsistently

## Integration with Development Workflow

### Pre-Implementation

- Review code evaluation checklist
- Identify potential pattern applications
- Plan refactoring if needed

### During Implementation

- Apply patterns as code is written
- Refactor existing code when touching it
- Update tests and documentation

### Post-Implementation

- Evaluate code quality improvements
- Document pattern applications
- Share learnings with team

## Metrics and Reporting

### Code Quality Dashboard

Track:
- Code duplication percentage
- Average cyclomatic complexity
- Test coverage percentage
- Pattern application rate
- Refactoring velocity

### Regular Reviews

- **Weekly**: Review new code for pattern opportunities
- **Monthly**: Comprehensive code quality audit
- **Quarterly**: Architecture pattern review

## Related Documentation

- [Design Patterns Library](./DESIGN_PATTERNS.md)
- [Architecture Patterns Library](./ARCHITECTURE_PATTERNS.md)
- [Architecture Review Process](./ARCHITECTURE_REVIEW_PROCESS.md)
- [Lint Error Prevention](./LINT_ERROR_PREVENTION.md)

---

*This policy ensures code quality improvements are systematic, consistent, and aligned with established design patterns.*

