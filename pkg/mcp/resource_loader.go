package mcp

import (
	"os"
	"path/filepath"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
)

// ResourceLoadResult represents the result of loading a resource from a URI
type ResourceLoadResult struct {
	Content  []byte
	MimeType string
	FilePath string
	Scheme   string
}

// ResourceLoader provides shared functionality for loading resources from URIs
// This can be used by MCP handlers, CLI tools, and other components
type ResourceLoader struct {
	// Project root for resolving relative paths
	projectRoot string
	// URI scheme resolver for validating and resolving schemes
	schemeResolver *ResourceURISchemeResolver
	// MIME type adapter registry for detecting content types
	mimeRegistry *ResourceMIMEAdapterRegistry
}

// NewResourceLoader creates a new resource loader
func NewResourceLoader(projectRoot string, schemeResolver *ResourceURISchemeResolver, mimeRegistry *ResourceMIMEAdapterRegistry) *ResourceLoader {
	return &ResourceLoader{
		projectRoot:    projectRoot,
		schemeResolver: schemeResolver,
		mimeRegistry:   mimeRegistry,
	}
}

// NewResourceLoaderFromContext creates a resource loader from CLI initialization context
// This is a convenience method for components that have access to initCtx
func NewResourceLoaderFromContext(initCtx *pkgctx.CliInitializationContext, schemeResolver *ResourceURISchemeResolver, mimeRegistry *ResourceMIMEAdapterRegistry) *ResourceLoader {
	projectRoot := emptyValue
	if initCtx != nil {
		projectRoot = initCtx.GetProjectRoot()
	}
	return NewResourceLoader(projectRoot, schemeResolver, mimeRegistry)
}

// LoadResource loads a resource from a URI and returns the content, MIME type, and metadata
// Supports various URI schemes (file://, docs://, internal://, etc.)
// Returns an error if the resource cannot be loaded
func (rl *ResourceLoader) LoadResource(uri string) (*ResourceLoadResult, error) {
	// Parse and validate URI scheme
	scheme, filePath, err := rl.parseAndValidateURI(uri)
	if err != nil {
		return nil, err
	}

	// Resolve file path (handle relative paths)
	resolvedPath := rl.resolveFilePath(filePath)

	// Read file content
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, errfmt.Errorf("failed to read resource from %s: %w", resolvedPath, err)
	}

	// Detect MIME type
	mimeType := rl.detectMIMEType(resolvedPath)

	return &ResourceLoadResult{
		Content:  content,
		MimeType: mimeType,
		FilePath: resolvedPath,
		Scheme:   scheme,
	}, nil
}

// LoadResourceWithMimeType loads a resource with an explicit MIME type override
// If mimeTypeOverride is empty, MIME type will be detected automatically
func (rl *ResourceLoader) LoadResourceWithMimeType(uri string, mimeTypeOverride string) (*ResourceLoadResult, error) {
	result, err := rl.LoadResource(uri)
	if err != nil {
		return nil, err
	}

	// Override MIME type if provided
	if mimeTypeOverride != emptyValue {
		result.MimeType = mimeTypeOverride
	}

	return result, nil
}

// parseAndValidateURI parses a URI and validates its scheme
// Returns scheme, file path, and error
func (rl *ResourceLoader) parseAndValidateURI(uri string) (string, string, error) {
	// Parse URI scheme - must have a valid scheme
	idx := strings.Index(uri, "://")
	if idx <= 0 {
		// No scheme found - fail hard: this indicates a configuration or registration error
		return "", "", errfmt.Errorf("MCP server configuration error: resource URI '%s' has no scheme. All resource URIs must include a scheme (e.g., file://, docs://, internal://). This indicates a problem with resource registration. Use resourceURISchemeResolver.ResolveURI() to determine appropriate scheme when registering resources", uri)
	}

	scheme := uri[:idx]
	filePath := uri[idx+3:]

	if scheme == emptyValue {
		// Empty scheme after parsing - fail hard
		return "", "", errfmt.Errorf("MCP server configuration error: resource URI '%s' has empty scheme after parsing. This indicates a malformed URI or configuration issue", uri)
	}

	// Validate scheme against supported schemes
	supportedSchemes := rl.getSupportedSchemes()
	if !supportedSchemes[scheme] {
		// Unsupported scheme - fail hard: this indicates a configuration mismatch
		var supportedList []string
		for s := range supportedSchemes {
			supportedList = append(supportedList, s)
		}
		return "", "", errfmt.Errorf("MCP server configuration error: resource URI '%s' uses unsupported scheme '%s'. Supported schemes: %v. This indicates a mismatch between resource registration and URI scheme configuration. Ensure all registered resources use schemes defined in resourceURISchemeResolver rules or add the scheme to the resolver configuration", uri, scheme, supportedList)
	}

	return scheme, filePath, nil
}

// getSupportedSchemes returns a map of supported URI schemes
func (rl *ResourceLoader) getSupportedSchemes() map[string]bool {
	supportedSchemes := map[string]bool{
		"file":     true,
		"docs":     true,
		"internal": true,
		"schema":   true, // Handled separately in handlers
	}

	// Add schemes from resolver if available (this is the source of truth for configured schemes)
	if rl.schemeResolver != nil {
		for _, rule := range rl.schemeResolver.GetRules() {
			// Extract scheme from rule (remove :// suffix if present)
			ruleScheme := strings.TrimSuffix(rule.Scheme, "://")
			if ruleScheme != emptyValue {
				supportedSchemes[ruleScheme] = true
			}
		}
	}

	return supportedSchemes
}

// resolveFilePath resolves a file path, handling relative paths relative to project root
func (rl *ResourceLoader) resolveFilePath(filePath string) string {
	// If path is absolute, return as-is
	if strings.HasPrefix(filePath, "/") {
		return filePath
	}

	// If path is relative and we have a project root, resolve it
	if rl.projectRoot != emptyValue {
		return filepath.Join(rl.projectRoot, filePath)
	}

	// No project root, return path as-is (may be relative to current working directory)
	return filePath
}

// detectMIMEType detects the MIME type of a file
// Uses file extension detection (DetectMIMEType function)
func (rl *ResourceLoader) detectMIMEType(filePath string) string {
	// Use standard MIME type detection based on file extension
	mimeType := DetectMIMEType(filePath)
	if mimeType == "application/octet-stream" {
		// Fallback to text/plain for unknown types
		mimeType = "text/plain"
	}

	return mimeType
}

// LoadResourceText loads a resource and returns its content as a string
// Convenience method for text-based resources
func (rl *ResourceLoader) LoadResourceText(uri string) (string, error) {
	result, err := rl.LoadResource(uri)
	if err != nil {
		return "", err
	}
	return string(result.Content), nil
}

// LoadResourceTextWithMimeType loads a resource with MIME type override and returns text
func (rl *ResourceLoader) LoadResourceTextWithMimeType(uri string, mimeTypeOverride string) (string, string, error) {
	result, err := rl.LoadResourceWithMimeType(uri, mimeTypeOverride)
	if err != nil {
		return "", "", err
	}
	return string(result.Content), result.MimeType, nil
}
