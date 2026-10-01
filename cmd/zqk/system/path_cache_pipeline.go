package system

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

const pipelineKindPathCache = "system_path_cache"

// Outcome keys for system_path_cache pipeline observability (wire shape unchanged).
const (
	pathCacheOutcomeKeyIngestDone      = "ingest_done"
	pathCacheOutcomeKeyBuilt           = "built"
	pathCacheOutcomeKeyStale           = "stale"
	pathCacheOutcomeKeyAction          = "action"
	pathCacheOutcomeKeyNormalizeDone   = "normalize_done"
	pathCacheOutcomeKeyDecideDone      = "decide_done"
	pathCacheOutcomeKeyCommitDone      = "commit_done"
	pathCacheOutcomeKeyCommitElapsedMS = "commit_elapsed_ms"
	pathCacheOutcomeKeyFinalizeDone    = "finalize_done"
)

type pathCacheAction string

const (
	pathCacheActionNone         pathCacheAction = "none"
	pathCacheActionBuild        pathCacheAction = "build"
	pathCacheActionRefreshWait  pathCacheAction = "refresh_wait"
	pathCacheActionRefreshAsync pathCacheAction = "refresh_async"
)

// decidePathCacheAction picks build/refresh behavior. When showPaths is true and the cache is stale,
// we refresh synchronously so printed paths match the refreshed snapshot (same as --wait for stale-only).
func decidePathCacheAction(built bool, stale bool, wait bool, showPaths bool) pathCacheAction {
	if !built {
		return pathCacheActionBuild
	}
	if built && !stale {
		return pathCacheActionNone
	}
	if wait || showPaths {
		return pathCacheActionRefreshWait
	}
	return pathCacheActionRefreshAsync
}

type pathCachePipelinePayload struct {
	projectRoot string
	profile     string
	built       bool
	stale       bool
	wait        bool
	showPaths   bool
	action      pathCacheAction

	logger logging.Logger
}

