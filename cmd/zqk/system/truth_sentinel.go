package system

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation/qa"
)

// NewTruthSentinelCmd creates a new truth-sentinel command
func NewTruthSentinelCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemTruthSentinelCommandBuilder()
	cmd.RunE = runTruthSentinel
	return cmd
}

func runTruthSentinel(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
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

		var flags clipkg.FlagBag
		once := flags.Bool(cmd, truthSentinelFlagOnce)
		ids := flags.StringArray(cmd, truthSentinelFlagIDs)
		if err := flags.Err(); err != nil {
			return err
		}
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
			flushCtx, cancelFlush := storage.DurabilityFlushContext()
			defer cancelFlush()
			if flushErr := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), projectRoot, []string{qa.KindQASuccess}); flushErr != nil {
				// Log visibility error but don't fail once audit
			}
			return cli.WriteOutput(cmd, []byte(fmt.Sprintf(truthSentinelOnceDoneFmt, len(ids))))
		}

		_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf(truthSentinelStatusMsgFmt, projectRoot)))

		return svc.Run(proc.OperationContext())
	})(cmd, args)
}
