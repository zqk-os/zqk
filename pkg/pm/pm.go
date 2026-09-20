package pm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/dna"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// InvariantPredicateFunc executes domain invariant checks against target objects.
type InvariantPredicateFunc func(ctx context.Context, obj any) error

// BacklogItem models an atomic unit of execution in the cellular PM domain.
type BacklogItem struct {
	dna.BaseObject           `yaml:",inline" json:",inline"`
	dna.Auditable            `yaml:",inline" json:",inline"`
	dna.Lifecycle            `yaml:",inline" json:",inline"`
	ID                       string     `json:"id,omitempty" yaml:"id,omitempty"`
	Title                    string     `json:"title" yaml:"title"`
	Description              string     `json:"description" yaml:"description"`
	ProblemStatement         string     `json:"problem_statement,omitempty" yaml:"problem_statement,omitempty"`
	AcceptanceConsiderations string     `json:"acceptance_considerations,omitempty" yaml:"acceptance_considerations,omitempty"`
	Priority                 string     `json:"priority,omitempty" yaml:"priority,omitempty"`
	PriorityTier             string     `json:"priority_tier,omitempty" yaml:"priority_tier,omitempty"`
	EstimatedEffort          string     `json:"estimated_effort,omitempty" yaml:"estimated_effort,omitempty"`
	ActualEffort             string     `json:"actual_effort,omitempty" yaml:"actual_effort,omitempty"`
	StartedAt                *time.Time `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	CompletedAt              *time.Time `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	CommitHashes             []string   `json:"commit_hashes,omitempty" yaml:"commit_hashes,omitempty"`
	RequirementRefs          []string   `json:"requirement_refs,omitempty" yaml:"requirement_refs,omitempty"`
	CriteriaRefs             []string   `json:"criteria_refs,omitempty" yaml:"criteria_refs,omitempty"`
	MilestoneRefs            []string   `json:"milestone_refs,omitempty" yaml:"milestone_refs,omitempty"`
	WorkstreamRefs           []string   `json:"workstream_refs,omitempty" yaml:"workstream_refs,omitempty"`
	PersonaRefs              []string   `json:"persona_refs,omitempty" yaml:"persona_refs,omitempty"`
	PriorityPlanRef          string     `json:"priority_plan_ref,omitempty" yaml:"priority_plan_ref,omitempty"`
	GoalRefs                 []string   `json:"goal_refs,omitempty" yaml:"goal_refs,omitempty"`
}

// GetID returns the entity ID, resolving from URN if ID field is empty.
func (b *BacklogItem) GetID() string {
	if b.ID != "" {
		return b.ID
	}
	return b.URN.ID
}

// NewBacklogItem constructs a new BacklogItem with valid cellular BaseObject and Lifecycle.
func NewBacklogItem(id, title, description string) (*BacklogItem, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errfmt.Errorf("backlog item ID cannot be empty")
	}
	if strings.TrimSpace(title) == "" {
		return nil, errfmt.Errorf("backlog item title cannot be empty")
	}
	if strings.TrimSpace(description) == "" {
		return nil, errfmt.Errorf("backlog item description cannot be empty")
	}

	urn, err := dna.NewURN("project-mgmt", "backlog_item", id)
	if err != nil {
		return nil, err
	}

	allowedTransitions := map[string][]string{
		"conceptual":  {"originated", "archived"},
		"originated":  {"exploring", "archived"},
		"exploring":   {"planned", "deferred", "archived"},
		"planned":     {"testing", "in_progress", "deferred", "archived"},
		"testing":     {"in_progress", "planned", "archived"},
		"in_progress": {"complete", "planned", "error", "archived"},
		"complete":    {"archived"},
	}

	return &BacklogItem{
		BaseObject:      dna.NewBaseObject(urn, "urn:zqk:spec:project-mgmt:backlog_item"),
		Lifecycle:       dna.NewLifecycle("conceptual", allowedTransitions),
		ID:              id,
		Title:           title,
		Description:     description,
		CommitHashes:    make([]string, 0),
		RequirementRefs: make([]string, 0),
		CriteriaRefs:    make([]string, 0),
		MilestoneRefs:   make([]string, 0),
		WorkstreamRefs:  make([]string, 0),
		PersonaRefs:     make([]string, 0),
		GoalRefs:        make([]string, 0),
	}, nil
}

