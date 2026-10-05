package system

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/memgraph"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewHydrateGraphCmd creates the hydrate-graph command
func NewHydrateGraphCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemHydrateGraphCommandBuilder()
	cmd.RunE = runHydrateGraph

	cmd.Flags().String("input", filepath.Join(paths.DefaultProjectStateDir, "system-state.csnap"), "Path to read the compressed snapshot")
	cmd.Flags().String("graph-host", "localhost", "Graph DB host")
	cmd.Flags().Int("graph-port", 7687, "Graph DB port")

	return cmd
}

func runHydrateGraph(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		ctx := proc.OperationContext()
		expanded, err := loadSnapshotFromProcessor(cmd, proc)
		if err != nil {
			return err
		}

		graphHost, _ := cmd.Flags().GetString("graph-host")
		graphPort, _ := cmd.Flags().GetInt("graph-port")

		mgConfig := &memgraph.MemGraphConfig{
			Host:     graphHost,
			Port:     graphPort,
			PoolSize: 10,
		}
		mgProvider := memgraph.NewMemGraphProvider(mgConfig)
		pool, err := mgProvider.CreatePool(ctx, provider.ConnectionConfig{
			Host:     graphHost,
			Port:     graphPort,
			MaxConns: 10,
		})
		if err != nil {
			return errfmt.Newf("failed to initialize memgraph pool").Wrap(err)
		}
		defer pool.Close()

		stats, err := processExpandedSnapshotParallel(
			ctx,
			expanded,
			expandedObjectProcessorConfig{
				NumWorkers:   10,
				RoutineLabel: "hydrate_graph",
				Timeout:      30 * time.Minute,
			},
			func(kind, id string, obj map[string]any) error {
				conn, err := pool.GetConnection(ctx)
				if err != nil {
					return err
				}
				defer func() { _ = pool.ReturnConnection(conn) }()

				toLabel := func(k string) string {
					parts := strings.Split(k, "_")
					var labelParts []string
					for _, part := range parts {
						if part != "" {
							labelParts = append(labelParts, strings.ToUpper(part[:1])+strings.ToLower(part[1:]))
						}
					}
					return strings.Join(labelParts, "")
				}

				node := provider.Node{
					ID:         id,
					Labels:     []string{toLabel(kind), "Entity"},
					Properties: obj,
				}

				var writeErr error
				for attempt := 0; attempt < 5; attempt++ {
					exists := false
					nodeVal, getErr := conn.GetNode(ctx, id, []string{toLabel(kind), "Entity"})
					if getErr == nil && nodeVal != nil {
						exists = true
					}

					if !exists {
						writeErr = conn.CreateNode(ctx, node)
						if writeErr != nil {
							cmd.PrintErrf("⚠️  CreateNode failed for ID=%s Kind=%s: %v\n", id, kind, writeErr)
							// Fallback to update just in case of race
							exists = true
						}
					}

					if exists {
						writeErr = conn.UpdateNode(ctx, id, provider.NodeUpdates{
							Properties: obj,
						})
					}

					if writeErr == nil {
						break
					}

					// If it's a transient memgraph conflict or connectivity timeout, retry after sleep
					errMsg := writeErr.Error()
					if strings.Contains(errMsg, "conflicting transactions") ||
						strings.Contains(errMsg, "TransientError") ||
						strings.Contains(errMsg, "ConnectivityError") {
						time.Sleep(50 * time.Millisecond)
						continue
					}
					break
				}

				if writeErr != nil {
					cmd.PrintErrf("❌ Failed to update node ID=%s Kind=%s: %v\n", id, kind, writeErr)
					for k := range obj {
						cmd.PrintErrf("   key: %s\n", k)
					}
					return writeErr
				}

				return nil
			},
		)
		if err != nil {
			return errfmt.Newf("failed to write some objects").Wrap(err)
		}

		cmd.Printf("✅ Graph Hydration complete. %d objects written, %d skipped.\n", stats.Written, stats.Skipped)
		return nil
	})(cmd, args)
}
