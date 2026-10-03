package system

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type asyncCheckCoordinationSetup struct {
	cliCtx          *cli.Context
	operationID     string
	projectRoot     string
	storageProvider storage.ObjectStorageProvider
	cleanup         func()
	profile         string
}

func setupAsyncCheckCoordination(ctx *cli.Context) asyncCheckCoordinationSetup {
	root := ProjectRootOrResolve(ctx.ProjectRoot)
	sp, cleanup := initCheckCoordinationStorage(root)
	return asyncCheckCoordinationSetup{
		cliCtx:          ctx,
		operationID:     fmt.Sprintf("check_%d", time.Now().UnixNano()),
		projectRoot:     root,
		storageProvider: sp,
		cleanup:         cleanup,
		profile:         profileOrDefault(ctx.Profile, systemProfileHuman),
	}
}

func prepareAsyncCheckCoordination(cmd *cobra.Command, resolveCtx func(*cobra.Command) (*cli.Context, error)) (asyncCheckCoordinationSetup, error) {
	ctx, err := resolveCtx(cmd)
	if err != nil {
		return asyncCheckCoordinationSetup{}, err
	}
	return setupAsyncCheckCoordination(ctx), nil
}

// runCheckAsyncNonBlocking runs system check in the background and returns immediately
// with an operation ID. The check continues in the background and emits completion
// events via the coordinator that clients can subscribe to.
func runCheckAsyncNonBlocking(cmd *cobra.Command, args []string) error {
	setup, err := prepareAsyncCheckCoordination(cmd, getSystemCliContext)
	if err != nil {
		return err
	}
	defer setup.cleanup()

	// Emit start event immediately
	if setup.projectRoot != emptyValue && setup.storageProvider != nil {
		opCallback := coordination.NewCoordinatorOperationCallback(
			pkgctx.NewSystemContext(),
			setup.projectRoot,
			setup.storageProvider,
			"system_check",
			setup.profile,
		)
		opCallback.OnStart(setup.operationID, map[string]any{
			"operation_type": "system_check",
			"mode":           "async_non_blocking",
		})
	}

	// Start check in background goroutine with timeout wrapper
	// The check will complete normally if it can, otherwise timeout and notify
	backgroundCtx := pkgctx.NewSystemContext()
	nonBlockBud := goroutinelabels.DefaultBudget()
	nonBlockBuilder := goroutinelabels.NewGoroutine("system_check_background", fmt.Sprintf("running system check %s in background", setup.operationID))
	if nonBlockBud != nil {
		nonBlockBuilder = nonBlockBuilder.WithBudget(nonBlockBud)
	}
	nonBlockBuilder.StartWithContext(backgroundCtx, func(bgCtx context.Context) error {
		started := time.Now()
		checkErr := runCheckAsyncWithContext(cmd, setup.cliCtx, args)
		emitSystemCheckOperationResult(setup.projectRoot, setup.storageProvider, setup.operationID, setup.profile, checkErr, time.Since(started))

		return checkErr
	})

	// Return immediately with operation ID
	output := map[string]any{
		objects.FieldKeyOperationID: setup.operationID,
		objects.FieldKeyStatus:      objects.ObjectStatusStarted,
		"message":                   "System check started in background. Subscribe to coordinator events or poll for status.",
	}
	format := cli.GetFormat(cmd)
	switch format {
	case cli.FormatJSON:
		return cli.FormatOutputAs(cmd, cli.FormatJSON, output)
	case cli.FormatYAML:
		return cli.FormatOutputAs(cmd, cli.FormatYAML, output)
	default:
		// Table format
		msg := fmt.Sprintf("System check started in background\nOperation ID: %s\n\nSubscribe to coordinator events with operation_type='system_check' and operation_id='%s' to receive completion notification.\n\nTip: Omit --background to wait for completion and see results.\n", setup.operationID, setup.operationID)
		return cli.WriteOutput(cmd, []byte(msg))
	}
}
