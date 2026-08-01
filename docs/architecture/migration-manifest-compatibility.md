# Migration Binary Manifest Compatibility Constraints

**Version:** 1.0.0  
**Created:** 2025-12-24  
**Status:** Implemented  
**Related:** Migration Binary Detection Strategy, CLI Version Handling

## Overview

The migration binary manifest can specify compatibility constraints to ensure that the migration binary is only used with compatible versions of the CLI and dependencies. This prevents situations where older migration logic may not align with newer modules or vice versa.

## Manifest Structure

### Compatibility Constraints

The manifest supports compatibility constraints at two levels:

1. **Manifest-level**: Applies to all entries
2. **Entry-level**: Overrides manifest-level for specific platform entries

### Example Manifest

```yaml
version: "1.0.0"
algorithm: "sha256"

# Manifest-level compatibility (applies to all entries unless overridden)
compatibility:
  # CLI version requirement
  cli_version: ">=1.0.0"  # Requires CLI version 1.0.0 or higher
  
  # Module version requirements
  modules:
    "github.com/lanceman/zqk/pkg/graph": ">=1.0.0"
    "github.com/lanceman/zqk/pkg/migration": "^1.0.0"  # Compatible with 1.x.x
  
  # Backend version requirements
  backends:
    "memgraph": ">=2.0.0"
    "neo4j": ">=5.0.0"

entries:
  - binary: "zqk-migrate"
    version: "1.0.0"
    platform: "darwin/arm64"
    hash: "abc123def456..."
    source: "github.com/lanceman/zqk/cmd/zqk-migrate"
    verified_at: "2025-12-24T10:00:00Z"
    
    # Entry-specific compatibility (overrides manifest-level)
    compatibility:
      cli_version: ">=1.0.0 <2.0.0"  # This platform requires CLI 1.x.x
    
  - binary: "zqk-migrate"
    version: "1.5.0"
    platform: "linux/amd64"
    hash: "def456ghi789..."
    source: "github.com/lanceman/zqk/cmd/zqk-migrate"
    verified_at: "2025-12-24T10:00:00Z"
    # Uses manifest-level compatibility constraints
```

## Constraint Syntax

Compatibility constraints use the same syntax as version constraints:

- **Exact**: `1.0.0` - Must match exactly
- **Minimum**: `>=1.0.0` - At least version 1.0.0
- **Range**: `>=1.0.0 <2.0.0` - Between 1.0.0 (inclusive) and 2.0.0 (exclusive)
- **Compatible**: `^1.0.0` - Compatible with 1.0.0 (>=1.0.0 <2.0.0)
- **Maximum**: `<=2.0.0` - At most version 2.0.0

## Compatibility Checking

### Automatic Verification

When the migration binary is detected, the system automatically:

1. **Loads the manifest** (if available)
2. **Checks CLI version** against `cli_version` constraint
3. **Checks module versions** against `modules` constraints
4. **Checks backend versions** against `backends` constraints
5. **Fails if any constraint is not met**

### Error Messages

When compatibility check fails:

```
Error: compatibility check failed: CLI version requirement not met: 
CLI version 0.9.0 does not meet requirement >=1.0.0

The migration binary requires CLI version >=1.0.0, but you are running 0.9.0.

Please upgrade the CLI:
  go install github.com/lanceman/zqk/cmd/zqk@latest
```

## Use Cases

### Use Case 1: Prevent Older Migration Logic

**Scenario**: Migration binary v1.0.0 was built against CLI v1.0.0. CLI is now v2.0.0 with breaking changes.

**Solution**: Manifest specifies `cli_version: ">=1.0.0 <2.0.0"` to prevent using old binary with new CLI.

```yaml
compatibility:
  cli_version: ">=1.0.0 <2.0.0"  # Only works with CLI 1.x.x
```

### Use Case 2: Require Specific Module Versions

