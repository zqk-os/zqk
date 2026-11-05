package gate

import (
	"encoding/json"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	// KindApprovalRequest is the kernel kind for staged approval requests.
	KindApprovalRequest = "approval_request"

	// Approval statuses
	StatusStaged   = "staged"
	StatusApproved = "approved"
	StatusRejected = "rejected"

	// Audit event types
	AuditEventTypeGateApproval  = "gate_approval"
	AuditEventTypeGateRejection = "gate_rejection"
	AuditEventTypeGateStaged    = "gate_staged"
)

func init() {
	// Register lifecycle aliases so that "staged" maps to the kernel's "proposed"
	// lifecycle status, and "rejected" maps to "archived", making them 100% compliant
	// with base_object lifecycle invariants while preserving gate domain nomenclature.
	objects.RegisterStatusAlias("staged", "proposed")
	objects.RegisterStatusAlias("rejected", "archived")
}

// ApprovalRequest represents a cryptographic human-in-the-loop mutation approval request.
type ApprovalRequest struct {
	ID          string `json:"id" yaml:"id"`
	Kind        string `json:"kind" yaml:"kind"`
	Title       string `json:"title" yaml:"title"`
	Type        string `json:"type" yaml:"type"`
	Target      string `json:"target" yaml:"target"`
	Payload     string `json:"payload" yaml:"payload"`
	PayloadHash string `json:"payload_hash" yaml:"payload_hash"`
	Hash        string `json:"hash" yaml:"hash"`
	Status      string `json:"status" yaml:"status"`
	Reviewer    string `json:"reviewer,omitempty" yaml:"reviewer,omitempty"`
	ReviewedAt  string `json:"reviewed_at,omitempty" yaml:"reviewed_at,omitempty"`
	Reason      string `json:"reason,omitempty" yaml:"reason,omitempty"`
	CreatedAt   string `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	CreatedBy   string `json:"created_by,omitempty" yaml:"created_by,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

// ToMap serializes the ApprovalRequest into a generic map for storage.
func (a *ApprovalRequest) ToMap() map[string]any {
	title := a.Title
	if title == "" {
		title = "Gate Approval Request: " + a.Type + " -> " + a.Target
	}
	m := map[string]any{
		objects.FieldKeyID:        a.ID,
		objects.FieldKeyKind:      KindApprovalRequest,
		objects.FieldKeyTitle:     title,
		"type":                    a.Type,
		"target":                  a.Target,
		"payload":                 a.Payload,
		"payload_hash":            a.PayloadHash,
		"hash":                    a.Hash,
		objects.FieldKeyStatus:    a.Status,
		objects.FieldKeyCreatedAt: a.CreatedAt,
		objects.FieldKeyCreatedBy: a.CreatedBy,
	}
	if a.Reviewer != "" {
		m["reviewer"] = a.Reviewer
	}
	if a.ReviewedAt != "" {
		m["reviewed_at"] = a.ReviewedAt
	}
	if a.Reason != "" {
		m["reason"] = a.Reason
	}
	return m
}

// FromMap deserializes an ApprovalRequest from a storage object map.
func FromMap(m map[string]any) (*ApprovalRequest, error) {
	if m == nil {
		return nil, errfmt.Errorf("cannot parse nil map into ApprovalRequest")
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, errfmt.Newf("failed to marshal map: %v", err)
	}
	var req ApprovalRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, errfmt.Newf("failed to unmarshal ApprovalRequest: %v", err)
	}

	if req.ID == "" {
		if id, ok := m[objects.FieldKeyID].(string); ok {
			req.ID = id
		}
	}
	if req.Title == "" {
		if t, ok := m[objects.FieldKeyTitle].(string); ok {
			req.Title = t
		}
	}
	if req.Status == "" {
		if s, ok := m[objects.FieldKeyStatus].(string); ok {
			req.Status = s
		}
	}
	// Normalize mapped lifecycle statuses back to gate domain statuses
	if req.Status == "proposed" {
		req.Status = StatusStaged
	} else if req.Status == "archived" {
		req.Status = StatusRejected
	}

	if req.CreatedAt == "" {
		if c, ok := m[objects.FieldKeyCreatedAt].(string); ok {
			req.CreatedAt = c
		}
	}
	if req.UpdatedAt == "" {
		if u, ok := m[objects.FieldKeyUpdatedAt].(string); ok {
			req.UpdatedAt = u
		}
	}
	if req.Hash == "" && req.PayloadHash != "" {
		req.Hash = req.PayloadHash
	}
	if req.PayloadHash == "" && req.Hash != "" {
		req.PayloadHash = req.Hash
	}
	return &req, nil
}

// CurrentTimestamp returns current UTC RFC3339 timestamp.
func CurrentTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
