package system

import (
	stdcontext "context"
	"fmt"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewRecoverCASCmd creates a command to recover CAS objects with missing files
func NewRecoverCASCmd() *cobra.Command {
	var (
		kind                     string
		removeFromIndexIfMissing bool
		recreateFromIDBased      bool
	)

	cmdLong := fmt.Sprintf(`Recover CAS objects that have index entries but missing files.

This command helps fix CAS index inconsistencies where:
  - Objects exist in the CAS index but their hash files are missing
  - Objects can be recovered from ID-based files (if they exist)
  - Stale index entries need to be cleaned up

Examples:
  # Recover a specific object
  %s system recover-cas AAM-009

  # Recover all objects of a kind
  %s system recover-cas --kind command_metric

  # Recover without removing from index (only recreate)
  %s system recover-cas --kind command_metric --no-remove-from-index

  # Only remove from index, don't try to recreate
  %s system recover-cas --kind command_metric --no-recreate-from-id`,
		paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName)

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemRecoverCasCommandBuilder(), &cobra.Command{
		Use:   "recover-cas [object-id...]",
		Short: "Recover CAS objects with missing files",
		Long:  cmdLong,
		Args:  cobra.ArbitraryArgs,
	})
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		stdctx := cmd.Context()
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		projectRoot := ProjectRootOrResolve("")
		if projectRoot == emptyValue {
			return errfmt.Errorf("project root not found")
		}
		storageProvider, err := createSystemStorageProvider(stdctx, projectRoot)
		if err != nil {
			return err
		}
		options := &caspkg.CASRecoveryOptions{
			RemoveFromIndexIfMissing: removeFromIndexIfMissing,
			RecreateFromIDBased:      recreateFromIDBased,
			Logger:                   logger,
		}
		secCtx := pkgctx.NewSystemSecurityContext()
		if len(args) > 0 {
			return recoverSpecificObjects(stdctx, secCtx, storageProvider, args, options, logger)
		}
		if kind != emptyValue {
			return recoverKindObjects(stdctx, projectRoot, kind, options, logger)
		}
		return errfmt.Errorf("must provide object IDs, --kind, or both")
	})

	cmd.Flags().StringVar(&kind, "kind", "",
		"Recover all objects of this kind")
	cmd.Flags().BoolVar(&removeFromIndexIfMissing, "remove-from-index", true,
		"Remove objects from index if file is missing and can't be recovered")
	cmd.Flags().BoolVar(&recreateFromIDBased, "recreate-from-id", true,
		"Attempt to recreate files from ID-based files if they exist")

	return cmd
}

func recoverSpecificObjects(
	ctx stdcontext.Context,
	secCtx *pkgctx.SecurityContext,
	storageProvider storage.ObjectStorageProvider,
	objectIDs []string,
	options *caspkg.CASRecoveryOptions,
	logger logging.Logger,
) error {
	fileStorage := storage.UnwrapToFileObjectStorage(storageProvider)
	if fileStorage == nil {
		return errfmt.Errorf("storage provider is not FileObjectStorage and cannot be unwrapped to one")
	}

	var recovered, failed int
	for _, objectID := range objectIDs {
		result, err := fileStorage.RecoverCASObjectViaStorage(ctx, secCtx, objectID, options)
		if err != nil {
			logging.NewEvent("Failed to recover CAS object").
				String("object_id", objectID).
				Warn(logger)
			failed++
			continue
		}

		if result.Success {
			logging.NewEvent("Recovered CAS object").
				String("object_id", objectID).
				String("kind", result.Kind).
				String("action", result.Action).
				String("message", result.Message).
				Info(logger)
			recovered++
		} else {
			logging.NewEvent("CAS object recovery skipped").
				String("object_id", objectID).
				String("kind", result.Kind).
				String("message", result.Message).
				Debug(logger)
		}
	}

	logging.NewEvent("CAS recovery completed").
		Int("recovered", recovered).
		Failed(failed).
		Int("total", len(objectIDs)).
		Info(logger)

	return nil
}

func recoverKindObjects(
	ctx stdcontext.Context,
	projectRoot string,
	kind string,
	options *caspkg.CASRecoveryOptions,
	logger logging.Logger,
) error {
	results, err := caspkg.RecoverCASKind(ctx, projectRoot, kind, options, nil)
	if err != nil {
		return errfmt.Newf("failed to recover kind %s", kind).Wrap(err)
	}

	var recovered, failed, skipped int
	for _, result := range results {
		if result.Success {
			if result.Action == "recreated" || result.Action == "removed_from_index" {
				recovered++
			} else {
				skipped++
			}
		} else {
			failed++
		}
	}

	logging.Fluent(logger).Info("CAS recovery completed for kind").
		Kind(kind).
		Recovered(recovered).
		FailedOps(failed).
		SkippedOps(skipped).
		Total(len(results)).
		Log()

	return nil
}
