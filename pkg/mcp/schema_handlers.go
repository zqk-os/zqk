package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// SchemaHandler is a function type for handling schema:// URI requests
type SchemaHandler func(server *Server, uri string) (any, error)

// RegisterSchemaHandler registers a schema handler for a URI pattern
func (s *Server) RegisterSchemaHandler(pattern string, handler SchemaHandler, isPrefix bool) {
	_ = concurrency.RunInLockWithLogger(
		&s.schemaHandlersMu, LockNameMcpServerRegisterSchemaHandler, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if s.schemaHandlers == nil {
				s.schemaHandlers = make(map[string]SchemaHandler)
			}
			s.schemaHandlers[pattern] = handler
			// Store prefix flag in metadata if needed (for now, we check prefix in handleSchemaResource)
			return nil
		},
	)
}

// handleSchemaResource handles schema:// URI requests and returns JSON-LD content
// Uses schema handlers loaded from spec, with fallback to default handlers
func (s *Server) handleSchemaResource(uri string) (any, error) {
	var handler SchemaHandler
	var found bool
	_ = concurrency.RunInRLock(&s.schemaHandlersMu, func() error {
		handler, found = s.schemaHandlers[uri]
		if !found {
			for pattern, h := range s.schemaHandlers {
				if strings.HasPrefix(uri, pattern) {
					handler = h
					found = true
					break
				}
			}
		}
		return nil
	})

	if found && handler != nil {
		return handler(s, uri)
	}

	// Fallback to default handlers if not found in spec
	return s.handleSchemaResourceDefault(uri)
}

// handleSchemaResourceDefault provides default schema handlers (backward compatibility)
func (s *Server) handleSchemaResourceDefault(uri string) (any, error) {
	switch {
	case uri == "schema://registry":
		return s.handleSchemaRegistry()
	case uri == "schema://cli":
		return s.handleCLIOntology()
	case uri == "schema://common_fields":
		return s.handleCommonFieldsSchema()
	case strings.HasPrefix(uri, "schema://object/"):
		kind := strings.TrimPrefix(uri, "schema://object/")
		return s.handleObjectSchema(kind)
	default:
		return nil, &JSONRPCError{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("unknown schema URI: %s", uri),
		}
	}
}

// handleSchemaRegistry returns the schema registry (list of all object kinds)
func (s *Server) handleSchemaRegistry() (any, error) {
	registry := objects.GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		return nil, errfmt.Newf("failed to load fields").Wrap(err)
	}

	kinds, err := registry.GetAllKinds()
	if err != nil {
		return nil, errfmt.Newf("failed to get all kinds").Wrap(err)
	}

	// Build registry structure
	registryData := map[string]any{
		"@context":        getJSONLDContext(),
		"@type":           "zqk:SchemaRegistry",
		"@id":             "zqk:schema:registry",
		"rdfs:label":      "Object Schema Registry",
		"rdfs:comment":    "Registry of all available object schemas",
		"zqk:version":     objects.DefaultSchemaVersion,
		"zqk:objectKinds": make([]map[string]any, 0, len(kinds)),
	}

	// Build object kinds list
	objectKinds := make([]map[string]any, 0, len(kinds))
	for _, kind := range kinds {
		kindFields, err := registry.GetFieldsForKind(kind)
		if err != nil {
			continue // Skip if can't get fields
		}

		objectKinds = append(objectKinds, map[string]any{
			"@id":                    fmt.Sprintf("zqk:%s", kind),
			"rdfs:label":             kind,
			"zqk:schemaUri":          fmt.Sprintf("schema://object/%s", kind),
			"zqk:fieldCount":         len(kindFields.AllFields),
			"zqk:requiredFieldCount": countRequiredFields(kindFields),
		})
	}
	registryData["zqk:objectKinds"] = objectKinds

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(registryData, "", "  ")
	if err != nil {
		return nil, errfmt.Newf("failed to marshal schema registry").Wrap(err)
	}

	// Infer response structure from registered resource metadata
	return s.buildSchemaResourceResponse("schema://registry", string(jsonData))
}

