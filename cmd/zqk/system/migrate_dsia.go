package system

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
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

	fileStorage, err := getFileObjectStorage(cmd.Context(), projectRoot)
	if err != nil {
		return err
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSecurityContext("system", []string{"admin"}, []string{"read:*", "write:*", "execute:*"})

	migratedByKind, errorsByKind := migration.NewDSIAMigrationUtility(fileStorage).MigrateAllToDSIA(ctx, secCtx, removeOldFiles)
	summary := formatMigrationSummary("DSIA", "No legacy CAS objects found to migrate.", migratedByKind, errorsByKind, ctx)
	return cli.WriteOutput(cmd, []byte(summary))
}
