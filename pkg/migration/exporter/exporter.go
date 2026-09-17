package exporter

import (
	"context"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

// GraphExporter exports graph data back to YAML format
type GraphExporter struct {
	conn provider.GraphConnection
}

// NewGraphExporter creates a new graph exporter
func NewGraphExporter(conn provider.GraphConnection) *GraphExporter {
	return &GraphExporter{
		conn: conn,
	}
}

// ExportOptions configures export behavior
type ExportOptions struct {
	OutputDir    string
	ObjectTypes  []string // Filter by object types (empty = all)
	PreservePath bool     // Preserve original file structure
}

// ExportAll exports all entities from graph to YAML files
func (e *GraphExporter) ExportAll(ctx context.Context, opts ExportOptions) error {
	if opts.OutputDir == emptyValue {
		return errfmt.Errorf("output directory is required")
	}

	// Create output directory
	if err := fileutil.MkdirAll(opts.OutputDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create output directory").Wrap(err)
	}

	// Execute query and export entities
	// Note: This is a simplified implementation
	// Full implementation would:
	// 1. Query entities from graph (filtered by ObjectTypes if specified)
	// 2. Use the graph connection's query interface
	// 3. Export each entity to YAML format
	return e.exportEntities(ctx, opts)
}

// ExportEntity exports a single entity to YAML
func (e *GraphExporter) ExportEntity(ctx context.Context, entityID, outputPath string) error {
	// Query entity by ID
	// This is a placeholder - actual implementation would query the graph
	// and reconstruct the YAML from entity properties

	// For now, return error indicating not fully implemented
	return errfmt.Errorf("export entity not yet fully implemented - requires graph query interface")
}

// exportEntities exports entities to YAML files
func (e *GraphExporter) exportEntities(_ context.Context, opts ExportOptions) error {
	// Group entities by type for directory structure
	typeGroups := make(map[string][]map[string]any)

	// This is a placeholder implementation
	// Full implementation would:
	// 1. Query all entities from graph
	// 2. Group by object type (from kind property or label)
	// 3. For each entity, reconstruct YAML from properties
	// 4. Write to appropriate directory structure

	// Create directories for each object type
	for objType := range typeGroups {
		dirPath := filepath.Join(opts.OutputDir, objType)
		if err := fileutil.MkdirAll(dirPath, paths.DirPerm755); err != nil {
			return errfmt.Errorf("failed to create directory %s: %w", dirPath, err)
		}
	}

	return nil
}

// ExportToYAML converts entity properties to YAML format
func (e *GraphExporter) ExportToYAML(properties map[string]any) ([]byte, error) {
	// Convert properties map to YAML
	data, err := yaml.Marshal(properties)
	if err != nil {
		return nil, errfmt.Newf("failed to marshal YAML").Wrap(err)
	}

	return data, nil
}

// toLabel converts a snake_case kind to PascalCase label
func toLabel(kind string) string {
	parts := strings.Split(kind, "_")
	var labelParts []string
	for _, part := range parts {
		if part != emptyValue {
			labelParts = append(labelParts, strings.ToUpper(part[:1])+strings.ToLower(part[1:]))
		}
	}
	return strings.Join(labelParts, "")
}

// GetEntitySourcePath retrieves the original source path from Document node
func (e *GraphExporter) GetEntitySourcePath(ctx context.Context, entityID string) (string, error) {
	// Query for HAS_SOURCE edge to find Document node
	// This is a placeholder - requires graph query interface
	return "", errfmt.Errorf("get entity source path not yet fully implemented - requires graph query interface")
}
