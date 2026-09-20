# Spec-Driven Builder Pattern - Core Package

This package provides the core infrastructure for the Spec-Driven Builder Pattern, a reusable pattern for generating artifacts from declarative YAML specifications using programmatic builder APIs.

## Overview

The Spec-Driven Builder Pattern combines:
1. **Declarative YAML specifications** (human-readable "what")
2. **Programmatic builder APIs** (type-safe "how")
3. **Generator layer** (orchestration that connects specs to builders)

## Package Structure

```
pkg/specbuilder/
├── core/              # Core interfaces and types
│   ├── builder.go     # Builder interface
│   ├── generator.go   # Generator interface
│   ├── spec.go        # Spec interface
│   └── writer.go      # Writer interface
├── yaml/              # YAML-specific implementations
│   ├── loader.go      # YAML spec loader
│   └── writer.go      # YAML writer
└── README.md          # This file
```

## Core Concepts

### Builder
A fluent API for programmatically constructing objects. Builders provide type-safe, chainable methods for building complex structures.

### Generator
Orchestrates the transformation from specifications to artifacts using builders. Handles reading specs, using builders to construct objects, and writing outputs.

### Spec
A declarative specification (typically YAML) that defines "what" should be built. Specs are human-readable and version-control friendly.

### Writer
Handles writing generated artifacts to various outputs (files, strings, bytes, streams).

## Usage

```go
// 1. Define a builder (domain-specific)
type MyBuilder struct {
    // ... builder state
}

// 2. Define a spec (YAML structure)
type MySpec struct {
    Name string `yaml:"name"`
    // ... spec fields
}

// 3. Create a generator
generator := yaml.NewYAMLGenerator(outputDir)
generator.RegisterBuilder("my-domain", func(spec MySpec) (*MyObject, error) {
    // Build using domain-specific builder
    builder := NewMyBuilder()
    return builder.FromSpec(spec).Build(), nil
})

// 4. Generate from spec file
err := generator.GenerateFromFile("my-specs.yaml")
```

## Domain-Specific Packages

The core package is extended by domain-specific packages:

- `pkg/specbuilder/generators` - Test scenario generation (new implementation)
- `pkg/mcp/testing` - Test scenario generation (legacy, being migrated)
- `pkg/templates/builder` - Object template generation
- `pkg/config/builder` - Configuration file generation
- `pkg/validation/builder` - Validation rule generation
- (more to come...)

**Note**: The scenario generator has been migrated to use specbuilder infrastructure. See `MIGRATION_GUIDE.md` for migration details.

Each domain package:
- Extends core interfaces for its specific domain
- Provides domain-specific builders
- Provides domain-specific generators
- Reuses core YAML loading/writing infrastructure

## Design Principles

1. **Separation of Concerns**: Core is generic, domain packages are specific
2. **Reusability**: Common patterns (YAML loading, writing) are shared
3. **Extensibility**: Easy to add new domains and adapters
4. **Type Safety**: Builders provide compile-time checking
5. **Flexibility**: Can use programmatically OR from specs

## Future Evolution

This pattern can evolve to support:
- Multiple spec formats (JSON, TOML, etc.)
- Multiple output formats (code, configs, docs, etc.)
- Code generation (generate builders from schemas)
- Validation and schema checking
- Template engines for complex transformations
- Plugin system for custom builders/generators
