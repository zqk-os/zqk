package system

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/graph/memgraph"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/hivemind/indexer"
	"github.com/lanceman/zqk/pkg/hivemind/providers"
	"github.com/lanceman/zqk/pkg/lifecycle"
	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/spf13/cobra"
)

const (
	ConstErrProjectRootNotFound        = "project root not found"
	ConstSemanticIndexDirName          = "semantic_index"
	ConstSemanticBridgeStartupMsg      = "Starting Semantic Bridge Daemon...\nMaintaining vector index at %s\nPress Ctrl+C to stop.\n"
	ConstErrFailedToCreateMemgraphPool = "failed to create memgraph pool: %w"
	ConstErrFailedToGetMemgraphConn    = "failed to get memgraph connection: %w"
	ConstErrFailedToOpenLifecycleWAL   = "failed to open lifecycle WAL: %w"
	ConstDefaultLocalhost              = "localhost"
)

var semanticBridgeDaemonCmd *cobra.Command

// NewSemanticBridgeDaemonCmd creates the semantic-bridge-daemon command
func NewSemanticBridgeDaemonCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemSemanticBridgeDaemonCommandBuilder(), &cobra.Command{Use: "semantic-bridge-daemon"})
	cli.BindAsyncProgress(cmd, runSemanticBridgeDaemon)
	semanticBridgeDaemonCmd = cmd
	return cmd
}

func runSemanticBridgeDaemon(cmd *cobra.Command, args []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == "" {
		return errors.New(ConstErrProjectRootNotFound)
	}

	indexDir := filepath.Join(projectRoot, paths.ProjectDataDir, ConstSemanticIndexDirName)
	_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf(ConstSemanticBridgeStartupMsg, indexDir)))

	ctx := cmd.Context()

	// 1. Setup MemGraph connection
	// We'll use defaults for the daemon unless env vars specify otherwise
	mgConfig := &memgraph.MemGraphConfig{
		Host:     ConstDefaultLocalhost,
		Port:     7687,
		PoolSize: 10,
	}
	mgProvider := memgraph.NewMemGraphProvider(mgConfig)
	pool, err := mgProvider.CreatePool(ctx, provider.ConnectionConfig{
		Host:     ConstDefaultLocalhost,
		Port:     7687,
		MaxConns: 10,
	})
	if err != nil {
		return fmt.Errorf(ConstErrFailedToCreateMemgraphPool, err)
	}
	defer pool.Close()

	graphConn, err := pool.GetConnection(ctx)
	if err != nil {
		return fmt.Errorf(ConstErrFailedToGetMemgraphConn, err)
	}
	defer graphConn.Close()

	// 2. Setup Memory Store
	memoryStore := providers.NewMemGraphMemoryStore(graphConn)

	// 3. Setup Embedding Service
	llmConfig := llm.DefaultConfig(ctx)
	llmClient := llm.NewClient(ctx, llmConfig)
	embeddingService := indexer.NewLLMEmbeddingService(llmClient)

	// 4. Setup Worker
	worker := indexer.NewIndexerWorker(embeddingService, memoryStore)

	// 5. Setup WAL
	wal, err := lifecycle.GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		return fmt.Errorf(ConstErrFailedToOpenLifecycleWAL, err)
	}

	// 6. Setup Listener
	listener := indexer.NewIndexerListener(wal, worker)

	// Run Listener blocking
	return listener.Run(ctx)
}
