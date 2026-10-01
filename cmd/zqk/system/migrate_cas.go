package system

import (
	"fmt"
	"path/filepath"
	"strings"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storage/migration"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewMigrateCasCmd creates the CAS migration command
func NewMigrateCasCmd() *cobra.Command {
	var removeOldFiles bool

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Migrate legacy UUID identities to deterministic CAS hashes",
		"Scans the .zqk/process/ object tree, migrates any legacy UUID identities to deterministic CAS (Content-Addressable Storage) SHA256 hashes, and safely cleans up orphaned object files.",
		"",
		"This command helps transition legacy object IDs to the new stable CAS storage scheme.",
	).
		AddExample("Migrate all legacy items", "%s system migrate-cas").
		AddExample("Migrate but keep old files (not recommended)", "%s system migrate-cas --remove-old-files=false")

	cmd := &cobra.Command{
		Use: "migrate-cas",
	}

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().BoolVar(&removeOldFiles, "remove-old-files", true, "Remove old ID-based files after migration")

	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		return runMigrateCas(cmd, removeOldFiles)
	})

	return cmd
}

func runMigrateCas(cmd *cobra.Command, removeOldFiles bool) error {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}
	if _, err := fileutil.Stat(filepath.Join(projectRoot, paths.ProjectDataDir)); err != nil {
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

	migratedByKind, errorsByKind := migration.NewCASMigrationUtility(fileStorage).MigrateAllToCAS(ctx, secCtx, removeOldFiles)

	var b strings.Builder
	fmt.Fprintf(&b, "CAS Migration completed.\n")

	totalMigrated := 0
	for kind, count := range migratedByKind {
		fmt.Fprintf(&b, "  %s: %d objects migrated\n", kind, count)
		totalMigrated += count
	}

	if totalMigrated == 0 {
		fmt.Fprintf(&b, "  No legacy objects found to migrate.\n")
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
