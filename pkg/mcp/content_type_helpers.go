package mcp

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/outputtypes"
)

// FormatToMIMEType maps format strings to MIME types.
// Prefer outputtypes registry for canonical formats; this map covers extended MCP-only values.
var FormatToMIMEType = map[string]string{
	"json":     "application/json; charset=utf-8",
	"jsonl":    "application/x-ndjson; charset=utf-8",
	"yaml":     "application/yaml; charset=utf-8",
	"table":    "text/plain; charset=utf-8", // Tables are typically plain text
	"text":     "text/plain; charset=utf-8",
	"markdown": "text/markdown",
	"html":     "text/html",
	"xml":      "application/xml",
	"csv":      "text/csv",
}

// MIMETypeToContentType maps MIME types to MCP Content types
// MCP Content types are: "text", "image", "resource"
// For most structured data, we use "text" but can specify MIME type
var MIMETypeToContentType = map[string]string{
	"application/json; charset=utf-8":     "text",
	"application/x-ndjson; charset=utf-8": "text",
	"application/yaml; charset=utf-8":     "text",
	"text/plain; charset=utf-8":           "text",
	"application/json":                    "text",
	"application/yaml":                    "text",
	"text/plain":                          "text",
	"text/markdown":                       "text",
	"text/html":                           "text",
	"application/xml":                     "text",
	"text/csv":                            "text",
	"image/png":                           "image",
	"image/jpeg":                          "image",
	"image/gif":                           "image",
}

// determineContentType determines the content type and MIME type for tool results
// Returns (contentType, mimeType) based on config and result format
func (s *Server) determineContentType(result any, formatHint string) (string, string) {
	// Determine format from hint, config, or result
	format := formatHint
	if format == emptyValue {
		// Try to get from config default
		if s.config != nil && s.config.MCPServer.Security.DefaultFormat != emptyValue {
			format = s.config.MCPServer.Security.DefaultFormat
		} else {
			// Default to json for structured data
			format = "json"
		}
	}

	// Normalize format
	format = strings.ToLower(strings.TrimSpace(format))

	// Map format to MIME type.
	var mimeType string
	if def, ok := outputtypes.Get(format); ok {
		mimeType = def.MIMEType
	} else if mapped, ok := FormatToMIMEType[format]; ok {
		mimeType = mapped
	} else {
		// Unknown format - default to application/json for structured data
		mimeType = "application/json; charset=utf-8"
	}

	// Check if format is allowed for this client
	if len(s.allowedFormats) > 0 {
		// Check if the format is in allowed list
		formatAllowed := false
		for _, allowed := range s.allowedFormats {
			if strings.EqualFold(allowed, format) {
				formatAllowed = true
				break
			}
		}
		if !formatAllowed {
			// Format not allowed - use first allowed format or default
			if len(s.allowedFormats) > 0 {
				allowedFormat := strings.ToLower(strings.TrimSpace(s.allowedFormats[0]))
				if def, ok := outputtypes.Get(allowedFormat); ok {
					mimeType = def.MIMEType
				} else if mappedMime, ok := FormatToMIMEType[allowedFormat]; ok {
					mimeType = mappedMime
				}
			}
		}
	}

	// Map MIME type to MCP Content type
	contentType, ok := MIMETypeToContentType[mimeType]
	if !ok {
		// Unknown MIME type - default to text
		contentType = "text"
	}

	return contentType, mimeType
}

// buildToolCallContent builds Content array for tool call results (BLI-958: MimeType on Content)
func (s *Server) buildToolCallContent(text string, formatHint string) []Content {
	contentType, mimeType := s.determineContentType(nil, formatHint)
	return []Content{
		{
			Type:     contentType,
			Text:     text,
			MimeType: mimeType,
		},
	}
}
