package mcp

import (
	"slices"

	"github.com/lanceman/zqk/pkg/objects"
)

// MCPToolBuilder provides a fluent API for building MCP tool schemas
// This eliminates handcoded map[string]any structures and makes tool definitions more maintainable
type MCPToolBuilder struct {
	name        string
	description string
	properties  map[string]map[string]any
	required    []string
}

// NewToolBuilder creates a new MCP tool builder
func NewToolBuilder(name, description string) *MCPToolBuilder {
	return &MCPToolBuilder{
		name:        name,
		description: description,
		properties:  make(map[string]map[string]any),
		required:    []string{},
	}
}

// AddStringProperty adds a string property to the schema
func (b *MCPToolBuilder) AddStringProperty(name, description string) *MCPToolBuilder {
	b.properties[name] = map[string]any{
		objects.FieldKeyType:        "string",
		objects.FieldKeyDescription: description,
	}
	return b
}

// AddStringPropertyWithDefault adds a string property with a default value
func (b *MCPToolBuilder) AddStringPropertyWithDefault(name, description, defaultValue string) *MCPToolBuilder {
	b.properties[name] = map[string]any{
		objects.FieldKeyType:        "string",
		objects.FieldKeyDescription: description,
		"default":                   defaultValue,
	}
	return b
}

// AddStringPropertyWithEnum adds a string property with enum values
func (b *MCPToolBuilder) AddStringPropertyWithEnum(name, description string, enum []string) *MCPToolBuilder {
	prop := map[string]any{
		objects.FieldKeyType:        "string",
		objects.FieldKeyDescription: description,
		"enum":                      enum,
	}
	b.properties[name] = prop
	return b
}

// AddStringPropertyWithEnumAndDefault adds a string property with enum values and a default
func (b *MCPToolBuilder) AddStringPropertyWithEnumAndDefault(name, description string, enum []string, defaultValue string) *MCPToolBuilder {
	prop := map[string]any{
		objects.FieldKeyType:        "string",
		objects.FieldKeyDescription: description,
		"enum":                      enum,
		"default":                   defaultValue,
	}
	b.properties[name] = prop
	return b
}

// AddNumberProperty adds a number property to the schema
func (b *MCPToolBuilder) AddNumberProperty(name, description string) *MCPToolBuilder {
	b.properties[name] = map[string]any{
		objects.FieldKeyType:        "number",
		objects.FieldKeyDescription: description,
	}
	return b
}

// AddNumberPropertyWithDefault adds a number property with a default value
func (b *MCPToolBuilder) AddNumberPropertyWithDefault(name, description string, defaultValue float64) *MCPToolBuilder {
	b.properties[name] = map[string]any{
		objects.FieldKeyType:        "number",
		objects.FieldKeyDescription: description,
		"default":                   defaultValue,
	}
	return b
}

// AddBooleanProperty adds a boolean property to the schema
func (b *MCPToolBuilder) AddBooleanProperty(name, description string) *MCPToolBuilder {
	b.properties[name] = map[string]any{
		objects.FieldKeyType:        "boolean",
		objects.FieldKeyDescription: description,
	}
	return b
}

// AddBooleanPropertyWithDefault adds a boolean property with a default value
func (b *MCPToolBuilder) AddBooleanPropertyWithDefault(name, description string, defaultValue bool) *MCPToolBuilder {
	b.properties[name] = map[string]any{
		objects.FieldKeyType:        "boolean",
		objects.FieldKeyDescription: description,
		"default":                   defaultValue,
	}
	return b
}

// AddArrayProperty adds an array property to the schema
func (b *MCPToolBuilder) AddArrayProperty(name, description string, itemType string) *MCPToolBuilder {
	b.properties[name] = map[string]any{
		objects.FieldKeyType:        "array",
		objects.FieldKeyDescription: description,
		"items": map[string]any{
			objects.FieldKeyType: itemType,
		},
	}
	return b
}

// MarkRequired marks a property as required
func (b *MCPToolBuilder) MarkRequired(name string) *MCPToolBuilder {
	// Check if already in required list
	if !slices.Contains(b.required, name) {
		b.required = append(b.required, name)
	}
	return b
}

// AddFormatProperty adds a standard format property (json, yaml, table)
func (b *MCPToolBuilder) AddFormatProperty(allowedFormats []string, defaultFormat string) *MCPToolBuilder {
	return b.AddStringPropertyWithEnumAndDefault("format", "Output format", allowedFormats, defaultFormat)
}

// AddStandardFormatProperty adds the standard format property (json, yaml, table)
func (b *MCPToolBuilder) AddStandardFormatProperty() *MCPToolBuilder {
	return b.AddFormatProperty([]string{"json", "yaml", "table"}, "json")
}

// AddJSONYAMLFormatProperty adds format property with only json and yaml options
func (b *MCPToolBuilder) AddJSONYAMLFormatProperty() *MCPToolBuilder {
	return b.AddFormatProperty([]string{"json", "yaml"}, "json")
}

// Build returns the complete input schema
func (b *MCPToolBuilder) Build() map[string]any {
	schema := map[string]any{
		objects.FieldKeyType: "object",
		"properties":         b.properties,
	}
	if len(b.required) > 0 {
		schema["required"] = b.required
	}
	return schema
}

// Register registers the tool with the server using the built schema
func (b *MCPToolBuilder) Register(server *Server, handler ToolHandler) {
	server.RegisterTool(b.name, b.description, b.Build(), handler)
}
