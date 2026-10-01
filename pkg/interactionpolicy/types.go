package interactionpolicy

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// BacklogItemSummary represents a typed, lightweight projection of a backlog item
// used for interaction policy decisions and ambience detection.
type BacklogItemSummary struct {
	ID              string   `json:"id"`
	Status          string   `json:"status"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
	CreatedAt       string   `json:"created_at,omitempty"`
	CriteriaRefs    []string `json:"criteria_refs,omitempty"`
	Title           string   `json:"title,omitempty"`
	PriorityPlanRef string   `json:"priority_plan_ref,omitempty"`
}

// TestCaseSummary represents a typed projection of a test case for linkage resolution.
type TestCaseSummary struct {
	ID              string   `json:"id"`
	Status          string   `json:"status,omitempty"`
	BacklogItemRefs []string `json:"backlog_item_refs,omitempty"`
	CriteriaRefs    []string `json:"criteria_refs,omitempty"`
	Title           string   `json:"title,omitempty"`
}

// AgentTaskSummary represents a typed projection of an agent task for stale detection.
type AgentTaskSummary struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	BacklogItemRef  string `json:"backlog_item_ref,omitempty"`
	PriorityPlanRef string `json:"priority_plan_ref,omitempty"`
	ClaimedBy       string `json:"claimed_by,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
	ClaimedAt       string `json:"claimed_at,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
}

// PolicyDocument represents a typed projection of a CAS policy object.
type PolicyDocument struct {
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	Status      string `json:"status,omitempty"`
	Body        string `json:"body,omitempty"`
	Description string `json:"description,omitempty"`
}

// safeString extracts trimmed string from an untyped value.
func safeString(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// BacklogItemSummaryFromMap safely converts a raw map into a typed BacklogItemSummary.
func BacklogItemSummaryFromMap(m map[string]any) BacklogItemSummary {
	if m == nil {
		return BacklogItemSummary{}
	}
	return BacklogItemSummary{
		ID:              safeString(m[objects.FieldKeyID]),
		Status:          safeString(m[objects.FieldKeyStatus]),
		UpdatedAt:       safeString(m[objects.FieldKeyUpdatedAt]),
		CreatedAt:       safeString(m[objects.FieldKeyCreatedAt]),
		CriteriaRefs:    stringSliceFromAny(m[objects.FieldKeyCriteriaRefs]),
		Title:           safeString(m[objects.FieldKeyTitle]),
		PriorityPlanRef: safeString(m[objects.FieldKeyPriorityPlanRef]),
	}
}

// BacklogItemSummariesFromMaps converts a slice of raw maps into BacklogItemSummary slice.
func BacklogItemSummariesFromMaps(blis []map[string]any) []BacklogItemSummary {
	if blis == nil {
		return nil
	}
	out := make([]BacklogItemSummary, 0, len(blis))
	for _, m := range blis {
		out = append(out, BacklogItemSummaryFromMap(m))
	}
	return out
}

// TestCaseSummaryFromMap safely converts a raw map into a typed TestCaseSummary.
func TestCaseSummaryFromMap(m map[string]any) TestCaseSummary {
	if m == nil {
		return TestCaseSummary{}
	}
	return TestCaseSummary{
		ID:              safeString(m[objects.FieldKeyID]),
		Status:          safeString(m[objects.FieldKeyStatus]),
		BacklogItemRefs: stringSliceFromAny(m[objects.FieldKeyBacklogItemRefs]),
		CriteriaRefs:    stringSliceFromAny(m[objects.FieldKeyCriteriaRefs]),
		Title:           safeString(m[objects.FieldKeyTitle]),
	}
}

// TestCaseSummariesFromMaps converts a slice of raw maps into TestCaseSummary slice.
func TestCaseSummariesFromMaps(tcs []map[string]any) []TestCaseSummary {
	if tcs == nil {
		return nil
	}
	out := make([]TestCaseSummary, 0, len(tcs))
	for _, m := range tcs {
		out = append(out, TestCaseSummaryFromMap(m))
	}
	return out
}

// AgentTaskSummaryFromMap safely converts a raw map into a typed AgentTaskSummary.
func AgentTaskSummaryFromMap(m map[string]any) AgentTaskSummary {
	if m == nil {
		return AgentTaskSummary{}
	}
	return AgentTaskSummary{
		ID:              safeString(m[objects.FieldKeyID]),
		Status:          safeString(m[objects.FieldKeyStatus]),
		BacklogItemRef:  safeString(m[objects.FieldKeyBacklogItemRef]),
		PriorityPlanRef: safeString(m[objects.FieldKeyPriorityPlanRef]),
		ClaimedBy:       safeString(m[objects.FieldKeyClaimedBy]),
		UpdatedAt:       safeString(m[objects.FieldKeyUpdatedAt]),
		ClaimedAt:       safeString(m[objects.FieldKeyClaimedAt]),
		CreatedAt:       safeString(m[objects.FieldKeyCreatedAt]),
	}
}

// AgentTaskSummariesFromMaps converts a slice of raw maps into AgentTaskSummary slice.
func AgentTaskSummariesFromMaps(tasks []map[string]any) []AgentTaskSummary {
	if tasks == nil {
		return nil
	}
	out := make([]AgentTaskSummary, 0, len(tasks))
	for _, m := range tasks {
		out = append(out, AgentTaskSummaryFromMap(m))
	}
	return out
}

// PolicyDocumentFromMap safely converts a raw map into a typed PolicyDocument.
func PolicyDocumentFromMap(m map[string]any) PolicyDocument {
	if m == nil {
		return PolicyDocument{}
	}
	return PolicyDocument{
		ID:          safeString(m[objects.FieldKeyID]),
		Title:       safeString(m[objects.FieldKeyTitle]),
		Status:      safeString(m[objects.FieldKeyStatus]),
		Body:        safeString(m[objects.FieldKeyBody]),
		Description: safeString(m[objects.FieldKeyDescription]),
	}
}
