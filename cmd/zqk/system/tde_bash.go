package system

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/tde"
)

func init() {
	tde.RegisterAction("bash_command", func(ctx context.Context, env tde.Envelope) error {
		scriptBytes, err := base64.StdEncoding.DecodeString(env.PayloadB64)
		if err != nil {
			return fmt.Errorf("failed to decode bash payload: %w", err)
		}

		script := string(scriptBytes)
		logger := logging.GetLoggerFromContext(ctx)
		logging.FluentEvent(logger).Info("TDE-Bash Executing").Script(script).Log()

		cmd := execwrap.CommandContext(ctx, "bash", "-c", script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		return cmd.Run()
	})
}

func generateID() string {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// NewSubmitTDEBashCmd creates a new command to submit a bash script to the TDE inbox.
func NewSubmitTDEBashCmd() *cobra.Command {
	var commandStr string
	var dependsOn []string

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemSubmitTdeBashCommandBuilder(), &cobra.Command{
		Use:   "submit-tde-bash",
		Short: "Submit a bash command to the Autonomy Inbox for Time-Delayed Execution",
		RunE: func(cmd *cobra.Command, args []string) error {
			if commandStr == "" {
				return fmt.Errorf("must provide --command")
			}

			ctx := cli.GetContext(cmd)
			projectRoot := ProjectRootOrResolve(ctx.ProjectRoot)

			wal, err := tde.NewStagingWAL(projectRoot)
			if err != nil {
				return err
			}
			defer wal.Close()

			env := tde.Envelope{
				ID:         "TDE-BASH-" + generateID(),
				Kind:       "os_command",
				TargetID:   "local_os",
				Operation:  "bash_command",
				PayloadB64: base64.StdEncoding.EncodeToString([]byte(commandStr)),
				ExecuteAt:  time.Now().UTC(),
				DependsOn:  dependsOn,
			}

			if err := wal.Stage(env); err != nil {
				return err
			}

			outMsg := fmt.Sprintf("Successfully staged bash command to Autonomy Inbox.\nEnvelope ID: %s\n", env.ID)
			_ = cli.WriteOutput(cmd, []byte(outMsg))
			return nil
		},
	})

	cmd.Flags().StringVar(&commandStr, "command", "", "The bash command to execute")
	cmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "Comma-separated list of TDE Envelope IDs this command depends on")
	return cmd
}
