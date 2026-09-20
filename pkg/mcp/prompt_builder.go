package mcp

// PromptBuilder provides a fluent API for building MCP prompt definitions
// This eliminates handcoded structures and makes prompt definitions more maintainable
type PromptBuilder struct {
	name        string
	description string
	arguments   []PromptArgument
}

// NewPromptBuilder creates a new prompt builder
func NewPromptBuilder(name, description string) *PromptBuilder {
	return &PromptBuilder{
		name:        name,
		description: description,
		arguments:   []PromptArgument{},
	}
}

// AddArgument adds an argument to the prompt
func (b *PromptBuilder) AddArgument(name, description string) *PromptBuilder {
	b.arguments = append(b.arguments, PromptArgument{
		Name:        name,
		Description: description,
		Required:    promptArgOptional,
	})
	return b
}

// AddRequiredArgument adds a required argument to the prompt
func (b *PromptBuilder) AddRequiredArgument(name, description string) *PromptBuilder {
	b.arguments = append(b.arguments, PromptArgument{
		Name:        name,
		Description: description,
		Required:    promptArgRequired,
	})
	return b
}

// Build returns the complete Prompt
func (b *PromptBuilder) Build() Prompt {
	return Prompt{
		Name:        b.name,
		Description: b.description,
		Arguments:   b.arguments,
	}
}

// Register registers the prompt with the server
func (b *PromptBuilder) Register(server *Server) {
	server.RegisterPrompt(b.name, b.description, b.arguments)
}
