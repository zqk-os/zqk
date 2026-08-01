package mcp

import "maps"

// ResourceBuilder provides a fluent API for building MCP resource definitions
// This eliminates handcoded structures and makes resource definitions more maintainable
type ResourceBuilder struct {
	uri         string
	name        string
	description string
	mimeType    string
	category    string
	priority    string
	tags        []string
	metadata    map[string]string
}

// NewResourceBuilder creates a new resource builder
func NewResourceBuilder(uri, name, description, mimeType string) *ResourceBuilder {
	return &ResourceBuilder{
		uri:         uri,
		name:        name,
		description: description,
		mimeType:    mimeType,
		category:    resourceCategoryUnset,
		priority:    resourcePriorityUnset,
		tags:        []string{},
		metadata:    make(map[string]string),
	}
}

// WithCategory sets the resource category
func (b *ResourceBuilder) WithCategory(category string) *ResourceBuilder {
	b.category = category
	return b
}

// WithPriority sets the resource priority
func (b *ResourceBuilder) WithPriority(priority string) *ResourceBuilder {
	b.priority = priority
	return b
}

// AddTag adds a tag to the resource
func (b *ResourceBuilder) AddTag(tag string) *ResourceBuilder {
	b.tags = append(b.tags, tag)
	return b
}

// AddTags adds multiple tags to the resource
func (b *ResourceBuilder) AddTags(tags ...string) *ResourceBuilder {
	b.tags = append(b.tags, tags...)
	return b
}

// WithMetadata sets a metadata key-value pair
func (b *ResourceBuilder) WithMetadata(key, value string) *ResourceBuilder {
	if b.metadata == nil {
		b.metadata = make(map[string]string)
	}
	b.metadata[key] = value
	return b
}

// WithMetadataMap sets multiple metadata key-value pairs
func (b *ResourceBuilder) WithMetadataMap(metadata map[string]string) *ResourceBuilder {
	if b.metadata == nil {
		b.metadata = make(map[string]string)
	}
	maps.Copy(b.metadata, metadata)
	return b
}

// Build returns the complete Resource
func (b *ResourceBuilder) Build() Resource {
	return Resource{
		URI:         b.uri,
		Name:        b.name,
		Description: b.description,
		MimeType:    b.mimeType,
		Category:    b.category,
		Priority:    b.priority,
		Tags:        b.tags,
		Metadata:    b.metadata,
	}
}

// Register registers the resource with the server
func (b *ResourceBuilder) Register(server *Server) {
	if b.category != resourceCategoryUnset || b.priority != resourcePriorityUnset || len(b.tags) > 0 || len(b.metadata) > 0 {
		server.RegisterResourceWithMetadata(
			b.uri,
			b.name,
			b.description,
			b.mimeType,
			b.category,
			b.priority,
			b.tags,
			b.metadata,
		)
	} else {
		server.RegisterResource(b.uri, b.name, b.description, b.mimeType)
	}
}
