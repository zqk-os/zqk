package system

import (
	"fmt"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// NewSeedDefaultAgentSeatingCmd wires seed-default-agent-seating from the command-spec builder.
func NewSeedDefaultAgentSeatingCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSeedDefaultAgentSeatingCommandBuilder()
	cli.RequireSession(cmd, false)
	cli.RequireStorage(cmd, false)
	cmd.RunE = runSeedDefaultAgentSeating
	return cmd
}

func runSeedDefaultAgentSeating(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		root := proc.ProjectRoot()
		if root == emptyValue {
			return errfmt.Errorf("project root not found")
		}
		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		created, err := SeedDefaultAgentSeatingPack(root, logger)
		if err != nil {
			return errfmt.Newf("seed default agent seating").Wrap(err)
		}
		return cli.FormatOutput(cmd, map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusSuccess,
			"created":              created,
			objects.FieldKeyNote:   fmt.Sprintf("Idempotent; 0 created means %s / %s / ASK-DEFAULT-FEED-CORRESPONDENCE already present", objects.ConstPersonaDefaultOperator, objects.ConstPersonaDefaultAgent),
		})
	})(cmd, nil)
}
