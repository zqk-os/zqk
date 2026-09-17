package mcp

// MCPSpecBuilder provides a fluent API for building MCP specifications programmatically
// This allows programmatic construction of specs that can then be serialized to YAML
// Following the spec-driven builder pattern for consistency
type MCPSpecBuilder struct {
	spec *MCPSpec
}

// NewMCPSpecBuilder creates a new MCP spec builder
func NewMCPSpecBuilder(name string) *MCPSpecBuilder {
	return &MCPSpecBuilder{
		spec: &MCPSpec{
			Name:       name,
			Prompts:    []PromptSpec{},
			Resources:  []ResourceSpec{},
			Tools:      []ToolSpec{},
			ToolGroups: []ToolGroupSpec{},
		},
	}
}

// Description sets the spec description
func (b *MCPSpecBuilder) Description(desc string) *MCPSpecBuilder {
	b.spec.Description = desc
	return b
}

// Version sets the spec version
func (b *MCPSpecBuilder) Version(version string) *MCPSpecBuilder {
	b.spec.Version = version
	return b
}

// AddPrompt adds a prompt specification
func (b *MCPSpecBuilder) AddPrompt(prompt PromptSpec) *MCPSpecBuilder {
	b.spec.Prompts = append(b.spec.Prompts, prompt)
	return b
}

// AddResource adds a resource specification
func (b *MCPSpecBuilder) AddResource(resource ResourceSpec) *MCPSpecBuilder {
	b.spec.Resources = append(b.spec.Resources, resource)
	return b
}

// AddTool adds a tool specification
func (b *MCPSpecBuilder) AddTool(tool ToolSpec) *MCPSpecBuilder {
	b.spec.Tools = append(b.spec.Tools, tool)
	return b
}

// AddToolGroup adds a tool group specification
func (b *MCPSpecBuilder) AddToolGroup(group ToolGroupSpec) *MCPSpecBuilder {
	b.spec.ToolGroups = append(b.spec.ToolGroups, group)
	return b
}

// SetResourceDiscovery sets the resource discovery configuration
func (b *MCPSpecBuilder) SetResourceDiscovery(discovery ResourceDiscoverySpec) *MCPSpecBuilder {
	b.spec.ResourceDiscovery = &discovery
	return b
}

// Build returns the built MCP spec
func (b *MCPSpecBuilder) Build() *MCPSpec {
	return b.spec
}

// PromptSpecBuilder provides a fluent API for building prompt specifications
type PromptSpecBuilder struct {
	prompt *PromptSpec
}

// NewPromptSpecBuilder creates a new prompt spec builder
func NewPromptSpecBuilder(name, description string) *PromptSpecBuilder {
	return &PromptSpecBuilder{
		prompt: &PromptSpec{
			Name:        name,
			Description: description,
			Arguments:   []PromptArgSpec{},
			Variables:   make(map[string]string),
		},
	}
}

// AddArgument adds an argument to the prompt
func (b *PromptSpecBuilder) AddArgument(name, description string, required bool) *PromptSpecBuilder {
	b.prompt.Arguments = append(b.prompt.Arguments, PromptArgSpec{
		Name:        name,
		Description: description,
		Required:    required,
	})
	return b
}

// SetTemplate sets the template content
func (b *PromptSpecBuilder) SetTemplate(template string) *PromptSpecBuilder {
	b.prompt.Template = template
	return b
}

// SetTemplateRef sets a reference to an external template file
func (b *PromptSpecBuilder) SetTemplateRef(ref string) *PromptSpecBuilder {
	b.prompt.TemplateRef = ref
	return b
}

// AddVariable adds a template variable
func (b *PromptSpecBuilder) AddVariable(name, value string) *PromptSpecBuilder {
	if b.prompt.Variables == nil {
		b.prompt.Variables = make(map[string]string)
	}
	b.prompt.Variables[name] = value
	return b
}

// Build returns the built prompt spec
func (b *PromptSpecBuilder) Build() PromptSpec {
	return *b.prompt
}

// ResourceSpecBuilder provides a fluent API for building resource specifications
type ResourceSpecBuilder struct {
	resource *ResourceSpec
}

