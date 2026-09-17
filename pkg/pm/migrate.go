package pm

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/dna"
	"github.com/lanceman/zqk/pkg/errfmt"
	"gopkg.in/yaml.v3"
)

// DefaultGenesisParentHash is the standard root genesis hash for legacy provenance blocks.
const DefaultGenesisParentHash = "0000000000000000000000000000000000000000000000000000000000000000"

// LegacyBacklogItem models the pre-cellular schema for legacy BacklogItems.
type LegacyBacklogItem struct {
	ID                       string     `json:"id" yaml:"id"`
	Kind                     string     `json:"kind" yaml:"kind"`
	NamespaceID              string     `json:"namespace_id" yaml:"namespace_id"`
	SchemaVersion            string     `json:"schema_version" yaml:"schema_version"`
	Status                   string     `json:"status" yaml:"status"`
	Title                    string     `json:"title" yaml:"title"`
	Description              string     `json:"description" yaml:"description"`
	ProblemStatement         string     `json:"problem_statement" yaml:"problem_statement"`
	AcceptanceConsiderations string     `json:"acceptance_considerations" yaml:"acceptance_considerations"`
	Priority                 string     `json:"priority" yaml:"priority"`
	PriorityTier             string     `json:"priority_tier" yaml:"priority_tier"`
	EstimatedEffort          string     `json:"estimated_effort" yaml:"estimated_effort"`
	ActualEffort             string     `json:"actual_effort" yaml:"actual_effort"`
	StartedAt                *time.Time `json:"started_at" yaml:"started_at"`
	CompletedAt              *time.Time `json:"completed_at" yaml:"completed_at"`
	CreatedAt                *time.Time `json:"created_at" yaml:"created_at"`
	CreatedBy                string     `json:"created_by" yaml:"created_by"`
	UpdatedAt                *time.Time `json:"updated_at" yaml:"updated_at"`
	UpdatedBy                string     `json:"updated_by" yaml:"updated_by"`
	CommitHashes             []string   `json:"commit_hashes" yaml:"commit_hashes"`
	RequirementRefs          []string   `json:"requirement_refs" yaml:"requirement_refs"`
	CriteriaRefs             []string   `json:"criteria_refs" yaml:"criteria_refs"`
	MilestoneRefs            []string   `json:"milestone_refs" yaml:"milestone_refs"`
	WorkstreamRefs           []string   `json:"workstream_refs" yaml:"workstream_refs"`
	PersonaRefs              []string   `json:"persona_refs" yaml:"persona_refs"`
	PriorityPlanRef          string     `json:"priority_plan_ref" yaml:"priority_plan_ref"`
	GoalRefs                 []string   `json:"goal_refs" yaml:"goal_refs"`
	VersionContext           string     `json:"version_context" yaml:"version_context"`
}

// ExtractCellID extracts a valid cellular cell name from a legacy namespace identifier.
func ExtractCellID(namespaceID string) string {
	trimmed := strings.TrimSpace(namespaceID)
	if trimmed == "" {
		return "kernel"
	}
	parts := strings.Split(trimmed, ":")
	if len(parts) > 1 && parts[1] != "" {
		return parts[1]
	}
	return parts[0]
}

// MigrateLegacyBacklogItem ingests raw YAML or JSON data of a legacy BacklogItem
// and produces a valid cellular BacklogItem composing BaseObject, Auditable (with Genesis Provenance), and Lifecycle.
func MigrateLegacyBacklogItem(rawData []byte) (*BacklogItem, error) {
	if len(rawData) == 0 {
		return nil, errfmt.Errorf("empty raw data for backlog item migration")
	}

	var legacy LegacyBacklogItem
	if err := yaml.Unmarshal(rawData, &legacy); err != nil {
		return nil, errfmt.Errorf("failed to unmarshal legacy backlog item: %w", err)
	}

	return ConvertLegacyBacklogItem(&legacy, rawData)
}

