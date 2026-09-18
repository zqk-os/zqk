package resolver

import (
	"context"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

// ReferenceResolver resolves object references in the graph
type ReferenceResolver struct {
	conn provider.GraphConnection
}

// NewReferenceResolver creates a new reference resolver
func NewReferenceResolver(conn provider.GraphConnection) *ReferenceResolver {
	return &ReferenceResolver{
		conn: conn,
	}
}

// ResolveReference checks if a reference exists in the graph
func (r *ReferenceResolver) ResolveReference(ctx context.Context, refID string) (bool, error) {
	// Query for node with matching ID
	query := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query:    "MATCH (n {id: $id}) RETURN n",
		Params: map[string]any{
			objects.FieldKeyID: refID,
		},
	}

	result, err := r.conn.ExecuteQuery(ctx, query)
	if err != nil {
		return false, errfmt.Newf("failed to execute query").Wrap(err)
	}

	// If we have at least one row, the reference exists
	return len(result.Rows) > 0, nil
}

// ResolveReferences resolves multiple references and returns a map of results
func (r *ReferenceResolver) ResolveReferences(ctx context.Context, refIDs []string) map[string]bool {
	results := make(map[string]bool)

	for _, refID := range refIDs {
		exists, err := r.ResolveReference(ctx, refID)
		if err != nil {
			// On error, mark as unresolved
			results[refID] = false
			continue
		}
		results[refID] = exists
	}

	return results
}

// ResolveReferenceField resolves references in a field value and returns unresolved references
func (r *ReferenceResolver) ResolveReferenceField(ctx context.Context, fieldName string, fieldValue any) []string {
	var unresolved []string

	switch v := fieldValue.(type) {
	case string:
		// Single reference
		exists, err := r.ResolveReference(ctx, v)
		if err != nil || !exists {
			unresolved = append(unresolved, v)
		}

	case []any:
		// Array of references
		for _, ref := range v {
			if refStr, ok := ref.(string); ok {
				exists, err := r.ResolveReference(ctx, refStr)
				if err != nil || !exists {
					unresolved = append(unresolved, refStr)
				}
			}
		}

	case []string:
		// Array of string references
		for _, ref := range v {
			exists, err := r.ResolveReference(ctx, ref)
			if err != nil || !exists {
				unresolved = append(unresolved, ref)
			}
		}
	}

	return unresolved
}
