# SpecBuilder Architecture

**Last Verified:** 2026-08-31


## Overview

The SpecBuilder package (`pkg/specbuilder`) provides a reusable foundation for the Spec-Driven Builder Pattern. It's designed to be:

1. **Core Generic**: Provides interfaces and common implementations
2. **Domain-Specific Extensions**: Domain packages extend the core for specific use cases
3. **Eventually Extractable**: Can become a separate library when mature

## Package Structure

```
pkg/specbuilder/
├── core/                    # Core interfaces and base implementations
│   ├── interfaces.go        # Core interfaces (Builder, Spec, Generator, Writer)
│   ├── generator.go         # BaseGenerator with common functionality
│   └── README.md
├── yaml/                    # YAML-specific implementations
│   ├── loader.go            # YAML spec loading utilities
│   ├── writer.go            # YAML writer implementation
│   └── README.md
└── README.md                # Package overview

Domain-specific packages:
├── pkg/mcp/testing/         # Test scenario generation (already exists)
│   └── scenario_builder.go  # Uses specbuilder core
├── pkg/templates/builder/   # Object template generation (future)
├── pkg/config/builder/      # Configuration generation (future)
└── pkg/validation/builder/  # Validation rule generation (future)
```

## Core Components

### Interfaces (`core/interfaces.go`)

**Builder[T]**: Fluent API for building objects
```go
type Builder[T any] interface {
    Build() T
}
```

**Spec**: Declarative specification
```go
type Spec interface {
    Validate() error
    GetName() string
}
```

**Generator[S, T]**: Orchestrates spec → artifact transformation
```go
type Generator[S Spec, T any] interface {
    GenerateFromSpec(spec S) (T, error)
    GenerateFromSpecs(specs []S) ([]T, error)
}
```

**Writer[T]**: Writes artifacts to outputs
```go
type Writer[T any] interface {
    Write(artifact T, writer io.Writer) error
    WriteToFile(artifact T, filePath string) error
    WriteToBytes(artifact T) ([]byte, error)
    WriteToString(artifact T) (string, error)
}
```

**BuilderFactory[S, T]**: Creates builders from specs
```go
type BuilderFactory[S Spec, T any] interface {
    CreateBuilder(spec S) Builder[T]
}
```

### Base Generator (`core/generator.go`)

`BaseGenerator` provides common functionality that domain-specific generators can embed:

- `GenerateFromSpec`: Generate single artifact
- `GenerateFromSpecs`: Generate multiple artifacts
- `GenerateAndWriteFromSpec`: Generate and write to file
- `GenerateAndWriteFromSpecs`: Generate and write multiple files
- Filename generation and sanitization

### YAML Utilities (`yaml/`)

- **Loader**: Generic YAML spec loading
  - `LoadYAMLSpec`: Load single spec
  - `LoadYAMLSpecs`: Load list of specs
  - `LoadYAMLSpecList`: Load from file with wrapper key (e.g., `scenarios:`)
  - `FindSpecFiles`: Find spec files in directory

- **Writer**: Generic YAML writing
  - Implements `core.Writer` interface
  - Supports files, bytes, strings, and io.Writer

## Domain-Specific Implementation Pattern

Each domain package follows this pattern:

```go
// 1. Define domain-specific types
type MySpec struct {
    Name string `yaml:"name"`
    // ... domain fields
}

func (s MySpec) Validate() error { /* ... */ }
func (s MySpec) GetName() string { return s.Name }

type MyArtifact struct {
    // ... artifact structure
}

// 2. Implement builder
type MyBuilder struct {
    artifact *MyArtifact
}

func (b *MyBuilder) Build() MyArtifact { return *b.artifact }

// 3. Implement builder factory
type MyBuilderFactory struct{}

func (f *MyBuilderFactory) CreateBuilder(spec MySpec) core.Builder[MyArtifact] {
    builder := NewMyBuilder()
    // Configure builder from spec
    return builder
}

// 4. Create domain-specific generator
type MyGenerator struct {
    *core.BaseGenerator[MySpec, MyArtifact]
}

func NewMyGenerator(outputDir string) *MyGenerator {
    factory := &MyBuilderFactory{}
    writer := yaml.NewYAMLWriter[MyArtifact]()
    base := core.NewBaseGenerator(factory, writer, outputDir)
    return &MyGenerator{BaseGenerator: base}
}

// 5. Add domain-specific convenience methods
func (g *MyGenerator) GenerateFromFile(filePath string) error {
    specs, err := yaml.LoadYAMLSpecList[MySpec](filePath, "my_specs")
    if err != nil {
        return err
    }
    return g.GenerateAndWriteFromSpecs(specs)
}
```

## Migration Strategy

### Phase 1: Extract Core (Current)
1. ✅ Create `pkg/specbuilder/core` with interfaces
2. ✅ Create `pkg/specbuilder/yaml` with YAML utilities
3. ✅ Document architecture

### Phase 2: Refactor Existing Code
1. Refactor `pkg/mcp/testing/scenario_builder` to use core
2. Extract common patterns to core
3. Update tests

### Phase 3: Expand to New Domains
1. Object template generation
2. Configuration generation
3. Validation rule generation
4. Other domains as needed

### Phase 4: Mature and Extract (Future)
1. When pattern is stable and well-tested
2. Extract to separate library/module
3. Use as external dependency
4. Maintain backward compatibility

## Benefits of This Architecture

1. **Reusability**: Core patterns shared across domains
2. **Consistency**: Same pattern, different domains
3. **Testability**: Core can be tested independently
4. **Extensibility**: Easy to add new domains
5. **Maintainability**: Changes to core benefit all domains
6. **Evolution Path**: Can extract to library when mature

## Design Decisions

### Why Generic Interfaces?
- Type safety at compile time
- Clear contracts between components
- Enables code generation in future

### Why Base Generator?
- Reduces boilerplate in domain packages
- Ensures consistent behavior
- Common functionality (filename generation, etc.)

### Why Separate YAML Package?
- Can add other formats (JSON, TOML) later
- Clear separation of concerns
- Reusable across domains

### Why Domain Packages?
- Domain-specific logic stays in domain
- Core remains generic
- Easy to find domain-specific code

## Future Enhancements

1. **Schema Validation**: JSON Schema or similar for spec validation
2. **Code Generation**: Generate builders from schemas
3. **Multiple Formats**: JSON, TOML, etc.
4. **Template Engine**: For complex transformations
5. **Plugin System**: Custom builders/generators
6. **CLI Tools**: Command-line tools for generation
7. **IDE Support**: Language server, autocomplete
8. **Documentation Generation**: Auto-generate docs from specs

## Example: Test Scenario Domain

The test scenario domain (`pkg/mcp/testing`) uses specbuilder like this:

```go
// Domain types
type TestScenarioSpec struct {
    Name string `yaml:"name"`
    Tests []TestStep `yaml:"tests"`
    // ...
}

// Builder (existing ScenarioBuilder)
type ScenarioBuilder struct {
    scenario *TestScenario
}

// Generator (existing ScenarioGenerator, refactored to use core)
type ScenarioGenerator struct {
    *core.BaseGenerator[TestScenarioSpec, TestScenario]
}

func NewScenarioGenerator(outputDir string) *ScenarioGenerator {
    factory := &ScenarioBuilderFactory{}
    writer := yaml.NewYAMLWriter[TestScenario]()
    base := core.NewBaseGenerator(factory, writer, outputDir)
    return &ScenarioGenerator{BaseGenerator: base}
}
```

This allows the test scenario domain to focus on test-specific logic while reusing core infrastructure.
