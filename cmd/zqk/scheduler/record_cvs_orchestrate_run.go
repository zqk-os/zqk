package scheduler

import (
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

// NewRecordCvsOrchestrateRunCmd wires RunE for record-cvs-orchestrate-run (spec-generated builder).
func NewRecordCvsOrchestrateRunCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSchedulerRecordCvsOrchestrateRunCommandBuilder()
	cmd.RunE = runRecordCvsOrchestrateRun
	return cmd
}

func runRecordCvsOrchestrateRun(cmd *cobra.Command, _ []string) error {
	cliCtx := cli.GetContext(cmd)
	if cliCtx == nil {
		return errfmt.Errorf("failed to get context")
	}
	projectRoot := cliCtx.ProjectRoot
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found (run from repo or set ZQK_PROJECT_ROOT)")
		}
	}

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return errfmt.Newf("read stdin").Wrap(err)
	}
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return errfmt.Errorf("stdin is empty (expected one JSON object for cvs_orchestrate_run_v1)")
	}

	var rec schedulerpkg.CVSOrchestrateRunV1
	if err := json.Unmarshal(raw, &rec); err != nil {
		return errfmt.Newf("parse stdin JSON").Wrap(err)
	}

	jsonlPath, _ := cmd.Flags().GetString("jsonl-path")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if dryRun {
		if rec.SchemaVersion != schedulerpkg.CVSOrchestrateRunSchemaVersion {
			return errfmt.Errorf("cvs_orchestrate_run: schema_version must be %s, got %s",
				schedulerpkg.CVSOrchestrateRunSchemaVersion, rec.SchemaVersion)
		}
		return nil
	}

	if err := schedulerpkg.AppendCVSOrchestrateRunV1(projectRoot, jsonlPath, &rec); err != nil {
		return err
	}

	return nil
}
