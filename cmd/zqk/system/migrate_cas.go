package system

import (
	"context"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storage/migration"
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
	return runStorageMigration(cmd, "CAS", "No legacy objects found to migrate.", removeOldFiles, func(fileStorage *storage.FileObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext) (map[string]int, map[string][]error) {
		return migration.NewCASMigrationUtility(fileStorage).MigrateAllToCAS(ctx, secCtx, removeOldFiles)
	})
}
