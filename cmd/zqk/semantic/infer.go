package semantic

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/semantic"
)

// NewInferCmd creates the semantic infer command
func NewInferCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSemanticInferCommandBuilder()
	cmd.Args = cobra.NoArgs
	cmd.RunE = runInfer
	return cmd
}

func runInfer(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		projectRoot := proc.ProjectRoot()
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found")
		}

		apply, _ := cmd.Flags().GetBool("apply")
		sessionID, _ := cmd.Flags().GetString("session-id")

		// 1. Maturity Assessment
		assessment, err := runMaturityAssessment(projectRoot)
		if err != nil {
			return errfmt.Newf("assessment failed").Wrap(err)
		}

		// 2. Convergence & Drift Check
		reconciler := semantic.NewSemanticReconciler(proc.Storage())
		secCtx := proc.SecurityContext()

		convResults, err := reconciler.CheckConvergenceDrift(cmd.Context(), secCtx)
		if err != nil {
			return errfmt.Newf("convergence drift check failed").Wrap(err)
		}

		// Filter by session-id if provided
		if sessionID != "" {
			filtered := make([]semantic.ConvergenceResult, 0)
			for _, res := range convResults {
				if res.SessionID == sessionID {
					filtered = append(filtered, res)
				}
			}
			convResults = filtered
		}

		// 3. Inference
		engine := semantic.NewInferenceEngine(proc.Storage())
		inferences := engine.Infer(cmd.Context(), assessment)
		inferences = append(inferences, engine.InferFromConvergence(convResults)...)

		if apply {
			return applyInferences(cmd, proc, inferences)
		}

		return outputInferences(cmd, inferences)
	})(cmd, args)
}

func applyInferences(cmd *cobra.Command, proc *cli.Processor, inferences []map[string]any) error {
	if len(inferences) == 0 {
		return cli.WriteOutput(cmd, []byte("No inferences to apply.\n"))
	}

	store := proc.Storage()
	secCtx := proc.SecurityContext()

	created := 0
	for _, inf := range inferences {
		err := store.Create(cmd.Context(), secCtx, inf)
		if err != nil {
			// Skip if already exists, but report other errors
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			return errfmt.Newf("failed to create backlog item %v", inf[objects.FieldKeyID]).Wrap(err)
		}
		created++
	}

	return cli.WriteOutput(cmd, []byte(fmt.Sprintf("Successfully applied %d inferences to the backlog.\n", created)))
}

func outputInferences(cmd *cobra.Command, inferences []map[string]any) error {
	if len(inferences) == 0 {
		return cli.WriteOutput(cmd, []byte("No strategic inferences identified at this time.\n"))
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, inferences)
	default:
		var out strings.Builder
		_, _ = fmt.Fprintln(&out, "Strategic Inferences & Proposed Backlog Items")
		_, _ = fmt.Fprintln(&out, "=============================================")
		for _, inf := range inferences {
			_, _ = fmt.Fprintf(&out, "- [%s] %s\n", inf[objects.FieldKeyPriorityTier], inf[objects.FieldKeyTitle])
			_, _ = fmt.Fprintf(&out, "  Description: %s\n\n", inf[objects.FieldKeyDescription])
		}
		_, _ = fmt.Fprintln(&out, "Use --apply to create these items in the Knowledge Kernel.")
		return cli.WriteOutput(cmd, []byte(out.String()))
	}
}
