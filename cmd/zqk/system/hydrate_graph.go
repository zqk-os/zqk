package system

import (
	"path/filepath"
	"sync"
	"time"

	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/graph/memgraph"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewHydrateGraphCmd creates the hydrate-graph command
func NewHydrateGraphCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemHydrateGraphCommandBuilder()
	cmd.RunE = runHydrateGraph

	cmd.Flags().String("input", ".zqk-state/system-state.csnap", "Path to read the compressed snapshot")
	cmd.Flags().String("graph-host", "localhost", "Graph DB host")
	cmd.Flags().Int("graph-port", 7687, "Graph DB port")

	return cmd
}

func runHydrateGraph(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err

		ctx := proc.OperationContext()
		projectRoot := proc.ProjectRoot()

		inputPath, _ := cmd.Flags().GetString("input")
		if !filepath.IsAbs(inputPath) {
			inputPath = filepath.Join(projectRoot, inputPath)
		}

		if _, err := fileutil.Stat(inputPath); fileutil.IsNotExist(err) {
			return errfmt.Errorf("state file %s does not exist", inputPath)
		}

		cmd.Printf("Reading compressed snapshot from %s...\n", inputPath)
		cs, err := storage.ReadCompressedSnapshot(inputPath)
		if err != nil {
			return errfmt.Newf("failed to read compressed snapshot").Wrap(err)
		}

		cmd.Printf("Snapshot Checksum: %s\n", cs.Header.Checksum)
		cmd.Printf("Expanding %d objects...\n", cs.Header.ObjectCount)

		expanded, err := cs.Expand()
		if err != nil {
			return errfmt.Newf("failed to expand snapshot").Wrap(err)
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

		var wg sync.WaitGroup
		var mu sync.Mutex
		written := 0
		skipped := 0
		var firstError error

		type writeJob struct {
			obj map[string]any
		}

		writeOneJob := func(job writeJob) {
			obj := job.obj
			kind, ok := obj[objects.FieldKeyKind].(string)
			if !ok || kind == "" {
				_ = concurrency.RunInLock(&mu, func() error { skipped++; return nil })
				return
			}
			id, ok := obj[objects.FieldKeyID].(string)
			if !ok || id == "" {
				_ = concurrency.RunInLock(&mu, func() error { skipped++; return nil })
				return
			}

			conn, err := pool.GetConnection(ctx)
			if err != nil {
				_ = concurrency.RunInLock(&mu, func() error {
					if firstError == nil {
						firstError = err
					}
					return nil
				})
				return
			}
			defer pool.ReturnConnection(conn)

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

				if written < 5 {
					cmd.Printf("DEBUG [Attempt %d] ID=%s Kind=%s exists=%v\n", attempt, id, kind, exists)
				}

				if !exists {
					writeErr = conn.CreateNode(ctx, node)
					if writeErr != nil {
						cmd.PrintErrf("⚠️  CreateNode failed for ID=%s Kind=%s: %v\n", id, kind, writeErr)
						// Fallback to update just in case of race
						exists = true
					} else {
						if written < 5 {
							cmd.Printf("DEBUG CreateNode succeeded for ID=%s\n", id)
						}
					}
				}

				if exists {
					writeErr = conn.UpdateNode(ctx, id, provider.NodeUpdates{
						Properties: obj,
					})
					if writeErr == nil {
						if written < 5 {
							cmd.Printf("DEBUG UpdateNode succeeded for ID=%s\n", id)
						}
					}
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
				_ = concurrency.RunInLock(&mu, func() error {
					if firstError == nil {
						firstError = writeErr
					}
					return nil
				})
				return
			}

			_ = concurrency.RunInLock(&mu, func() error { written++; return nil })
		}

		const numWorkers = 10
		jobs := make(chan writeJob, len(expanded))
		for _, obj := range expanded {
			jobs <- writeJob{obj: obj}
		}
		close(jobs)

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			goroutinelabels.NewGoroutine("hydrate_graph", "writing expanded objects to graph").StartSimple(func() {
				defer wg.Done()
				for job := range jobs {
					if ctx.Err() != nil {
						return
					}
					writeOneJob(job)
				}
			})
		}
		wg.Wait()

		if firstError != nil {
			return errfmt.Newf("failed to write some objects").Wrap(firstError)
		}

		cmd.Printf("✅ Graph Hydration complete. %d objects written, %d skipped.\n", written, skipped)
		return nil
	})(cmd, args)
}
