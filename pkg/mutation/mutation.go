package mutation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

type Action string

const (
	ActionCreateNode Action = "create_node"
	ActionUpdateNode Action = "update_node"
	ActionAddEdge    Action = "add_edge"
	ActionRemoveEdge Action = "remove_edge"
)

type SafetyClass string

const (
	SafetyRead        SafetyClass = "read"
	SafetyWrite       SafetyClass = "write"
	SafetyDestructive SafetyClass = "destructive"
	SafetyHilRequired SafetyClass = "hil_required"
)

type EdgeMutation struct {
	TargetID string         `json:"target_id"`
	Relation string         `json:"relation"`
	Fields   map[string]any `json:"fields,omitempty"`
}

type Mutation struct {
	Action           Action         `json:"action"`
	TargetKind       string         `json:"target_kind"`
	TargetID         string         `json:"target_id,omitempty"`
	Fields           map[string]any `json:"fields,omitempty"`
	Edges            []EdgeMutation `json:"edges,omitempty"`
	StatusTransition string         `json:"status_transition,omitempty"`
	SafetyClass      SafetyClass    `json:"safety_class,omitempty"`
}

func OutputSchema() string {
	return `{
  "type": "object",
  "required": ["action", "target_kind"],
  "properties": {
    "action": {"enum": ["create_node", "update_node", "add_edge", "remove_edge"]},
    "target_kind": {"type": "string"},
    "target_id": {"type": "string"},
    "fields": {"type": "object"},
    "edges": {"type": "array", "items": {"$ref": "#/definitions/EdgeMutation"}},
    "status_transition": {"type": "string"},
    "safety_class": {"enum": ["read", "write", "destructive", "hil_required"]}
  }
}`
}

func FromMap(m map[string]any) (Mutation, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return Mutation{}, err
	}
	var mut Mutation
	err = json.Unmarshal(b, &mut)
	return mut, err
}

func BuildIDKey(taskID string, mut *Mutation) string {
	// JSON marshal maps to sorted keys implicitly in Go
	b, _ := json.Marshal(mut)
	return "ik::" + taskID + "::" + fmt.Sprintf("%x", sha256.Sum256(b))
}

func (m *Mutation) ResultHashHint() string {
	b, _ := json.Marshal(m)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