// TransitionTo executes a lifecycle transition and updates plane/timestamps on BacklogItem.
func (b *BacklogItem) TransitionTo(ctx context.Context, targetStatus string) error {
	if err := b.Lifecycle.Transition(ctx, b, targetStatus); err != nil {
		return err
	}
	b.Status = targetStatus
	now := time.Now().UTC()
	b.UpdatedAt = now
	switch targetStatus {
	case "in_progress":
		b.Plane = dna.PlaneStaged
		if b.StartedAt == nil {
			b.StartedAt = &now
		}
	case "complete":
		b.Plane = dna.PlanePromoted
		if b.CompletedAt == nil {
			b.CompletedAt = &now
		}
	case "archived":
		b.Plane = dna.PlaneApoptotic
	}
	return nil
}

// InvariantGate models a verifiable quality or security barrier for lifecycle transitions.
type InvariantGate struct {
	dna.BaseObject     `yaml:",inline" json:",inline"`
	dna.Auditable      `yaml:",inline" json:",inline"`
	Title              string                 `json:"title" yaml:"title"`
	Description        string                 `json:"description" yaml:"description"`
	TargetKind         string                 `json:"target_kind" yaml:"target_kind"`
	TargetStatus       string                 `json:"target_status" yaml:"target_status"`
	Predicate          InvariantPredicateFunc `json:"-" yaml:"-"`
	RequireAttestation bool                   `json:"require_attestation,omitempty" yaml:"require_attestation,omitempty"`
}

// NewInvariantGate constructs a new InvariantGate entity.
func NewInvariantGate(id, targetKind, targetStatus string, predicate InvariantPredicateFunc) (*InvariantGate, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errfmt.Errorf("invariant gate ID cannot be empty")
	}
	if strings.TrimSpace(targetKind) == "" {
		return nil, errfmt.Errorf("invariant gate targetKind cannot be empty")
	}
	if strings.TrimSpace(targetStatus) == "" {
		return nil, errfmt.Errorf("invariant gate targetStatus cannot be empty")
	}

	urn, err := dna.NewURN("kernel", "invariant_gate", id)
	if err != nil {
		return nil, err
	}

	return &InvariantGate{
		BaseObject:   dna.NewBaseObject(urn, "urn:zqk:spec:kernel:invariant_gate"),
		Title:        fmt.Sprintf("Gate for %s -> %s", targetKind, targetStatus),
		Description:  fmt.Sprintf("Guards lifecycle hops of kind %s into %s", targetKind, targetStatus),
		TargetKind:   targetKind,
		TargetStatus: targetStatus,
		Predicate:    predicate,
	}, nil
}

