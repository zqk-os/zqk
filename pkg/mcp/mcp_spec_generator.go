package mcp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// MCPSpecGenerator orchestrates the transformation from MCP specs to server registration
// Following the spec-driven builder pattern
type MCPSpecGenerator struct {
	server *Server
}

// NewMCPSpecGenerator creates a new MCP spec generator
func NewMCPSpecGenerator(server *Server) *MCPSpecGenerator {
	return &MCPSpecGenerator{
		server: server,
	}
}

// GenerateFromSpec applies an MCP spec to the server
// Registers all prompts, resources, and tools defined in the spec
func (g *MCPSpecGenerator) GenerateFromSpec(spec *MCPSpec) error {
	if err := spec.Validate(); err != nil {
		return errfmt.Newf("invalid MCP spec").Wrap(err)
	}

	// Register prompts
	for _, promptSpec := range spec.Prompts {
		if err := g.RegisterPrompt(promptSpec); err != nil {
			return errfmt.Errorf("failed to register prompt %s: %w", promptSpec.Name, err)
		}
	}

	// Register resources
	for _, resourceSpec := range spec.Resources {
		if err := g.registerResource(resourceSpec); err != nil {
			return errfmt.Errorf("failed to register resource %s: %w", resourceSpec.Name, err)
		}
	}

	// Register tools
	for _, toolSpec := range spec.Tools {
		if err := g.registerTool(toolSpec); err != nil {
			return errfmt.Errorf("failed to register tool %s: %w", toolSpec.Name, err)
		}
	}

	// Register schema handlers
	for _, handlerSpec := range spec.SchemaHandlers {
		if err := g.registerSchemaHandler(handlerSpec); err != nil {
			return errfmt.Errorf("failed to register schema handler %s: %w", handlerSpec.URIPattern, err)
		}
	}

	return nil
}

// GenerateFromSpecs applies multiple MCP specs to the server
func (g *MCPSpecGenerator) GenerateFromSpecs(specs []*MCPSpec) error {
	for _, spec := range specs {
		if err := g.GenerateFromSpec(spec); err != nil {
			return errfmt.Errorf("failed to apply spec %s: %w", spec.Name, err)
		}
	}
	return nil
}

// RegisterPrompt registers a prompt from a spec
func (g *MCPSpecGenerator) RegisterPrompt(spec PromptSpec) error {
	arguments := make([]PromptArgument, len(spec.Arguments))
	for i, arg := range spec.Arguments {
		arguments[i] = PromptArgument(arg)
	}
	g.server.RegisterPrompt(spec.Name, spec.Description, arguments)
	return nil
}

// registerResource registers a resource from a spec
func (g *MCPSpecGenerator) registerResource(spec ResourceSpec) error {
	if spec.Category != emptyValue || spec.Priority != emptyValue || len(spec.Tags) > 0 || len(spec.Metadata) > 0 {
		g.server.RegisterResourceWithMetadata(
			spec.URI,
			spec.Name,
			spec.Description,
			spec.MimeType,
			spec.Category,
			spec.Priority,
			spec.Tags,
			spec.Metadata,
		)
	} else {
		g.server.RegisterResource(spec.URI, spec.Name, spec.Description, spec.MimeType)
	}
	return nil
}

// registerTool registers a tool from a spec
func (g *MCPSpecGenerator) registerTool(spec ToolSpec) error {
	// Build input schema from properties
	schema := g.buildInputSchema(spec)

	// Get handler if specified via ToolHandlerRegistry
	var handler ToolHandler
	if spec.Handler != emptyValue {
		if h, ok := GlobalToolHandlerRegistry.Get(spec.Handler); ok {
			handler = h
		} else {
			return errfmt.Errorf("unregistered tool handler '%s' specified for tool '%s'", spec.Handler, spec.Name)
		}
	}

	g.server.RegisterTool(spec.Name, spec.Description, schema, handler)
	return nil
}

