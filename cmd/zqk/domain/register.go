package domain

import (
	"fmt"
	"regexp"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	internal "github.com/zqk-os/zqk/pkg/zqkcli"
	"github.com/spf13/cobra"
)

var domainRegistryIDRe = regexp.MustCompile(`^DOMAIN-REG-(\d+)$`)

// NewRegisterCmd creates the register command from the generated builder
func NewRegisterCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewDomainRegisterCommandBuilder()
	cli.BindAsyncProgress(cmd, runRegister)
	return cmd
}

func runRegister(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		domainID, _ := cmd.Flags().GetString("domain-id")
		namespace, _ := cmd.Flags().GetString(objects.KindNamespace)
		specFile, _ := cmd.Flags().GetString("spec-file")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		var err error
		_ = err

		if domainID == emptyValue {
			return errfmt.Errorf("%s", errfmt.Newf("--domain-id is required").Build())
		}

		if dryRun {
			msg := fmt.Sprintf("Dry run: would register domain %q (namespace=%q, spec-file=%q)\n", domainID, namespace, specFile)
			return cli.WriteOutput(cmd, []byte(msg))
		}

		ctx := proc.OperationContext()
		secCtx := proc.SecurityContext()
		store := proc.Storage()
		storageCtx := pkgctx.NewStorageContext()

		entry := map[string]any{
			objects.FieldKeyID: domainID,
		}
		if namespace != emptyValue {
			entry[objects.KindNamespace] = namespace
		}
		if specFile != emptyValue {
			entry["spec_file"] = specFile
		}

		listResult, err := store.List(ctx, secCtx, storageCtx, storage.ListFilter{
			Kind:  objects.KindDomainRegistry,
			Limit: 1000,
		})
		if err != nil {
			return errfmt.Newf("failed to list domain_registry").Wrap(err)
		}

		if len(listResult.Objects) == 0 {
			regID := internal.NextSequentialID(listResult.Objects, domainRegistryIDRe, "DOMAIN-REG-%03d")
			obj := map[string]any{
				objects.FieldKeyID:            regID,
				objects.FieldKeyKind:          objects.KindDomainRegistry,
				objects.FieldKeyTitle:         "Domain registry",
				objects.FieldKeyDomains:       []any{entry},
				objects.FieldKeyStatus:        objects.ObjectStatusActive,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			}
			opCtx := pkgctx.WithCacheUpdate(ctx, regID, objects.KindDomainRegistry, "")
			if err := store.Create(opCtx, secCtx, obj); err != nil {
				return errfmt.Newf("failed to create domain_registry").Wrap(err)
			}
			msg := fmt.Sprintf("Domain registered: %s\nRegistry: %s\n", domainID, regID)
			return cli.WriteOutput(cmd, []byte(msg))
		}

		regObj := listResult.Objects[0]
		regID, _ := regObj[objects.FieldKeyID].(string)
		domains, _ := regObj[objects.FieldKeyDomains].([]any)
		if domains == nil {
			domains = []any{}
		}
		domains = append(domains, entry)
		regObj[objects.FieldKeyDomains] = domains
		opCtx := pkgctx.WithCacheUpdate(ctx, regID, objects.KindDomainRegistry, "")
		if err := store.Update(opCtx, secCtx, regID, map[string]any{objects.FieldKeyDomains: domains}); err != nil {
			return errfmt.Newf("failed to update domain_registry").Wrap(err)
		}
		msg := fmt.Sprintf("Domain registered: %s\nRegistry: %s\n", domainID, regID)
		return cli.WriteOutput(cmd, []byte(msg))
	})(cmd, args)
}
