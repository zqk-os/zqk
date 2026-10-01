package explain

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/acronyms"
)

// NewExplainCmd creates the `zqk explain` (and `zqk glossary`) command.
func NewExplainCmd() *cobra.Command {
	var formatFlag string
	var syncFlag bool

	cmd := &cobra.Command{
		Use:     "explain [ACRONYM]",
		Aliases: []string{"glossary", "acronym", "acronyms"},
		Short:   "Explain kernel acronyms, ontology terms, and architectural concepts",
		Long: `Provides progressive disclosure and single-source-of-truth explanations
for all core ZQK Knowledge Kernel acronyms (BLI, PRI, REQ, CRIT, VDS, TCFG, CVS,
ATK, PPLAN, ZPARQL, ZQL, CAS, WAL, CAP, CEF, TDE).

If an acronym is provided, displays its detailed definition, context, and related references.
If called without arguments, lists all documented acronyms.`,
		Example: `  # Explain a specific acronym
  zqk explain BLI
  zqk explain vds

  # List all documented acronyms
  zqk explain
  zqk glossary

  # Sync all acronym definitions to the Knowledge Kernel scheme (VOC-KERNEL-ACRONYMS)
  zqk explain --sync

  # Output in JSON format
  zqk explain PRI --format json
  zqk explain --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if syncFlag {
				runner := cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
					store := proc.GetStorageProvider()
					if store == nil {
						return fmt.Errorf("storage provider unavailable")
					}
					synced, err := acronyms.SyncToKernel(cmd.Context(), proc.SecurityContext(), store)
					if err != nil {
						return fmt.Errorf("failed to synchronize acronyms to kernel: %w", err)
					}
					fmt.Printf("✓ Successfully synchronized %d acronym terms into scheme %s\n", synced, acronyms.KernelAcronymsSchemeID)
					return nil
				})
				return runner(cmd, args)
			}

			if len(args) == 0 {
				all := acronyms.ListAll()
				if formatFlag == "json" {
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(all)
				}
				fmt.Print(acronyms.FormatTable(all))
				return nil
			}

			query := args[0]
			item, found := acronyms.Lookup(query)
			if !found {
				suggestions := acronyms.FindClosest(query)
				var hint string
				if len(suggestions) > 0 {
					hint = fmt.Sprintf("\nDid you mean: %s?", strings.Join(suggestions, ", "))
				}
				return fmt.Errorf("unknown kernel acronym %q.%s\nRun 'zqk explain' without arguments to see all registered acronyms", query, hint)
			}

			if formatFlag == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(item)
			}

			fmt.Printf("ACRONYM    : %s\n", item.Code)
			fmt.Printf("FULL NAME  : %s\n", item.FullName)
			fmt.Printf("CATEGORY   : %s\n", item.Category)
			fmt.Printf("DEFINITION : %s\n", item.Definition)
			fmt.Printf("CONTEXT    : %s\n", item.Context)
			if len(item.RelatedRefs) > 0 {
				fmt.Printf("RELATED    : %s\n", strings.Join(item.RelatedRefs, ", "))
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&formatFlag, "format", "f", "table", "Output format (table, json)")
	cmd.Flags().BoolVar(&syncFlag, "sync", false, "Synchronize all registered acronyms to the Knowledge Kernel scheme (VOC-KERNEL-ACRONYMS)")

	return cmd
}