// registerSchemaHandler registers a schema handler from a spec
func (g *MCPSpecGenerator) registerSchemaHandler(spec SchemaHandlerSpec) error {
	// Get handler function by name
	handler := getSchemaHandlerByName(spec.Handler)
	if handler == nil {
		return errfmt.Errorf("unknown schema handler: %s", spec.Handler)
	}

	// Register handler with URI pattern
	g.server.RegisterSchemaHandler(spec.URIPattern, handler, spec.IsPrefix)
	return nil
}

// getSchemaHandlerByName returns a schema handler function by name
func getSchemaHandlerByName(name string) SchemaHandler {
	switch name {
	case "handleSchemaRegistry":
		return func(server *Server, uri string) (any, error) {
			return server.handleSchemaRegistry()
		}
	case "handleCLIOntology":
		return func(server *Server, uri string) (any, error) {
			return server.handleCLIOntology()
		}
	case "handleCommonFieldsSchema":
		return func(server *Server, uri string) (any, error) {
			return server.handleCommonFieldsSchema()
		}
	case "handleObjectSchema":
		return func(server *Server, uri string) (any, error) {
			// Extract kind from URI
			kind := strings.TrimPrefix(uri, "schema://object/")
			return server.handleObjectSchema(kind)
		}
	default:
		return nil
	}
}

// buildInputSchema builds an input schema from tool spec properties
func (g *MCPSpecGenerator) buildInputSchema(spec ToolSpec) map[string]any {
	properties := make(map[string]any)
	for name, prop := range spec.Properties {
		propMap := map[string]any{
			objects.FieldKeyType:        prop.Type,
			objects.FieldKeyDescription: prop.Description,
		}
		if prop.Default != nil {
			propMap["default"] = prop.Default
		}
		if len(prop.Enum) > 0 {
			propMap["enum"] = prop.Enum
		}
		if prop.Items != nil {
			propMap["items"] = map[string]any{
				objects.FieldKeyType: prop.Items.Type,
			}
		}
		properties[name] = propMap
	}

	schema := map[string]any{
		objects.FieldKeyType: "object",
		"properties":         properties,
	}
	if len(spec.Required) > 0 {
		schema["required"] = spec.Required
	}
	return schema
}

// MCPSpecLoader loads MCP specs from YAML files
type MCPSpecLoader struct{}

// NewMCPSpecLoader creates a new MCP spec loader
func NewMCPSpecLoader() *MCPSpecLoader {
	return &MCPSpecLoader{}
}

// LoadSpec loads a single spec from a YAML file
func (l *MCPSpecLoader) LoadSpec(filePath string) (*MCPSpec, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Newf("failed to read spec file").Wrap(err)
	}

	var spec MCPSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, errfmt.Newf("failed to unmarshal spec").Wrap(err)
	}

	if err := spec.Validate(); err != nil {
		return nil, errfmt.Newf("spec validation failed").Wrap(err)
	}

	return &spec, nil
}

// LoadSpecs loads multiple specs from a directory or file
func (l *MCPSpecLoader) LoadSpecs(source string) ([]*MCPSpec, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, errfmt.Newf("failed to stat source").Wrap(err)
	}

	var specs []*MCPSpec

	if info.IsDir() {
		// Load all YAML files in directory
		entries, err := os.ReadDir(source)
		if err != nil {
			return nil, errfmt.Newf("failed to read directory").Wrap(err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if filepath.Ext(entry.Name()) != ".yaml" && filepath.Ext(entry.Name()) != ".yml" {
				continue
			}

			filePath := filepath.Join(source, entry.Name())
			spec, err := l.LoadSpec(filePath)
			if err != nil {
				return nil, errfmt.Errorf("failed to load spec from %s: %w", filePath, err)
			}
			specs = append(specs, spec)
		}
	} else {
		// Load single file
		spec, err := l.LoadSpec(source)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}

	return specs, nil
}
