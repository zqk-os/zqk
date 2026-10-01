package system

import (
	"fmt"
	"strings"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storage/migration"
)

// NewMigrateDsiaCmd creates the DSIA migration command
func NewMigrateDsiaCmd() *cobra.Command {
	var removeOldFiles bool

	cmd := bldr_cli_cmd_v1.NewSystemMigrateDsiaCommandBuilder()
	cmd.Flags().BoolVar(&removeOldFiles, "remove-old-files", true, "Remove old CAS hash-based files after migration")

	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		return runMigrateDsia(cmd, removeOldFiles)
	})

	return cmd
}

func runMigrateDsia(cmd *cobra.Command, removeOldFiles bool) error {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == "" {
		return errfmt.Errorf("project root not found")
	}

	factory, err := storage.NewStorageFactory(cmd.Context(), projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage factory").Wrap(err)
	}

	mainStorage := factory.GetStorage()
	fileStorage, ok := mainStorage.(*storage.FileObjectStorage)
	if !ok {
		if hybrid, isHybrid := mainStorage.(*storage.HybridObjectStorage); isHybrid {
			fileStorage, ok = hybrid.GetPrimary().(*storage.FileObjectStorage)
			if !ok {
				return errfmt.Errorf("primary storage in HybridObjectStorage is not FileObjectStorage")
			}
		} else {
			return errfmt.Errorf("storage provider is not a FileObjectStorage")
		}
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"read:*", "write:*", "execute:*"})

	migratedByKind, errorsByKind := migration.NewDSIAMigrationUtility(fileStorage).MigrateAllToDSIA(ctx, secCtx, removeOldFiles)

	var b strings.Builder
	fmt.Fprintf(&b, "DSIA Migration completed.\n")

	totalMigrated := 0
	for kind, count := range migratedByKind {
		fmt.Fprintf(&b, "  %s: %d objects migrated\n", kind, count)
		totalMigrated += count
	}

	if totalMigrated == 0 {
		fmt.Fprintf(&b, "  No legacy CAS objects found to migrate.\n")
	}

	totalErrors := 0
	for kind, errs := range errorsByKind {
		if len(errs) > 0 {
			fmt.Fprintf(&b, "  %s: %d errors\n", kind, len(errs))
			totalErrors += len(errs)
			for _, err := range errs {
				fmt.Fprintf(&b, "    - %v\n", err)
			}
		}
	}

	q := caspkg.GetGlobalCASOrphanCleanupQueue()
	if q != nil {
		fmt.Fprintf(&b, "\nRunning Zero-Orphan Cleanup Pipeline...\n")
		processed, err := q.ProcessQueueIfIdle(ctx)
		if err != nil {
			fmt.Fprintf(&b, "  Warning: failed to process orphan cleanup queue: %v\n", err)
		} else {
			fmt.Fprintf(&b, "  %d orphan files processed and safely cleaned up.\n", processed)
		}
	} else {
		fmt.Fprintf(&b, "\nWarning: Zero-Orphan Cleanup Queue is not initialized.\n")
	}

	return cli.WriteOutput(cmd, []byte(b.String()))
}
