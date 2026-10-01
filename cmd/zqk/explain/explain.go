package explain

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/acronyms"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewExplainCmd creates the `zqk explain` (and `zqk glossary`) command.
func NewExplainCmd() *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewExplainCommandBuilder(), &cobra.Command{
		RunE: runExplain,
	})
}

func runExplain(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		store := proc.GetStorageProvider()
		ctx := proc.OperationContext()
		secCtx := proc.SecurityContext()

		syncFlag, _ := cmd.Flags().GetBool("sync")
		formatFlag, _ := cmd.Flags().GetString("format")
		schemeFlag, _ := cmd.Flags().GetString("scheme")
		if schemeFlag == "" {
			schemeFlag = acronyms.KernelAcronymsSchemeID
		}

		if syncFlag {
			if store == nil {
				return fmt.Errorf("storage provider unavailable")
			}
			synced, err := acronyms.SyncToKernel(ctx, secCtx, store)
			if err != nil {
				return fmt.Errorf("failed to synchronize acronyms to kernel: %w", err)
			}
			return cli.WriteOutput(cmd, []byte(fmt.Sprintf("✓ Successfully synchronized %d acronym terms into scheme %s\n", synced, acronyms.KernelAcronymsSchemeID)))
		}

		if len(args) == 0 {
			var all []acronyms.Acronym
			var err error
			if store != nil {
				all, err = acronyms.ListFromStore(ctx, secCtx, store, schemeFlag)
			}
			if err != nil || len(all) == 0 {
				all = acronyms.ListAll()
			}

			if formatFlag == "json" {
				data, err := json.MarshalIndent(all, "", "  ")
				if err != nil {
					return err
				}
				return cli.WriteOutput(cmd, append(data, '\n'))
			}
			return cli.WriteOutput(cmd, []byte(acronyms.FormatTable(all)))
		}

		query := args[0]
		var item acronyms.Acronym
		var found bool

		if store != nil {
			item, found, _ = acronyms.LookupInStore(ctx, secCtx, store, query, schemeFlag)
		}
		if !found {
			item, found = acronyms.Lookup(query)
		}

		if !found {
			var suggestions []string
			if store != nil {
				suggestions, _ = acronyms.FindClosestInStore(ctx, secCtx, store, query, schemeFlag)
			}
			if len(suggestions) == 0 {
				suggestions = acronyms.FindClosest(query)
			}
			var hint string
			if len(suggestions) > 0 {
				hint = fmt.Sprintf("\nDid you mean: %s?", strings.Join(suggestions, ", "))
			}
			return fmt.Errorf("unknown kernel acronym %q.%s\nRun 'zqk explain' without arguments to see all registered acronyms", query, hint)
		}

		if formatFlag == "json" {
			data, err := json.MarshalIndent(item, "", "  ")
			if err != nil {
				return err
			}
			return cli.WriteOutput(cmd, append(data, '\n'))
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("ACRONYM    : %s\n", item.Code))
		sb.WriteString(fmt.Sprintf("FULL NAME  : %s\n", item.FullName))
		sb.WriteString(fmt.Sprintf("CATEGORY   : %s\n", item.Category))
		sb.WriteString(fmt.Sprintf("DEFINITION : %s\n", item.Definition))
		sb.WriteString(fmt.Sprintf("CONTEXT    : %s\n", item.Context))
		if len(item.RelatedRefs) > 0 {
			sb.WriteString(fmt.Sprintf("RELATED    : %s\n", strings.Join(item.RelatedRefs, ", ")))
			sb.WriteString("\n")
		}
		return cli.WriteOutput(cmd, []byte(sb.String()))
	})(cmd, args)
}
