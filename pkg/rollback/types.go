// Package rollback provides a reusable snapshot-and-rollback pattern for lifecycle
// transitions, maintenance tasks, and other operations. See docs/architecture
// LIFECYCLE_ROLLBACK_SNAPSHOT_AND_AUDIT_CHAIN.md.
package rollback

import (
	"time"

	"github.com/zqk-os/zqk/pkg/kindnames"
)

// ObjectState is the minimal state needed to restore one object.
type ObjectState struct {
	Kind  string         `json:"kind"`
	ID    string         `json:"id"`
	State map[string]any `json:"state"`
}

// RollbackPoint is a single snapshot of a set of object states.
type RollbackPoint struct {
	ID           string         `json:"id"`
	Timestamp    time.Time      `json:"ts"`
	ScopeType    string         `json:"scope_type"`
	ScopeID      string         `json:"scope_id"`
	ObjectStates []ObjectState  `json:"object_states"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// Meta is lightweight info for listing rollback points.
type Meta struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"ts"`
	ScopeType   string    `json:"scope_type"`
	ScopeID     string    `json:"scope_id"`
	ObjectCount int       `json:"object_count"`
}

// ScopeType constants for consumers.
const (
	ScopeTypeLifecycle   = kindnames.Lifecycle
	ScopeTypeMaintenance = "maintenance"
	ScopeTypeMigration   = "migration"
)
