# Instance Builders

Instance builders extend the spec builder pattern to programmatically create and manage object instances. **Unlike spec builders (one per spec), there's ONE instance builder per object type** that handles ALL instances of that type.

## Overview

Instance builders provide:
- **Code-driven instance creation**: Create instances programmatically (fluent API)
- **YAML file support**: Load and write instances from/to YAML files
- **Sequence file support**: Compact storage format (values in order, no field names)
- **Version-aware**: Support different schema versions; use `objects.DefaultSchemaVersion` for the project default
- **Storage efficiency**: Sequence format significantly reduces file size

## Key Distinction

| Type | Count | Builder Pattern |
|------|-------|----------------|
| **Specs** | ONE per object type | One spec builder per spec version |
| **Instances** | MANY per object type | **ONE instance builder per object type** |

Example:
- **Policy spec**: ONE spec → ONE `PolicySpecBuilder`
- **Policy instances**: MANY (POL-CODE-009, POL-CODE-010, etc.) → **ONE** `PolicyInstanceBuilder`

## Architecture

### Core Components

1. **BaseInstanceBuilder**: Base class providing common functionality
2. **InstanceBuilder**: Interface that all instance builders implement
3. **VersionedInstanceBuilderRegistry**: Registry for managing builders by kind and version
4. **InstanceGenerator**: Generates YAML instance files from builders

### Key Differences from Spec Builders

| Aspect | Spec Builders | Instance Builders |
|--------|---------------|-------------------|
| **Output Type** | `*objects.Spec` | `map[string]any` |
| **Fields** | Schema definition | Instance data (values) |
| **Versioning** | Builder version (v1_0_0) | Instance schema_version (1.0.0) |
| **Purpose** | Define object structure | Create object data |

## Usage

### Creating Instances Programmatically

```go
import "github.com/lanceman/zqk/pkg/objects"

// Create a policy instance builder (ONE builder for ALL policy instances)
builder := NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)
instance, err := builder.
    SetID("POL-CODE-009").
    SetTitle("Code Quality Maintenance").
    SetCategory("code_quality").
    SetStatus("active").
    Build()
```

### Loading from YAML

```go
import "github.com/lanceman/zqk/pkg/objects"

builder := NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)
instance, err := builder.LoadFromYAML("docs/architecture/policies/POL-CODE-009.yaml")
```

### Loading from Sequence File (Compact Format)

```go
import "github.com/lanceman/zqk/pkg/objects"

builder := NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)
instance, err := builder.LoadFromSequence("docs/architecture/policies/POL-CODE-009.seq")
```

### Writing to Compact Format

```go
import "github.com/lanceman/zqk/pkg/objects"

builder := NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)
instance := map[string]any{
    "id": "POL-CODE-009",
    "title": "Code Quality Maintenance",
    // ...
}
err := builder.WriteToSequence(instance, "policies/POL-CODE-009.seq")
```

### Version-Aware Creation

```go
import "github.com/lanceman/zqk/pkg/objects"

// Create instance for an older spec schema (example)
builder1 := NewPolicyInstanceBuilder("1.0.0")
instance1, _ := builder1.SetID("POL-001").Build()

// Create instance for the current default schema
builder2 := NewPolicyInstanceBuilder(objects.DefaultSchemaVersion)
instance2, _ := builder2.SetID("POL-002").Build()
```

### Using Registry

```go
import "github.com/lanceman/zqk/pkg/objects"

// Get builder from registry
registry := GetGlobalRegistry()
builder, err := registry.GetBuilder("policy", objects.DefaultSchemaVersion)
instance, err := builder.Build()
```

## Implementation Pattern

### Creating an Instance Builder

**In this repo, instance builders are generated** (see “Code Generation” above). Do not hand-write new builders in `bldr_instance_v1/`; add or change the object spec and run `zqk system generate-instance-builders --overwrite`. The following describes the structure the generator produces:

1. Create a builder struct embedding `BaseInstanceBuilder`
2. Create constructor that initializes base builder
3. Add fluent methods for setting fields (generated from spec)
4. Register builder in `init()` function

### Example Structure

