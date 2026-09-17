package mcp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ResourceMIMEAdapter extracts metadata (description, title, etc.) from resources based on MIME type
type ResourceMIMEAdapter interface {
	// ExtractDescription extracts a description from the resource content
	ExtractDescription(filePath string) string
	// ExtractTitle extracts a title from the resource content
	ExtractTitle(filePath string) string
	// ExtractMetadata extracts additional metadata from the resource content
	ExtractMetadata(filePath string) map[string]string
}

// ResourceMIMEAdapterRegistry manages MIME type adapters
type ResourceMIMEAdapterRegistry struct {
	adapters map[string]ResourceMIMEAdapter
}

// NewResourceMIMEAdapterRegistry creates a new registry with default adapters
func NewResourceMIMEAdapterRegistry() *ResourceMIMEAdapterRegistry {
	registry := &ResourceMIMEAdapterRegistry{
		adapters: make(map[string]ResourceMIMEAdapter),
	}

	// Register default adapters
	registry.RegisterAdapter("text/markdown", &MarkdownAdapter{})
	registry.RegisterAdapter("text/x-markdown", &MarkdownAdapter{})
	registry.RegisterAdapter("text/plain", &PlainTextAdapter{})
	registry.RegisterAdapter("application/json", &JSONAdapter{})
	registry.RegisterAdapter("application/yaml", &YAMLAdapter{})
	registry.RegisterAdapter("text/yaml", &YAMLAdapter{})
	registry.RegisterAdapter("application/x-yaml", &YAMLAdapter{})

	return registry
}

// RegisterAdapter registers a MIME type adapter
func (r *ResourceMIMEAdapterRegistry) RegisterAdapter(mimeType string, adapter ResourceMIMEAdapter) {
	r.adapters[normalizeMIMEType(mimeType)] = adapter
}

// GetAdapter returns the adapter for a MIME type, or nil if not found
func (r *ResourceMIMEAdapterRegistry) GetAdapter(mimeType string) ResourceMIMEAdapter {
	return r.adapters[normalizeMIMEType(mimeType)]
}

func normalizeMIMEType(mimeType string) string {
	base := strings.ToLower(strings.TrimSpace(mimeType))
	if idx := strings.Index(base, ";"); idx >= 0 {
		base = strings.TrimSpace(base[:idx])
	}
	return base
}

// ExtractDescription extracts description using the appropriate adapter
func (r *ResourceMIMEAdapterRegistry) ExtractDescription(filePath, mimeType string) string {
	adapter := r.GetAdapter(mimeType)
	if adapter == nil {
		// Fallback to plain text adapter
		adapter = &PlainTextAdapter{}
	}
	return adapter.ExtractDescription(filePath)
}

// ExtractTitle extracts title using the appropriate adapter
func (r *ResourceMIMEAdapterRegistry) ExtractTitle(filePath, mimeType string) string {
	adapter := r.GetAdapter(mimeType)
	if adapter == nil {
		// Fallback to plain text adapter
		adapter = &PlainTextAdapter{}
	}
	return adapter.ExtractTitle(filePath)
}

// ExtractMetadata extracts metadata using the appropriate adapter
func (r *ResourceMIMEAdapterRegistry) ExtractMetadata(filePath, mimeType string) map[string]string {
	adapter := r.GetAdapter(mimeType)
	if adapter == nil {
		// Fallback to plain text adapter
		adapter = &PlainTextAdapter{}
	}
	return adapter.ExtractMetadata(filePath)
}

// MarkdownAdapter extracts metadata from Markdown files
type MarkdownAdapter struct{}

