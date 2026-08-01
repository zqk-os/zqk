package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/appledouble"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
)

// getCriticalResourcesFromSpec loads critical resources from MCP spec
// Uses spec-driven builder pattern: attempts to load from storage (mcp_spec objects) first,
// then tries spec files, fails hard if not found
func getCriticalResourcesFromSpec(server *Server) ([]ResourceSpec, error) {
	// Try to load from storage first (formal system objects)
	if server.storageProvider != nil {
		loader := NewStorageMCPSpecLoader(server.storageProvider)
		ctx := pkgctx.NewSystemContext()
		secCtx, _ := server.secCtx.(*pkgctx.SecurityContext)
		if secCtx == nil {
			secCtx = pkgctx.NewSystemSecurityContext()
		}

		// Load specs from storage (discover mcp_spec objects)
		specs, err := loader.LoadMCPSpecsWithFilter(ctx, secCtx, "critical_resources")
		if err != nil {
			return nil, errfmt.Newf("storage provider configured but failed to load critical_resources spec from storage").Wrap(err)
		}
		if len(specs) > 0 {
			// Return resources from first matching spec
			return specs[0].Resources, nil
		}
		// Storage provider configured but no specs found - fail hard
		return nil, errfmt.Errorf("storage provider configured but no critical_resources spec found in storage. Ensure mcp_spec objects with name 'critical_resources' are available in storage or provide a spec file")
	}

	// Try to load from spec file (externalized configuration)
	specPaths := []string{
		".zqk/mcp_specs/critical_resources.yaml",
		filepath.Join(datacell.ProcessPrimaryDir("."), "mcp_specs", "critical_resources.yaml"),
		"mcp_specs/critical_resources.yaml",
	}

	var lastErr error
	var checkedPaths []string
	for _, specPath := range specPaths {
		checkedPaths = append(checkedPaths, specPath)
		if _, err := os.Stat(specPath); err == nil {
			// Spec file exists, try to load it
			loader := NewMCPSpecLoader()
			specs, err := loader.LoadSpecs(specPath)
			if err == nil && len(specs) > 0 {
				// Return resources from first spec
				return specs[0].Resources, nil
			} else if err != nil {
				lastErr = errfmt.Errorf("failed to load spec from %s: %w", specPath, err)
			}
		}
	}

	// Fail hard if no spec was loaded
	var errMsg string
	if lastErr != nil {
		errMsg = fmt.Sprintf("critical resources spec file found but failed to load: %v. ", lastErr)
	} else {
		errMsg = "critical resources spec file not found in any of the expected locations. "
	}
	errMsg += fmt.Sprintf("Checked paths: %v. ", checkedPaths)
	errMsg += "Please create a critical_resources.yaml spec file in one of these locations or ensure mcp_spec objects are available in storage."
	return nil, errfmt.Errorf("MCP server initialization failed: %s", errMsg)
}

// getCriticalResourcePaths returns the file paths (relative to project root) of critical resources
// Used to exclude them from additional discovery
// Retrieves paths from spec by extracting file paths from resource URIs
func getCriticalResourcePaths(server *Server) []string {
	// Load critical resources from spec
	resourceSpecs, err := getCriticalResourcesFromSpec(server)
	if err != nil {
		// If spec loading fails, return empty list (discovery will proceed without exclusions)
		// This allows discovery to work even if critical resources spec is not available
		return []string{}
	}

	// Extract file paths from URIs (for file:// URIs)
	var paths []string
	for _, resourceSpec := range resourceSpecs {
		if strings.HasPrefix(resourceSpec.URI, "file://") {
			// Extract relative path from file:// URI
			relPath := strings.TrimPrefix(resourceSpec.URI, "file://")
			paths = append(paths, relPath)
		}
	}

	return paths
}