// RunPathCacheViaPipeline wraps `system path-cache` in the canonical pipeline lifecycle.
func RunPathCacheViaPipeline(cmd *cobra.Command, _ []string) error {
	if cmd == nil {
		return errfmt.Errorf("path-cache: cmd required")
	}

	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = pkgctx.NewSystemContext()
	}

	stageLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	pl := pipeline.NewBuilder(pipelineKindPathCache, stageLogger).
		WithProfile(string(pkgctx.ProfileSystem)).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: noopMetricsSink{}, Strategy: pipeline.NoopBucketing{}}).
		AddStage("INGEST", func(pctx *pipeline.Context, _ any) (any, error) {
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			projectRoot := ProjectRootOrResolve("")
			if projectRoot == emptyValue {
				return nil, errfmt.Errorf("project root not found; run from repo or pass --project-root")
			}

			profile := systemProfileHuman
			if c := cli.GetContext(cmd); c != nil {
				profile = c.Profile
			}
			logger := logging.GetLoggerFromProfile(profile)

			wait, _ := cmd.Flags().GetBool("wait")
			showPaths, _ := cmd.Flags().GetBool("show-paths")

			payload := &pathCachePipelinePayload{
				projectRoot: projectRoot,
				profile:     profile,
				wait:        wait,
				showPaths:   showPaths,
				logger:      logger,
			}
			pctx.Outcome[pathCacheOutcomeKeyIngestDone] = true
			return payload, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*pathCachePipelinePayload](payload)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *pathCachePipelinePayload, got %T", payload)
			}
			in.built = paths.IsPathCacheBuilt(in.projectRoot)
			in.stale = paths.IsPathCacheStale(in.projectRoot, paths.DefaultStalenessCheckDirs())

			in.action = decidePathCacheAction(in.built, in.stale, in.wait, in.showPaths)
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[pathCacheOutcomeKeyBuilt] = in.built
			pctx.Outcome[pathCacheOutcomeKeyStale] = in.stale
			pctx.Outcome[pathCacheOutcomeKeyAction] = string(in.action)
			pctx.Outcome[pathCacheOutcomeKeyNormalizeDone] = true
			return in, nil
		}).
		AddStage("DECIDE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*pathCachePipelinePayload](payload)
			if !ok {
				return nil, errfmt.Errorf("DECIDE expected *pathCachePipelinePayload, got %T", payload)
			}
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[pathCacheOutcomeKeyDecideDone] = true
			return in, nil
		}).
		AddStage("COMMIT", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*pathCachePipelinePayload](payload)
			if !ok {
				return nil, errfmt.Errorf("COMMIT expected *pathCachePipelinePayload, got %T", payload)
			}

			start := time.Now()
			switch in.action {
			case pathCacheActionNone:
				logging.Fluent(in.logger).Info("Path alias cache is built and up to date").
					ProjectRoot(in.projectRoot).
					Log()
			case pathCacheActionBuild:
				storagepkg.BuildPathAliasCacheForProject(in.projectRoot)
				logging.Fluent(in.logger).Info("Path alias cache built").
					ProjectRoot(in.projectRoot).
					Log()
			case pathCacheActionRefreshWait, pathCacheActionRefreshAsync:
				ctx := cmd.Context()
				if ctx == nil {
					ctx = context.Background() // Background: request-or-shutdown derived
				}
				provider := getStorageProviderForCache(in.projectRoot)
				opts := &storagepkg.PathCacheRefreshOptions{}
				if in.action == pathCacheActionRefreshWait {
					opts.OnHeartbeat = func(msg string) {
						logging.Fluent(in.logger).Info(msg).
							ProjectRoot(in.projectRoot).
							Log()
					}
				} else {
					opts.OnComplete = func(err error) {
						if err != nil {
							logging.Fluent(in.logger).Warn("Path alias cache refresh finished with error").
								ProjectRoot(in.projectRoot).
								WithError(err).
								Log()
						} else {
							logging.Fluent(in.logger).Info("Path alias cache refreshed (background)").
								ProjectRoot(in.projectRoot).
								Log()
						}
					}
				}

				doneCh := storagepkg.RefreshPathCacheForProject(ctx, in.projectRoot, provider, opts)
				if in.action == pathCacheActionRefreshWait {
					if err := <-doneCh; err != nil {
						return nil, errfmt.Newf("path cache refresh").Wrap(err)
					}
					logging.Fluent(in.logger).Info("Path alias cache refreshed").
						ProjectRoot(in.projectRoot).
						Log()
				} else {
					logging.Fluent(in.logger).Info("Path alias cache refresh started in background; cache will swap when ready (use --wait to block with progress)").
						ProjectRoot(in.projectRoot).
						Log()
				}
			default:
				return nil, errfmt.Errorf("unknown path cache action: %s", in.action)
			}

			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[pathCacheOutcomeKeyCommitDone] = true
			pctx.Outcome[pathCacheOutcomeKeyCommitElapsedMS] = time.Since(start).Milliseconds()
			return in, nil
		}).
		AddStage("FINALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[pathCacheOutcomeKeyFinalizeDone] = true
			in, ok := nildecode.DecodeNonNilPayload[*pathCachePipelinePayload](payload)
			if ok && in.showPaths {
				if err := writePathCacheResolvedPaths(cmd, in.projectRoot); err != nil {
					return nil, errfmt.Newf("path-cache --show-paths").Wrap(err)
				}
			}
			return payload, nil
		}).
		Build()

	pctx := &pipeline.Context{Ctx: baseCtx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, struct{}{})
	return err
}

func writePathCacheResolvedPaths(cmd *cobra.Command, projectRoot string) error {
	m, err := datacell.ReadRuntimeManifest(projectRoot)
	if err != nil {
		return err
	}
	pv := datacell.EffectiveProtocolVersion(m)
	var b strings.Builder
	rp := datacell.RuntimeOrganismMembraneReadPaths(projectRoot).AllRuntimePaths()
	b.WriteString("project_root: ")
	b.WriteString(projectRoot)
	b.WriteByte('\n')
	b.WriteString("datacell.feature_flags: ")
	b.WriteString(rp.FeatureFlags)
	b.WriteByte('\n')
	b.WriteString("datacell.cli_hook_profile: ")
	b.WriteString(rp.CLIHookProfile)
	b.WriteByte('\n')
	b.WriteString("datacell.tray_yaml: ")
	b.WriteString(rp.TrayYAML)
	b.WriteByte('\n')
	b.WriteString("datacell.runtime_manifest: ")
	b.WriteString(rp.RuntimeManifest)
	b.WriteByte('\n')
	b.WriteString("datacell.agent_chat_channel_config: ")
	b.WriteString(rp.AgentChatChannelConfig)
	b.WriteByte('\n')
	b.WriteString("datacell.agent_chat_channel_events: ")
	b.WriteString(rp.AgentChatChannelEvents)
	b.WriteByte('\n')
	b.WriteString("datacell.steward_enqueue: ")
	b.WriteString(rp.StewardEnqueue)
	b.WriteByte('\n')
	b.WriteString("datacell.steward_metrics: ")
	b.WriteString(rp.StewardMetrics)
	b.WriteByte('\n')
	b.WriteString("datacell.protocol_version: ")
	b.WriteString(pv)
	b.WriteByte('\n')
	return cli.WriteOutput(cmd, []byte(b.String()))
}