func (a *MarkdownAdapter) ExtractDescription(filePath string) string {
	file, err := fileutil.Open(filePath)
	if err != nil {
		return emptyValue
	}
	defer file.Close()

	// Read first 4KB
	buf := make([]byte, 4096)
	n, err := file.Read(buf)
	if err != nil && n == 0 {
		return emptyValue
	}

	content := string(buf[:n])
	lines := strings.Split(content, "\n")

	// Look for markdown title (# Title or ## Title) in first 10 lines
	for i := 0; i < len(lines) && i < 10; i++ {
		line := strings.TrimSpace(lines[i])
		// Check for markdown heading
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimPrefix(line, "# ")
			if title != emptyValue {
				return fmt.Sprintf("Documentation: %s", title)
			}
		} else if strings.HasPrefix(line, "## ") {
			title := strings.TrimPrefix(line, "## ")
			if title != emptyValue {
				return fmt.Sprintf("Documentation: %s", title)
			}
		}
	}

	return emptyValue
}

func (a *MarkdownAdapter) ExtractTitle(filePath string) string {
	file, err := fileutil.Open(filePath)
	if err != nil {
		return emptyValue
	}
	defer file.Close()

	// Read first 4KB
	buf := make([]byte, 4096)
	n, err := file.Read(buf)
	if err != nil && n == 0 {
		return emptyValue
	}

	content := string(buf[:n])
	lines := strings.Split(content, "\n")

	// Look for first markdown title
	for i := 0; i < len(lines) && i < 10; i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}

	return emptyValue
}

func (a *MarkdownAdapter) ExtractMetadata(filePath string) map[string]string {
	metadata := make(map[string]string)

	// Extract frontmatter if present
	file, err := fileutil.Open(filePath)
	if err != nil {
		return metadata
	}
	defer file.Close()

	buf := make([]byte, 4096)
	n, err := file.Read(buf)
	if err != nil {
		return metadata
	}

	content := string(buf[:n])

	// Check for YAML frontmatter
	if strings.HasPrefix(content, "---\n") {
		endIdx := strings.Index(content[4:], "\n---")
		if endIdx > 0 {
			frontmatter := content[4 : endIdx+4]
			// Parse YAML frontmatter
			var fm map[string]any
			if err := yaml.Unmarshal([]byte(frontmatter), &fm); err == nil {
				for k, v := range fm {
					if vStr, ok := v.(string); ok {
						metadata[k] = vStr
					}
				}
			}
		}
	}

	return metadata
}

// PlainTextAdapter extracts metadata from plain text files
type PlainTextAdapter struct{}

func (a *PlainTextAdapter) ExtractDescription(filePath string) string {
	file, err := fileutil.Open(filePath)
	if err != nil {
		return emptyValue
	}
	defer file.Close()

	// Read first 4KB
	buf := make([]byte, 4096)
	n, err := file.Read(buf)
	if err != nil && n == 0 {
		return emptyValue
	}

	content := string(buf[:n])
	lines := strings.Split(content, "\n")

	// Use first non-empty line as description
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != emptyValue && len(line) < 200 {
			return fmt.Sprintf("Documentation: %s", line)
		}
	}

	return emptyValue
}

