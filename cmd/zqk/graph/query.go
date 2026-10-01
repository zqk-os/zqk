package graph

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

const emptyValue = ""

// NewQueryCmd creates a new query command
func NewQueryCmd() *cobra.Command {
	var (
		language string
		limit    int
	)

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Run raw graph queries",
		"Execute raw queries against the graph backend using Cypher or other supported languages.",
		"",
		"This command provides direct access to the graph database, allowing for complex traversals",
		"and analysis that are not exposed through the standard object commands.",
	).
		AddExample("List all backlog items", "%s graph query \"MATCH (n:BacklogItem) RETURN n\"").
		AddExample("Find dependency chain", "%s graph query \"MATCH p=(:BacklogItem {id: 'BLI-001'})-[:DEPENDS_ON*]->(target) RETURN p\"").
		AddExample("Count objects by kind", "%s graph query \"MATCH (n:Entity) RETURN labels(n)[0] as kind, count(n) as count\"")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewGraphQueryCommandBuilder(), &cobra.Command{
		Use:   "query [query-string]",
		Short: "Run raw graph queries",
		Args:  cobra.MaximumNArgs(1),
	})

	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		var queryStr string
		if len(args) > 0 {
			queryStr = args[0]
		} else {
			return errfmt.Errorf("query string required")
		}
		return runQuery(cmd, queryStr, language, limit)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().StringVarP(&language, "language", "l", "cypher", "Query language (cypher)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit the number of results (0 = no limit)")

	return cmd
}

func runQuery(cmd *cobra.Command, queryStr string, language string, limit int) error {
	// Create processor
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return err
	}

	// Ensure we have a graph storage
	// We use the storage for a common kind to get the provider
	storageForKind := proc.StorageFactory().GetStorageForKind(objects.KindBacklogItem)

	var graphStorage storage.ObjectStorageProvider
	if hybrid, ok := storageForKind.(*storage.HybridObjectStorage); ok {
		graphStorage = hybrid.GetPrimary()
	} else {
		// If it's not hybrid, check if it's already graph
		if proc.StorageFactory().IsGraphBackend(objects.KindBacklogItem) {
			graphStorage = storageForKind
		} else {
			return errfmt.Errorf("graph backend is not enabled or available for kind %s", objects.KindBacklogItem)
		}
	}

	// Prepare query
	query := storage.Query{
		Kind:       objects.KindBacklogItem, // Used for routing if not handled by explicit targeting
		Type:       storage.QueryTypeCypher,
		Expression: queryStr,
		Parameters: make(map[string]any),
	}

	// Execute query
	result, err := graphStorage.Query(proc.OperationContext(), proc.SecurityContext(), proc.StorageContext(), query)
	if err != nil {
		return errfmt.Newf("query execution failed").Wrap(err)
	}

	// Output results
	return outputQueryResults(cmd, proc, result)
}

func outputQueryResults(cmd *cobra.Command, proc *cli.Processor, result *storage.QueryResult) error {
	format := cli.GetFormat(cmd)

	// For structured formats (JSON/YAML), output the whole result
	if format == cli.FormatJSON || format == cli.FormatYAML {
		return cli.FormatOutput(cmd, result)
	}

	// For table/human output, show objects if present, else inform user
	if len(result.Objects) > 0 {
		var sb strings.Builder
		for _, obj := range result.Objects {
			id, _ := obj[objects.FieldKeyID].(string)
			kind, _ := obj[objects.FieldKeyKind].(string)
			title, _ := obj[objects.FieldKeyTitle].(string)

			if id != "" {
				fmt.Fprintf(&sb, "[%s] %s: %s\n", kind, id, title)
			} else {
				// Non-object row
				fmt.Fprintf(&sb, "%v\n", obj)
			}
		}
		return cli.WriteOutput(cmd, []byte(sb.String()))
	}

	// No results
	return cli.WriteOutput(cmd, []byte("No results found.\n"))
}
