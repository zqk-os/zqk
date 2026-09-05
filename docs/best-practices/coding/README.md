# Coding Best Practices

**Status**: Active  
**Last Updated**: 2026-01-27

This directory contains coding standards, best practices, and guidelines for the codebase.

## Documentation Index

### Error Handling
- Error wrapping patterns (use `%w` verb)
- Error handling consistency guidelines
- See: [ERROR_WRAPPING_AUDIT.md](../../process/refactoring/ERROR_WRAPPING_AUDIT.md)

### Resource Management
- Goroutine leak detection patterns
- Channel leak detection patterns
- File handle management
- See: `pkg/runtime/leak_detection_test.go` and `pkg/runtime/channel_leak_detection_test.go`

### Concurrency
- Goroutine management patterns
- Channel usage patterns
- Mutex and lock patterns
- See: `docs/process/architecture/concurrency/` and `docs/process/architecture/GOROUTINE_ARCHITECTURE_POLICY.md`

### Code Organization
- File splitting guidelines
- Package organization
- Test organization
- See: `docs/process/refactoring/` for refactoring patterns

### Semantic density, DRY, and post-verify passes
- **Primary goal:** semantic density (domain logic stands out; scaffolding is shared). **Rule of two** (extract on second copy when obvious); **by the third**, extract or table-drive. See **[DRY_PATTERN_EXTRACTION.md](./DRY_PATTERN_EXTRACTION.md)**.
- Checklist: top-of-file callout + **`docs/architecture/PRE_CHANGE_CHECKLIST.md`** §13.

### Logging (POL-CODE-007)
- Pooled fluent builders for multi-field structured logs: **`[FLUENT_LOGGING.md](./FLUENT_LOGGING.md)`**.

### Environment Variables
- Core system configurations and brand-prefixed toggles: **`[ENVIRONMENT_VARIABLES.md](./ENVIRONMENT_VARIABLES.md)`**.

## Related Documentation

- [Architecture Documentation](../process/architecture/README.md) - Architecture patterns
- [Testing Documentation](../process/testing/README.md) - Testing best practices
- [Enforcement Documentation](../process/enforcement/README.md) - Code quality enforcement
- [Agent Persona Directives](./AGENT_PERSONA_DIRECTIVES.md) - Strict standards for sub-agent persona definitions

## Code Inspector Requirements

All pull requests and direct commits must be reviewed by the **Pedantic Code Inspector**. 
1. The inspector verifies that all pre-commit hooks and `make verify` checks pass.
2. The inspector performs a lint pass (e.g. `golangci-lint`) to ensure the codebase is impeccably clean.
3. Code must adhere strictly to the project's RBAC gates and token configurations.

This mechanism ensures architectural traceablity and consistent baseline quality across AI and human operations.
