package demo

import (
	"strings"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	demopkg "github.com/zqk-os/zqk/pkg/demo"
)

// NewDemoCmd creates the 'zqk demo' interactive terminal showcase command.
func NewDemoCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewDemoCommandBuilder(), &cobra.Command{
		Use:   "demo",
		Short: "Interactive terminal showcase of ZQK capabilities",
		Long: `Interactive terminal demonstration showcasing sub-15ms zgrep AST search,
Verifiable Delivery Standard (VDS) verification gates, and multi-agent coordination,
concluding with a community star and fork invitation.

Examples:
  # Run interactive terminal showcase
  zqk demo

  # Run automated non-interactive demonstration
  zqk demo --non-interactive

  # Run only the AST search showcase
  zqk demo --step ast
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDemoExecution(cmd, args)
		},
	})

	return cmd
}

func runDemoExecution(cmd *cobra.Command, args []string) error {
	var flags clipkg.FlagBag
	nonInteractive := flags.Bool(cmd, "non-interactive")
	speed := flags.String(cmd, "speed")
	step := flags.String(cmd, "step")
	if err := flags.Err(); err != nil {
		return err
	}

	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		step = strings.TrimSpace(args[0])
	}

	opts := demopkg.DemoOptions{
		Step:           step,
		Speed:          speed,
		NonInteractive: nonInteractive,
		Writer:         cmd.OutOrStdout(),
		Reader:         cmd.InOrStdin(),
	}

	sc := demopkg.NewShowcase(opts)
	_, err := sc.Run()
	return err
}
