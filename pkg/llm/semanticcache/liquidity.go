package semanticcache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

// SharedContextState represents a context state that agents can share in memory.
// This allows agents to pass lightweight references instead of full context windows.
type SharedContextState struct {
	ID        string    `json:"id"`
	State     any       `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CognitiveLiquidityPool provides a registry for sharing context states
// across standard boundaries, fulfilling the Semantic Caching & Cognitive Liquidity requirement.
type CognitiveLiquidityPool struct {
	graphPool provider.ConnectionPool
	ttl       time.Duration
}

// NewCognitiveLiquidityPool initializes a new CognitiveLiquidityPool.
func NewCognitiveLiquidityPool(graphPool provider.ConnectionPool, ttl time.Duration) *CognitiveLiquidityPool {
	return &CognitiveLiquidityPool{
		graphPool: graphPool,
		ttl:       ttl,
	}
}

// Store saves a context state into the pool and returns a unique liquidity handle (ID).
func (p *CognitiveLiquidityPool) Store(ctx context.Context, state any) (string, error) {
	now := time.Now()

	// Generate a unique ID using crypto/rand
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", errfmt.Errorf("failed to generate random ID: %v", err)
	}
	id := hex.EncodeToString(b)

	stateBytes, err := json.Marshal(state)
	if err != nil {
		return "", errfmt.Errorf("failed to marshal state: %v", err)
	}

	expiresAt := now.Add(p.ttl)

	err = p.graphPool.Execute(ctx, func(conn provider.GraphConnection) error {
		node := provider.Node{
			ID:     id,
			Labels: []string{"CognitiveLiquidityState"},
			Properties: map[string]any{
				objects.FieldKeyID:        id,
				"state":                   string(stateBytes),
				objects.FieldKeyCreatedAt: now.Format(time.RFC3339),
				objects.FieldKeyExpiresAt: expiresAt.Format(time.RFC3339),
			},
		}
		return conn.CreateNode(ctx, node)
	})

	if err != nil {
		return "", errfmt.Errorf("failed to store state in graph: %v", err)
	}

	return id, nil
}

// Retrieve fetches a context state by its unique handle.
// Returns an error if the state is not found or has expired.
func (p *CognitiveLiquidityPool) Retrieve(ctx context.Context, id string) (any, error) {
	var stateStr string
	var expiresAtStr string

	err := p.graphPool.Execute(ctx, func(conn provider.GraphConnection) error {
		node, err := conn.GetNode(ctx, id, []string{"CognitiveLiquidityState"})
		if err != nil {
			return err
		}

		if val, ok := node.Properties["state"].(string); ok {
			stateStr = val
		} else {
			return errfmt.Errorf("invalid state format in graph")
		}

		if val, ok := node.Properties[objects.FieldKeyExpiresAt].(string); ok {
			expiresAtStr = val
		} else {
			return errfmt.Errorf("invalid expires_at format in graph")
		}

		return nil
	})

	if err != nil {
		return nil, errfmt.Errorf("context state %q not found or error retrieving: %v", id, err)
	}

	expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		return nil, errfmt.Errorf("failed to parse expiration time: %v", err)
	}

	if time.Now().After(expiresAt) {
		return nil, errfmt.Errorf("context state %q has expired", id)
	}

	var state any
	if err := json.Unmarshal([]byte(stateStr), &state); err != nil {
		return nil, errfmt.Errorf("failed to unmarshal state: %v", err)
	}

	return state, nil
}

// Cleanup purges expired context states from the pool.
func (p *CognitiveLiquidityPool) Cleanup(ctx context.Context) error {
	now := time.Now().Format(time.RFC3339)

	query := provider.Query{
		Language: provider.QueryLanguageCypher,
		Query: `
			MATCH (n:CognitiveLiquidityState)
			WHERE n.expires_at < $now
			DELETE n
		`,
		Params: map[string]any{
			"now": now,
		},
	}

	err := p.graphPool.Execute(ctx, func(conn provider.GraphConnection) error {
		_, err := conn.ExecuteQuery(ctx, query)
		return err
	})

	if err != nil {
		return errfmt.Errorf("failed to cleanup expired states: %v", err)
	}
	return nil
}
