package observer

import (
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	observerpkg "github.com/zqk-os/zqk/pkg/observer"
	"github.com/zqk-os/zqk/pkg/process"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const emptyValue = ""

// NewObserverCmd creates the observer command (AST extraction, graph population).
func NewObserverCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Observer agent operations",
		"Extract code entities (functions, types) for the knowledge kernel (BLI-OBS-001).",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewObserverCommandBuilder(), &cobra.Command{
		Use: "observer",
	})
	helpBuilder.ApplyToCommand(cmd)
	cmd.AddCommand(newExtractCmd())
	cmd.AddCommand(newPopulateCmd())
	return cmd
}

func newExtractCmd() *cobra.Command {
	var dir string
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewObserverExtractCommandBuilder(), &cobra.Command{
		Use:   "extract",
		Short: "Extract code entities from source (Go AST)",
		Long:  "Walk a directory and extract functions, methods, types, and interfaces from Go files. Output is structured for the knowledge kernel graph (BLI-OBS-001).",
		RunE: func(cmd *cobra.Command, args []string) error {
			process.TouchMeaningfulActivity()
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			if dir == emptyValue {
				dir = "."
			}
			fsys := os.DirFS(dir)
			ctx := cmd.Context()
			start := time.Now()
			observerpkg.DefaultEmitter.Emit(ctx, observerpkg.ObserverEvent{
				Type: observerpkg.EventExtractStarted,
				Dir:  dir,
			})
			extractors := []observerpkg.Extractor{observerpkg.GoExtractor{}}
			result, err := observerpkg.ExtractFromDir(ctx, fsys, ".", extractors)
			duration := time.Since(start)
			if err != nil {
				observerpkg.DefaultEmitter.Emit(ctx, observerpkg.ObserverEvent{
					Type: observerpkg.EventExtractFailed, Dir: dir, Error: err.Error(), Duration: duration,
				})
				logging.Fluent(logger).Warn("Observer extract failed").WithError(err).Log()
				return err
			}
			process.TouchMeaningfulActivity()
			observerpkg.DefaultEmitter.Emit(ctx, observerpkg.ObserverEvent{
				Type: observerpkg.EventExtractCompleted, Dir: dir, EntityCount: len(result.Entities), Duration: duration,
			})
			for _, e := range result.Errors {
				logging.Fluent(logger).Debug("Extract warning").File(e.File).Message(e.Msg).Log()
			}
			return cli.FormatOutput(cmd, result)
		},
	})
	cmd.Flags().StringVar(&dir, "dir", ".", "Directory to scan for source files")
	return cmd
}

func newPopulateCmd() *cobra.Command {
	var dir string
	var extractID string
	var semantic bool
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewObserverPopulateCommandBuilder(), &cobra.Command{
		Use:     "populate",
		Aliases: []string{"ingest-ast"},
		Short:   "Extract entities and populate the knowledge kernel graph",
		Long:    "Extract code entities from source (Go AST), optionally enhance with semantics, then create graph nodes and edges (CodeEntity, SourceFile, Package, CONTAINS, METHOD_OF, CALLS, DEPENDS_ON, IMPORTS) in the knowledge kernel. Requires graph backend (BLI-OBS-002).",
		RunE: func(cmd *cobra.Command, args []string) error {
			process.TouchMeaningfulActivity()
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			mg := mcp.GetGraphConnectionManager()
			if !mg.IsEnabled() {
				return errfmt.Errorf("graph backend not enabled: set %s=true and ensure graph is running", zqkenv.RawGraphEnabled())
			}

			if dir == emptyValue {
				dir = "."
			}
			ctx := cmd.Context()
			fsys := os.DirFS(dir)
			extractors := []observerpkg.Extractor{observerpkg.GoExtractor{}}
			result, err := observerpkg.ExtractFromDir(ctx, fsys, ".", extractors)
			if err != nil {
				logging.Fluent(logger).Warn("Observer extract failed").WithError(err).Log()
				return err
			}
			process.TouchMeaningfulActivity()
			for _, e := range result.Errors {
				logging.Fluent(logger).Debug("Extract warning").File(e.File).Message(e.Msg).Log()
			}

			var mgProvider storage.ObjectStorageProvider
			pool, err := mg.GetPool(ctx)
			if err != nil {
				return errfmt.Newf("graph pool").Wrap(err)
			}
			mgProvider = storage.NewPoolAwareGraphStorage(pool, zqkenv.ProjectRoot().Name())

			if semantic {
				enhancer := observerpkg.NewSemanticEnhancer(cmd.Context(), nil, mgProvider) // uses DefaultConfig + SemanticCache
				if err := enhancer.Enhance(ctx, result); err != nil {
					logging.Fluent(logger).Warn("Semantic enhancement failed").WithError(err).Log()
					return errfmt.Newf("semantic enhancement failed").Wrap(err)
				}
				process.TouchMeaningfulActivity()
			}
			if extractID == emptyValue {
				extractID = time.Now().UTC().Format("2006-01-02T15:04:05Z07:00")
			}
			popStart := time.Now()
			observerpkg.DefaultEmitter.Emit(ctx, observerpkg.ObserverEvent{
				Type: observerpkg.EventPopulateStarted, Dir: dir, ExtractID: extractID,
			})
			var popRes *observerpkg.PopulateResult
			err = pool.Execute(ctx, func(conn provider.GraphConnection) error {
				var popErr error
				popRes, popErr = observerpkg.PopulateBatch(ctx, conn, result, extractID)
				return popErr
			})
			popDuration := time.Since(popStart)
			if err != nil {
				observerpkg.DefaultEmitter.Emit(ctx, observerpkg.ObserverEvent{
					Type: observerpkg.EventPopulateFailed, Dir: dir, ExtractID: extractID, Error: err.Error(), Duration: popDuration,
				})
				return err
			}
			observerpkg.DefaultEmitter.Emit(ctx, observerpkg.ObserverEvent{
				Type: observerpkg.EventPopulateCompleted, Dir: dir, ExtractID: extractID,
				EntityCount: len(result.Entities), Duration: popDuration,
				Payload: map[string]any{"nodes_created": popRes.NodesCreated, "edges_created": popRes.EdgesCreated},
			})
			return cli.FormatOutput(cmd, popRes)
		},
	})
	cmd.Flags().StringVar(&dir, "dir", ".", "Directory to scan for source files")
	cmd.Flags().StringVar(&extractID, "extract-id", "", "Extract run id for versioning (default: current UTC timestamp)")
	cmd.Flags().BoolVar(&semantic, "semantic", false, "Generate LLM-powered semantic intents and vector embeddings for extracted entities")
	return cmd
}
