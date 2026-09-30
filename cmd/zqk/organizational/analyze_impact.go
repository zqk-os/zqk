package organizational

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	orgdomain "github.com/zqk-os/zqk/pkg/domain/organizational"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

const emptyValue = ""

// NewAnalyzeImpactCmd creates the analyze-impact command from the generated builder.
func NewAnalyzeImpactCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewOrganizationalAnalyzeImpactCommandBuilder()
	cli.BindAsyncProgress(cmd, runAnalyzeImpact)
	return cmd
}

func runAnalyzeImpact(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		changeID, _ := cmd.Flags().GetString("change")
		if changeID == emptyValue {
			return errfmt.Errorf("--change is required")
		}

		secCtx := proc.SecurityContext()
		domainLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		analyzer := orgdomain.NewImpactAnalyzer(storage.NewOrganizationalStorageAdapter(proc.Storage()), domainLogger, secCtx)

		impactAnalysisID, err := analyzer.AnalyzeChange(proc.OperationContext(), changeID)
		if err != nil {
			return err
		}

		impactAnalysis, err := proc.Storage().Read(proc.OperationContext(), secCtx, impactAnalysisID)
		if err != nil {
			logging.Fluent(domainLogger).Warn("Failed to read created impact analysis for output").WithError(err).Log()
			output := fmt.Sprintf("Impact analysis created: %s\n", impactAnalysisID)
			return cli.WriteOutput(cmd, []byte(output))
		}

		changeRef, _ := impactAnalysis[objects.FieldKeyChangeRef].(string)
		changeType, _ := impactAnalysis[objects.FieldKeyChangeType].(string)
		affectedObjects, _ := impactAnalysis[objects.FieldKeyAffectedObjects].(map[string]any)

		output := fmt.Sprintf("Impact analysis created: %s\n", impactAnalysisID)
		output += fmt.Sprintf("Change: %s (%s)\n", changeRef, changeType)
		output += fmt.Sprintf("Affected object types: %d\n", len(affectedObjects))
		for objType, objList := range affectedObjects {
			if list, ok := objList.([]any); ok {
				output += fmt.Sprintf("  - %s: %d objects\n", objType, len(list))
			}
		}

		return cli.WriteOutput(cmd, []byte(output))
	})(cmd, args)
}
