package semantic

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/semantic"
)

// NewInferCmd creates the semantic infer command
func NewInferCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSemanticInferCommandBuilder()
	cmd.Flags().Bool("apply", false, "Apply discovered inferences to the knowledge kernel")
	cmd.Flags().String("session-id", "", "Filter inferences by convergence session ID")
	cmd.Args = cobra.NoArgs
	cmd.RunE = runInfer
	return cmd
}

func runInfer(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		projectRoot := proc.ProjectRoot()
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found")
		}

		apply, err := cmd.Flags().GetBool("apply")
		if err != nil {
			return errfmt.Newf("failed to get apply flag").Wrap(err)
		}
		sessionID, err := cmd.Flags().GetString("session-id")
		if err != nil {
			return errfmt.Newf("failed to get session-id flag").Wrap(err)
		}

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
		out.WriteString("Strategic Inferences & Proposed Backlog Items\n")
		out.WriteString("=============================================\n")
		for _, inf := range inferences {
			out.WriteString(fmt.Sprintf("- [%s] %s\n", inf[objects.FieldKeyPriorityTier], inf[objects.FieldKeyTitle]))
			out.WriteString(fmt.Sprintf("  Description: %s\n\n", inf[objects.FieldKeyDescription]))
		}
		out.WriteString("Use --apply to create these items in the Knowledge Kernel.\n")
		return cli.WriteOutput(cmd, []byte(out.String()))
	}
}