```go
package instance_builders

type PolicyInstanceBuilder struct {
    *BaseInstanceBuilder
}

func NewPolicyInstanceBuilder(schemaVersion string) *PolicyInstanceBuilder {
    // Optionally get spec builder for validation/structure
    var specBuilder SpecBuilderInterface = nil // Can be nil
    
    builder := &PolicyInstanceBuilder{
        BaseInstanceBuilder: NewBaseInstanceBuilder("policy", schemaVersion, specBuilder),
    }
    
    // Set defaults
    builder.SetField("status", "active")
    builder.SetField("origin_system", "zqk")
    
    return builder
}

func (b *PolicyInstanceBuilder) SetTitle(title string) *PolicyInstanceBuilder {
    b.SetField("title", title)
    return b
}

func (b *PolicyInstanceBuilder) SetCategory(category string) *PolicyInstanceBuilder {
    b.SetField("category", category)
    return b
}
```

## Integration with Spec Builders

Instance builders can optionally reference spec builders to:
- Validate field names and types
- Apply default values from spec definitions
- Ensure instances conform to spec structure

The spec builder reference is optional - instance builders can work standalone.

## Code Generation (Generated Files)

**All files in `pkg/specbuilder/bldr_instance_v1/*_instance_builder.go` are generated** from object spec YAML. Do not edit them by hand; changes will be overwritten on the next run.

- **Source**: Object spec YAML files (e.g. `docs/architecture/_internal/object_specs/*.yaml` or bldr_v2-backed specs).
- **Generator**: `pkg/specbuilder/instance_builders/codegen.go` → `GenerateInstanceBuilderFromSpec(specPath, outputDir, schemaVersion)`.
- **Regenerate**: Run `zqk system generate-instance-builders` (optionally `--specs-dir`, `--output-dir`, `--overwrite`).

Generated files include a header: `// Code generated by ... DO NOT EDIT. Regenerate with: zqk system generate-instance-builders --overwrite`.

## Required Pattern: Creating Objects in Code

When creating metric or other spec-backed objects (e.g. in storage, scheduler, or CLI):

1. **Use the instance builder** for that kind (e.g. `bldr_instance_v1.NewBaseMetricInstanceBuilder`, `NewAuditAggregationMetricInstanceBuilder`, `NewSchedulerHealthMetricInstanceBuilder`). Do not build literal `map[string]any` for objects that have a generated builder.
2. **Set lifecycle-valid status**: Use a status value allowed by that kind’s lifecycle (e.g. `audit_aggregation_metric`: `completed`, `archived`, `error`; `scheduler_health_metric`: `active`, `archived`, `error`). Check `pkg/specbuilder/bldr_lifecycle_v1/*_builder.go` or lifecycle YAML.
3. **Use typed setters** where the generated builder exposes them; use `SetField(name, value)` for inherited or extra fields (e.g. base_metric fields on a child metric builder that has no dedicated setter).
4. **Call `Build()`** and pass the result to storage/create; handle build errors before persisting.

Example (metric creation):

```go
builder := bldr_instance_v1.NewAuditAggregationMetricInstanceBuilder(objects.DefaultSchemaVersion)
builder.SetID(metricID)
builder.SetStatus("completed")  // lifecycle-valid for audit_aggregation_metric
builder.SetField("title", title)
// ... other fields ...
obj, err := builder.Build()
if err != nil { return err }
return storage.Create(ctx, secCtx, obj)
```

## Status

**Current Status**: Core infrastructure and codegen in place
- ✅ BaseInstanceBuilder
- ✅ InstanceBuilder interface
- ✅ VersionedInstanceBuilderRegistry
- ✅ InstanceGenerator
- ✅ Code generation from spec YAML (`GenerateInstanceBuilderFromSpec`)
- ✅ CLI: `zqk system generate-instance-builders`
- ✅ Generated builders in `pkg/specbuilder/bldr_instance_v1/`

## Convergence / consistency (ongoing)

Aligned with active convergence work on pipeline and DRY:

- **Field typing and validation**: `BaseInstanceBuilder` centralizes field-type hints (`fieldTypeEnum`, `fieldTypeObject`, …), YAML extension (`.yaml`), and validation keys (`validation`, `required`, `enum`) in `base_builder.go`; prefer extending those constants over new literals in hand-maintained code.
- **Map handling**: `maps.Copy` from the standard library is used when merging or cloning `map[string]any` payloads; avoid ad-hoc `for k, v := range` copy loops when types match.
- **Gaps**: New object kinds should get generated instance builders when possible; one-off `map[string]any` construction for spec-backed kinds is a known gap—call `GenerateInstanceBuilderFromSpec` / `zqk system generate-instance-builders` instead of growing literals.
