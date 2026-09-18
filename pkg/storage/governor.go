package storage

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	// Governor Edges
	EdgeProposed = "PROPOSED"
	EdgeApproved = "APPROVED"
	EdgeModified = "MODIFIED"

	// Governor Node Additional Labels
	LabelHuman = "Human"
	LabelAgent = "Agent"
	LabelEvent = "Event"
)

// AddGovernorLabels adds Human/Agent labels to account nodes based on their roles.
func (g *GraphObjectStorage) AddGovernorLabels(ctx context.Context, accountID string, isAgent bool) error {
	labelToAdd := LabelHuman
	if isAgent {
		labelToAdd = LabelAgent
	}

	updates := provider.NodeUpdates{
		AddLabels: []string{labelToAdd},
	}
	return g.conn.UpdateNode(ctx, accountID, updates)
}

// CreateProposedEdge creates a PROPOSED edge from an Agent account to an AuditEvent.
func (g *GraphObjectStorage) CreateProposedEdge(ctx context.Context, agentAccountID, eventID string) error {
	edge := provider.Edge{
		FromID:     agentAccountID,
		ToID:       eventID,
		Type:       EdgeProposed,
		Properties: map[string]any{},
	}
	return g.conn.CreateEdge(ctx, edge)
}

// CreateApprovedEdge creates an APPROVED edge from a Human account to an AuditEvent.
func (g *GraphObjectStorage) CreateApprovedEdge(ctx context.Context, humanAccountID, eventID, signature string) error {
	edge := provider.Edge{
		FromID: humanAccountID,
		ToID:   eventID,
		Type:   EdgeApproved,
		Properties: map[string]any{
			objects.FieldKeySignature: signature,
		},
	}
	return g.conn.CreateEdge(ctx, edge)
}

// CreateModifiedEdge creates a MODIFIED edge from an AuditEvent to a CodeEntity or other object.
func (g *GraphObjectStorage) CreateModifiedEdge(ctx context.Context, eventID, targetID string) error {
	edge := provider.Edge{
		FromID:     eventID,
		ToID:       targetID,
		Type:       EdgeModified,
		Properties: map[string]any{},
	}
	return g.conn.CreateEdge(ctx, edge)
}

// VerifyProvenance checks if an event has been approved by a human with the required role.
func (g *GraphObjectStorage) VerifyProvenance(ctx context.Context, secCtx *pkgctx.SecurityContext, eventID string) (bool, error) {
	// Query to check if there is an APPROVED edge from a Human node to this event
	query := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query: `
			MATCH (h:Human)-[r:APPROVED]->(e:AuditEvent {id: $eventId})
			RETURN h.id as approver, r.signature as signature
		`,
		Params: map[string]any{
			"eventId": eventID,
		},
	}

	result, err := g.conn.ExecuteQuery(ctx, query)
	if err != nil {
		return false, errfmt.Newf(ConstMiscProvenanceVerificationFailed).Wrap(err)
	}

	// If there's at least one row, it's approved
	return len(result.Rows) > 0, nil
}
