package ambient

import (
	"encoding/json"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func newStatusCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAmbientStatusCommandBuilder()
	cmd.RunE = runStatus
	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	statusPayload := map[string]any{
		objects.FieldKeyComponent: "ambient",
	}

	if pid, running := readAmbientPID(projectRoot); running {
		statusPayload[objects.FieldKeyStatus] = "running"
		statusPayload["pid"] = pid
		statusPayload["project_root"] = projectRoot
	} else {
		statusPayload[objects.FieldKeyStatus] = "stopped"
	}

	metricsFile := filepath.Join(paths.StateDirPath(projectRoot), "ambient", "convergence_metrics.json")
	if data, err := fileutil.ReadFile(metricsFile); err == nil {
		var metrics map[string]any
		if err := json.Unmarshal(data, &metrics); err == nil {
			statusPayload["convergence"] = metrics
		}
	}

	return cli.FormatOutput(cmd, statusPayload)
}
