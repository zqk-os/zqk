package agent

import (
	"strings"

	"github.com/spf13/cobra"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	"github.com/zqk-os/zqk/pkg/seatworker"
)

// NewInstallSeatWorkersCmd creates zqk agent install-seat-workers.
func NewInstallSeatWorkersCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentInstallSeatWorkersCommandBuilder()
	cmd.RunE = cli.WithProcessor(runAgentInstallSeatWorkers)
	return cmd
}

func runAgentInstallSeatWorkers(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
	root := proc.ProjectRoot()
	if root == "" {
		return errfmt.Errorf("project root not found")
	}
	var flags clipkg.FlagBag
	seat := strings.TrimSpace(flags.String(cmd, "seat"))
	persona := strings.TrimSpace(flags.String(cmd, "persona-ref"))
	poll := flags.Int(cmd, "poll-seconds")
	execNC := flags.Bool(cmd, "execute-non-comms")
	writeOnly := flags.Bool(cmd, "write-only")
	if err := flags.Err(); err != nil {
		return err
	}
	cfg := seatworker.InstallConfig{
		ProjectRoot:     root,
		PersonaRef:      persona,
		PollSeconds:     poll,
		ExecuteNonComms: execNC,
		WriteOnly:       writeOnly,
	}
	if seat != "" {
		cfg.Seats = []string{seat}
	}
	if err := seatworker.Install(cmd.Context(), cfg); err != nil {
		return err
	}
	return cli.FormatOutput(cmd, map[string]any{
		"status":      "installed",
		"seated_root": root,
		"root_id":     hostservice.RootID(root),
		"write_only":  writeOnly,
	})
}
