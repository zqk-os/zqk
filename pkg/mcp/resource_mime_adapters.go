package mcp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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

func (r *ResourceMIMEAdapterRegistry) resolveAdapter(mimeType string) ResourceMIMEAdapter {
	if adapter := r.GetAdapter(mimeType); adapter != nil {
		return adapter
	}
	return &PlainTextAdapter{}
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
	return r.resolveAdapter(mimeType).ExtractDescription(filePath)
}

// ExtractTitle extracts title using the appropriate adapter
func (r *ResourceMIMEAdapterRegistry) ExtractTitle(filePath, mimeType string) string {
	return r.resolveAdapter(mimeType).ExtractTitle(filePath)
}

// ExtractMetadata extracts metadata using the appropriate adapter
func (r *ResourceMIMEAdapterRegistry) ExtractMetadata(filePath, mimeType string) map[string]string {
	return r.resolveAdapter(mimeType).ExtractMetadata(filePath)
}

// readPrefixContent reads up to maxBytes from the given file path.
func readPrefixContent(filePath string, maxBytes int) string {
	file, err := fileutil.Open(filePath)
	if err != nil {
		return emptyValue
	}
	defer file.Close()

	buf := make([]byte, maxBytes)
	n, err := file.Read(buf)
	if err != nil && n == 0 {
		return emptyValue
	}
	return string(buf[:n])
}

func forEachLine(content string, maxLines int, fn func(line string) (string, bool)) string {
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines) && (maxLines <= 0 || i < maxLines); i++ {
		if res, stop := fn(strings.TrimSpace(lines[i])); stop {
			return res
		}
	}
	return emptyValue
}

// MarkdownAdapter extracts metadata from Markdown files
type MarkdownAdapter struct{}

func (a *MarkdownAdapter) ExtractDescription(filePath string) string {
	content := readPrefixContent(filePath, 4096)
	if content == emptyValue {
		return emptyValue
	}

	return forEachLine(content, 10, func(line string) (string, bool) {
		if strings.HasPrefix(line, "# ") {
			if title := strings.TrimPrefix(line, "# "); title != emptyValue {
				return fmt.Sprintf("Documentation: %s", title), true
			}
		} else if strings.HasPrefix(line, "## ") {
			if title := strings.TrimPrefix(line, "## "); title != emptyValue {
				return fmt.Sprintf("Documentation: %s", title), true
			}
		}
		return emptyValue, false
	})
}

func (a *MarkdownAdapter) ExtractTitle(filePath string) string {
	content := readPrefixContent(filePath, 4096)
	if content == emptyValue {
		return emptyValue
	}

	return forEachLine(content, 10, func(line string) (string, bool) {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# "), true
		}
		return emptyValue, false
	})
}

func (a *MarkdownAdapter) ExtractMetadata(filePath string) map[string]string {
	metadata := make(map[string]string)
	content := readPrefixContent(filePath, 4096)
	if content == emptyValue {
		return metadata
	}

	if strings.HasPrefix(content, "---\n") {
		if endIdx := strings.Index(content[4:], "\n---"); endIdx > 0 {
			frontmatter := content[4 : endIdx+4]
			var fm map[string]any
			if err := yaml.Unmarshal([]byte(frontmatter), &fm); err == nil {
				return extractStringMetadata(fm)
			}
		}
	}
	return metadata
}

// PlainTextAdapter extracts metadata from plain text files
type PlainTextAdapter struct{}

func (a *PlainTextAdapter) ExtractDescription(filePath string) string {
	content := readPrefixContent(filePath, 4096)
	if content == emptyValue {
		return emptyValue
	}

	return forEachLine(content, 0, func(line string) (string, bool) {
		if line != emptyValue && len(line) < 200 {
			return fmt.Sprintf("Documentation: %s", line), true
		}
		return emptyValue, false
	})
}

func (a *PlainTextAdapter) ExtractTitle(filePath string) string {
	name := filepath.Base(filePath)
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func (a *PlainTextAdapter) ExtractMetadata(filePath string) map[string]string {
	return make(map[string]string)
}

func decodeStructuredFile(filePath string, unmarshalFn func([]byte, any) error) (map[string]any, error) {
	bytes, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := unmarshalFn(bytes, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func extractStructuredDescription(data map[string]any) string {
	if len(data) == 0 {
		return emptyValue
	}
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

func extractStructuredTitle(data map[string]any) string {
	if len(data) == 0 {
		return emptyValue
	}
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

func extractStringMetadata(data map[string]any) map[string]string {
	metadata := make(map[string]string)
	for k, v := range data {
		if vStr, ok := v.(string); ok {
			metadata[k] = vStr
		}
	}
	return metadata
}

// JSONAdapter extracts metadata from JSON files
type JSONAdapter struct{}

func (a *JSONAdapter) ExtractDescription(filePath string) string {
	data, err := decodeStructuredFile(filePath, json.Unmarshal)
	if err != nil {
		return emptyValue
	}
	return extractStructuredDescription(data)
}

func (a *JSONAdapter) ExtractTitle(filePath string) string {
	data, err := decodeStructuredFile(filePath, json.Unmarshal)
	if err != nil {
		return emptyValue
	}
	return extractStructuredTitle(data)
}

func (a *JSONAdapter) ExtractMetadata(filePath string) map[string]string {
	data, err := decodeStructuredFile(filePath, json.Unmarshal)
	if err != nil {
		return make(map[string]string)
	}
	return extractStringMetadata(data)
}

// YAMLAdapter extracts metadata from YAML files
type YAMLAdapter struct{}

func (a *YAMLAdapter) ExtractDescription(filePath string) string {
	data, err := decodeStructuredFile(filePath, yaml.Unmarshal)
	if err != nil {
		return emptyValue
	}
	return extractStructuredDescription(data)
}

func (a *YAMLAdapter) ExtractTitle(filePath string) string {
	data, err := decodeStructuredFile(filePath, yaml.Unmarshal)
	if err != nil {
		return emptyValue
	}
	return extractStructuredTitle(data)
}

func (a *YAMLAdapter) ExtractMetadata(filePath string) map[string]string {
	data, err := decodeStructuredFile(filePath, yaml.Unmarshal)
	if err != nil {
		return make(map[string]string)
	}
	return extractStringMetadata(data)
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
