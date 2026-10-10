package matrix

import (
	"path/filepath"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
)

// NewMatrixInventoryCmd creates the matrix inventory CLI command.
func NewMatrixInventoryCmd() *cobra.Command {
	builder := clipkg.NewCommandBuilder("inventory")
	builder.WithShort("Inspect the repository-wide string literal inventory and deduplication status")
	builder.WithLong(`Queries the Global String Literal Inventory to detect duplicated literals across
the codebase. Identifies strings violating the single-occurrence tolerance policy (>=2 occurrences).`)
	builder.WithRunE(runMatrixInventory)

	cmd := builder.Build()
	cmd.Example = paths.RewriteCanonicalCLIInvocations(`  zqk matrix inventory
  zqk matrix inventory --duplicates-only
  zqk matrix inventory --all`)
	cmd.Flags().Bool("duplicates-only", true, "Show only string literals with >= 2 occurrences")
	cmd.Flags().Bool("all", false, "Show all recorded string literals")
	return cmd
}

func runMatrixInventory(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolveProjectRoot(cmd)
	if err != nil {
		return err
	}

	duplicatesOnly, _ := cmd.Flags().GetBool("duplicates-only")
	showAll, _ := cmd.Flags().GetBool("all")
	if showAll {
		duplicatesOnly = false
	}

	invPath := filepath.Join(projectRoot, paths.ProjectDataDir, "literal_inventory.json")
	inv, err := matrix.NewLiteralInventory(invPath)
	if err != nil {
		return errfmt.Errorf("failed to load literal inventory: %w", err)
	}

	total := inv.TotalCount()
	dups := inv.FindDuplicates()

	cmd.Println("================================================================================")
	cmd.Println("  Global String Literal Inventory & Deduplication Report")
	cmd.Println("================================================================================")
	cmd.Printf("Total Unique Literals Indexed : %d\n", total)
	cmd.Printf("Deduplication Violations (N>=2): %d\n", len(dups))
	cmd.Println("--------------------------------------------------------------------------------")

	if len(dups) == 0 && duplicatesOnly {
		cmd.Println("✓ Zero duplicate string literals found. Repository conforms to single-occurrence policy.")
		cmd.Println("================================================================================")
		return nil
	}

	if duplicatesOnly {
		cmd.Println("Violating Literals (Require extraction to package constants):")
		for i, rec := range dups {
			cmd.Printf("\n[%d] %q (Count: %d)\n", i+1, rec.Literal, rec.Count)
			for _, loc := range rec.Locations {
				cmd.Printf("     - %s:%d\n", loc.Path, loc.Line)
			}
		}
	}

	cmd.Println("================================================================================")
	return nil
}
