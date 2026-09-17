# Build System Documentation

**Last Verified:** 2026-08-31


## Overview

The zqk build system provides a centralized configuration for building multiple binary targets with consistent settings and verification.

## Configuration

Build targets are defined in `build-config.yaml` at the project root. The configuration supports:

- **Main binaries**: Production binaries like `zqk` and `zqk-mcp`
- **Test scenario binaries**: Test-specific binaries for each test scenario
- **Build settings**: Default flags, tags, and verification options

## Usage

### Using the Build Tool

```bash
# Build all enabled targets
go run ./cmd/build build

# Build specific target
go run ./cmd/build build zqk
go run ./cmd/build build zqk-mcp
go run ./cmd/build build content-addressable-storage

# List all targets
go run ./cmd/build list

# Verify all binaries
go run ./cmd/build verify

# Clean all binaries
go run ./cmd/build clean
```

### Using Make

```bash
# Build all targets
make build-all

# Build specific target
make build TARGET=zqk
make build TARGET=zqk-mcp
make build TARGET=content-addressable-storage

# List targets
make list

# Verify binaries
make verify

# Clean binaries
make clean
```

## Build Targets

### Main Binaries

- **zqk**: Main zqk CLI binary (`bin/zqk`)
- **zqk-mcp**: MCP server binary (`bin/zqk-mcp`)

### Test Scenario Binaries

- **content-addressable-storage**: Test binary for content-addressable storage scenario (`test-scenarios/content-addressable-storage/zqk-test-init`)

## Adding New Targets

To add a new build target, edit `build-config.yaml`:

```yaml
build_targets:
  main:
    - name: new-binary
      output_path: bin/new-binary
      source_path: ./cmd/zqk
      description: Description of the new binary
      build_flags: []
      tags: []
      enabled: true

  scenarios:
    - name: new-scenario
      output_path: test-scenarios/new-scenario/zqk-test-init
      source_path: ./cmd/zqk
      description: Test binary for new scenario
      build_flags: []
      tags: []
      enabled: true
```

## Build Settings

The build system supports:

- **Default flags**: Applied to all builds (e.g., `-trimpath`)
- **Default tags**: Build tags applied to all builds
- **Create directories**: Automatically create output directories
- **Verify binaries**: Run `--help` test after building
- **Show progress**: Display build progress and status

## Binary Verification

After building, binaries are automatically verified by:

1. Checking the file exists and is executable
2. Running `--help` to ensure the binary works
3. Reporting any failures

## Integration with CI/CD

The build system can be integrated into CI/CD pipelines:

```yaml
# Example GitHub Actions
- name: Build all binaries
  run: go run ./cmd/build build

- name: Verify binaries
  run: go run ./cmd/build verify
```

## Future Enhancements

- Version tagging and management
- Cross-platform builds
- Build caching
- Parallel builds
- Build artifacts management

