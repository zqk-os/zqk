package system

import (
	"path/filepath"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
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

	fileStorage, err := getFileObjectStorage(cmd.Context(), projectRoot)
	if err != nil {
		return err
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"read:*", "write:*", "execute:*"})

	migratedByKind, errorsByKind := migration.NewCASMigrationUtility(fileStorage).MigrateAllToCAS(ctx, secCtx, removeOldFiles)
	summary := formatMigrationSummary("CAS", "No legacy objects found to migrate.", migratedByKind, errorsByKind, ctx)
	return cli.WriteOutput(cmd, []byte(summary))
}