**Scenario**: Migration binary uses new graph API features that require `pkg/graph` v1.5.0+.

**Solution**: Manifest specifies module version requirement.

```yaml
compatibility:
  modules:
    "github.com/lanceman/zqk/pkg/graph": ">=1.5.0"
```

### Use Case 3: Backend Version Requirements

**Scenario**: Migration binary uses MemGraph features that require MemGraph v2.0.0+.

**Solution**: Manifest specifies backend version requirement.

```yaml
compatibility:
  backends:
    "memgraph": ">=2.0.0"
```

### Use Case 4: Platform-Specific Requirements

**Scenario**: Linux binary requires newer CLI version than macOS binary.

**Solution**: Entry-level compatibility overrides manifest-level.

```yaml
compatibility:
  cli_version: ">=1.0.0"  # Default for all platforms

entries:
  - platform: "linux/amd64"
    compatibility:
      cli_version: ">=1.5.0"  # Linux requires newer CLI
  - platform: "darwin/arm64"
    # Uses manifest-level >=1.0.0
```

## Implementation

### CLI Version Detection

The CLI version is set at build time and exposed via `detector.GetCLIVersion()`:

```go
// In cmd/zqk/root.go
detector.GetCLIVersion = func() string {
    return version  // Set via ldflags at build time
}
```

### Verification Flow

```go
// 1. Load manifest
manifest, err := LoadBinaryManifest(manifestPath)

// 2. Get entry for platform
entry := manifest.GetEntryForPlatform(runtime.GOOS, runtime.GOARCH)

// 3. Get compatibility constraints (entry-level or manifest-level)
compat := entry.Compatibility
if compat == nil {
    compat = manifest.Compatibility
}

// 4. Verify compatibility
if err := VerifyCompatibility(compat); err != nil {
    return fmt.Errorf("compatibility check failed: %w", err)
}
```

## Best Practices

1. **Always specify CLI version**: Prevents using incompatible CLI versions
2. **Specify module versions**: When migration depends on specific module features
3. **Specify backend versions**: When migration uses backend-specific features
4. **Use compatible ranges**: Use `^1.0.0` for forward compatibility within major version
5. **Platform-specific overrides**: Use entry-level constraints when needed
6. **Clear error messages**: Help users understand what needs to be upgraded

## Example: Complete Manifest

```yaml
version: "1.0.0"
algorithm: "sha256"

compatibility:
  cli_version: ">=1.0.0"
  modules:
    "github.com/lanceman/zqk/pkg/graph": "^1.0.0"
    "github.com/lanceman/zqk/pkg/migration": ">=1.0.0"
  backends:
    "memgraph": ">=2.0.0"

entries:
  - binary: "zqk-migrate"
    version: "1.0.0"
    platform: "darwin/arm64"
    hash: "abc123..."
    source: "github.com/lanceman/zqk/cmd/zqk-migrate"
    verified_at: "2025-12-24T10:00:00Z"
    
  - binary: "zqk-migrate"
    version: "1.0.0"
    platform: "linux/amd64"
    hash: "def456..."
    source: "github.com/lanceman/zqk/cmd/zqk-migrate"
    verified_at: "2025-12-24T10:00:00Z"
    compatibility:
      cli_version: ">=1.5.0"  # Linux version requires newer CLI
```

## Testing

```bash
# Test with compatible CLI version
zqk migrate --source docs (PRUNED)/process
# Should work if CLI version meets requirements

# Test with incompatible CLI version
# (Would need to downgrade CLI to test)
# Error: compatibility check failed: CLI version requirement not met
```

## Future Enhancements

1. **Module version detection**: Implement `VerifyModuleVersion()` to check actual module versions
2. **Backend version detection**: Implement `VerifyBackendVersion()` to query backend versions
3. **Automatic manifest generation**: Generate manifest with compatibility constraints during build
4. **Compatibility matrix**: Document compatibility between CLI, binary, and module versions

