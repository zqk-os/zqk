package docman

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	_ "github.com/zqk-os/zqk/pkg/librarypack"
	"github.com/zqk-os/zqk/pkg/storage"
)

const emptyValue = ""

func resolveDocmanContext(cmd *cobra.Command) (context.Context, string, string, storage.ObjectStorageProvider, error) {
	cmdCtx := cmd.Context()
	if cmdCtx == nil {
		cmdCtx = pkgctx.NewSystemContext()
	}

	profile := string(pkgctx.ProfileHuman)
	if f := cmd.Flags().Lookup("context"); f != nil {
		if val, err := cmd.Flags().GetString("context"); err == nil && val != emptyValue {
			profile = val
		}
	}

	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		return nil, "", "", nil, errfmt.Errorf("project root not found")
	}

	factory, err := storage.NewStorageFactory(cmdCtx, projectRoot)
	if err != nil {
		return nil, "", "", nil, errfmt.Newf("failed to initialize storage factory").Wrap(err)
	}
	storageProvider := factory.GetStorageForKind("doc_entry")

	return cmdCtx, profile, projectRoot, storageProvider, nil
}

// NewDocmanCmd creates a new docman command group
func NewDocmanCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Documentation management operations",
		"Documentation management operations for discovering and registering documentation files.",
		"",
		"This command group provides operations for managing documentation:",
		"  - register: Discover and register markdown files as doc_entry objects",
		"  - verify: Cryptographically verify doc_entry objects against on-disk files and detect drift",
	).
		AddExample("Register all documentation files", "%s docman register").
		AddExample("Dry run to see what would be registered", "%s docman register --dry-run").
		AddExample("Verify shipped documentation integrity", "%s docman verify --shipped-only")

	docmanCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewDocmanCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "docman",
	})

	helpBuilder.ApplyToCommand(docmanCmd)

	docmanCmd.AddCommand(NewRegisterCmd())
	docmanCmd.AddCommand(NewVerifyCmd())

	return docmanCmd
}
