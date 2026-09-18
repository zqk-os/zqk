package adapters

import (
	mcptesting "github.com/zqk-os/zqk/pkg/mcp/testing"
	sbcore "github.com/zqk-os/zqk/pkg/specbuilder/core"
)

const emptyValue = ""

// ScenarioBuilderAdapter adapts the existing mcptesting.ScenarioBuilder to work with specbuilder core
// This allows the existing builder to be used with the new infrastructure
type ScenarioBuilderAdapter struct {
	builder *mcptesting.ScenarioBuilder
}

// NewScenarioBuilderAdapter creates an adapter for an existing scenario builder
func NewScenarioBuilderAdapter(builder *mcptesting.ScenarioBuilder) *ScenarioBuilderAdapter {
	return &ScenarioBuilderAdapter{
		builder: builder,
	}
}

// Build implements core.Builder by delegating to the existing builder
func (a *ScenarioBuilderAdapter) Build() *mcptesting.TestScenario {
	return a.builder.Build()
}

// GetBuilder returns the underlying builder (for access to builder methods if needed)
func (a *ScenarioBuilderAdapter) GetBuilder() *mcptesting.ScenarioBuilder {
	return a.builder
}

// ScenarioSpecAdapter adapts mcptesting.ScenarioSpec to implement core.Spec
type ScenarioSpecAdapter struct {
	spec mcptesting.ScenarioSpec
}

// NewScenarioSpecAdapter creates an adapter for a scenario spec
func NewScenarioSpecAdapter(spec mcptesting.ScenarioSpec) *ScenarioSpecAdapter {
	return &ScenarioSpecAdapter{
		spec: spec,
	}
}

// Validate implements core.Spec
func (a *ScenarioSpecAdapter) Validate() error {
	if a.spec.Name == emptyValue {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if len(a.spec.Tests) == 0 {
		return &ValidationError{Field: "tests", Message: "at least one test is required"}
	}
	return nil
}

// GetName implements core.Spec
func (a *ScenarioSpecAdapter) GetName() string {
	return a.spec.Name
}

// GetSpec returns the underlying spec
func (a *ScenarioSpecAdapter) GetSpec() mcptesting.ScenarioSpec {
	return a.spec
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// ScenarioBuilderFactoryAdapter creates scenario builders from specs using the existing builder API
type ScenarioBuilderFactoryAdapter struct{}

// NewScenarioBuilderFactoryAdapter creates a new factory adapter
func NewScenarioBuilderFactoryAdapter() *ScenarioBuilderFactoryAdapter {
	return &ScenarioBuilderFactoryAdapter{}
}

// CreateBuilder implements core.BuilderFactory
func (f *ScenarioBuilderFactoryAdapter) CreateBuilder(spec *ScenarioSpecAdapter) sbcore.Builder[*mcptesting.TestScenario] {
	// Use the existing scenario builder API
	builder := mcptesting.NewScenarioBuilder().
		Name(spec.spec.Name)

	if spec.spec.Description != emptyValue {
		builder.Description(spec.spec.Description)
	}

	if spec.spec.ResponseProcessor != emptyValue {
		builder.ResponseProcessor(spec.spec.ResponseProcessor)
	}

	// Add imports
	for _, importPath := range spec.spec.Imports {
		builder.AddImport(importPath)
	}

	// Add setup steps
	for _, step := range spec.spec.Setup {
		stepCopy := step
		builder.AddSetupStep(&stepCopy)
	}

	// Add test steps
	for _, step := range spec.spec.Tests {
		stepCopy := step
		builder.AddTestStep(&stepCopy)
	}

	// Add cleanup steps
	for _, step := range spec.spec.Cleanup {
		stepCopy := step
		builder.AddCleanupStep(&stepCopy)
	}

	// Return adapter wrapping the builder
	return NewScenarioBuilderAdapter(builder)
}
