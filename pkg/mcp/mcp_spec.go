package mcp

import (
	"github.com/lanceman/zqk/pkg/errfmt"
)

// MCPSpec represents a declarative specification for MCP server configuration
// This allows externalizing prompts, resources, tools, and other MCP components
// Following the spec-driven builder pattern for consistency with the rest of the codebase
type MCPSpec struct {
	// Metadata
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Version     string `yaml:"version,omitempty"`

	// Prompts configuration
	Prompts []PromptSpec `yaml:"prompts,omitempty"`

	// Resources configuration
	Resources []ResourceSpec `yaml:"resources,omitempty"`

	// Tool configurations (for custom tools beyond CLI discovery)
	Tools []ToolSpec `yaml:"tools,omitempty"`

	// Tool groups (for organizing and filtering tools)
	ToolGroups []ToolGroupSpec `yaml:"tool_groups,omitempty"`

	// Resource discovery patterns
	ResourceDiscovery *ResourceDiscoverySpec `yaml:"resource_discovery,omitempty"`

	// Schema handlers configuration (for schema:// URI routing)
	SchemaHandlers []SchemaHandlerSpec `yaml:"schema_handlers,omitempty"`
}

// PromptSpec defines a prompt template specification
type PromptSpec struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Arguments   []PromptArgSpec   `yaml:"arguments,omitempty"`
	Template    string            `yaml:"template,omitempty"`     // Template content (if externalized)
	TemplateRef string            `yaml:"template_ref,omitempty"` // Reference to external template file
	Variables   map[string]string `yaml:"variables,omitempty"`    // Template variables
}

// PromptArgSpec defines a prompt argument
type PromptArgSpec struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required,omitempty"`
}

// ResourceSpec defines a resource specification
type ResourceSpec struct {
	URI         string            `yaml:"uri"`
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	MimeType    string            `yaml:"mime_type,omitempty"`
	Category    string            `yaml:"category,omitempty"`
	Priority    string            `yaml:"priority,omitempty"`
	Tags        []string          `yaml:"tags,omitempty"`
	Metadata    map[string]string `yaml:"metadata,omitempty"`
}

// ToolSpec defines a custom tool specification (beyond CLI discovery)
type ToolSpec struct {
	Name        string                  `yaml:"name"`
	Description string                  `yaml:"description"`
	Properties  map[string]PropertySpec `yaml:"properties,omitempty"`
	Required    []string                `yaml:"required,omitempty"`
	Handler     string                  `yaml:"handler,omitempty"` // Handler function name or type
}

// PropertySpec defines a tool property schema
type PropertySpec struct {
	Type        string        `yaml:"type"` // string, number, boolean, array
	Description string        `yaml:"description"`
	Default     any           `yaml:"default,omitempty"`
	Enum        []string      `yaml:"enum,omitempty"`
	Items       *PropertySpec `yaml:"items,omitempty"` // For array types
}

// ToolGroupSpec defines a group of tools for organization and filtering
type ToolGroupSpec struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Tools       []string `yaml:"tools"` // Tool names in this group
	Tags        []string `yaml:"tags,omitempty"`
}

// ResourceDiscoverySpec defines patterns for automatic resource discovery
type ResourceDiscoverySpec struct {
	Enabled    bool              `yaml:"enabled,omitempty"`
	BasePaths  []string          `yaml:"base_paths,omitempty"`
	Patterns   []string          `yaml:"patterns,omitempty"`   // Glob patterns for file discovery
	Exclusions []string          `yaml:"exclusions,omitempty"` // Patterns to exclude
	Categories map[string]string `yaml:"categories,omitempty"` // Path pattern -> category mapping
}

// SchemaHandlerSpec defines a schema URI handler mapping
type SchemaHandlerSpec struct {
	URIPattern string `yaml:"uri_pattern"`         // Exact URI match or prefix pattern (e.g., "schema://registry" or "schema://object/")
	Handler    string `yaml:"handler"`             // Handler function name (e.g., "handleSchemaRegistry", "handleCLIOntology", "handleObjectSchema")
	IsPrefix   bool   `yaml:"is_prefix,omitempty"` // If true, treat as prefix match (for patterns like "schema://object/")
}

// Validate validates the MCP spec
func (s *MCPSpec) Validate() error {
	// Basic validation
	if s.Name == emptyValue {
		return errfmt.Errorf("spec name is required")
	}

	// Validate prompts
	for i, prompt := range s.Prompts {
		if prompt.Name == emptyValue {
			return errfmt.Errorf("prompt[%d]: name is required", i)
		}
	}

	// Validate resources
	for i, resource := range s.Resources {
		if resource.URI == emptyValue {
			return errfmt.Errorf("resource[%d]: uri is required", i)
		}
		if resource.Name == emptyValue {
			return errfmt.Errorf("resource[%d]: name is required", i)
		}
	}

	// Validate tools
	for i, tool := range s.Tools {
		if tool.Name == emptyValue {
			return errfmt.Errorf("tool[%d]: name is required", i)
		}
	}

	return nil
}

// GetName returns the spec name
func (s *MCPSpec) GetName() string {
	return s.Name
}
