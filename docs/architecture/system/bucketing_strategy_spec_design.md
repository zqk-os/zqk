# Bucketing Strategy Spec Design

**Last Verified:** 2026-08-31

## Architecture Decision

**Decision**: Use spec-based approach following the existing `SpecLoader` pattern instead of hardcoding strategies in Go.

## Benefits

1. **Declarative Configuration**: Strategies defined in YAML, not code
2. **Runtime Configuration**: Change strategies without recompiling
3. **Validation**: Strategy specs can be validated like object specs
4. **Inheritance**: Strategies can extend base strategies
5. **Composition**: Multiple strategies can be combined
6. **Consistency**: Follows existing patterns in the codebase

## Spec Structure

```yaml
ontology: bucketing_strategy
kind: bucketing_strategy
schema_version: "2.0.0"
strategy_type: chronological | state | size | composite
strategy_name: monthly | status_based | size_ranges | hybrid
field: created_at  # Field to extract bucket key from
format: "2006-01"  # For chronological strategies
enabled: true
applies_to:
  - audit_event
  - change_journal_entry
description: |
  Human-readable description of the strategy
```

## Loader Implementation

```go
type BucketStrategyLoader struct {
    strategiesDir string
    cache         map[string]*BucketStrategy
    mu            sync.RWMutex
}

type BucketStrategy struct {
    Ontology      string
    StrategyType  string
    StrategyName  string
    Field         string
    Format        string
    Enabled       bool
    AppliesTo     []string
    Description   string
}
```

## Integration Points

1. **Spec Loader**: Load strategy specs from `docs/process/_internal/bucketing_strategies/`
2. **Registry**: Build registry from loaded specs
3. **Hash Registry Pool**: Use strategy-aware keys from loaded strategies
4. **Validation**: Validate strategy configurations

## Migration Path

1. Create spec files for existing strategies
2. Implement `BucketStrategyLoader`
3. Update `directoryRegistryPool` to use loaded strategies
4. Remove hardcoded strategy definitions

