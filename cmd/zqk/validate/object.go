package validate

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/strutil"
	"github.com/spf13/cobra"
)

// NewValidateObjectCmd creates a new validate object command
func NewValidateObjectCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewValidateObjectCommandBuilder()
	cmd.Use = "object <id>"
	cmd.Long = `Reads an object and looks for an embedded CLI command inside its machine_hints field
(e.g., "CMD: zqk object list ...") and automatically executes it to derive its real-time status.`
	cmd.Args = cobra.ExactArgs(1)
	cli.BindAsyncProgress(cmd, runVerify)
	cli.RequireStorage(cmd, true)
	return cmd
}

func runVerify(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		id := args[0]

		obj, err := proc.Storage().Read(cmd.Context(), proc.SecurityContext(), id)
		if err != nil {
			return fmt.Errorf("failed to read object %s: %w", id, err)
		}

		rawHints, ok := obj[objects.FieldKeyMachineHints]
		if !ok || rawHints == nil {
			return fmt.Errorf("object %s does not contain machine_hints", id)
		}

		var lines []string
		switch v := rawHints.(type) {
		case string:
			lines = strings.Split(v, "\n")
		case []interface{}:
			for _, item := range v {
				if str, ok := item.(string); ok {
					lines = append(lines, str)
				}
			}
		case []string:
			lines = v
		default:
			return fmt.Errorf("object %s machine_hints is of unsupported type %T", id, v)
		}

		if len(lines) == 0 {
			return fmt.Errorf("object %s machine_hints is empty", id)
		}

		// Extract command string
		commandToRun := strutil.ExtractFirstPrefixedValue(lines, "CMD:", "VERIFY:")

		// If no prefix found, check if any line looks like a raw command
		if commandToRun == "" {
			commandToRun = strutil.FindLineWithPrefix(lines, "zqk ", "./bin/zqk ")
		}

		if commandToRun == "" {
			return fmt.Errorf("no executable command found in machine_hints for %s. (Expected 'CMD: <command>' or 'VERIFY: <command>')\nHints found: %v", id, rawHints)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Executing machine hint for %s: %s\n\n", id, commandToRun)

		shell := zqkenv.OSShell().Get()
		if shell == "" {
			shell = "sh"
		}

		execCmd := execwrap.CommandContext(cmd.Context(), shell, "-c", commandToRun)
		execCmd.Stdout = cmd.OutOrStdout()
		execCmd.Stderr = cmd.ErrOrStderr()
		execCmd.Stdin = cmd.InOrStdin()

		return execCmd.Run()
	})(cmd, args)
}
