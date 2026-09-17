# Configuration System Edge Cases

This document identifies edge cases and potential issues in the configuration system that should be addressed.

## 1. Regex Pattern Compilation Failures

**Location**: `pkg/objects/kind_mappings_config.go`, `pkg/validation/namespaces_config.go`

**Issue**: When regex patterns fail to compile, they are silently skipped with `continue`. This makes debugging difficult.

**Current Behavior**:
```go
re, err := regexp.Compile(rule.Pattern)
if err != nil {
    continue  // Silently skip invalid patterns
}
```

**Recommendation**: Log warnings when patterns fail to compile so users can identify configuration errors.

## 2. Priority Sorting Not Implemented

**Location**: `pkg/objects/kind_mappings_config.go:313-340`

**Issue**: Inference rules have a `Priority` field, but rules are not sorted by priority before being applied. The comment says "Sort by priority if available" but no sorting is actually done.

**Current Behavior**:
```go
// Sort patterns by priority (lower number = higher priority)
patterns := c.InferenceRules.DirectoryPatterns
// Sort by priority if available  <-- Comment but no actual sorting

for _, rule := range patterns {
    // Rules processed in order they appear in config, not by priority
}
```

**Recommendation**: Implement priority sorting before processing rules.

## 3. Empty Default Namespace

**Location**: `pkg/validation/namespaces_config.go:123-158`

**Issue**: If `default_namespace` is empty or missing in the config, `GetNamespaceForKind()` will return an empty string for unknown kinds.

**Current Behavior**:
```go
// Default fallback
return c.DefaultNamespace  // Could be empty string
```

**Recommendation**: Validate that `default_namespace` is set, or ensure `getDefaultNamespacesConfig()` always sets it.

## 4. Empty Config Sections

**Location**: Multiple config loaders

**Issue**: If config files have empty maps or slices (e.g., `namespaces: {}` or `kinds: []`), the code may not handle them gracefully.

**Current Behavior**: Empty maps/slices are valid YAML and will be loaded as empty, potentially causing issues if code expects non-empty values.

**Recommendation**: Add validation or ensure all code paths handle empty collections.

## 5. Config File Not Found vs. Parse Error

**Location**: `pkg/validation/id_prefixes_config.go:44-65`

**Issue**: `LoadIDPrefixesConfig()` returns an error for both "file not found" and "parse error", but `GetGlobalIDPrefixesConfig()` only creates an empty config for errors. This is inconsistent with other configs that return defaults.

**Current Behavior**:
- `paths_config.go`: Returns default config on any error
- `namespaces_config.go`: Returns default config on any error  
- `id_prefixes_config.go`: Returns error on file not found, but `GetGlobalIDPrefixesConfig()` handles it

**Recommendation**: Make behavior consistent - either all return defaults, or all return errors and let callers handle.

## 6. No Config Validation

**Location**: All config loaders

**Issue**: Configs are loaded without validation that required fields are present or have valid values.

**Examples**:
- `default_namespace` could be empty
- `Priority` values could be negative
- Regex patterns could be invalid (handled by compilation, but not validated upfront)
- Backend types could be invalid strings

**Recommendation**: Add validation functions that check required fields and value ranges.

## 7. Config Reloading Not Supported

**Location**: All config loaders use `sync.Once`

**Issue**: Configs are loaded once and cached. If a config file is updated at runtime, the changes won't be picked up.

**Current Behavior**:
```go
globalConfigOnce.Do(func() {
    // Load config once, never reload
})
```

**Recommendation**: Consider adding a reload mechanism for development/testing, or document that configs are loaded once at startup.

## 8. Circular Dependency Workaround

**Location**: `pkg/validation/namespace_registry.go:98-106`

**Issue**: To avoid circular dependency with `objects` package, the skip list is hardcoded instead of using `kind_mappings_config.ShouldSkipSpec()`.

**Current Behavior**:
```go
// Use hardcoded skip list for now (matches kind_mappings_config defaults)
// TODO: Could load from kind_mappings_config if we want to avoid circular dependency
skipSpecs := []string{"base_object", "auditable", "extensible_object"}
```

**Recommendation**: Consider moving shared config logic to a common package, or accept the duplication.

## 9. Backend Type Handling Inconsistency

**Location**: `pkg/objects/kind_mappings_config.go`

**Issue**: Only `kind_mappings_config` supports backend-specific configurations. Other configs don't have this capability.

**Current Behavior**: `kind_mappings_config` has `Backends` map and `SetBackendType()`, but other configs are backend-agnostic.

**Recommendation**: Document this as intentional, or consider if other configs need backend-specific overrides.

## 10. Empty Config Files

**Location**: All config loaders

**Issue**: If a config file exists but is empty or contains only comments, YAML unmarshaling may succeed but produce empty structs.

**Current Behavior**: Empty files may unmarshal to zero values, which may or may not be valid depending on the config.

**Recommendation**: Validate that configs have at least some content, or ensure defaults are always applied.

## 11. Missing Error Context

**Location**: `pkg/objects/kind_mappings_config.go:79-82`

**Issue**: When config file is not found, an error is returned but the path that was searched is not included in the error message.

**Current Behavior**:
```go
if configPath == "" {
    err := fmt.Errorf("kind mappings config file not found")
    // No indication of where we looked
}
```

**Recommendation**: Include search paths in error messages for better debugging.

## 12. Concurrent Access to Config Fields

**Location**: `pkg/objects/kind_mappings_config.go:137-145`

**Issue**: `SetBackendType()` modifies `currentBackend` without locking, but it's accessed in `getBackendConfig()` with locking.

**Current Behavior**:
```go
func (c *KindMappingsConfig) SetBackendType(backendType string) {
    c.currentBackend = backendType  // No lock
}

func (c *KindMappingsConfig) getBackendConfig() BackendConfig {
    c.mu.RLock()  // Locked here
    defer c.mu.RUnlock()
    // ...
}
```

**Recommendation**: Add locking to `SetBackendType()` or document that it should only be called during initialization.

## Priority Recommendations

1. **High Priority**: Add regex compilation error logging (#1)
2. **High Priority**: Implement priority sorting (#2)
3. **Medium Priority**: Validate default_namespace (#3)
4. **Medium Priority**: Add config validation (#6)
5. **Low Priority**: Document config reloading limitation (#7)
6. **Low Priority**: Add error context (#11)

