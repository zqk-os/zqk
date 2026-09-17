package semantic

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// InferenceEngine analyzes maturity data to suggest actionable backlog items.
type InferenceEngine struct {
	store storage.ObjectStorageProvider
}

// NewInferenceEngine creates a new InferenceEngine.
func NewInferenceEngine(store storage.ObjectStorageProvider) *InferenceEngine {
	return &InferenceEngine{
		store: store,
	}
}

// Infer generates backlog_item objects based on a MaturityAssessment.
func (e *InferenceEngine) Infer(ctx context.Context, assessment *MaturityAssessment) []map[string]any {
	var inferences []map[string]any

	// 1. Static fallback logic (legacy support)
	if assessment.Level == 0 { // Naive
		inferences = append(inferences, e.createBacklogItem(
			"Standardize Core Objects",
			"The organization is at Level 0 (Naive). Create structured YAML files for core business objects to reach Level 1.",
			"P1",
		))
	}

	// 2. Dynamic heuristics from storage
	if e.store != nil {
		secCtx := pkgctx.NewSystemSecurityContext()
		list, err := e.store.List(ctx, secCtx, nil, storage.ListFilter{
			Kind: objects.KindInferenceHeuristic,
			Filters: map[string]any{
				objects.FieldKeyTargetMaturityLevel: assessment.Level,
			},
		})
		if err == nil {
			for _, obj := range list.Objects {
				title, _ := obj[objects.FieldKeyProposedTitle].(string)
				desc, _ := obj[objects.FieldKeyProposedDescription].(string)
				priority, _ := obj[objects.FieldKeyProposedPriority].(string)

				if title != "" {
					inferences = append(inferences, e.createBacklogItem(title, desc, priority))
				}
			}
		}
	}

	return inferences
}

// InferFromConvergence generates backlog_item objects based on failing convergence sessions.
func (e *InferenceEngine) InferFromConvergence(results []ConvergenceResult) []map[string]any {
	var inferences []map[string]any

	for _, res := range results {
		if res.TrendingAway {
			inferences = append(inferences, e.createBacklogItem(
				fmt.Sprintf("Remediate %s", res.Title),
				fmt.Sprintf("Session %s is trending away from its goal. Immediate investigation of failing fingerprints is required.", res.SessionID),
				"P1",
			))
		}
	}

	return inferences
}

func (e *InferenceEngine) createBacklogItem(title, description, priority string) map[string]any {
	id := fmt.Sprintf("BLI-INF-%s", strings.ToUpper(strings.ReplaceAll(title[:10], " ", "-")))
	return map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         title,
		objects.FieldKeyDescription:   description,
		objects.FieldKeyPriorityTier:  priority,
		objects.FieldKeyStatus:        "exploring",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
}
