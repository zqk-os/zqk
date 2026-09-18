package mcp

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// getSchemaResourcesFromSpec loads schema resources from MCP spec
// Uses spec-driven builder pattern: attempts to load from storage (mcp_spec objects) first,
// then tries spec files, fails hard if not found
func getSchemaResourcesFromSpec(server *Server) ([]ResourceSpec, error) {
	selection, err := selectStoredMCPSpec(pkgctx.NewSystemContext(), server, "schema_resources")
	if err != nil {
		return nil, errfmt.Newf("persisted schema_resources spec is unusable").Wrap(err)
	}
	if selection.Spec != nil {
		return selection.Spec.Resources, nil
	}

	// Try to load from spec file (externalized configuration)
	specPaths := []string{
		".zqk/mcp_specs/schema_resources.yaml",
		filepath.Join(datacell.ProcessPrimaryDir("."), "mcp_specs", "schema_resources.yaml"),
		"mcp_specs/schema_resources.yaml",
	}

	var lastErr error
	var checkedPaths []string
	for _, specPath := range specPaths {
		checkedPaths = append(checkedPaths, specPath)
		if _, err := fileutil.Stat(specPath); err == nil {
			// Spec file exists, try to load it
			loader := NewMCPSpecLoader()
			specs, err := loader.LoadSpecs(specPath)
			if err == nil && len(specs) > 0 {
				logMCPSpecFallbackSelection(server, "schema_resources", specPath, selection.Reason)
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
		errMsg = fmt.Sprintf("schema resources spec file found but failed to load: %v. ", lastErr)
	} else {
		errMsg = "schema resources spec file not found in any of the expected locations. "
	}
	errMsg += fmt.Sprintf("Checked paths: %v. ", checkedPaths)
	errMsg += "Please create a schema_resources.yaml spec file in one of these locations or ensure mcp_spec objects are available in storage."
	return nil, errfmt.Errorf("MCP server initialization failed: %s", errMsg)
}

// RegisterSchemaResources registers JSON-LD schema resources for object schemas and CLI ontology
// Loads schema resources from spec, with fallback to defaults if spec is not available
func RegisterSchemaResources(server *Server) {
	// Register default schema handlers if not already registered from spec
	RegisterDefaultSchemaHandlers(server)

	// Try to load schema resources from spec
	resourceSpecs, err := getSchemaResourcesFromSpec(server)
	if err != nil {
		if errors.Is(err, ErrStoredMCPSpecUnusable) {
			logMCPSpecRuntimeError(server, "schema_resources", err)
			return
		}
		registerDefaultSchemaResources(server)
		return
	}

	// Register each resource from spec
	for _, resourceSpec := range resourceSpecs {
		// Only register schema:// URIs (filter out non-schema resources)
		if !strings.HasPrefix(resourceSpec.URI, "schema://") {
			continue
		}

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

	// Register individual object schemas for all object kinds
	// This allows them to appear in resources/list
	// Schemas are generated on-demand when requested
	if err := RegisterAllObjectSchemaResources(server); err != nil {
		// Log error but don't fail - schema registration is non-critical
		// Individual schemas can still be generated on-demand
	}
}

// registerDefaultSchemaResources registers default schema resources (backward compatibility)
// Used when schema_resources spec is not available
func registerDefaultSchemaResources(server *Server) {
	// Register schema registry (lists all available schemas)
	server.RegisterResourceWithMetadata(
		"schema://registry",
		"object_schema_registry",
		"Registry of all object schemas available in JSON-LD format",
		"application/ld+json",
		"schema",
		"critical",
		[]string{"schema", "ontology", "registry"},
		nil,
	)

	// Register CLI ontology
	server.RegisterResourceWithMetadata(
		"schema://cli",
		"cli_ontology",
		"Complete CLI command structure and organization in JSON-LD format. Includes command hierarchy, flags, permissions, and metrics.",
		"application/ld+json",
		"schema",
		"critical",
		[]string{"schema", "ontology", "cli", "commands"},
		nil,
	)

	// Register common fields resource
	server.RegisterResourceWithMetadata(
		"schema://common_fields",
		"common_fields_schema",
		"Fields common to all objects (inherited from base_object and auditable)",
		"application/ld+json",
		"schema",
		"reference",
		[]string{"schema", "common_fields"},
		nil,
	)
}

// RegisterObjectSchemaResource registers a schema resource for a specific object kind
func RegisterObjectSchemaResource(server *Server, kind string) {
	uri := fmt.Sprintf("schema://object/%s", kind)
	name := fmt.Sprintf("%s_schema", kind)
	description := fmt.Sprintf("JSON-LD schema for %s objects. Includes field definitions, types, requirements, traits, and relationships.", kind)

	server.RegisterResourceWithMetadata(
		uri,
		name,
		description,
		"application/ld+json",
		"schema",
		"reference",
		[]string{"schema", "object", kind},
		nil,
	)
}

// RegisterAllObjectSchemaResources registers schema resources for all object kinds
func RegisterAllObjectSchemaResources(server *Server) error {
	registry := objects.GetGlobalFieldRegistry()

	// Ensure fields are loaded
	if err := registry.LoadFields(); err != nil {
		return errfmt.Newf("failed to load fields").Wrap(err)
	}

	// Get all kinds
	kinds, err := registry.GetAllKinds()
	if err != nil {
		return errfmt.Newf("failed to get all kinds").Wrap(err)
	}

	// Register schema resource for each kind
	for _, kind := range kinds {
		RegisterObjectSchemaResource(server, kind)
	}

	return nil
}

// RegisterDefaultSchemaHandlers registers default schema handlers for backward compatibility
// These are used if no schema handlers are provided in the MCP spec
func RegisterDefaultSchemaHandlers(server *Server) {
	// Check if handlers are already registered (from spec)
	var hasHandlers bool
	_ = concurrency.RunInRLockWithLogger(
		&server.schemaHandlersMu, LockNameMcpServerCheckSchemaHandlers, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			hasHandlers = len(server.schemaHandlers) > 0
			return nil
		},
	)

	// Only register defaults if no handlers from spec
	if !hasHandlers {
		// Register default handlers
		server.RegisterSchemaHandler("schema://registry", func(s *Server, uri string) (any, error) {
			return s.handleSchemaRegistry()
		}, false)
		server.RegisterSchemaHandler("schema://cli", func(s *Server, uri string) (any, error) {
			return s.handleCLIOntology()
		}, false)
		server.RegisterSchemaHandler("schema://common_fields", func(s *Server, uri string) (any, error) {
			return s.handleCommonFieldsSchema()
		}, false)
		server.RegisterSchemaHandler("schema://object/", func(s *Server, uri string) (any, error) {
			kind := strings.TrimPrefix(uri, "schema://object/")
			return s.handleObjectSchema(kind)
		}, true) // Prefix match
	}
}