// ConvertLegacyBacklogItem converts a decoded LegacyBacklogItem struct into a cellular BacklogItem.
func ConvertLegacyBacklogItem(legacy *LegacyBacklogItem, rawPayload []byte) (*BacklogItem, error) {
	if legacy == nil {
		return nil, errfmt.Errorf("legacy backlog item cannot be nil")
	}
	if strings.TrimSpace(legacy.ID) == "" {
		return nil, errfmt.Errorf("legacy backlog item missing id")
	}
	if strings.TrimSpace(legacy.Title) == "" {
		return nil, errfmt.Errorf("legacy backlog item missing title")
	}

	cellID := ExtractCellID(legacy.NamespaceID)
	urn, err := dna.NewURN(cellID, "backlog_item", legacy.ID)
	if err != nil {
		return nil, errfmt.Errorf("failed to construct cellular URN for migrated item: %w", err)
	}

	now := time.Now().UTC()
	createdAt := now
	if legacy.CreatedAt != nil && !legacy.CreatedAt.IsZero() {
		createdAt = legacy.CreatedAt.UTC()
	}
	updatedAt := now
	if legacy.UpdatedAt != nil && !legacy.UpdatedAt.IsZero() {
		updatedAt = legacy.UpdatedAt.UTC()
	}

	// Plane determination: promoted if already completed/archived or in persistent CAS
	plane := dna.PlanePromoted
	if legacy.Status == "draft" || legacy.Status == "conceptual" {
		plane = dna.PlaneDraft
	}

	base := dna.BaseObject{
		URN:       urn,
		Kind:      "backlog_item",
		SchemaRef: "2.0.0",
		Plane:     plane,
		Version:   1,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	// Synthesize Provenance block
	agentID := strings.TrimSpace(legacy.CreatedBy)
	if agentID == "" {
		agentID = strings.TrimSpace(legacy.UpdatedBy)
	}
	if agentID == "" {
		agentID = "legacy-migrator"
	}

	payloadHashBytes := sha256.Sum256(rawPayload)
	payloadHash := hex.EncodeToString(payloadHashBytes[:])

	prov := dna.Provenance{
		ParentHash: DefaultGenesisParentHash,
		AgentID:    agentID,
		Hash:       payloadHash,
		Signature:  "migrated:" + payloadHash,
		Metadata: map[string]string{
			"migration_source": "legacy_v1",
			"migrated_at":      now.Format(time.RFC3339),
			"legacy_namespace": legacy.NamespaceID,
		},
	}

	allowedTransitions := map[string][]string{
		"conceptual":  {"originated", "archived"},
		"originated":  {"exploring", "archived"},
		"exploring":   {"planned", "deferred", "archived"},
		"planned":     {"testing", "in_progress", "deferred", "archived"},
		"testing":     {"in_progress", "planned", "archived"},
		"in_progress": {"complete", "planned", "error", "archived"},
		"complete":    {"archived"},
		"archived":    {},
	}

	status := strings.TrimSpace(legacy.Status)
	if status == "" {
		status = "planned"
	}

	lc := dna.NewLifecycle(status, allowedTransitions)

	// Ensure slice fields are non-nil for clean serialization
	nonNilSlice := func(s []string) []string {
		if s == nil {
			return make([]string, 0)
		}
		return s
	}

	return &BacklogItem{
		BaseObject:               base,
		Auditable:                dna.Auditable{Provenance: prov},
		Lifecycle:                lc,
		Title:                    legacy.Title,
		Description:              legacy.Description,
		ProblemStatement:         legacy.ProblemStatement,
		AcceptanceConsiderations: legacy.AcceptanceConsiderations,
		Priority:                 legacy.Priority,
		PriorityTier:             legacy.PriorityTier,
		EstimatedEffort:          legacy.EstimatedEffort,
		ActualEffort:             legacy.ActualEffort,
		StartedAt:                legacy.StartedAt,
		CompletedAt:              legacy.CompletedAt,
		CommitHashes:             nonNilSlice(legacy.CommitHashes),
		RequirementRefs:          nonNilSlice(legacy.RequirementRefs),
		CriteriaRefs:             nonNilSlice(legacy.CriteriaRefs),
		MilestoneRefs:            nonNilSlice(legacy.MilestoneRefs),
		WorkstreamRefs:           nonNilSlice(legacy.WorkstreamRefs),
		PersonaRefs:              nonNilSlice(legacy.PersonaRefs),
		PriorityPlanRef:          legacy.PriorityPlanRef,
		GoalRefs:                 nonNilSlice(legacy.GoalRefs),
	}, nil
}