// NewResourceSpecBuilder creates a new resource spec builder
func NewResourceSpecBuilder(uri, name, description string) *ResourceSpecBuilder {
	return &ResourceSpecBuilder{
		resource: &ResourceSpec{
			URI:         uri,
			Name:        name,
			Description: description,
			Tags:        []string{},
			Metadata:    make(map[string]string),
		},
	}
}

// SetMimeType sets the MIME type
func (b *ResourceSpecBuilder) SetMimeType(mimeType string) *ResourceSpecBuilder {
	b.resource.MimeType = mimeType
	return b
}

// SetCategory sets the resource category
func (b *ResourceSpecBuilder) SetCategory(category string) *ResourceSpecBuilder {
	b.resource.Category = category
	return b
}

// SetPriority sets the resource priority
func (b *ResourceSpecBuilder) SetPriority(priority string) *ResourceSpecBuilder {
	b.resource.Priority = priority
	return b
}

// AddTag adds a tag
func (b *ResourceSpecBuilder) AddTag(tag string) *ResourceSpecBuilder {
	b.resource.Tags = append(b.resource.Tags, tag)
	return b
}

// AddTags adds multiple tags
func (b *ResourceSpecBuilder) AddTags(tags ...string) *ResourceSpecBuilder {
	b.resource.Tags = append(b.resource.Tags, tags...)
	return b
}

// AddMetadata adds a metadata key-value pair
func (b *ResourceSpecBuilder) AddMetadata(key, value string) *ResourceSpecBuilder {
	if b.resource.Metadata == nil {
		b.resource.Metadata = make(map[string]string)
	}
	b.resource.Metadata[key] = value
	return b
}

// Build returns the built resource spec
func (b *ResourceSpecBuilder) Build() ResourceSpec {
	return *b.resource
}

// ToolSpecBuilder provides a fluent API for building tool specifications
type ToolSpecBuilder struct {
	tool *ToolSpec
}

// NewToolSpecBuilder creates a new tool spec builder
func NewToolSpecBuilder(name, description string) *ToolSpecBuilder {
	return &ToolSpecBuilder{
		tool: &ToolSpec{
			Name:        name,
			Description: description,
			Properties:  make(map[string]PropertySpec),
			Required:    []string{},
		},
	}
}

// AddProperty adds a property to the tool by name
func (b *ToolSpecBuilder) AddProperty(name string, prop PropertySpec) *ToolSpecBuilder {
	b.tool.Properties[name] = prop
	return b
}

// AddStringProperty adds a string property
func (b *ToolSpecBuilder) AddStringProperty(name, description string) *ToolSpecBuilder {
	b.tool.Properties[name] = PropertySpec{
		Type:        "string",
		Description: description,
	}
	return b
}

// AddStringPropertyWithDefault adds a string property with default
func (b *ToolSpecBuilder) AddStringPropertyWithDefault(name, description, defaultValue string) *ToolSpecBuilder {
	b.tool.Properties[name] = PropertySpec{
		Type:        "string",
		Description: description,
		Default:     defaultValue,
	}
	return b
}

// AddNumberProperty adds a number property
func (b *ToolSpecBuilder) AddNumberProperty(name, description string) *ToolSpecBuilder {
	b.tool.Properties[name] = PropertySpec{
		Type:        "number",
		Description: description,
	}
	return b
}

// AddBooleanProperty adds a boolean property
func (b *ToolSpecBuilder) AddBooleanProperty(name, description string) *ToolSpecBuilder {
	b.tool.Properties[name] = PropertySpec{
		Type:        "boolean",
		Description: description,
	}
	return b
}

// MarkRequired marks a property as required
func (b *ToolSpecBuilder) MarkRequired(name string) *ToolSpecBuilder {
	b.tool.Required = append(b.tool.Required, name)
	return b
}

// SetHandler sets the handler function name
func (b *ToolSpecBuilder) SetHandler(handler string) *ToolSpecBuilder {
	b.tool.Handler = handler
	return b
}

// Build returns the built tool spec
func (b *ToolSpecBuilder) Build() ToolSpec {
	return *b.tool
}
