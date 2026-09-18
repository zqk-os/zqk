package reports

import (
	"fmt"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
	"github.com/spf13/cobra"
)

const (
	presetQuestions         = "questions"
	presetMilestonesOverdue = "milestones-overdue"
)

// NewQuickCmd creates the "reports quick" command for lifecycle review presets (BLI-032).
func NewQuickCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Quick report with preset (questions, milestones-overdue)",
		"Generate a quick report for lifecycle reviews. Presets: questions (unanswered), milestones-overdue (past target_date).",
		"",
		"Presets:",
		"  questions         - Unanswered questions (status, owner, next steps, due date)",
		"  milestones-overdue - Milestones past target_date with assigned owners",
	).
		AddExample("Unanswered questions", "%s reports quick --preset questions").
		AddExample("Overdue milestones", "%s reports quick --preset milestones-overdue").
		AddExample("Exit non-zero when SLA exceeded", "%s reports quick --preset questions --strict")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewReportsQuickCommandBuilder(), &cobra.Command{
		Use:  "quick",
		Args: cobra.NoArgs,
		Annotations: map[string]string{
			"mcp.permissions": "read:objects",
		},
	})
	helpBuilder.ApplyToCommand(cmd)
	cli.BindAsyncProgress(cmd, runQuick)

	cmd.Flags().String("preset", "", "Preset: questions | milestones-overdue (required)")
	_ = cmd.MarkFlagRequired("preset")
	cmd.Flags().Bool("strict", false, "Exit with non-zero if any critical row exceeds SLA (e.g. unanswered P0 question, overdue milestone)")
	cli.AddCommonFlags(cmd)

	return cmd
}

func runQuick(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var err error
		_ = err

		preset, _ := cmd.Flags().GetString("preset")
		strict, _ := cmd.Flags().GetBool("strict")

		switch preset {
		case presetQuestions:
			return runQuickQuestions(cmd, proc, strict)
		case presetMilestonesOverdue:
			return runQuickMilestonesOverdue(cmd, proc, strict)
		default:
			return cli.Guard(cmd).Err(errfmt.Errorf("unknown preset %q (use: questions, milestones-overdue)", preset)).Return()
		}
	})(cmd, nil)
}

func runQuickQuestions(cmd *cobra.Command, proc *cli.Processor, strict bool) error {
	// Unanswered = not resolved, deferred, or error
	secCtx := proc.SecurityContext()
	storageCtx := proc.StorageContext()
	listFilter := storage.ListFilter{
		Kind: quickKindQuestion,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{"$nin": []any{quickStatusResolved, quickStatusDeferred, quickStatusError}},
		},
		Limit: 0,
	}

	result, err := proc.Storage().List(proc.OperationContext(), secCtx, storageCtx, listFilter)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to list questions: %w").Return()
	}

	rows := make([]map[string]any, 0, len(result.Objects))
	hasCritical := false
	for _, obj := range result.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		status, _ := obj[objects.FieldKeyStatus].(string)
		owner := ""
		if o, ok := obj[objects.FieldKeyUpdatedBy].(string); ok && o != emptyValue {
			owner = o
		} else if o, ok := obj[objects.FieldKeyOwnerRef].(string); ok && o != emptyValue {
			owner = o
		}
		due := ""
		if d, ok := obj[objects.FieldKeyAnswerDueBy].(string); ok {
			due = d
		}
		summary := ""
		if q, ok := obj[objects.FieldKeyQuestionText].(string); ok {
			summary = q
			if len(summary) > 60 {
				summary = summary[:57] + "..."
			}
		}
		nextSteps := ""
		if refs, ok := obj[objects.FieldKeyBlockingMilestoneRefs].([]any); ok && len(refs) > 0 {
			nextSteps = fmt.Sprintf("Blocking %d milestone(s)", len(refs))
		}
		priority := ""
		if p, ok := obj[objects.FieldKeyPriorityTier].(string); ok {
			priority = p
			if priority == quickPriorityP0 {
				hasCritical = true
			}
		}

		rows = append(rows, map[string]any{
			objects.FieldKeyID: id, objects.FieldKeyStatus: status, "owner": owner, "due_date": due,
			"next_steps": nextSteps, objects.FieldKeySummary: summary, objects.FieldKeyPriorityTier: priority,
		})
	}

	out := map[string]any{
		"preset": "questions",
		"rows":   rows,
		"meta":   map[string]any{"total_count": len(rows), "critical_exceeded": hasCritical},
	}

	if strict && hasCritical {
		return outputQuickAndExit(cmd, out, 1)
	}
	return outputQuick(cmd, out)
}

func runQuickMilestonesOverdue(cmd *cobra.Command, proc *cli.Processor, strict bool) error {
	secCtx := proc.SecurityContext()
	storageCtx := proc.StorageContext()
	today := zqktime.NowLayoutUTC(zqktime.LayoutDate)

	// Milestones not complete/deferred/archived with target_date < today
	listFilter := storage.ListFilter{
		Kind: quickKindMilestone,
		Filters: map[string]any{
			objects.FieldKeyStatus:     map[string]any{"$nin": []any{quickStatusComplete, quickStatusDeferred, quickStatusArchived}},
			objects.FieldKeyTargetDate: map[string]any{"$lt": today},
		},
		SortBy:  objects.FieldKeyTargetDate,
		SortAsc: true,
		Limit:   0,
	}

	result, err := proc.Storage().List(proc.OperationContext(), secCtx, storageCtx, listFilter)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to list milestones: %w").Return()
	}

	rows := make([]map[string]any, 0, len(result.Objects))
	for _, obj := range result.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		title, _ := obj[objects.FieldKeyTitle].(string)
		target, _ := obj[objects.FieldKeyTargetDate].(string)
		status, _ := obj[objects.FieldKeyStatus].(string)
		owner := ""
		if o, ok := obj[objects.FieldKeyUpdatedBy].(string); ok && o != emptyValue {
			owner = o
		} else if o, ok := obj[objects.FieldKeyOwnerRef].(string); ok && o != emptyValue {
			owner = o
		}
		rows = append(rows, map[string]any{
			objects.FieldKeyID: id, objects.FieldKeyTitle: title, objects.FieldKeyTargetDate: target, objects.FieldKeyStatus: status, "owner": owner,
		})
	}

	criticalExceeded := len(rows) > 0
	out := map[string]any{
		"preset": "milestones-overdue",
		"rows":   rows,
		"meta":   map[string]any{"total_count": len(rows), "critical_exceeded": criticalExceeded},
	}

	if strict && criticalExceeded {
		return outputQuickAndExit(cmd, out, 1)
	}
	return outputQuick(cmd, out)
}

func outputQuick(cmd *cobra.Command, data map[string]any) error {
	if err := cli.FormatOutput(cmd, data); err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("format output: %w").Return()
	}
	return nil
}

// errExitCode is returned when --strict and SLA is exceeded so the process exits non-zero.
type errExitCode int

func (e errExitCode) Error() string { return "critical SLA exceeded" }

// outputQuickAndExit writes output then returns an error so the process exits non-zero (--strict).
func outputQuickAndExit(cmd *cobra.Command, data map[string]any, code int) error {
	if err := outputQuick(cmd, data); err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}
	return errExitCode(code)
}
