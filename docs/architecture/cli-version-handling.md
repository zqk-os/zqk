> [!WARNING]
> **ARCHIVED DOCUMENT**: The primary commands referenced in this architectural document have been pruned from the `zqk` CLI.

# CLI Version Handling Strategy

**Version:** 1.0.0  
**Created:** 2025-12-24  
**Status:** Implemented  
**Related:** Migration Binary Detection Strategy, CLI Ontology v1.0

## Overview

This document describes how the zqk CLI handles different versions of commands and external binaries. The system supports:

1. **Version Constraints**: Flexible version requirements (>=, ^, ranges)
2. **Version Negotiation**: Automatic selection of compatible versions
3. **Multiple Command Versions**: Support for multiple versions of the same command
4. **Deprecation Handling**: Graceful deprecation with migration paths

## Version Constraint System

### Constraint Syntax

The system supports semantic version constraints:

- **Exact**: `1.0.0` - Must match exactly
- **Minimum**: `>=1.0.0` - At least version 1.0.0
- **Range**: `>=1.0.0 <2.0.0` - Between 1.0.0 (inclusive) and 2.0.0 (exclusive)
- **Compatible**: `^1.0.0` - Compatible with 1.0.0 (>=1.0.0 <2.0.0)
- **Maximum**: `<=2.0.0` - At most version 2.0.0

### Usage in Migration Command

```bash
# Require minimum version 1.0.0
zqk migrate --version-constraint (PRUNED) ">=1.0.0" --source docs/process

# Require compatible version (1.x.x, not 2.x.x)
zqk migrate --version-constraint (PRUNED) "^1.0.0" --source docs/process

# Require specific range
zqk migrate --version-constraint (PRUNED) ">=1.0.0 <2.0.0" --source docs/process
```

### Implementation

The detector uses `VersionConstraint` to verify binary compatibility:

```go
d := detector.NewBinaryDetector()
d, err := d.WithVersionConstraint(">=1.0.0")
if err != nil {
    // Handle error
}
```

## Multiple Command Versions

### Versioned Commands

For commands that have multiple versions (e.g., `migrate v1` vs `migrate v2`):

```go
versions := []cli.CommandVersion{
    {
        Version:     "1.0.0",
        Description: "Initial migration implementation",
        Deprecated:  false,
    },
    {
        Version:     "2.0.0",
        Description: "Enhanced migration with parallel processing",
        Deprecated:  false,
    },
    {
        Version:     "1.5.0",
        Description: "Legacy version",
        Deprecated:  true,
        ReplacedBy:  "2.0.0",
    },
}

cmd := cli.CreateVersionedCommand(
    "migrate",
    "Migrate data between backends",
    "Long description...",
    versions,
    "2.0.0", // default version
    func(version string) func(*cobra.Command, []string) error {
        return func(cmd *cobra.Command, args []string) error {
            // Version-specific handler
            switch version {
            case "1.0.0":
                return runMigrateV1(cmd, args)
            case "2.0.0":
                return runMigrateV2(cmd, args)
            default:
                return fmt.Errorf("unsupported version: %s", version)
            }
        }
    },
)
```

### Usage

```bash
# Use default version (2.0.0)
zqk migrate --source docs (PRUNED)/process

# Use specific version
zqk migrate --version 1 (PRUNED).0.0 --source docs/process

# Deprecated version shows warning
zqk migrate --version 1 (PRUNED).5.0 --source docs/process
# Warning: version 1.5.0 is deprecated (use 2.0.0 instead)
```

## Version Negotiation

### Automatic Version Selection

When multiple compatible versions are available, the system can negotiate:

```go
negotiation := cli.VersionNegotiation{
    CLIVersion:        "1.0.0",
    MinBinaryVersion:  "1.0.0",
    MaxBinaryVersion:  "2.0.0",
    PreferredVersion:  "1.5.0",
}

availableVersions := []string{"1.0.0", "1.5.0", "2.0.0"}
selectedVersion, err := cli.NegotiateVersion(negotiation, availableVersions)
// Returns "1.5.0" (preferred) or first compatible version
```

### Binary Version Detection

The detector automatically detects binary version:

```go
caps, err := d.GetCapabilities()
// caps.Version contains the binary version
```

## Version Compatibility Matrix

### Migration Binary Compatibility

| CLI Version | Min Binary | Max Binary | Notes |
|------------|------------|------------|-------|
| 1.0.0      | 1.0.0      | 1.x.x      | Initial support |
| 1.1.0      | 1.0.0      | 2.0.0      | Extended compatibility |
| 2.0.0      | 1.5.0      | 2.x.x      | Requires newer binary |

### Handling Incompatible Versions

When version mismatch is detected:

1. **Strict Mode** (default): Error with clear message
   ```
   Error: version compatibility check failed: version 0.9.0 does not match constraint >=1.0.0
   ```

2. **Negotiation Mode**: Attempt to find compatible version
   ```go
   // Try to find compatible version
   compatibleVersion, err := negotiateCompatibleVersion(availableVersions, constraint)
   ```

3. **Fallback Mode**: Use latest available version (with warning)
   ```
   Warning: Binary version 1.5.0 doesn't match constraint >=2.0.0, using available version
   ```

## Deprecation Strategy

### Deprecating Command Versions

1. Mark version as deprecated in registry:
   ```go
   {
       Version:     "1.0.0",
       Deprecated:  true,
       ReplacedBy:  "2.0.0",
   }
   ```

2. Show warning when deprecated version is used
3. Remove deprecated version in next major release

### Migration Path

```bash
# User uses deprecated version
$ zqk migrate --version 1 (PRUNED).0.0
Warning: version 1.0.0 is deprecated (use 2.0.0 instead)
# Command still works, but warns user
```

## Best Practices

1. **Use Version Constraints**: Always specify minimum version requirements
2. **Semantic Versioning**: Follow SemVer for predictable compatibility
3. **Clear Error Messages**: Explain version requirements in error messages
4. **Deprecation Warnings**: Warn users about deprecated versions
5. **Migration Guides**: Provide clear upgrade paths

## Examples

### Example 1: Version Constraint

```bash
# Require migration binary >= 1.0.0
zqk migrate --version-constraint (PRUNED) ">=1.0.0" --source docs/process
```

### Example 2: Multiple Command Versions

```bash
# Use default (latest) version
zqk migrate --source docs (PRUNED)/process

# Use specific version
zqk migrate --version 2 (PRUNED).0.0 --source docs/process

# List available versions
zqk migrate --help (PRUNED)
# Shows: Available versions: 1.0.0, 2.0.0
```

### Example 3: Version Negotiation

```go
// CLI automatically negotiates best version
d := detector.NewBinaryDetector()
d, _ = d.WithVersionConstraint("^1.0.0") // Accepts 1.x.x

// If binary is 1.5.0, it's accepted
// If binary is 2.0.0, it's rejected
// If binary is 0.9.0, it's rejected
```

## Implementation Files

- `pkg/migration/detector/version.go` - Version parsing and constraints
- `pkg/migration/detector/detector.go` - Version verification in detector
- `internal/cli/versioning.go` - Versioned command registry
- `cmd/zqk/migrate.go` - Migration command with version support

## Testing

```bash
# Test version parsing
go test ./pkg/migration/detector -run TestVersion

# Test version constraints
go test ./pkg/migration/detector -run TestVersionConstraint
```