// NewProvenanceInvariantGate constructs an InvariantGate verifying cryptographic provenance
// attestations and signatures before allowing state transitions into promoted or complete states.
func NewProvenanceInvariantGate(id, targetKind, targetStatus string) (*InvariantGate, error) {
	gate, err := NewInvariantGate(id, targetKind, targetStatus, func(ctx context.Context, obj any) error {
		type provGetter interface {
			GetProvenance() dna.Provenance
		}
		pg, ok := obj.(provGetter)
		if !ok {
			return errfmt.Errorf("object %T does not implement GetProvenance()", obj)
		}
		prov := pg.GetProvenance()
		if strings.TrimSpace(prov.Hash) == "" {
			return errfmt.Errorf("provenance attestation verification failed: missing content hash")
		}
		if strings.TrimSpace(prov.Signature) == "" {
			return errfmt.Errorf("provenance attestation verification failed: missing cryptographic signature")
		}
		if strings.TrimSpace(prov.ParentHash) == "" {
			return errfmt.Errorf("provenance attestation verification failed: missing causal parent hash")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	gate.RequireAttestation = true
	gate.Title = fmt.Sprintf("Cryptographic Provenance Gate for %s -> %s", targetKind, targetStatus)
	gate.Description = fmt.Sprintf("Verifies cryptographic provenance attestation before admitting %s into status %s", targetKind, targetStatus)
	return gate, nil
}

// Evaluate runs the gate predicate against an object.
func (g *InvariantGate) Evaluate(ctx context.Context, obj any) error {
	if g.Predicate == nil {
		return nil
	}
	return g.Predicate(ctx, obj)
}

// Epic represents a cellular objective and enclave scope container defining the
// bounding domain for compound goals, requirements, and child work units.
type Epic struct {
	dna.BaseObject   `yaml:",inline" json:",inline"`
	dna.Auditable    `yaml:",inline" json:",inline"`
	dna.Lifecycle    `yaml:",inline" json:",inline"`
	Title            string         `json:"title" yaml:"title"`
	Description      string         `json:"description" yaml:"description"`
	EnclaveScope     string         `json:"enclave_scope,omitempty" yaml:"enclave_scope,omitempty"`
	GoalRefs         []string       `json:"goal_refs,omitempty" yaml:"goal_refs,omitempty"`
	WorkstreamRefs   []string       `json:"workstream_refs,omitempty" yaml:"workstream_refs,omitempty"`
	RequirementRefs  []string       `json:"requirement_refs,omitempty" yaml:"requirement_refs,omitempty"`
	BacklogItemRefs  []string       `json:"backlog_item_refs,omitempty" yaml:"backlog_item_refs,omitempty"`
	ChildItemRefs    []string       `json:"child_item_refs,omitempty" yaml:"child_item_refs,omitempty"`
	PriorityPlanRefs []string       `json:"priority_plan_refs,omitempty" yaml:"priority_plan_refs,omitempty"`
	MemberWorkUnits  []*BacklogItem `json:"-" yaml:"-"`
}

// NewEpic constructs a new Epic entity with an automatic promotion invariant gate.
func NewEpic(id, title, description string) (*Epic, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errfmt.Errorf("epic ID cannot be empty")
	}
	if strings.TrimSpace(title) == "" {
		return nil, errfmt.Errorf("epic title cannot be empty")
	}

	urn, err := dna.NewURN("project-mgmt", "epic", id)
	if err != nil {
		return nil, err
	}

	allowedTransitions := map[string][]string{
		"conceptual": {"originated", "archived"},
		"originated": {"draft", "active", "archived"},
		"draft":      {"active", "archived"},
		"active":     {"complete", "archived"},
	}

	ep := &Epic{
		BaseObject:       dna.NewBaseObject(urn, "urn:zqk:spec:project-mgmt:epic"),
		Lifecycle:        dna.NewLifecycle("conceptual", allowedTransitions),
		Title:            title,
		Description:      description,
		GoalRefs:         make([]string, 0),
		WorkstreamRefs:   make([]string, 0),
		RequirementRefs:  make([]string, 0),
		BacklogItemRefs:  make([]string, 0),
		ChildItemRefs:    make([]string, 0),
		PriorityPlanRefs: make([]string, 0),
		MemberWorkUnits:  make([]*BacklogItem, 0),
	}

	// Register promotion invariant gate: prevents Epic transition to 'complete' / 'promoted'
	// until all member work units have cleared their invariant gates and reached complete status.
	ep.RegisterInvariantGate("complete", func(ctx context.Context, obj any) error {
		epicObj, ok := obj.(*Epic)
		if !ok {
			return errfmt.Errorf("expected *Epic for promotion gate evaluation")
		}
		return epicObj.CanPromote(ctx)
	})

	return ep, nil
}

// AddMemberWorkUnit registers a child work unit (BacklogItem) within the epic's enclave scope.
func (e *Epic) AddMemberWorkUnit(bli *BacklogItem) {
	if bli == nil {
		return
	}
	e.MemberWorkUnits = append(e.MemberWorkUnits, bli)
	id := bli.ID
	if id == "" {
		id = bli.URN.ID
	}
	found := false
	for _, ref := range e.BacklogItemRefs {
		if ref == id {
			found = true
			break
		}
	}
	if !found && id != "" {
		e.BacklogItemRefs = append(e.BacklogItemRefs, id)
		e.ChildItemRefs = append(e.ChildItemRefs, id)
	}
}

// CheckEnclaveScope checks whether a target package, file path, or URN falls within
// this Epic's declared EnclaveScope boundary.
func (e *Epic) CheckEnclaveScope(target string) bool {
	if strings.TrimSpace(e.EnclaveScope) == "" {
		return true // Unbounded if not explicitly specified
	}
	target = strings.TrimSpace(target)
	tokens := strings.Fields(e.EnclaveScope)
	for _, token := range tokens {
		cleanToken := strings.Trim(token, ",;:[]()\"'")
		if cleanToken == "" {
			continue
		}
		if strings.Contains(target, cleanToken) || strings.HasPrefix(target, cleanToken) {
			return true
		}
	}
	return false
}

// CanPromote verifies that all member work units have cleared their invariant gates
// and reached 'complete' status before allowing Epic promotion.
func (e *Epic) CanPromote(ctx context.Context) error {
	if len(e.MemberWorkUnits) == 0 && len(e.BacklogItemRefs) > 0 {
		return errfmt.Errorf("cannot promote epic %s: member work units referenced (%d) but not loaded for invariant verification", e.URN, len(e.BacklogItemRefs))
	}
	for _, unit := range e.MemberWorkUnits {
		if unit == nil {
			continue
		}
		unitID := unit.ID
		if unitID == "" {
			unitID = unit.URN.ID
		}
		if !strings.EqualFold(unit.Status, "complete") {
			return errfmt.Errorf("cannot promote epic %s: member work unit %s is not complete (current status: %q)", e.URN, unitID, unit.Status)
		}
		if unit.Plane != dna.PlanePromoted {
			return errfmt.Errorf("cannot promote epic %s: member work unit %s has not reached PlanePromoted (current plane: %s)", e.URN, unitID, unit.Plane)
		}
	}
	return nil
}

// ADR models an Architectural Decision Record in the cellular schema.
type ADR struct {
	dna.BaseObject  `yaml:",inline" json:",inline"`
	dna.Auditable   `yaml:",inline" json:",inline"`
	dna.Lifecycle   `yaml:",inline" json:",inline"`
	Title           string   `json:"title" yaml:"title"`
	Context         string   `json:"context" yaml:"context"`
	Decision        string   `json:"decision" yaml:"decision"`
	Consequences    string   `json:"consequences" yaml:"consequences"`
	SupersededByRef string   `json:"superseded_by_ref,omitempty" yaml:"superseded_by_ref,omitempty"`
	RelatedRefs     []string `json:"related_refs,omitempty" yaml:"related_refs,omitempty"`
	GoalRefs        []string `json:"goal_refs,omitempty" yaml:"goal_refs,omitempty"`
	RequirementRefs []string `json:"requirement_refs,omitempty" yaml:"requirement_refs,omitempty"`
	WorkstreamRefs  []string `json:"workstream_refs,omitempty" yaml:"workstream_refs,omitempty"`
}

// NewADR constructs a new ADR entity.
func NewADR(id, title, contextText, decision, consequences string) (*ADR, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errfmt.Errorf("ADR ID cannot be empty")
	}
	if strings.TrimSpace(title) == "" {
		return nil, errfmt.Errorf("ADR title cannot be empty")
	}

	urn, err := dna.NewURN("project-mgmt", "adr", id)
	if err != nil {
		return nil, err
	}

	allowedTransitions := map[string][]string{
		"conceptual": {"originated", "archived"},
		"originated": {"proposed", "archived"},
		"proposed":   {"accepted", "rejected", "archived"},
		"accepted":   {"deprecated", "superseded", "archived"},
		"deprecated": {"archived"},
		"superseded": {"archived"},
	}

	adr := &ADR{
		BaseObject:      dna.NewBaseObject(urn, "urn:zqk:spec:project-mgmt:adr"),
		Lifecycle:       dna.NewLifecycle("conceptual", allowedTransitions),
		Title:           title,
		Context:         contextText,
		Decision:        decision,
		Consequences:    consequences,
		RelatedRefs:     make([]string, 0),
		GoalRefs:        make([]string, 0),
		RequirementRefs: make([]string, 0),
		WorkstreamRefs:  make([]string, 0),
	}

	// Register invariant gate for superseded state requiring valid reference
	adr.RegisterInvariantGate("superseded", func(ctx context.Context, obj any) error {
		a, ok := obj.(*ADR)
		if !ok {
			return errfmt.Errorf("expected *ADR")
		}
		if strings.TrimSpace(a.SupersededByRef) == "" {
			return errfmt.Errorf("cannot transition ADR to superseded without specifying SupersededByRef")
		}
		return nil
	})

	return adr, nil
}
