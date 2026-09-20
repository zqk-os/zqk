# Bootstrap System

The bootstrap system provides a way to capture the current state of all system-wide specs (object specs, lifecycles, profiles, configs, traits) and recreate them flawlessly.

## Overview

The bootstrap system allows you to:
1. **Capture** the current state of all system specs into a single bootstrap file
2. **Delete** all original spec files (if desired)
3. **Recreate** all specs from the bootstrap file

This is useful for:
- Creating clean, reproducible system states
- Archiving system configuration
- Migration and backup scenarios
- Testing and validation

## Usage

### Capturing Current State

```go
import "github.com/zqk-os/zqk/pkg/specbuilder/bootstrap"

// Capture current state from .zqk/specs
capture, err := bootstrap.CaptureCurrentState(".zqk/specs")
if err != nil {
    log.Fatal(err)
}

// Save to bootstrap file
if err := capture.SaveToFile("bootstrap.yaml"); err != nil {
    log.Fatal(err)
}
```

### Restoring State

```go
// Load bootstrap file
capture, err := bootstrap.LoadFromFile("bootstrap.yaml")
if err != nil {
    log.Fatal(err)
}

// Restore to target directory
if err := bootstrap.RestoreState(capture, ".zqk/specs", false); err != nil {
    log.Fatal(err)
}
```

### Complete Workflow

```go
// Complete workflow: capture, delete, restore
capture, err := bootstrap.CaptureCurrentState(".zqk/specs")
if err != nil {
    log.Fatal(err)
}

// Save bootstrap
if err := capture.SaveToFile("bootstrap.yaml"); err != nil {
    log.Fatal(err)
}

// Delete original files (optional - be careful!)
// ... delete files ...

// Restore from bootstrap
if err := bootstrap.InitializeFromBootstrap("bootstrap.yaml", ".zqk/specs", true); err != nil {
    log.Fatal(err)
}
```

## What Gets Captured

The bootstrap system captures:

1. **Object Specs** (`object_specs/*.yaml`)
   - All object specification files
   - Preserves directory structure (e.g., `built-in/` subdirectories)

2. **Lifecycles** (`lifecycles/*.yaml`)
   - All lifecycle definition files
   - Preserves directory structure

3. **Profiles** (`profile_specs/*.yaml`)
   - All profile specification files
   - CLI profiles, metrics profiles, etc.

4. **Config Files** (top-level `*.yaml` files)
   - `id_prefixes_config.yaml`
   - `kind_mappings_config.yaml`
   - `namespaces_config.yaml`
   - `paths_config.yaml`
   - `blocking_check_config.yaml`
   - `scanner_config.yaml`
   - Any other top-level config files

5. **Traits** (`traits/*.yaml`)
   - All trait definition files

## Bootstrap File Format

The bootstrap file is a YAML file containing:
- Version information
- Metadata (capture timestamp, file counts, file lists)
- All files encoded as base64 strings

Example structure:
```yaml
version: "1.0.0"
metadata:
  captured_at: "2026-01-07T12:00:00Z"
  source_directory: ".zqk/specs"
  file_counts:
    object_specs: 50
    lifecycles: 40
    profiles: 15
    configs: 6
    traits: 25
files:
  object_specs/base_object.yaml: "b250b2xvZ3k6IGJhc2Vfb2JqZWN0Ci4uLg=="
  object_specs/backlog_item.yaml: "Li4u"
  # ... all other files
```

## Safety Features

- **Force Flag**: The `force` parameter controls whether existing files can be overwritten
- **Metadata Tracking**: Bootstrap files include metadata about what was captured
- **File Verification**: Files are verified during restore to ensure integrity

## Integration with SpecBuilder Pattern

The bootstrap system complements the specbuilder pattern:
- **Specbuilder**: Generates artifacts from specs (forward direction)
- **Bootstrap**: Captures and restores specs (preservation/recreation)

Together, they enable a complete spec lifecycle:
1. Define specs (YAML files)
2. Generate artifacts (specbuilder)
3. Capture state (bootstrap)
4. Restore state (bootstrap)

## Examples

See `capture_test.go` for complete examples of:
- Capturing state
- Saving/loading bootstrap files
- Restoring state
- Complete round-trip testing