// RegisterCriticalResources registers critical documentation as MCP resources
// These resources provide agents with access to workflows, lifecycles, and system health information
// Resources are loaded from spec and registered using URIs and schemes from the spec
func RegisterCriticalResources(server *Server) {
	// Load critical resources from spec
	resourceSpecs, err := getCriticalResourcesFromSpec(server)
	if err != nil {
		// Log error but don't panic - allow server to start without critical resources
		// This makes the server more resilient and allows it to function even if spec is missing
		// Always log to both trace (if enabled) and system logger for visibility
		server.traceLogf("[MCP_WARN] Failed to load critical resources spec: %v. Server will continue without critical resources.", err)
		// Also log to system logger (writes to log files) for better visibility
		logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
		logging.Fluent(logger).Warn("Failed to load critical resources spec").
			WithError(err).
			EmitComponent("mcp_server").
			Log()
		return // Exit gracefully - server can function without critical resources
	}

	// Get project root for file existence checks (if needed)
	projectRoot := emptyValue
	if server.initCtx != nil {
		projectRoot = server.initCtx.GetProjectRoot()
	}

	// Register each resource from spec
	// Use URI and scheme directly from spec - don't extract or reconstruct
	for _, resourceSpec := range resourceSpecs {
		// For file:// URIs, optionally check if file exists (allows for optional resources)
		// But always use the URI from spec as-is
		if strings.HasPrefix(resourceSpec.URI, "file://") {
			// Extract file path from URI for existence check
			relPath := strings.TrimPrefix(resourceSpec.URI, "file://")

			// Build absolute path for file existence check
			var filePath string
			if projectRoot != emptyValue {
				filePath = filepath.Join(projectRoot, relPath)
			} else {
				filePath = relPath
			}

			// Check if file exists (skip if not found - allows for optional resources)
			if _, err := os.Stat(filePath); err != nil {
				continue // Skip missing files
			}
		}

		// Register resource using URI and all metadata from spec
		// Use ResourceBuilder to handle optional fields
		builder := NewResourceBuilder(resourceSpec.URI, resourceSpec.Name, resourceSpec.Description, resourceSpec.MimeType)
		if resourceSpec.Category != emptyValue {
			builder = builder.WithCategory(resourceSpec.Category)
		}
		if resourceSpec.Priority != emptyValue {
			builder = builder.WithPriority(resourceSpec.Priority)
		}
		if len(resourceSpec.Tags) > 0 {
			builder = builder.AddTags(resourceSpec.Tags...)
		}
		if len(resourceSpec.Metadata) > 0 {
			builder = builder.WithMetadataMap(resourceSpec.Metadata)
		}
		builder.Register(server)
	}
}

// DiscoverAdditionalResources discovers additional files in docs/architecture/architecture
// and registers them as resources dynamically, excluding files already registered as critical resources
// Supports multiple file types based on MIME type adapters
func DiscoverAdditionalResources(server *Server) {
	// Get project root from server's init context
	projectRoot := emptyValue
	if server.initCtx != nil {
		projectRoot = server.initCtx.GetProjectRoot()
	}

	if projectRoot == emptyValue {
		return // Can't discover without project root
	}

	// Get list of critical resource paths to exclude (from spec)
	criticalPaths := getCriticalResourcePaths(server)
	criticalPathSet := make(map[string]bool)
	for _, path := range criticalPaths {
		// Normalize path separators for comparison
		normalized := filepath.ToSlash(path)
		criticalPathSet[normalized] = true
	}

	// Discover markdown files in docs/architecture/architecture
	archDir := filepath.Join(projectRoot, paths.ProcessArchitectureDir)
	if _, err := os.Stat(archDir); os.IsNotExist(err) {
		return // Directory doesn't exist
	}

	// Get or create MIME adapter registry
	var mimeRegistry *ResourceMIMEAdapterRegistry
	if server.mimeAdapterRegistry != nil {
		mimeRegistry = server.mimeAdapterRegistry
	} else {
		mimeRegistry = NewResourceMIMEAdapterRegistry()
		server.mimeAdapterRegistry = mimeRegistry
	}

	// Walk the architecture directory and register discovered resources
	err := filepath.Walk(archDir, func(filePath string, info os.FileInfo, walkErr error) error {
		// Handle walk errors gracefully - skip problematic entries
		if walkErr != nil {
			return nil
		}

		// Process only regular files (skip directories)
		if !shouldProcessFile(info) {
			return nil
		}

		if appledouble.SkipPathInTreeWalk(filePath) {
			return nil
		}

		// Register the file as a resource if it meets all criteria
		registerDiscoveredResource(server, filePath, projectRoot, criticalPathSet, mimeRegistry)
		return nil
	})

	if err != nil {
		// Log error but don't fail - discovery is best-effort
		return
	}
}

