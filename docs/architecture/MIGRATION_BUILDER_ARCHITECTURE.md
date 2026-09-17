# Migration Builder Architecture

**Last Verified:** 2026-08-31


**Created:** 2026-01-12  
**Status:** Design  
**Purpose:** Builder pattern for migration specs and instances

## Overview

Migrations should follow the specbuilder pattern:
1. **Migration Spec Builder** → Generates migration spec YAML files
2. **Migration Instance Builder** → Generates migration instance objects from specs
3. **Kind Migration** → Specific migration type for object kind transformations
4. **Custom Migrations** → Extensible system for custom migration types

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Migration Spec Builder (bldr_migration_v1/)                │
│ - MigrationSpecBuilder                                      │
│ - StepBuilder (scan_files, transform, create_objects, etc.) │
│ - Generates migration spec YAML files                       │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ generates
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Migration Spec (YAML)                                       │
│ .zqk/specs/migrations/*.yaml                    │
│ - Describes migration steps                                 │
│ - Prerequisites, validation, rollback                       │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ loads
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Migration Instance Builder (bldr_instance_v1/)             │
│ - MigrationInstanceBuilder                                  │
│ - Creates migration instance objects                        │
│ - Validates migration specs                                 │
└─────────────────────────────────────────────────────────────┘
                            │
                            │ creates
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Migration Types                                             │
│                                                             │
│  ┌──────────────────────────────────────────┐             │
│  │ Kind Migration                           │             │
│  │ - Migrates object kinds                  │             │
│  │ - Examples: lifecycle files → objects    │             │
│  │            audit events → buckets        │             │
│  └──────────────────────────────────────────┘             │
│                                                             │
│  ┌──────────────────────────────────────────┐             │
│  │ Custom Migration                        │             │
│  │ - User-defined migration types          │             │
│  │ - Extensible via migration registry     │             │
│  └──────────────────────────────────────────┘             │
└─────────────────────────────────────────────────────────────┘
```

## Migration Spec Builder

### Structure

```go
package bldr_migration_v1

type MigrationSpecBuilder struct {
    *BaseBuilder
    spec *MigrationSpec
}

type MigrationSpec struct {
    SchemaVersion string
    ID            string
    Name          string
    Description   string
    From          StateDescription
    To            StateDescription
    Prerequisites []Prerequisite
    Steps         []Step
    Rollback      []Step
    Validation    []ValidationRule
    Options       MigrationOptions
}

type Step struct {
    ID          string
    Type        string  // scan_files, transform, create_objects, etc.
    Description string
    DependsOn   []string
    ForEach     string  // Reference to previous step
    Config      map[string]any
}

type Prerequisite struct {
    Type        string  // kind, directory, config
    Kind        string
    Registered  bool
    InKindMapper bool
    InOnDemandKinds bool
    Directory   string
    Exists      bool
    OnDemand    bool
}

type ValidationRule struct {
    Type        string  // object_count, file_count, etc.
    Description string
    Config      map[string]any
}
```

### Builder API

```go
builder := bldr_migration_v1.NewMigrationSpecBuilder("1.0.0")

builder.
    SetID("migration-lifecycle-files-to-objects").
    SetName("Migrate Lifecycle Files to Objects").
    SetDescription("Convert lifecycle YAML files to lifecycle objects").
    SetFromState("lifecycle_files", "Lifecycle definitions stored as YAML files").
    SetToState("lifecycle_objects", "Lifecycle definitions stored as objects").
    AddPrerequisite(bldr_migration_v1.NewPrerequisite().
        SetType("kind").
        SetKind("lifecycle").
        SetRegistered(true).
        SetInKindMapper(true).
        SetInOnDemandKinds(true)).
    AddStep(bldr_migration_v1.NewStep("scan_lifecycle_files").
        SetType("scan_files").
        SetDescription("Scan for lifecycle YAML files").
        SetConfig(map[string]any{
            "directory": ".zqk/specs/lifecycles",
            "pattern": "*_lifecycle.yaml",
            "exclude": []string{"*.bak", "built-in/*"},
        })).
    AddStep(bldr_migration_v1.NewStep("convert_lifecycle_to_object").
        SetType("transform").
        SetDescription("Convert lifecycle file to lifecycle object").
        SetForEach("scan_lifecycle_files").
        SetConfig(map[string]any{
            "source": "file_content",
            "target": "object_map",
            "transform": map[string]any{
                "builder": "lifecycle_instance_builder",
                "version": "v1_0_0",
            },
        })).
    AddStep(bldr_migration_v1.NewStep("create_lifecycle_objects").
        SetType("create_objects").
        SetDescription("Create lifecycle objects in storage").
        SetDependsOn([]string{"convert_lifecycle_to_object"}).
        SetConfig(map[string]any{
            "storage": "file",
            "skip_existing": true,
        }))

spec := builder.Build()
builder.WriteToFile(".zqk/specs/migrations/lifecycle-files-to-objects.yaml")
```

## Migration Instance Builder

### Structure

```go
package bldr_instance_v1

type MigrationInstanceBuilder struct {
    *BaseInstanceBuilder
}

// Creates a migration instance object from a migration spec
func (b *MigrationInstanceBuilder) BuildFromSpec(spec *migration.MigrationSpec) (map[string]any, error) {
    // Validate spec
    if err := b.validateSpec(spec); err != nil {
        return nil, err
    }
    
    // Create migration instance object
    obj := map[string]any{
        "id":             fmt.Sprintf("MIGRATION-%s", spec.ID),
        "kind":           "migration",
        "schema_version": "2.0.0",
        "migration_id":   spec.ID,
        "name":           spec.Name,
        "description":    spec.Description,
        "from_state":     spec.From.State,
        "to_state":       spec.To.State,
        "prerequisites":  spec.Prerequisites,
        "steps":          spec.Steps,
        "rollback":       spec.Rollback,
        "validation":     spec.Validation,
        "options":        spec.Options,
        "status":         "pending",
        "created_at":     time.Now().UTC().Format(time.RFC3339),
        "created_by":     "account:system",
    }
    
    return obj, nil
}
```

## Kind Migration

### Structure

```go
package migration

// KindMigration is a specific migration type for object kind transformations
type KindMigration struct {
    *BaseMigration
    SourceKind string
    TargetKind string
    Transform  KindTransform
}

type KindTransform interface {
    TransformObject(source map[string]any) (map[string]any, error)
    ValidateSource(source map[string]any) error
    ValidateTarget(target map[string]any) error
}

// Examples:
// - LifecycleFilesToObjects: lifecycle files → lifecycle objects
// - AuditToBuckets: audit events → bucketed audit events
// - CASToFile: CAS objects → file-based objects (rollback)
```

### Implementation Example

```go
// LifecycleFilesToObjectsMigration
type LifecycleFilesToObjectsMigration struct {
    *KindMigration
}

func NewLifecycleFilesToObjectsMigration() *LifecycleFilesToObjectsMigration {
    return &LifecycleFilesToObjectsMigration{
        KindMigration: &KindMigration{
            SourceKind: "lifecycle_file",
            TargetKind: "lifecycle",
        },
    }
}

func (m *LifecycleFilesToObjectsMigration) TransformObject(source map[string]any) (map[string]any, error) {
    // Use lifecycle instance builder to convert file to object
    builder := bldr_instance_v1.NewLifecycleInstanceBuilder("2.0.0")
    // ... transform logic
    return builder.Build(), nil
}
```

## Custom Migrations

### Registration

```go
package migration

type MigrationType string

const (
    MigrationTypeKind   MigrationType = "kind"
    MigrationTypeCustom MigrationType = "custom"
)

type CustomMigration interface {
    Execute(ctx context.Context, spec *MigrationSpec) error
    Validate(ctx context.Context, spec *MigrationSpec) error
    Rollback(ctx context.Context, spec *MigrationSpec) error
}

var migrationRegistry = make(map[string]CustomMigration)

func RegisterCustomMigration(id string, migration CustomMigration) {
    migrationRegistry[id] = migration
}

func GetCustomMigration(id string) (CustomMigration, error) {
    if migration, ok := migrationRegistry[id]; ok {
        return migration, nil
    }
    return nil, fmt.Errorf("custom migration %s not found", id)
}
```

### Usage

```go
// Register custom migration
migration.RegisterCustomMigration("custom-data-migration", &MyCustomMigration{})

// In migration spec
steps:
  - id: custom_step
    type: custom
    migration_type: custom-data-migration
    config:
      # Custom configuration
```

## Migration Execution

### Executor

```go
package migration

type Executor struct {
    storage storage.ObjectStorageProvider
    logger  logging.Logger
}

func (e *Executor) Execute(ctx context.Context, specPath string, options ExecutionOptions) error {
    // Load migration spec
    spec, err := LoadMigrationSpec(specPath)
    if err != nil {
        return err
    }
    
    // Build migration instance
    instanceBuilder := bldr_instance_v1.NewMigrationInstanceBuilder("2.0.0")
    instance, err := instanceBuilder.BuildFromSpec(spec)
    if err != nil {
        return err
    }
    
    // Determine migration type
    if spec.Type == "kind" {
        kindMigration := NewKindMigration(spec)
        return kindMigration.Execute(ctx, spec, options)
    } else if spec.Type == "custom" {
        customMigration, err := GetCustomMigration(spec.CustomType)
        if err != nil {
            return err
        }
        return customMigration.Execute(ctx, spec)
    }
    
    // Generic step-based execution
    return e.executeSteps(ctx, spec, options)
}
```

## Directory Structure

```
pkg/
├── migration/
│   ├── spec.go                    # Migration spec structures
│   ├── executor.go                # Migration executor
│   ├── kind_migration.go          # Kind migration implementation
│   ├── custom_migration.go        # Custom migration interface
│   └── registry.go                # Migration registry

pkg/specbuilder/
├── bldr_migration_v1/
│   ├── migration_spec_builder.go  # Migration spec builder
│   ├── step_builder.go            # Step builder
│   └── prerequisite_builder.go    # Prerequisite builder
└── bldr_instance_v1/
    └── migration_instance_builder.go  # Migration instance builder

.zqk/specs/
└── migrations/
    ├── lifecycle-files-to-objects.yaml
    ├── audit-to-buckets.yaml
    ├── cas-migration.yaml
    └── README.md
```

## Benefits

1. **Consistency**: Follows existing builder pattern
2. **Type Safety**: Builders provide compile-time validation
3. **Extensibility**: Custom migrations via registry
4. **Reusability**: Common patterns in kind migrations
5. **Validation**: Instance builders validate specs
6. **Testability**: Each component can be tested independently
