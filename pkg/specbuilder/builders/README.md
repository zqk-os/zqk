# Versioned Spec Builders

This package provides versioned builders for generating object specs. Each builder is versioned by its filename, allowing immutable version history and easy spec regeneration.

## Pattern

### File Naming Convention

Builders are named: `{spec_name}_v{major}_{minor}_{patch}_builder.go`

Examples:
- `auditable_v1_0_0_builder.go` - auditable spec, version 1.0.0
- `base_object_v1_0_0_builder.go` - base_object spec, version 1.0.0
- `base_object_v1_1_0_builder.go` - base_object spec, version 1.1.0 (minor update)
- `base_object_v2_0_0_builder.go` - base_object spec, version 2.0.0 (major update)

### Versioning Strategy

1. **Initial Version**: Create `{spec_name}_v1_0_0_builder.go` (semantic versioning: major.minor.patch)
2. **Update Spec**: 
   - Minor change: Create `{spec_name}_v1_1_0_builder.go`
   - Major change: Create `{spec_name}_v2_0_0_builder.go`
   - Patch change: Create `{spec_name}_v1_0_1_builder.go`
3. **Version is Immutable**: Once created, never modify a versioned builder file
4. **Version in Filename**: Version is encoded in the filename (semantic version format), not in the code

## Usage

### Creating a Versioned Builder

```go
// File: backlog_item_v1_0_0_builder.go
package builders

type BacklogItemV1Builder struct {
	*BaseSpecBuilder
}

func NewBacklogItemV1Builder() *BacklogItemV1Builder {
	builder := &BacklogItemV1Builder{
		BaseSpecBuilder: NewBaseSpecBuilder("backlog_item", "v1_0_0"),
	}
	
	builder.
		SetExtends("base_object").
		SetDescription("...")
	
	// Add fields
	builder.AddField("title", fieldDef)
	
	return builder
}
```

### Registering Builders

Builders should register themselves in an `init()` function:

```go
func init() {
	// Register with global registry if available
	builder := NewBacklogItemV1Builder()
	// registry.Register(builder)
}
```

### Using Builders

```go
// Get builder for specific version
builder := NewBacklogItemV1Builder()
spec := builder.Build()

// Or use registry
registry := NewVersionedBuilderRegistry()
registry.Register(NewBacklogItemV1Builder())

builder, err := registry.GetBuilder("backlog_item", "v1_0_0")
if err != nil {
	log.Fatal(err)
}
spec := builder.Build()
```

## Benefits

1. **Immutable Versions**: Version encoded in filename, cannot be changed
2. **Version History**: Easy to see all versions (just list files)
3. **Reproducible**: Can regenerate any version of any spec
4. **Clear Migration**: New file = new version, obvious what changed
5. **Testable**: Each builder can be unit tested independently

## Example: Updating a Spec

### Before (v1.0.0)
```
backlog_item_v1_0_0_builder.go  // Original version
```

### After (v1.1.0 - minor update)
```
backlog_item_v1_0_0_builder.go  // Original version (unchanged)
backlog_item_v1_1_0_builder.go  // Updated version (new file)
```

When you need to update `backlog_item`:
1. Copy `backlog_item_v1_0_0_builder.go` to `backlog_item_v1_1_0_builder.go`
2. Update the version in the constructor: `NewBaseSpecBuilder("backlog_item", "v1_1_0")`
3. Update the type name: `BacklogItemV1_1Builder` (Go naming: underscores in version become underscores in type)
4. Make your changes to fields, description, etc.
5. Register the new builder in `registry.go`

### Semantic Versioning Guidelines
- **Major version (v2.0.0)**: Breaking changes, incompatible updates
- **Minor version (v1.1.0)**: New fields, backwards-compatible additions
- **Patch version (v1.0.1)**: Bug fixes, small corrections

## Integration with Bootstrap System

Versioned builders work with the bootstrap system:

1. **Capture Current State**: Bootstrap system captures current YAML files
2. **Generate from Builders**: Builders can regenerate specs programmatically
3. **Version Control**: Each version is immutable and reproducible
4. **Migration**: Can regenerate specs at any version

## Generator

The `SpecGenerator` uses versioned builders to generate spec files:

```go
generator := NewSpecGenerator("output_dir")

// Generate latest version of a spec
err := generator.GenerateLatestSpec("backlog_item")

// Generate all specs (latest versions)
err := generator.GenerateAllSpecs()

// Generate specific version
err := generator.GenerateSpec("backlog_item", "v1_0_0")
```

## Structure

```
pkg/specbuilder/builders/
├── interfaces.go                  # SpecBuilder interface, VersionedBuilderRegistry
├── base_builder.go                # BaseSpecBuilder helper
├── generator.go                   # SpecGenerator for generating spec files
├── registry.go                    # Global registry and auto-registration
├── auditable_v1_0_0_builder.go   # auditable spec v1.0.0
├── base_object_v1_0_0_builder.go # base_object spec v1.0.0
├── generator_test.go              # Tests
└── README.md                      # This file
```

## Complete Workflow

1. **Create Builder**: Create `{spec}_v1_0_0_builder.go` with builder implementation
2. **Register Builder**: Builder auto-registers via `registry.go`
3. **Generate Spec**: Use `SpecGenerator` to generate spec files
4. **Update Spec**: Copy builder file to `{spec}_v1_1_0_builder.go` (or v2_0_0 for major), modify, register

## Future Enhancements

- Auto-registration system (scan for builders, register automatically)
- Builder generator (generate builder from YAML spec)
- Version comparison tools
- Migration helpers
- Integration with bootstrap system for complete regeneration