// shouldProcessFile determines if a file should be processed for resource discovery
func shouldProcessFile(info os.FileInfo) bool {
	return !info.IsDir()
}

// registerDiscoveredResource registers a discovered file as an MCP resource
// Skips files that are critical resources, already registered, or binary/unknown types
func registerDiscoveredResource(server *Server, filePath, projectRoot string, criticalPathSet map[string]bool, mimeRegistry *ResourceMIMEAdapterRegistry) {
	// Detect MIME type and skip binary/unknown types
	mimeType := DetectMIMEType(filePath)
	if !isProcessableMimeType(mimeType) {
		return
	}

	// Calculate and normalize relative path from project root
	normalizedPath, ok := normalizeResourcePath(filePath, projectRoot)
	if !ok {
		return // Skip if path normalization fails
	}

	// Skip if this is a critical resource (already registered from spec)
	if criticalPathSet[normalizedPath] {
		return
	}

	// Resolve URI and category using configurable scheme resolver
	uri, category := resolveResourceURI(server, normalizedPath)

	// Skip if already registered (avoid duplicates)
	if isResourceAlreadyRegistered(server, uri) {
		return
	}

	// Extract resource metadata
	name := generateResourceName(filePath)
	description := extractDescriptionForDiscovery(filePath, mimeType, mimeRegistry)
	metadata := mimeRegistry.ExtractMetadata(filePath, mimeType)

	// Build and register the resource
	buildAndRegisterResource(server, uri, name, description, mimeType, category, metadata)
}

// isProcessableMimeType checks if a MIME type should be processed
// Only processes text-based content, skips binary/unknown types
func isProcessableMimeType(mimeType string) bool {
	return mimeType != "application/octet-stream"
}

// normalizeResourcePath calculates the relative path from project root and normalizes it
// Returns the normalized path and true if successful, empty string and false otherwise
func normalizeResourcePath(filePath, projectRoot string) (string, bool) {
	relPath, err := filepath.Rel(projectRoot, filePath)
	if err != nil {
		return "", false
	}
	return filepath.ToSlash(relPath), true
}

// resolveResourceURI resolves the URI and category for a resource path
// Uses configurable scheme resolver if available, otherwise defaults to file:// scheme
func resolveResourceURI(server *Server, normalizedPath string) (uri, category string) {
	if server.resourceURISchemeResolver != nil {
		return server.resourceURISchemeResolver.ResolveURI(normalizedPath)
	}
	// Fallback to default file:// scheme
	return fmt.Sprintf("file://%s", normalizedPath), "documentation"
}

// isResourceAlreadyRegistered checks if a resource URI is already registered
func isResourceAlreadyRegistered(server *Server, uri string) bool {
	_, exists := server.resources[uri]
	return exists
}

// generateResourceName generates a resource name from a file path
// Converts filename (without extension) to lowercase with underscores
func generateResourceName(filePath string) string {
	ext := filepath.Ext(filePath)
	name := strings.TrimSuffix(filepath.Base(filePath), ext)
	name = strings.ToLower(name)
	return strings.ReplaceAll(name, "-", "_")
}

// extractDescriptionForDiscovery extracts description using MIME type adapter for resource discovery
// Falls back to filename-based description if extraction fails
func extractDescriptionForDiscovery(filePath, mimeType string, mimeRegistry *ResourceMIMEAdapterRegistry) string {
	description := mimeRegistry.ExtractDescription(filePath, mimeType)
	if description == emptyValue {
		description = fmt.Sprintf("Documentation: %s", filepath.Base(filePath))
	}
	return description
}

// buildAndRegisterResource builds and registers a resource with all metadata
func buildAndRegisterResource(server *Server, uri, name, description, mimeType, category string, metadata map[string]string) {
	builder := NewResourceBuilder(uri, name, description, mimeType)
	if category != emptyValue {
		builder = builder.WithCategory(category)
	}
	if len(metadata) > 0 {
		builder = builder.WithMetadataMap(metadata)
	}
	builder.Register(server)
}
