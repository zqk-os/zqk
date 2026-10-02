package writer

import (
	"context"
	"strings"

	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

const emptyValue = ""

// GraphWriter writes objects to the graph backend
type GraphWriter struct {
	conn provider.GraphConnection
}

// NewGraphWriter creates a new graph writer
func NewGraphWriter(conn provider.GraphConnection) *GraphWriter {
	return &GraphWriter{
		conn: conn,
	}
}

// CreateDocumentNode creates a Document node (Phase 1)
func (w *GraphWriter) CreateDocumentNode(ctx context.Context, docID, filePath, content string) error {
	node := provider.Node{
		ID:     docID,
		Labels: []string{"Document"},
		Properties: map[string]any{
			"source_path":           filePath,
			objects.FieldKeyContent: content,
			objects.FieldKeyType:    "yaml",
		},
	}

	return w.conn.CreateNode(ctx, node)
}

// CreateEntityNode creates an Entity node (Phase 2)
func (w *GraphWriter) CreateEntityNode(ctx context.Context, entityID, entityKind string, properties map[string]any, embedding []float32) error {
	// Convert entity kind to label (e.g., "backlog_item" -> "BacklogItem")
	label := toLabel(entityKind)

	node := provider.Node{
		ID:         entityID,
		Labels:     []string{label, "Entity"},
		Properties: properties,
	}

	// Add embedding if provided
	if embedding != nil {
		node.Properties["embedding"] = embedding
	}

	return w.conn.CreateNode(ctx, node)
}

// CreateRelationshipEdge creates a relationship edge (Phase 3)
func (w *GraphWriter) CreateRelationshipEdge(ctx context.Context, fromID, toID, edgeType string, properties map[string]any) error {
	edge := provider.Edge{
		FromID:     fromID,
		ToID:       toID,
		Type:       edgeType,
		Properties: properties,
	}

	if edge.Properties == nil {
		edge.Properties = make(map[string]any)
	}

	return w.conn.CreateEdge(ctx, edge)
}

// CreateHASSOURCEEdge creates a HAS_SOURCE edge linking Entity to Document
func (w *GraphWriter) CreateHASSOURCEEdge(ctx context.Context, entityID, documentID string) error {
	return w.CreateRelationshipEdge(ctx, entityID, documentID, "HAS_SOURCE", nil)
}

// toLabel converts a snake_case kind to PascalCase label
func toLabel(kind string) string {
	return provider.ToLabel(kind)
}

// CreateBatch creates multiple nodes/edges in a batch
func (w *GraphWriter) CreateBatch(ctx context.Context, operations []provider.Operation) (*provider.BatchResult, error) {
	return w.conn.ExecuteBatch(ctx, operations)
}
