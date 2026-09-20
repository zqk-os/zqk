package workflow

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewLinkCmd creates the workflow link command.
func NewLinkCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewWorkflowLinkCommandBuilder()
	cli.BindAsyncProgress(cmd, runLinkCommand)
	cli.RequireStorage(cmd, true)
	return cmd
}

func extractTargetCriteria(pipelineRefID string, pipelineObj map[string]any) (string, error) {
	kind, _ := pipelineObj[objects.FieldKeyKind].(string)

	if kind == objects.KindCriteria {
		return pipelineRefID, nil
	}

	if kind == objects.KindRequirement || kind == objects.KindGoal || kind == objects.KindMilestone {
		if refs, ok := pipelineObj[objects.FieldKeyCriteriaRefs].([]any); ok && len(refs) > 0 {
			return fmt.Sprint(refs[0]), nil
		}
		return "", fmt.Errorf("target pipeline object %s has no criteria established yet", pipelineRefID)
	}

	return "", fmt.Errorf("unsupported pipeline reference kind: %s", kind)
}

func runLinkCommand(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		ctx := proc.OperationContext()
		storageProvider := proc.Storage()
		secCtx := proc.SecurityContext()

		if len(args) < 2 {
			return fmt.Errorf("missing arguments. Usage: workflow link <object-id> <pipeline-ref>")
		}
		objectID := args[0]
		pipelineRefID := args[1]

		// Read the pipeline ref
		pipelineObj, err := storageProvider.Read(ctx, secCtx, pipelineRefID)
		if err != nil {
			return fmt.Errorf("failed to load pipeline reference %s: %w", pipelineRefID, err)
		}

		// Extract target criteria
		targetCriteria, err := extractTargetCriteria(pipelineRefID, pipelineObj)
		if err != nil {
			return err
		}

		// Read the object to link
		objToLink, err := storageProvider.Read(ctx, secCtx, objectID)
		if err != nil {
			return fmt.Errorf("failed to load object %s: %w", objectID, err)
		}

		// Check if it already has this criteria
		var existingCriteria []string
		if refs, ok := objToLink[objects.FieldKeyCriteriaRefs].([]any); ok {
			for _, r := range refs {
				if fmt.Sprint(r) == targetCriteria {
					fmt.Fprintf(cmd.OutOrStdout(), "Object %s is already linked to criteria %s\n", objectID, targetCriteria)
					return nil
				}
				existingCriteria = append(existingCriteria, fmt.Sprint(r))
			}
		}

		// Append the new link
		existingCriteria = append(existingCriteria, targetCriteria)

		// Convert string slice to any slice for JSON storage
		var updatedRefs []any
		for _, c := range existingCriteria {
			updatedRefs = append(updatedRefs, c)
		}

		objToLink[objects.FieldKeyCriteriaRefs] = updatedRefs

		// Bypass strict edge validation to allow loosely attached orphans
		if err := storageProvider.Update(ctx, secCtx, objectID, objToLink); err != nil {
			return fmt.Errorf("failed to update object %s: %w", objectID, err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Successfully linked %s into pipeline (via criteria %s)\n", objectID, targetCriteria)
		return nil
	})(cmd, args)
}
