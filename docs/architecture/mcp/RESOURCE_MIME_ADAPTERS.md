# Resource MIME Type Adapters

## Overview

The resource MIME type adapter system allows the MCP server to extract metadata (descriptions, titles, etc.) from resources based on their content type, not just markdown. This enables the server to function like a proper HTTP server that handles different content types appropriately.

## Architecture

### Components

1. **`ResourceMIMEAdapter`**: Interface for extracting metadata from different content types
2. **`ResourceMIMEAdapterRegistry`**: Manages adapters for different MIME types
3. **Built-in Adapters**: Markdown, PlainText, JSON, YAML

### Supported MIME Types

- `text/markdown` - Markdown files (.md, .markdown)
- `text/plain` - Plain text files (.txt)
- `application/json` - JSON files (.json)
- `application/yaml` - YAML files (.yaml, .yml)
- `text/html` - HTML files (.html, .htm)
- `application/xml` - XML files (.xml)
- `text/csv` - CSV files (.csv)

## Usage

### Automatic MIME Type Detection

The system automatically detects MIME types from file extensions:

```go
mimeType := DetectMIMEType("document.md")  // Returns "text/markdown"
mimeType := DetectMIMEType("config.json")  // Returns "application/json"
mimeType := DetectMIMEType("data.yaml")    // Returns "application/yaml"
```

### Extracting Metadata

```go
registry := NewResourceMIMEAdapterRegistry()

// Extract description
description := registry.ExtractDescription("document.md", "text/markdown")

// Extract title
title := registry.ExtractTitle("config.json", "application/json")

// Extract all metadata
metadata := registry.ExtractMetadata("data.yaml", "application/yaml")
```

## Adapter Implementations

### MarkdownAdapter

Extracts metadata from Markdown files:
- **Description**: First `# Title` or `## Title` heading
- **Title**: First `# Title` heading
- **Metadata**: YAML frontmatter if present

### PlainTextAdapter

Extracts metadata from plain text files:
- **Description**: First non-empty line (up to 200 chars)
- **Title**: Filename without extension
- **Metadata**: Empty (no structured metadata)

### JSONAdapter

Extracts metadata from JSON files:
- **Description**: `description` or `Description` field, or `title`/`Title` field
- **Title**: `title`, `Title`, `name`, or `Name` field
- **Metadata**: All string fields in the JSON object

### YAMLAdapter

Extracts metadata from YAML files:
- **Description**: `description` or `Description` field, or `title`/`Title` field
- **Title**: `title`, `Title`, `name`, or `Name` field
- **Metadata**: All string fields in the YAML document

## Custom Adapters

You can register custom adapters for additional MIME types:

```go
type CustomAdapter struct{}

func (a *CustomAdapter) ExtractDescription(filePath string) string {
	// Custom extraction logic
	return "Custom description"
}

func (a *CustomAdapter) ExtractTitle(filePath string) string {
	return "Custom title"
}

func (a *CustomAdapter) ExtractMetadata(filePath string) map[string]string {
	return map[string]string{"key": "value"}
}

// Register the adapter
registry := NewResourceMIMEAdapterRegistry()
registry.RegisterAdapter("application/custom", &CustomAdapter{})
```

## Integration

The MIME adapter system is integrated into:

1. **Resource Discovery** (`DiscoverAdditionalResources`):
   - Automatically detects MIME type from file extension
   - Uses appropriate adapter to extract description and metadata
   - Registers resources with extracted metadata

2. **Resource Retrieval** (`handleResourcesGet`):
   - Detects MIME type if not already set
   - Returns proper MIME type in response

## Benefits

1. **Content-Type Aware**: Handles different file types appropriately
2. **Extensible**: Easy to add support for new MIME types
3. **Metadata Extraction**: Automatically extracts structured metadata
4. **Server-Like Behavior**: Functions like a proper HTTP server
5. **Backward Compatible**: Falls back to plain text adapter for unknown types

## Future Enhancements

- Support for binary file types (images, PDFs, etc.)
- Content-Type detection from file content (magic bytes)
- Streaming support for large files
- Caching of extracted metadata
- Support for multi-part MIME types
