package pipeline

// Owner represents a typed ownership claim on the graph.
type Owner struct {
	Type      string `json:"type"`
	SubjectID string `json:"subject_id"`
}

// TypedGraph manages typed ownership over the process graph.
type TypedGraph struct {
	Ownership map[string]map[string]bool // type -> subjectID -> true
}

// NewTypedGraph creates an empty TypedGraph ready for ownership tracking.
func NewTypedGraph() *TypedGraph {
	return &TypedGraph{
		Ownership: make(map[string]map[string]bool),
	}
}

// Owns checks whether the given type and subject have a verified ownership claim.
func (tg *TypedGraph) Owns(t, id string) bool {
	if _, ok := tg.Ownership[t]; !ok {
		return false
	}
	return tg.Ownership[t][id]
}

// Claim sets up an ownership record for the given type and subject.
func (tg *TypedGraph) Claim(owner Owner) {
	if tg.Ownership[owner.Type] == nil {
		tg.Ownership[owner.Type] = make(map[string]bool)
	}
	tg.Ownership[owner.Type][owner.SubjectID] = true
}

// Invalidate removes an ownership claim.
func (tg *TypedGraph) Invalidate(owner Owner) {
	if _, ok := tg.Ownership[owner.Type]; ok {
		delete(tg.Ownership[owner.Type], owner.SubjectID)
	}
}
