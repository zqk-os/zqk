package system

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/validation/qa"
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
		AddExample(truthSentinelHelpExample, truthSentinelHelpExampleCmd).
		AddExample(truthSentinelHelpOnceExample, truthSentinelHelpOnceExampleCmd)

	cmd := &cobra.Command{
		Use:   truthSentinelCmdUse,
		Short: truthSentinelCmdShort,
		Args:  cobra.NoArgs,
	}
	cmd.Flags().Bool(truthSentinelFlagOnce, false, truthSentinelFlagOnceUsage)
	cmd.Flags().StringSlice(truthSentinelFlagIDs, nil, truthSentinelFlagIDsUsage)

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
		gate := qa.NewAuditorGateForProject(proc.Storage(), projectRoot)

		// 4. Create Service
		svc := qa.NewAuditorService(wal, proc.Storage(), signer, emitter, engine, gate)

		once, _ := cmd.Flags().GetBool(truthSentinelFlagOnce)
		ids, _ := cmd.Flags().GetStringSlice(truthSentinelFlagIDs)
		if once {
			if len(ids) == 0 {
				return errors.New(truthSentinelErrOnceNeedsIDs)
			}
			ctx := proc.OperationContext()
			secCtx := pkgctx.NewSystemSecurityContext()
			for _, id := range ids {
				kind := objects.KindBacklogItem
				if obj, err := proc.Storage().Read(ctx, secCtx, id); err == nil && obj != nil {
					if k, _ := obj[objects.FieldKeyKind].(string); k != "" {
						kind = k
					}
				}
				svc.AuditNow(ctx, id, kind)
			}
			return cli.WriteOutput(cmd, []byte(fmt.Sprintf(truthSentinelOnceDoneFmt, len(ids))))
		}

		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf(truthSentinelStatusMsgFmt, projectRoot)))

		return svc.Run(proc.OperationContext())
	})(cmd, args)
}
