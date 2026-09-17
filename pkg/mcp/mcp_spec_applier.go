package mcp

import (
	"fmt"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// ApplyMCPSpec applies an MCP spec to the server
// This is a convenience function that creates a generator and applies the spec
func ApplyMCPSpec(server *Server, spec *MCPSpec) error {
	generator := NewMCPSpecGenerator(server)
	return generator.GenerateFromSpec(spec)
}

// ApplyMCPSpecs applies multiple MCP specs to the server
func ApplyMCPSpecs(server *Server, specs []*MCPSpec) error {
	generator := NewMCPSpecGenerator(server)
	return generator.GenerateFromSpecs(specs)
}

// LoadAndApplyMCPSpecs loads MCP specs from a directory or file and applies them
// This is the main entry point for externalizing MCP configuration
func LoadAndApplyMCPSpecs(server *Server, specPath string) error {
	loader := NewMCPSpecLoader()
	specs, err := loader.LoadSpecs(specPath)
	if err != nil {
		return errfmt.Errorf("failed to load MCP specs from %s: %w", specPath, err)
	}

	if len(specs) == 0 {
		return nil // No specs to apply
	}

	return ApplyMCPSpecs(server, specs)
}

// RegisterOnboardingPromptsFromSpec loads and registers prompts from a spec file
// This allows externalizing prompt definitions to YAML files
func RegisterOnboardingPromptsFromSpec(server *Server, specPath string) error {
	loader := NewMCPSpecLoader()
	specs, err := loader.LoadSpecs(specPath)
	if err != nil {
		return errfmt.Newf("failed to load prompt specs").Wrap(err)
	}

	generator := NewMCPSpecGenerator(server)
	for _, spec := range specs {
		// Only register prompts from this spec
		for _, promptSpec := range spec.Prompts {
			if err := generator.RegisterPrompt(promptSpec); err != nil {
				return errfmt.Errorf("failed to register prompt %s: %w", promptSpec.Name, err)
			}
		}
	}

	return nil
}

// ConvertPromptsToSpec converts the current hardcoded prompts to a spec
// This is a migration helper to externalize existing prompts
func ConvertPromptsToSpec() *MCPSpec {
	builder := NewMCPSpecBuilder("onboarding_prompts").
		Description("Standard onboarding and help prompts for MCP server").
		Version("1.0")

	// Welcome prompt
	builder.AddPrompt(NewPromptSpecBuilder("welcome",
		"Welcome prompt that instructs the AI agent to introduce itself and get started. Provides project context, current priority plans, and guidance on first steps. Use this at the start of a session to understand the project state.",
	).Build())

	// Getting started guide
	builder.AddPrompt(NewPromptSpecBuilder("getting_started",
		"Comprehensive getting started guide for new users of the MCP server. Explains available tools, permissions, system health checks, and common workflows. Essential reading for understanding how to use the MCP server effectively.",
	).Build())

	// Query help
	builder.AddPrompt(NewPromptSpecBuilder("query_help",
		fmt.Sprintf("Help guide for querying and filtering objects. Use this to learn how to use %s, %s, and filtering syntax. Essential for finding and working with objects.", GetToolName("object_list"), GetToolName("object_count")),
	).Build())

	// Create object guide
	builder.AddPrompt(NewPromptSpecBuilder("create_object_guide",
		"Guide and template for creating new objects. Use this before creating objects to understand required fields, optional fields, and best practices. Requires 'kind' argument to specify object type (e.g., backlog_item, goal, milestone).",
	).AddArgument("kind", "The object kind to create (e.g., backlog_item, goal, milestone)", true).Build())

	// Common tasks
	builder.AddPrompt(NewPromptSpecBuilder("common_tasks",
		"Examples of common tasks you can perform. Provides step-by-step examples for listing objects, checking system health, querying by status, and accessing documentation. Use this to learn common workflows.",
	).Build())

	// Object lifecycle
	builder.AddPrompt(NewPromptSpecBuilder("object_lifecycle",
		"Guide to understanding object lifecycles and state transitions. Essential reading before updating object status to understand valid transitions and lifecycle requirements. Use this with resources/get for the full lifecycle guide.",
	).Build())

	// Filter syntax
	builder.AddPrompt(NewPromptSpecBuilder("filter_syntax",
		fmt.Sprintf("Detailed guide to filter syntax for querying objects. Explains how to use filters with %s and %s, including equality, negation, and multiple filter combinations. Essential for effective object queries.", GetToolName("object_list"), GetToolName("object_count")),
	).Build())

	// Role-based access
	builder.AddPrompt(NewPromptSpecBuilder("role_based_access",
		"Guide to understanding role-based access and permissions in the MCP server. Explains how tools are filtered by role, permissions, and server configuration. Use this to understand why certain tools may not be available and how to check your access.",
	).Build())

	// Dynamic prompts
	builder.AddPrompt(NewPromptSpecBuilder("execution_context",
		"Current execution context including active priority plans, workflow policies, and lifecycle requirements. Use this to understand what work is currently prioritized and what workflow requirements must be followed. Updated dynamically from project state.",
	).Build())

	builder.AddPrompt(NewPromptSpecBuilder("big_picture",
		"Comprehensive project big picture including mission, vision, strategy, policies, and current state. Use this to understand the overall project goals, strategic direction, and current priorities. Updated dynamically from project state.",
	).Build())

	builder.AddPrompt(NewPromptSpecBuilder("current_role",
		"Shows your current role, permissions, and what operations you can perform. Use this to understand your access level and what tools are available to you based on your security context.",
	).Build())

	return builder.Build()
}
