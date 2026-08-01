# Spec-Driven Builder Pattern

## Overview

The **Spec-Driven Builder Pattern** combines declarative YAML specifications with programmatic builder APIs to generate artifacts (code, configuration, test scenarios, etc.). This pattern provides:

1. **Human-readable specifications** in YAML format
2. **Type-safe builder APIs** for programmatic construction
3. **Generator layer** that reads specs and uses builders to produce outputs
4. **Separation of concerns**: specs define "what", builders define "how"

## Pattern Structure

```
┌─────────────────┐
│ YAML Spec File  │  (Declarative - "what")
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Generator     │  (Orchestration layer)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Builder API    │  (Programmatic - "how")
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Generated Files │  (Output artifacts)
└─────────────────┘
```

## Example: Test Scenario Generation

### YAML Specification (`scenarios.yaml`)
```yaml
scenarios:
  - name: "Interactive Creation - All Kinds"
    description: "Test creating one object of each kind"
    tests:
      - name: "Create backlog_item"
        tool: "zqk_create_object_interactive"
        args:
          kind: "backlog_item"
        expected:
          success: true
```

### Builder API Usage
```go
scenario := NewScenarioBuilder().
    Name("Interactive Creation - All Kinds").
    Description("Test creating one object of each kind").
    AddTestStep(NewStepBuilder().
        Name("Create backlog_item").
        Tool("zqk_create_object_interactive").
        Arg("kind", "backlog_item").
        Expected(ExpectSuccess()).
        Build()).
    Build()
```

### Generator
```go
generator := NewScenarioGenerator(outputDir)
generator.GenerateFromFile("scenarios.yaml")
```

## Benefits

1. **Declarative**: Specs are human-readable and easy to review/modify
2. **Type-Safe**: Builder API provides compile-time checking
3. **Flexible**: Can generate programmatically OR from specs
4. **Maintainable**: Changes to specs automatically reflect in outputs
5. **Testable**: Builder API can be unit tested independently
6. **Version Control Friendly**: Specs are plain YAML files

## When to Use This Pattern

Use this pattern when you have:
- **Repetitive structures** that can be defined declaratively
- **Complex objects** with many fields/configurations
- **Multiple variants** of similar structures
- **Need for both declarative and programmatic creation**
- **Artifacts that need to be generated from specifications**

## Potential Applications

### 1. Test Scenario Generation (✅ Implemented)
- **Spec**: YAML file with scenario definitions
- **Builder**: ScenarioBuilder, StepBuilder, ExpectationBuilder
- **Generator**: ScenarioGenerator
- **Output**: Individual test scenario YAML files

### 2. Object Template Generation
- **Spec**: YAML file defining object templates with placeholders
- **Builder**: ObjectTemplateBuilder (for building object structures)
- **Generator**: TemplateGenerator (could generate templates for interactive creation)
- **Output**: Pre-configured object templates

### 3. Validation Rule Generation
- **Spec**: YAML file defining validation rules and constraints
- **Builder**: ValidationRuleBuilder, ConstraintBuilder
- **Generator**: ValidationRuleGenerator
- **Output**: Validation rule configurations or code

### 4. API Client Generation
- **Spec**: OpenAPI/Swagger-like spec defining API endpoints
- **Builder**: EndpointBuilder, RequestBuilder, ResponseBuilder
- **Generator**: APIClientGenerator
- **Output**: Client SDK code or configuration

### 5. Database Migration Generation
- **Spec**: YAML file defining schema changes
- **Builder**: MigrationBuilder, TableBuilder, ColumnBuilder
- **Generator**: MigrationGenerator
- **Output**: Database migration scripts

### 6. Configuration File Generation
- **Spec**: YAML file defining configuration templates
- **Builder**: ConfigBuilder (for building configuration structures)
- **Generator**: ConfigGenerator
- **Output**: Environment-specific configuration files

### 7. Documentation Generation
- **Spec**: YAML file defining documentation structure
- **Builder**: DocumentBuilder, SectionBuilder, ExampleBuilder
- **Generator**: DocumentationGenerator
- **Output**: Generated documentation files

### 8. MCP Tool Definitions
- **Spec**: YAML file defining MCP tools and their parameters
- **Builder**: MCPToolBuilder, ParameterBuilder
- **Generator**: MCPToolGenerator
- **Output**: MCP tool registration code or configuration

### 9. Lifecycle Definition Generation
- **Spec**: YAML file defining object lifecycles and transitions
- **Builder**: LifecycleBuilder, StateBuilder, TransitionBuilder
- **Generator**: LifecycleGenerator
- **Output**: Lifecycle configuration files

### 10. Scheduler Job Definitions
- **Spec**: YAML file defining scheduler jobs and their configurations
- **Builder**: JobBuilder, ScheduleBuilder, HandlerBuilder
- **Generator**: JobGenerator
- **Output**: Job configuration files or registration code

## Implementation Guidelines

### 1. Define Clear Spec Structure
- Use YAML for human readability
- Keep specs simple and focused
- Use consistent naming conventions

### 2. Build Fluent Builder APIs
- Method chaining for readability
- Type-safe where possible
- Provide convenience functions for common patterns

### 3. Create Generator Layer
- Read and parse specs
- Use builders to construct objects
- Handle file I/O and output organization
- Provide error handling and validation

### 4. Maintain Symmetry
- Builder API should mirror spec structure
- Generated outputs should match spec definitions
- Keep builder and spec formats aligned

### 5. Test Independently
- Test builder API independently
- Test generator independently
- Test end-to-end: spec → generator → output

## Comparison with Other Patterns

### vs. Template-Based Generation
- **Template-Based**: Uses templates with placeholders, filled with data
- **Spec-Driven Builder**: Uses builder APIs to construct objects from specs
- **Advantage**: Type-safety, compile-time checking, better IDE support

### vs. Code Generation
- **Code Generation**: Generates source code from specifications
- **Spec-Driven Builder**: Generates artifacts (could be code, config, etc.) using builders
- **Advantage**: More flexible, can generate multiple artifact types

### vs. Configuration Files
- **Configuration Files**: Direct mapping of config to runtime behavior
- **Spec-Driven Builder**: Specs are transformed into artifacts via builders
- **Advantage**: Specs can generate multiple outputs, builders add programmatic flexibility

## Example: Applying Pattern to Object Templates

```yaml
# object_templates.yaml
templates:
  - name: "Backlog Item - Feature"
    kind: "backlog_item"
    fields:
      title: "New Feature: {name}"
      status: "exploring"
      category: "feature"
      tags: ["feature", "enhancement"]
  
  - name: "Milestone - Sprint"
    kind: "milestone"
    fields:
      title: "Sprint {number}"
      status: "planned"
      category: "sprint"
```

```go
// ObjectTemplateBuilder (hypothetical)
template := NewObjectTemplateBuilder().
    Name("Backlog Item - Feature").
    Kind("backlog_item").
    Field("title", "New Feature: {name}").
    Field("status", "exploring").
    Field("category", "feature").
    Tags("feature", "enhancement").
    Build()

// TemplateGenerator (hypothetical)
generator := NewTemplateGenerator()
generator.GenerateFromFile("object_templates.yaml")
```

## Conclusion

The Spec-Driven Builder Pattern provides a powerful way to combine declarative specifications with programmatic construction. It's particularly useful when you need:

- **Flexibility**: Both declarative and programmatic creation
- **Maintainability**: Human-readable specs that generate artifacts
- **Type Safety**: Builder APIs provide compile-time checking
- **Scalability**: Generate many artifacts from a single spec file

This pattern has been successfully applied to test scenario generation and can be extended to many other areas of the codebase.
