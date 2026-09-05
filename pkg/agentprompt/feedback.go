package agentprompt

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// AutonomousFeedback represents the compiled corrective feedback for an agent.
type AutonomousFeedback struct {
	FeedbackItems []map[string]any
}

// LoadAutonomousFeedback fetches active metrics_feedback objects from the Knowledge Kernel.
func LoadAutonomousFeedback(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) (*AutonomousFeedback, error) {
	storageCtx := &pkgctx.StorageContext{}

	// Fetch recent feedback (in a real system, we might filter by status or relevance to the plan)
	filter := storage.ListFilter{
		Kind:  objects.KindMetricsFeedback,
		Limit: 10,
	}

	result, err := sp.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf("failed to list autonomous feedback from kernel").Wrap(err)
	}

	return &AutonomousFeedback{
		FeedbackItems: result.Objects,
	}, nil
}

// GeneratePromptSection formats the autonomous feedback into a markdown section.
func (f *AutonomousFeedback) GeneratePromptSection() string {
	if len(f.FeedbackItems) == 0 {
		return "## Autonomous Corrective Feedback\nNo outstanding system-generated feedback items.\n"
	}

	var sb strings.Builder
	sb.WriteString("## Autonomous Corrective Feedback\n")
	sb.WriteString("The system has analyzed previous execution metrics and generated the following corrective feedback. You MUST incorporate these improvements into your current work:\n\n")

	for _, item := range f.FeedbackItems {
		title, _ := item[objects.FieldKeyTitle].(string)
		id, _ := item[objects.FieldKeyID].(string)
		report, _ := item[objects.FieldKeyTargetReport].(string)

		actions, _ := item[objects.FieldKeySuggestedActions].([]any)

		sb.WriteString(fmt.Sprintf("### %s (%s)\n", title, id))
		if report != "" {
			sb.WriteString(fmt.Sprintf("- **Context**: Derived from %s\n", report))
		}

		if len(actions) > 0 {
			sb.WriteString("- **Suggested Actions**:\n")
			for _, action := range actions {
				sb.WriteString(fmt.Sprintf("  - %v\n", action))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
