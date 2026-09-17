package tde

import (
	"time"
)

// EnvelopeStatus represents the lifecycle state of a TDE Envelope.
type EnvelopeStatus string

const (
	StatusStaged    EnvelopeStatus = "staged"
	StatusCommitted EnvelopeStatus = "committed"
	StatusCancelled EnvelopeStatus = "cancelled"
)

// Envelope wraps a mutation for Time-Delayed Execution.
type Envelope struct {
	ID         string         `json:"id"`
	Seq        int64          `json:"seq"` // set by WAL
	Kind       string         `json:"kind"`
	TargetID   string         `json:"target_id"`
	Operation  string         `json:"operation"` // "create", "update", "delete"
	PayloadB64 string         `json:"payload_b64"`
	ExecuteAt  time.Time      `json:"execute_at"`
	CreatedAt  time.Time      `json:"created_at"`
	DependsOn  []string       `json:"depends_on,omitempty"`
	Signature   string         `json:"signature,omitempty"`
	MerkleProof string         `json:"merkle_proof,omitempty"`
	Status     EnvelopeStatus `json:"status"`
}