// handleCLIOntology returns the CLI command structure ontology
func (s *Server) handleCLIOntology() (any, error) {
	if s.rootCommand == nil {
		return nil, &JSONRPCError{
			Code:    ServerError,
			Message: "root command not available",
		}
	}

	// Type assert to *cobra.Command
	rootCmd, ok := s.rootCommand.(*cobra.Command)
	if !ok {
		return nil, &JSONRPCError{
			Code:    ServerError,
			Message: "root command is not a cobra command",
		}
	}

	// Convert command tree to JSON-LD (include metrics)
	ontology, err := ConvertCommandTreeToJSONLD(rootCmd, true)
	if err != nil {
		return nil, errfmt.Newf("failed to convert CLI to JSON-LD").Wrap(err)
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(ontology, "", "  ")
	if err != nil {
		return nil, errfmt.Newf("failed to marshal CLI ontology").Wrap(err)
	}

	// Infer response structure from registered resource metadata
	return s.buildSchemaResourceResponse("schema://cli", string(jsonData))
}

// handleCommonFieldsSchema returns the common fields schema
func (s *Server) handleCommonFieldsSchema() (any, error) {
	registry := objects.GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		return nil, errfmt.Newf("failed to load fields").Wrap(err)
	}

	commonFields, err := registry.GetCommonFields()
	if err != nil {
		return nil, errfmt.Newf("failed to get common fields").Wrap(err)
	}

	// Convert to JSON-LD
	fields := make([]JSONLDField, 0, len(commonFields))
	for i := range commonFields {
		jsonldField := convertFieldToJSONLD(&commonFields[i], "common")
		fields = append(fields, jsonldField)
	}

	schemaData := map[string]any{
		"@context":     getJSONLDContext(),
		"@type":        "zqk:CommonFieldsSchema",
		"@id":          "zqk:common_fields",
		"rdfs:label":   "Common Fields",
		"rdfs:comment": "Fields common to all objects (inherited from base_object and auditable)",
		"zqk:fields":   fields,
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(schemaData, "", "  ")
	if err != nil {
		return nil, errfmt.Newf("failed to marshal common fields schema").Wrap(err)
	}

	// Infer response structure from registered resource metadata
	return s.buildSchemaResourceResponse("schema://common_fields", string(jsonData))
}

// handleObjectSchema returns the schema for a specific object kind
func (s *Server) handleObjectSchema(kind string) (any, error) {
	registry := objects.GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		return nil, errfmt.Newf("failed to load fields").Wrap(err)
	}

	kindFields, err := registry.GetFieldsForKind(kind)
	if err != nil {
		return nil, &JSONRPCError{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("unknown object kind: %s", kind),
		}
	}

	// Convert to JSON-LD
	schema, err := ConvertKindFieldsToJSONLD(kindFields)
	if err != nil {
		return nil, errfmt.Newf("failed to convert to JSON-LD").Wrap(err)
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, errfmt.Newf("failed to marshal schema").Wrap(err)
	}

	// Infer response structure from registered resource metadata
	uri := fmt.Sprintf("schema://object/%s", kind)
	return s.buildSchemaResourceResponse(uri, string(jsonData))
}

// buildSchemaResourceResponse builds a resource response structure from registered resource metadata
// Infers URI and MimeType from the registered resource instead of hardcoding
func (s *Server) buildSchemaResourceResponse(uri string, text string) (any, error) {
	// Look up the resource to get its metadata (URI, MimeType)
	var resource Resource
	var exists bool
	_ = concurrency.RunInRLockWithLogger(
		&s.resourcesMu, LockNameMcpServerBuildSchemaResource, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			resource, exists = s.resources[uri]
			return nil
		},
	)

	// Use resource metadata if available, otherwise use defaults
	responseURI := uri
	responseMimeType := "application/ld+json" // Default for schema resources
	if exists {
		if resource.URI != emptyValue {
			responseURI = resource.URI
		}
		if resource.MimeType != emptyValue {
			responseMimeType = resource.MimeType
		}
	}

	return map[string]any{
		"contents": []map[string]any{
			{
				"uri":      responseURI,
				"mimeType": responseMimeType,
				"text":     text,
			},
		},
	}, nil
}

// countRequiredFields counts required fields in KindFields
func countRequiredFields(kindFields *objects.KindFields) int {
	count := 0
	for i := range kindFields.AllFields {
		if kindFields.AllFields[i].Required {
			count++
		}
	}
	return count
}
