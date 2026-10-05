// spec_origination.go: spec.origination pipeline (DATA_ORIGINATION_PIPELINE_VISION.md).
// Command structure from spec: .zqk/cli/specs/system/spec_origination_command.yaml (builder: bldr_cli_cmd_v1.NewSystemSpecOriginationCommandBuilder).
package system

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/specorigination"
)

// NewSpecOriginationCmd runs the spec origination pipeline for one kind.
func NewSpecOriginationCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemSpecOriginationCommandBuilder(), &cobra.Command{Use: "spec-origination"})
	cli.BindAsyncProgress(cmd, runSpecOriginationAsync)
	return cmd
}

func runSpecOriginationAsync(cmd *cobra.Command, _ []string) error {
	ctx, logger := resolveCommandLogger(cmd, systemProfileHuman)
	_ = ctx

	ontology, _ := cmd.Flags().GetString("ontology")
	if ontology == emptyValue {
		return errfmt.Errorf("--ontology is required")
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	skipMaterializeIndex, _ := cmd.Flags().GetBool("skip-materialize-spec-index")
	materializeFieldKeys, _ := cmd.Flags().GetBool("materialize-field-keys")
	skipFinalizeValidation, _ := cmd.Flags().GetBool("skip-finalize-validation")
	applyTrigger, _ := cmd.Flags().GetBool("apply-trigger")

	projectRoot := ""
	if ctx != nil {
		projectRoot = ctx.ProjectRoot
	}
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found; run from a zqk project")
	}

	opts := specorigination.Options{
		ProjectRoot:              projectRoot,
		Ontology:                 ontology,
		DryRun:                   dryRun,
		SkipMaterializeSpecIndex: skipMaterializeIndex,
		MaterializeFieldKeys:     materializeFieldKeys,
		SkipFinalizeValidation:   skipFinalizeValidation,
		ApplyTrigger:             applyTrigger,
	}

	pctx := &pipeline.Context{Ctx: cmd.Context(), Outcome: make(map[string]any)}
	st, err := specorigination.Run(pctx, logger, opts)
	if err != nil {
		return errfmt.Newf("spec origination").Wrap(err)
	}

	logging.Fluent(logger).Info("spec origination completed").
		String("ontology", ontology).
		String("spec_path", st.SpecPath).
		Log()
	return cli.FormatOutput(cmd, map[string]any{
		objects.FieldKeyOntology: ontology,
		"spec_path":              st.SpecPath,
		"outcome":                pctx.Outcome,
	})
}
