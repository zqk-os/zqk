package teleology

import (
	"math"
)

// TeleologicalAlignmentReport summarizes teleological alignment across a set of objects.
type TeleologicalAlignmentReport struct {
	TotalItems        int              `json:"total_items"`
	AlignedCount      int              `json:"aligned_count"`
	DriftCount        int              `json:"drift_count"`
	DriftPercentage   float64          `json:"drift_pct"`
	TeleologicalScore float64          `json:"teleological_score"`
	Status            string           `json:"status"` // "ALIGNED" or "TELEOLOGICAL_DRIFT_DETECTED"
	TaxonomyBoundary  string           `json:"taxonomy_boundary"`
	InScopeDomains    []string         `json:"in_scope_domains,omitempty"`
	ProhibitedDomains []string         `json:"prohibited_domains,omitempty"`
	DriftReceipts     []map[string]any `json:"drift_receipts,omitempty"`
}

// EvaluateWorkItemsTeleologicalDrift inspects work items against the taxonomy
// and calculates the teleological alignment score and drift report.
func EvaluateWorkItemsTeleologicalDrift(items []map[string]any, taxonomy *TaxonomyConfig) *TeleologicalAlignmentReport {
	boundary := "MIS-*"
	if taxonomy != nil && taxonomy.MissionRef != "" {
		boundary = taxonomy.MissionRef
	}

	report := &TeleologicalAlignmentReport{
		TotalItems:        len(items),
		TaxonomyBoundary:  boundary,
		Status:            "ALIGNED",
		TeleologicalScore: 100.0,
	}

	if taxonomy != nil {
		report.InScopeDomains = taxonomy.InScope
		report.ProhibitedDomains = taxonomy.Prohibited
	}

	if taxonomy == nil || !taxonomy.Enabled || len(items) == 0 {
		report.AlignedCount = len(items)
		return report
	}

	var driftReceipts []map[string]any

	for _, item := range items {
		receipt := EvaluateTeleologicalAlignment(item, taxonomy)
		if receipt.Status == "TELEOLOGICAL_DRIFT_REFUSED" {
			report.DriftCount++
			id, _ := item["id"].(string)
			title, _ := item["title"].(string)
			driftReceipts = append(driftReceipts, map[string]any{
				"id":                id,
				"title":             title,
				"status":            receipt.Status,
				"cause":             receipt.Cause,
				"boundary":          receipt.Boundary,
				"violating_domains": receipt.ViolatingDomains,
			})
		} else {
			report.AlignedCount++
		}
	}

	if report.DriftCount > 0 {
		report.Status = "TELEOLOGICAL_DRIFT_DETECTED"
	}

	if report.TotalItems > 0 {
		report.DriftPercentage = math.Round((float64(report.DriftCount)/float64(report.TotalItems)*100.0)*100) / 100
		alignedRatio := float64(report.AlignedCount) / float64(report.TotalItems)
		report.TeleologicalScore = math.Round((alignedRatio*100.0)*100) / 100
	}

	report.DriftReceipts = driftReceipts
	return report
}
