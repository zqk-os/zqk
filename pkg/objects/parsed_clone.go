package objects

import "maps"

// Clone returns a deep copy of the ParsedObject, safe for modification
func (p *ParsedObject) Clone() *ParsedObject {
	if p == nil {
		return nil
	}

	clone := &ParsedObject{
		ID:            p.ID,
		Kind:          p.Kind,
		NamespaceID:   p.NamespaceID,
		Status:        p.Status,
		Title:         p.Title,
		Category:      p.Category,
		Priority:      p.Priority,
		SchemaVersion: p.SchemaVersion,
		TargetKind:    p.TargetKind,
		TargetID:      p.TargetID,
		Operation:     p.Operation,
		Severity:      p.Severity,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}

	// Deep clone Raw map
	if p.Raw != nil {
		clone.Raw = make(map[string]any, len(p.Raw))
		maps.Copy(clone.Raw, p.Raw)
	}

	// Clone slices
	if p.StatusHistory != nil {
		clone.StatusHistory = make([]StatusHistoryEntry, len(p.StatusHistory))
		copy(clone.StatusHistory, p.StatusHistory)
	}
	if p.ChangeLog != nil {
		clone.ChangeLog = make([]ChangeLogEntry, len(p.ChangeLog))
		copy(clone.ChangeLog, p.ChangeLog)
	}

	// String slices
	clone.GoalRefs = cloneStringSlice(p.GoalRefs)
	clone.MilestoneRefs = cloneStringSlice(p.MilestoneRefs)
	clone.RequirementRefs = cloneStringSlice(p.RequirementRefs)
	clone.WorkstreamRefs = cloneStringSlice(p.WorkstreamRefs)
	clone.BacklogItemRefs = cloneStringSlice(p.BacklogItemRefs)
	clone.CriteriaRefs = cloneStringSlice(p.CriteriaRefs)
	clone.TestCaseRefs = cloneStringSlice(p.TestCaseRefs)
	clone.DocEntryRefs = cloneStringSlice(p.DocEntryRefs)
	clone.Artifacts = cloneStringSlice(p.Artifacts)
	clone.Dependencies = cloneStringSlice(p.Dependencies)
	clone.Stakeholders = cloneStringSlice(p.Stakeholders)
	clone.Questions = cloneStringSlice(p.Questions)

	return clone
}

func cloneStringSlice(s []string) []string {
	if s == nil {
		return nil
	}
	c := make([]string, len(s))
	copy(c, s)
	return c
}
