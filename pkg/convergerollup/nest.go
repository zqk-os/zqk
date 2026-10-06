package convergerollup

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NestSpawnRequest is the contract for creating a child convergence_session under a parent.
type NestSpawnRequest struct {
	ParentID        string
	Title           string
	Hypothesis      string
	DesiredEndState string
	Predictions     map[string]any
	NextAction      string
	CurrentPhase    string
	MaxDepth        int // 0 → DefaultOverseerCVSTreeMaxDepth
}

// NestStatusResult is the BFS tree under a parent/coordinator.
type NestStatusResult struct {
	ParentID string        `json:"parent_id"`
	MaxDepth int           `json:"max_depth"`
	Nodes    []CVSTreeNode `json:"nodes"`
}

// CVSNodeLoader loads related_object_refs / status / phase for nest depth and cycle checks.
type CVSNodeLoader func(id string) (refs []string, status string, phase string, err error)

// ValidateNestLink checks depth and cycle before appending child to parent.
// coordinatorID is the tree root used for absolute depth (defaults to parentID).
func ValidateNestLink(coordinatorID, parentID, childID string, maxDepth int, nodeFor CVSNodeLoader) error {
	parentID = strings.TrimSpace(parentID)
	childID = strings.TrimSpace(childID)
	coordinatorID = strings.TrimSpace(coordinatorID)
	if coordinatorID == "" {
		coordinatorID = parentID
	}
	if parentID == "" || childID == "" {
		return errfmt.Errorf("convergerollup: nest link requires parent and child CVS ids")
	}
	if !strings.HasPrefix(parentID, "CVS-") || !strings.HasPrefix(childID, "CVS-") {
		return errfmt.Errorf("convergerollup: nest link requires CVS-* ids")
	}
	if parentID == childID {
		return errfmt.Errorf("convergerollup: cannot nest CVS under itself")
	}
	if maxDepth <= 0 {
		maxDepth = DefaultOverseerCVSTreeMaxDepth
	}
	tree, err := CollectCVSTreeBFS(coordinatorID, maxDepth, nodeFor)
	if err != nil {
		return err
	}
	parentDepth := -1
	for _, n := range tree {
		if n.ID == parentID {
			parentDepth = n.Depth
			break
		}
	}
	if parentDepth < 0 {
		return errfmt.Errorf("convergerollup: parent %s not found under coordinator %s", parentID, coordinatorID)
	}
	if parentDepth+1 > maxDepth {
		return errfmt.Errorf("convergerollup: nesting under %s would exceed max depth %d", parentID, maxDepth)
	}
	childTree, err := CollectCVSTreeBFS(childID, maxDepth, nodeFor)
	if err != nil {
		return err
	}
	for _, n := range childTree {
		if n.ID == parentID || n.ID == coordinatorID {
			return errfmt.Errorf("convergerollup: linking %s under %s would create a cycle", childID, parentID)
		}
	}
	return nil
}

// AppendRelatedObjectRef returns refs with id appended if missing.
func AppendRelatedObjectRef(refs []string, id string) []string {
	id = strings.TrimSpace(id)
	if id == "" {
		return refs
	}
	for _, r := range refs {
		if r == id {
			return refs
		}
	}
	return append(append([]string(nil), refs...), id)
}

// RelatedObjectRefsFromMap extracts string refs from an object map.
func RelatedObjectRefsFromMap(obj map[string]any) []string {
	if obj == nil {
		return nil
	}
	raw, ok := obj[objects.FieldKeyRelatedObjectRefs]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// NestStatus builds a BFS status tree for parentID.
func NestStatus(parentID string, maxDepth int, nodeFor CVSNodeLoader) (*NestStatusResult, error) {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return nil, errfmt.Errorf("convergerollup: nest status requires parent CVS id")
	}
	if maxDepth <= 0 {
		maxDepth = DefaultOverseerCVSTreeMaxDepth
	}
	nodes, err := CollectCVSTreeBFS(parentID, maxDepth, nodeFor)
	if err != nil {
		return nil, err
	}
	return &NestStatusResult{ParentID: parentID, MaxDepth: maxDepth, Nodes: nodes}, nil
}

// ChildSessionFields builds a minimal create payload for a nested convergence_session.
func ChildSessionFields(req NestSpawnRequest) map[string]any {
	phase := strings.TrimSpace(req.CurrentPhase)
	if phase == "" {
		phase = "c1_scope"
	}
	m := map[string]any{
		objects.FieldKeyKind:              objects.KindConvergenceSession,
		objects.FieldKeyTitle:             req.Title,
		objects.FieldKeyStatus:            "active",
		objects.FieldKeyCurrentPhase:      phase,
		objects.FieldKeyOutcomeCharacter:  "pending",
		objects.FieldKeyHypothesis:        req.Hypothesis,
		objects.FieldKeyDesiredEndState:   req.DesiredEndState,
		objects.FieldKeyRelatedObjectRefs: []string{req.ParentID},
	}
	if req.NextAction != "" {
		m[objects.FieldKeyNextAction] = req.NextAction
	}
	if len(req.Predictions) > 0 {
		m[objects.FieldKeyPredictions] = req.Predictions
	}
	return m
}
