package system

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// runCheckAsyncNonBlocking runs system check in the background and returns immediately
// with an operation ID. The check continues in the background and emits completion
// events via the coordinator that clients can subscribe to.
func runCheckAsyncNonBlocking(cmd *cobra.Command, args []string) error {
	// Get context
	initCtx := &pkgctx.CliInitializationContext{
		ProjectRoot: ProjectRootOrResolve(""),
	}
	ctx, err := cli.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		return err
	}

	// Generate operation ID before starting
	operationID := fmt.Sprintf("check_%d", time.Now().UnixNano())

	// Get project root and storage provider for coordination
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	var storageProvider storage.ObjectStorageProvider
	if projectRoot != emptyValue {
		factory, storageErr := storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
		if storageErr == nil {
			storageProvider = factory.GetStorage()
			defer func() { _ = storageProvider.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
		}
		if storageErr != nil {
			// Best effort - continue without coordinator if storage unavailable
			storageProvider = nil
		}
	}

	profile := profileOrDefault(ctx.Profile, systemProfileHuman)

	// Emit start event immediately
	if projectRoot != emptyValue && storageProvider != nil {
		opCallback := coordination.NewCoordinatorOperationCallback(
			pkgctx.NewSystemContext(),
			projectRoot,
			storageProvider,
			"system_check",
			profile,
		)
		opCallback.OnStart(operationID, map[string]any{
			"operation_type": "system_check",
			"mode":           "async_non_blocking",
		})
	}

	// Start check in background goroutine with timeout wrapper
	// The check will complete normally if it can, otherwise timeout and notify
	backgroundCtx := pkgctx.NewSystemContext()
	nonBlockBud := goroutinelabels.DefaultBudget()
	nonBlockBuilder := goroutinelabels.NewGoroutine("system_check_background", fmt.Sprintf("running system check %s in background", operationID))
	if nonBlockBud != nil {
		nonBlockBuilder = nonBlockBuilder.WithBudget(nonBlockBud)
	}
	nonBlockBuilder.StartWithContext(backgroundCtx, func(bgCtx context.Context) error {
		started := time.Now()
		checkErr := runCheckAsyncWithContext(cmd, ctx, args)
		emitSystemCheckOperationResult(projectRoot, storageProvider, operationID, profile, checkErr, time.Since(started))

		return checkErr
	})

	// Return immediately with operation ID
	output := map[string]any{
		objects.FieldKeyOperationID: operationID,
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
		msg := fmt.Sprintf("System check started in background\nOperation ID: %s\n\nSubscribe to coordinator events with operation_type='system_check' and operation_id='%s' to receive completion notification.\n\nTip: Omit --background to wait for completion and see results.\n", operationID, operationID)
		return cli.WriteOutput(cmd, []byte(msg))
	}
}
