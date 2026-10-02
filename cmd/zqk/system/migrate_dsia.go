package system

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
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
	return runStorageMigration(cmd, "DSIA", "No legacy CAS objects found to migrate.", removeOldFiles, func(fileStorage *storage.FileObjectStorage, ctx context.Context, secCtx *pkgctx.SecurityContext) (map[string]int, map[string][]error) {
		return migration.NewDSIAMigrationUtility(fileStorage).MigrateAllToDSIA(ctx, secCtx, removeOldFiles)
	})
}
