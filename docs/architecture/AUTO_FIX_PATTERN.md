# Auto-Fix Pattern Recommendation

## Current State

1. **Fix Command Generation** ✅ - Working
   - Generated at validation error time with contextual query hints
   - Format: `zqk object update OBJ-ID --field field_name+=<PLACEHOLDER:query_hint>`

2. **Dependency Hierarchy Ordering** ✅ - Working
   - Issues sorted by dependency priority (goal > priority_plan > milestone > backlog_item)
   - Ensures fixes are applied in correct order

3. **Placeholder Resolution Infrastructure** ✅ - Exists but not integrated
   - `FieldResolver` system routes resolution by semantic type
   - `parseQueryHintToMap` / `buildQueryHintFromMap` helpers exist
   - `ResolveFixCommandPlaceholder` exists in example code

4. **Fix Command Execution Infrastructure** ✅ - Exists but not integrated
   - `FixCommandExecutor` with async worker pool pattern
   - Designed for approval gates and callbacks

5. **Spec-Based Auto-Fixer** ⚠️ - Partial
   - Handles missing required fields with default values
   - Does NOT handle lifecycle violations requiring reference linking

## Recommended Pattern: Layered Auto-Fix Strategy

### Pattern Overview

Use a **layered strategy** that routes fixes based on complexity:

1. **Layer 1: Simple Spec-Based Fixes** (current)
   - Missing required fields → add default values from spec
   - Type mismatches → coerce types
   - Enum validation → use first valid value

2. **Layer 2: Fix Command Execution** (recommended addition)
   - If `Issue.FixCommand` exists → parse and execute
   - Resolve placeholders using `FieldResolver`
   - Update object via storage provider
   - Works for lifecycle violations, reference linking, etc.

3. **Layer 3: Async Fix Command Queue** (future)
   - For complex multi-object fixes
   - Requires approval gates
   - Uses `FixCommandExecutor` infrastructure

### Implementation Pattern

```go
// In autoFixIssues() or SpecBasedAutoFixer

if issue.FixCommand != "" {
    // Layer 2: Fix command execution
    success := executeFixCommand(ctx, fixCtx, issue, storageProvider)
    if success {
        return true, "Fixed via fix command", nil
    }
} else {
    // Layer 1: Simple spec-based fixes (current logic)
    // ... existing SpecBasedAutoFixer logic ...
}
```

### Key Functions Needed

1. **`executeFixCommand()`** - Main entry point
   ```go
   func executeFixCommand(
       ctx *clicontext.Context,
       fixCtx *AutoFixContext,
       issue Issue,
       storageProvider storage.ObjectStorageProvider,
   ) bool
   ```

2. **`parseFixCommand()`** - Parse command string
   ```go
   func parseFixCommand(cmdStr string) (*FixCommand, error)
   type FixCommand struct {
       ObjectID string
       Operation string // "update"
       Field string
       Operator string // "+=", "="
       Placeholder string // "<MILESTONE_ID:query_hint>"
   }
   ```

3. **`resolvePlaceholder()`** - Resolve placeholder using FieldResolver
   ```go
   func resolvePlaceholder(
       ctx context.Context,
       resolver *FieldResolver,
       placeholder, objKind string,
   ) ([]string, error)
   ```

4. **`applyFixToObject()`** - Update object via storage provider
   ```go
   func applyFixToObject(
       ctx context.Context,
       storageProvider storage.ObjectStorageProvider,
       fixCmd *FixCommand,
       resolvedValue interface{},
   ) error
   ```

### Integration Point

**Option A: Extend SpecBasedAutoFixer** (Recommended)
- Add fix command handling to `FixInstanceValidationIssue()`
- Keeps all auto-fix logic in one place
- Minimal changes to existing integration

**Option B: Add separate handler in autoFixIssues**
- Check for `FixCommand` before calling `SpecBasedAutoFixer`
- Routes simple vs complex fixes explicitly
- More explicit separation of concerns

**Recommendation: Option A**
- Simpler integration
- Less code duplication
- Single point of extension

### Execution Flow

```
autoFixIssues()
  → sortIssuesByDependency()
  → for each issue:
      → if FixCommand exists:
          → executeFixCommand()
            → parseFixCommand()
            → resolvePlaceholder() [FieldResolver]
            → applyFixToObject() [Storage Provider]
      → else:
          → SpecBasedAutoFixer.FixInstanceValidationIssue()
            → [existing logic for default values]
```

### Benefits

1. **Incremental**: Builds on existing infrastructure
2. **Testable**: Each layer can be tested independently
3. **Extensible**: Easy to add new fix strategies
4. **Non-breaking**: Doesn't change existing behavior
5. **Reuses code**: Leverages FieldResolver, storage provider

### Migration Path

1. **Phase 1**: Implement `executeFixCommand()` for synchronous execution
2. **Phase 2**: Integrate into `SpecBasedAutoFixer` or `autoFixIssues`
3. **Phase 3**: Add tests with test-scenario data
4. **Phase 4**: (Future) Add async execution for complex multi-object fixes
