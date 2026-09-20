package internal

// This file contains example usage patterns for the CLIExampleGenerator builder
// These examples demonstrate extensible maintainability patterns

/*
Example 1: Basic Usage with Default Generator

	generator, err := NewCLIExampleGenerator()
	if err != nil {
		return err
	}

	example, err := generator.GenerateExampleObject("component", "")
	if err != nil {
		return err
	}

Example 2: Builder Pattern with Custom Field Generator

	builder, err := NewExampleBuilder()
	if err != nil {
		return err
	}

	// Register custom generator for component_type
	customTypeGenerator := func(fieldName string, fieldDef map[string]any, kind string) (any, error) {
		// Custom logic: return a specific component type based on context
		return "custom_container", nil
	}

	generator := builder.
		WithFieldGenerator("component_type", customTypeGenerator).
		Build()

	example, err := generator.GenerateExampleObject("component", "COMP-001")

Example 3: Builder Pattern with Field Overrides

	builder, err := NewExampleBuilder()
	if err != nil {
		return err
	}

	generator := builder.
		WithFieldOverride("title", "My Custom Title").
		WithFieldOverride("status", "active").
		WithFieldOverride("priority", "high").
		Build()

	example, err := generator.GenerateExampleObject("backlog_item", "")

Example 4: Builder Pattern with Optional Fields and Exclusions

	builder, err := NewExampleBuilder()
	if err != nil {
		return err
	}

	generator := builder.
		WithOptionalFields(true).  // Include optional fields
		ExcludeField("updated_by"). // Exclude specific fields
		ExcludeFields("created_by", "archived_by"). // Exclude multiple fields
		Build()

	example, err := generator.GenerateExampleObject("component", "")

Example 5: Complex Builder Configuration

	builder, err := NewExampleBuilder()
	if err != nil {
		return err
	}

	// Custom generators for multiple fields
	generators := map[string]FieldValueGenerator{
		"component_type": func(fieldName string, fieldDef map[string]any, kind string) (any, error) {
			return "task_bar", nil
		},
		"status": func(fieldName string, fieldDef map[string]any, kind string) (any, error) {
			return "draft", nil
		},
	}

	// Field overrides
	overrides := map[string]any{
		"title": "Root Container",
		"id":    "COMP-ROOT-001",
	}

	generator := builder.
		WithFieldGenerators(generators).
		WithFieldOverrides(overrides).
		WithOptionalFields(false).
		ExcludeFields("change_log", "status_history", "artifacts").
		Build()

	example, err := generator.GenerateExampleObject("component", "")

Example 6: Domain-Specific Extension

	// Create a domain-specific builder configuration
	func NewComponentExampleBuilder() (*ExampleBuilder, error) {
		builder, err := NewExampleBuilder()
		if err != nil {
			return nil, err
		}

		// Component-specific field generators
		builder.WithFieldGenerator("component_type", func(fieldName string, fieldDef map[string]any, kind string) (any, error) {
			// Return a valid component type from the enum
			if validation, ok := fieldDef["validation"].(map[string]any); ok {
				if enumValues, ok := validation["enum"].([]any); ok && len(enumValues) > 0 {
					return enumValues[0], nil
				}
			}
			return "container", nil
		})

		return builder, nil
	}

	// Usage
	builder, _ := NewComponentExampleBuilder()
	generator := builder.Build()
	example, _ := generator.GenerateExampleObject("component", "")

Example 7: Extending for Different Output Formats

	// Create a builder that generates examples optimized for documentation
	func NewDocumentationExampleBuilder() (*ExampleBuilder, error) {
		builder, err := NewExampleBuilder()
		if err != nil {
			return nil, err
		}

		// Include all optional fields for complete documentation
		builder.WithOptionalFields(true)

		// Use descriptive values
		builder.WithFieldOverride("title", "Example Component Title")
		builder.WithFieldOverride("description", "This is an example component used in documentation")

		return builder, nil
	}

	// Create a builder that generates minimal examples for quick reference
	func NewMinimalExampleBuilder() (*ExampleBuilder, error) {
		builder, err := NewExampleBuilder()
		if err != nil {
			return nil, err
		}

		// Exclude all optional fields
		builder.WithOptionalFields(false)

		// Exclude verbose fields
		builder.ExcludeFields("change_log", "status_history", "artifacts", "stakeholders")

		return builder, nil
	}
*/
