package system

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/lifecycle"
	"github.com/lanceman/zqk/pkg/validation/qa"
	"github.com/spf13/cobra"
)

// NewTruthSentinelCmd creates a new truth-sentinel command
func NewTruthSentinelCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		truthSentinelCmdShort,
		truthSentinelCmdLong,
		"",
		truthSentinelCmdDescription,
		truthSentinelHelpDesc1,
		truthSentinelHelpDesc2,
		truthSentinelHelpDesc3,
		truthSentinelHelpDesc4,
	).
		AddExample(truthSentinelHelpExample, truthSentinelHelpExampleCmd)

	cmd := &cobra.Command{
		Use:   truthSentinelCmdUse,
		Short: truthSentinelCmdShort,
		Args:  cobra.NoArgs,
	}

	cmd.RunE = runTruthSentinel

	helpBuilder.ApplyToCommand(cmd)

	return cmd
}

func runTruthSentinel(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		projectRoot := proc.ProjectRoot()
		if projectRoot == "" {
			return errors.New(truthSentinelErrProjectRoot)
		}

		// 1. Initialize WAL
		wal, err := lifecycle.GetOrCreateLifecycleWAL(projectRoot)
		if err != nil {
			return fmt.Errorf(truthSentinelErrWalInit, err)
		}

		// 2. Initialize Signer
		privKeyPath := filepath.Join(projectRoot, paths.ProjectDataDir, "keystore", "auditor.priv")
		signer, err := qa.NewAuditorSigner(privKeyPath)
		if err != nil {
			return fmt.Errorf(truthSentinelErrSignerInit, err)
		}

		// 3. Initialize Helpers
		emitter := qa.NewInterruptEmitter(projectRoot)
		engine := qa.NewGuidanceEngine()
		gate := qa.NewAuditorGate(proc.Storage())

		// 4. Create Service
		svc := qa.NewAuditorService(wal, proc.Storage(), signer, emitter, engine, gate)

		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf(truthSentinelStatusMsgFmt, projectRoot)))

		// 5. Run
		return svc.Run(proc.OperationContext())
	})(cmd, args)
}
