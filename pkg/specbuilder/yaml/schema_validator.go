package yaml

import (
	"encoding/json"
	"io"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// SchemaValidator uses github.com/santhosh-tekuri/jsonschema/v5
// which is licensed under Apache License 2.0.
// See LICENSE file in project root for full license text.

// SchemaValidator validates YAML files against JSON schemas
type SchemaValidator struct {
	schemaCache map[string]*jsonschema.Schema
	schemasDir  string
}

// NewSchemaValidator creates a new schema validator
func NewSchemaValidator(schemasDir string) *SchemaValidator {
	return &SchemaValidator{
		schemaCache: make(map[string]*jsonschema.Schema),
		schemasDir:  schemasDir,
	}
}

// ValidateYAML validates a YAML file against a JSON schema
// schemaPath can be:
//   - Absolute path to schema file
//   - Relative path (resolved from schemasDir)
//   - Schema ID (e.g., "https://zqk.dev/schemas/object_spec.schema.json")
func (sv *SchemaValidator) ValidateYAML(yamlPath string, schemaPath string) error {
	// Read YAML file
	yamlData, err := fileutil.ReadFile(yamlPath)
	if err != nil {
		return errfmt.Newf("failed to read YAML file").Wrap(err)
	}

	// Parse YAML to extract $schema reference if schemaPath is empty
	if schemaPath == emptyValue {
		var yamlDoc map[string]any
		if err := yaml.Unmarshal(yamlData, &yamlDoc); err == nil {
			if schemaRef, ok := yamlDoc["$schema"].(string); ok && schemaRef != emptyValue {
				schemaPath = schemaRef
			}
		}
	}

	// If still no schema, skip validation (optional)
	if schemaPath == emptyValue {
		return nil // No schema specified, skip validation
	}

	// Resolve schema path
	resolvedSchemaPath, err := sv.resolveSchemaPath(schemaPath)
	if err != nil {
		return errfmt.Newf("failed to resolve schema path").Wrap(err)
	}

	// Load schema (with caching)
	schema, err := sv.loadSchema(resolvedSchemaPath)
	if err != nil {
		return errfmt.Newf("failed to load schema").Wrap(err)
	}

	// Convert YAML to JSON for validation
	var yamlDataAny any
	if err := yaml.Unmarshal(yamlData, &yamlDataAny); err != nil {
		return errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Convert to JSON bytes
	jsonData, err := json.Marshal(yamlDataAny)
	if err != nil {
		return errfmt.Newf("failed to convert YAML to JSON").Wrap(err)
	}

	// Validate - Validate expects a JSON value (any), not bytes
	var jsonValue any
	if err := json.Unmarshal(jsonData, &jsonValue); err != nil {
		return errfmt.Newf("failed to unmarshal JSON for validation").Wrap(err)
	}

	// Validate
	if err := schema.Validate(jsonValue); err != nil {
		return errfmt.Newf("schema validation failed").Wrap(err)
	}

	return nil
}

// resolveSchemaPath resolves a schema path to an absolute file path
func (sv *SchemaValidator) resolveSchemaPath(schemaPath string) (string, error) {
	// If it's an absolute path, use it directly
	if filepath.IsAbs(schemaPath) {
		return schemaPath, nil
	}

	// If it's a URL (starts with http:// or https://), try to resolve from schemasDir
	if strings.HasPrefix(schemaPath, "http://") || strings.HasPrefix(schemaPath, "https://") {
		// Extract schema filename from URL
		// e.g., "https://zqk.dev/schemas/object_spec.schema.json" -> "object_spec.schema.json"
		parts := strings.Split(schemaPath, "/")
		if len(parts) > 0 {
			schemaFile := parts[len(parts)-1]
			resolved := filepath.Join(sv.schemasDir, schemaFile)
			if _, err := fileutil.Stat(resolved); err == nil {
				return resolved, nil
			}
		}
		return "", errfmt.Errorf("schema not found: %s", schemaPath)
	}

	// If it's a relative path, resolve from schemasDir
	if sv.schemasDir != emptyValue {
		resolved := filepath.Join(sv.schemasDir, schemaPath)
		if _, err := fileutil.Stat(resolved); err == nil {
			return resolved, nil
		}
	}

	// Try relative to YAML file location
	if filepath.Dir(schemaPath) != "." {
		// It's a relative path, try resolving from current directory
		if abs, err := filepath.Abs(schemaPath); err == nil {
			if _, err := fileutil.Stat(abs); err == nil {
				return abs, nil
			}
		}
	}

	return "", errfmt.Errorf("schema not found: %s", schemaPath)
}

// loadSchema loads a JSON schema with caching
func (sv *SchemaValidator) loadSchema(schemaPath string) (*jsonschema.Schema, error) {
	// Check cache
	if schema, ok := sv.schemaCache[schemaPath]; ok {
		return schema, nil
	}

	// Convert to file:// URL for jsonschema compiler
	schemaURL := "file://" + schemaPath
	if !filepath.IsAbs(schemaPath) {
		abs, err := filepath.Abs(schemaPath)
		if err != nil {
			return nil, errfmt.Newf("failed to get absolute path").Wrap(err)
		}
		schemaURL = "file://" + abs
	}

	// Compile schema
	compiler := jsonschema.NewCompiler()
	// Allow relative $ref resolution from schema directory
	compiler.LoadURL = func(s string) (io.ReadCloser, error) {
		// Handle file:// URLs
		if strings.HasPrefix(s, "file://") {
			filePath := strings.TrimPrefix(s, "file://")
			if f, err := fileutil.Open(filePath); err == nil {
				return f, nil
			}
		}

		// Try resolving relative to schema directory
		if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "file://") {
			relPath := filepath.Join(filepath.Dir(schemaPath), s)
			if f, err := fileutil.Open(relPath); err == nil {
				return f, nil
			}
			// Also try from schemasDir
			if sv.schemasDir != emptyValue {
				relPath2 := filepath.Join(sv.schemasDir, s)
				if f, err := fileutil.Open(relPath2); err == nil {
					return f, nil
				}
			}
			// Try as absolute or relative path
			if f, err := fileutil.Open(s); err == nil {
				return f, nil
			}
		}
		return nil, errfmt.Errorf("failed to load schema reference: %s", s)
	}

	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		return nil, errfmt.Newf("failed to compile schema").Wrap(err)
	}

	// Cache schema (use original path as key)
	sv.schemaCache[schemaPath] = schema

	return schema, nil
}

// ValidateYAMLWithAutoSchema validates a YAML file using its $schema reference
// If no $schema is found, validation is skipped (optional validation)
func (sv *SchemaValidator) ValidateYAMLWithAutoSchema(yamlPath string) error {
	return sv.ValidateYAML(yamlPath, "")
}