func (a *PlainTextAdapter) ExtractTitle(filePath string) string {
	// Use filename without extension as title
	name := filepath.Base(filePath)
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func (a *PlainTextAdapter) ExtractMetadata(filePath string) map[string]string {
	return make(map[string]string)
}

// JSONAdapter extracts metadata from JSON files
type JSONAdapter struct{}

func (a *JSONAdapter) ExtractDescription(filePath string) string {
	file, err := fileutil.Open(filePath)
	if err != nil {
		return emptyValue
	}
	defer file.Close()

	// Read file content
	var data map[string]any
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil {
		return emptyValue
	}

	// Try common description fields
	if desc, ok := data[objects.FieldKeyDescription].(string); ok && desc != emptyValue {
		return desc
	}
	if desc, ok := data["Description"].(string); ok && desc != emptyValue {
		return desc
	}
	if title, ok := data[objects.FieldKeyTitle].(string); ok && title != emptyValue {
		return fmt.Sprintf("Documentation: %s", title)
	}
	if title, ok := data["Title"].(string); ok && title != emptyValue {
		return fmt.Sprintf("Documentation: %s", title)
	}

	return emptyValue
}

func (a *JSONAdapter) ExtractTitle(filePath string) string {
	file, err := fileutil.Open(filePath)
	if err != nil {
		return emptyValue
	}
	defer file.Close()

	var data map[string]any
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil {
		return emptyValue
	}

	// Try common title fields
	if title, ok := data[objects.FieldKeyTitle].(string); ok && title != emptyValue {
		return title
	}
	if title, ok := data["Title"].(string); ok && title != emptyValue {
		return title
	}
	if name, ok := data[objects.FieldKeyName].(string); ok && name != emptyValue {
		return name
	}
	if name, ok := data["Name"].(string); ok && name != emptyValue {
		return name
	}

	return emptyValue
}

func (a *JSONAdapter) ExtractMetadata(filePath string) map[string]string {
	metadata := make(map[string]string)

	file, err := fileutil.Open(filePath)
	if err != nil {
		return metadata
	}
	defer file.Close()

	var data map[string]any
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil {
		return metadata
	}

	// Extract string fields as metadata
	for k, v := range data {
		if vStr, ok := v.(string); ok {
			metadata[k] = vStr
		}
	}

	return metadata
}

// YAMLAdapter extracts metadata from YAML files
type YAMLAdapter struct{}

func (a *YAMLAdapter) ExtractDescription(filePath string) string {
	file, err := fileutil.Open(filePath)
	if err != nil {
		return emptyValue
	}
	defer file.Close()

	var data map[string]any
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil {
		return emptyValue
	}

	// Try common description fields
	if desc, ok := data[objects.FieldKeyDescription].(string); ok && desc != emptyValue {
		return desc
	}
	if desc, ok := data["Description"].(string); ok && desc != emptyValue {
		return desc
	}
	if title, ok := data[objects.FieldKeyTitle].(string); ok && title != emptyValue {
		return fmt.Sprintf("Documentation: %s", title)
	}
	if title, ok := data["Title"].(string); ok && title != emptyValue {
		return fmt.Sprintf("Documentation: %s", title)
	}

	return emptyValue
}

func (a *YAMLAdapter) ExtractTitle(filePath string) string {
	file, err := fileutil.Open(filePath)
	if err != nil {
		return emptyValue
	}
	defer file.Close()

	var data map[string]any
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil {
		return emptyValue
	}

	// Try common title fields
	if title, ok := data[objects.FieldKeyTitle].(string); ok && title != emptyValue {
		return title
	}
	if title, ok := data["Title"].(string); ok && title != emptyValue {
		return title
	}
	if name, ok := data[objects.FieldKeyName].(string); ok && name != emptyValue {
		return name
	}
	if name, ok := data["Name"].(string); ok && name != emptyValue {
		return name
	}

	return emptyValue
}

func (a *YAMLAdapter) ExtractMetadata(filePath string) map[string]string {
	metadata := make(map[string]string)

	file, err := fileutil.Open(filePath)
	if err != nil {
		return metadata
	}
	defer file.Close()

	var data map[string]any
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil {
		return metadata
	}

	// Extract string fields as metadata
	for k, v := range data {
		if vStr, ok := v.(string); ok {
			metadata[k] = vStr
		}
	}

	return metadata
}

// DetectMIMEType detects MIME type from file extension
func DetectMIMEType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))

	mimeTypes := map[string]string{
		".md":       "text/markdown",
		".markdown": "text/markdown",
		".txt":      "text/plain",
		".json":     "application/json; charset=utf-8",
		".jsonl":    "application/x-ndjson; charset=utf-8",
		".ndjson":   "application/x-ndjson; charset=utf-8",
		".yaml":     "application/yaml; charset=utf-8",
		".yml":      "application/yaml; charset=utf-8",
		".html":     "text/html",
		".htm":      "text/html",
		".xml":      "application/xml",
		".csv":      "text/csv",
		".tsv":      "text/tab-separated-values",
	}

	if mimeType, ok := mimeTypes[ext]; ok {
		return mimeType
	}

	return "application/octet-stream" // Default for unknown types
}
