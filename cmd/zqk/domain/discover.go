package domain

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

const emptyValue = ""

// NewDiscoverCmd creates the discover command from the generated builder
func NewDiscoverCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDomainDiscoverCommandBuilder()
	cli.BindAsyncProgress(cmd, runDiscover)
	return cmd
}

func runDiscover(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		ctx := proc.OperationContext()
		secCtx := proc.SecurityContext()
		store := proc.Storage()
		storageCtx := pkgctx.NewStorageContext()

		listResult, err := store.List(ctx, secCtx, storageCtx, storage.ListFilter{
			Kind:  objects.KindDomainRegistry,
			Limit: 100,
		})
		if err != nil {
			return errfmt.Newf("failed to list domain_registry objects").Wrap(err)
		}

		if len(listResult.Objects) == 0 {
			msg := "No domain registries found.\nUse 'zqk object create domain_registry --file <yaml>' to register a domain.\n"
			return cli.WriteOutput(cmd, []byte(msg))
		}

		msg := fmt.Sprintf("Discovered %d domain registr(y/ies):\n", len(listResult.Objects))
		for _, obj := range listResult.Objects {
			id, _ := obj[objects.FieldKeyID].(string)
			title, _ := obj[objects.FieldKeyTitle].(string)
			if id != emptyValue {
				msg += fmt.Sprintf("  - %s", id)
				if title != emptyValue {
					msg += fmt.Sprintf(" (%s)", title)
				}
				msg += "\n"
			}
		}
		return cli.WriteOutput(cmd, []byte(msg))
	})(cmd, args)
}
