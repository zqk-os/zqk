// Package storage: path alias cache build (project structure + stream kinds) and stream segment dir resolution.
// All path resolution uses the path alias cache at runtime (prefix:, abs:, web:). See pkg/paths/resolver.go and PATH_ALIAS_RESOLUTION.md.
package storage

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"time"

	clicontext "github.com/lanceman/zqk/internal/cli/context"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/pipeline"
)

// PathCacheRefreshOptions configures async path-cache refresh: heartbeat while building, notification when done, cancellation via context.
type PathCacheRefreshOptions struct {
	// OnHeartbeat is called periodically while the refresh runs (e.g. every HeartbeatInterval). Use when the caller is waiting and wants to show progress.
	OnHeartbeat func(message string)
	// HeartbeatInterval between OnHeartbeat calls. If zero, default 500ms.
	HeartbeatInterval time.Duration
	// OnComplete is called when the refresh finishes (with nil or the error). Use when not waiting to get notified.
	OnComplete func(err error)
}

const defaultHeartbeatInterval = 500 * time.Millisecond

// BuildPathAliasMapForProject returns the full alias map for projectRoot.
// Starts from [paths.DefaultPathAliases] (including datacell_* and operational roots), then merges
// brand settings (zqk-settings.yaml) paths.aliases on top so partial overrides do not drop defaults.
// Stream segment aliases (streams/<kind>) are always added using the "streams" base from the merged map.
func BuildPathAliasMapForProject(projectRoot string) map[string]string {
	if projectRoot == emptyValue {
		return nil
	}
	aliases := maps.Clone(paths.DefaultPathAliases())
	if s, err := clicontext.LoadBrandSettings(projectRoot); err == nil && len(s.Paths.Aliases) > 0 {
		for k, v := range s.Paths.Aliases {
			if k != emptyValue && v != emptyValue {
				aliases[k] = filepath.Clean(v)
			}
		}
	}
	// Add stream segment aliases (streams/<kind>). Base path comes from settings or defaults.
	streamsBase := aliases["streams"]
	if streamsBase == emptyValue {
		streamsBase = filepath.Join(paths.ProjectDataDir, paths.StreamsDir)
	}
	for _, kind := range StreamStorageEnabledKindsList() {
		aliases["streams/"+kind] = filepath.Join(streamsBase, kind)
	}
	return aliases
}

// BuildPathAliasCacheForProject populates the path alias cache for projectRoot with default
// project structure aliases and stream segment aliases (streams/<kind>). Call from scheduler
// cache pre-warm (Tier 0) or CLI path-cache so any path resolution uses the cache.
func BuildPathAliasCacheForProject(projectRoot string) {
	if projectRoot == emptyValue {
		return
	}
	aliases := BuildPathAliasMapForProject(projectRoot)
	paths.ReplacePathCache(projectRoot, aliases)
}

// EnsurePathAliasCacheReady creates .zqk/state if missing and builds the path alias cache for projectRoot.
// Call before storage Create when the project root is new or caches were cleared—stream-backed kinds and
// ID generation expect state; path resolution requires the cache (same as scheduler pre-warm / path-cache check).
func EnsurePathAliasCacheReady(projectRoot string) error {
	if projectRoot == emptyValue {
		return nil
	}
	stateDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir)
	if err := os.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		return err
	}
	BuildPathAliasCacheForProject(projectRoot)
	return nil
}

// RefreshPathCacheForProject runs a path-cache refresh (default + stream + doc_entry paths from provider).
// It respects ctx for cancellation. Returns a channel that receives exactly one value (nil or error) when done, then closes.
// Callers can wait with heartbeat by selecting on the returned channel and a ticker that calls opts.OnHeartbeat.
// If opts is nil, no callbacks. If opts.OnComplete is set, it is invoked when the refresh finishes (before the channel is closed).
func RefreshPathCacheForProject(ctx context.Context, projectRoot string, provider ObjectStorageProvider, opts *PathCacheRefreshOptions) <-chan error {
	if ctx == nil {
		ctx = context.Background()
	}
	done := make(chan error, 1)
	if projectRoot == emptyValue {
		done <- nil
		close(done)
		return done
	}
	if err := ctx.Err(); err != nil {
		if opts != nil && opts.OnComplete != nil {
			opts.OnComplete(err)
		}
		done <- err
		close(done)
		return done
	}
	interval := defaultHeartbeatInterval
	if opts != nil && opts.HeartbeatInterval > 0 {
		interval = opts.HeartbeatInterval
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	pl := pipeline.NewBuilder(ConstStreamPathCacheRefresh, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			// BuildPathAliasMapForProject is the default + settings-driven alias map.
			return BuildPathAliasMapForProject(projectRoot), nil
		}).
		AddStage(pipeline.StageNormalize, func(pctx *pipeline.Context, payload any) (any, error) {
			aliases, _ := payload.(map[string]string)
			if aliases == nil {
				aliases = make(map[string]string)
			}
			if ctx.Err() != nil {
				// Cancellation should be a no-op (legacy behavior: do not swap in a new cache snapshot).
				return nil, nil
			}
			if provider == nil {
				return aliases, nil
			}

			secCtx := pkgctx.NewSystemSecurityContext()
			storageCtx := pkgctx.GetStorageContext()
			results, listErr := provider.List(ctx, secCtx, storageCtx, ListFilter{Kind: objects.KindDocEntry})
			if listErr != nil {
				return nil, listErr
			}
			for _, obj := range results.Objects {
				if p := objects.GetString(obj, objects.FieldKeyPath); p != emptyValue {
					rel := paths.NormalizeDocEntryPathForKey(p)
					if rel != emptyValue {
						aliases[rel] = rel
					}
				}
			}
			return aliases, nil
		}).
		AddStage(pipeline.StageCommit, func(pctx *pipeline.Context, payload any) (any, error) {
			if payload == nil {
				return nil, nil
			}
			aliases, _ := payload.(map[string]string)
			paths.ReplacePathCache(projectRoot, aliases)
			return payload, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			return payload, nil
		}).
		Build()

	buildDone := make(chan struct{})
	buildErrCh := make(chan error, 1) // buffered so build goroutine never blocks
	bud := goroutinelabels.DefaultBudget()

	goroutinelabels.NewGoroutine(ConstStreamPathCacheRefreshBuild, ConstStreamBuildingPathCacheAliases).
		WithBudget(bud).
		StartWithContext(ctx, func(ctx context.Context) error {
			defer close(buildDone)
			_, runErr := pl.Run(&pipeline.Context{Ctx: ctx}, nil)
			select {
			case buildErrCh <- runErr:
			default:
			}
			return nil
		})

	goroutinelabels.NewGoroutine(ConstStreamPathCacheRefreshHeartbeat, ConstStreamPathCacheRefreshHeartbeatCompletion).
		WithBudget(bud).
		StartWithContext(ctx, func(ctx context.Context) error {
			// Heartbeat loop: tick until build done or ctx cancelled
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-buildDone:
					var buildErr error
					select {
					case buildErr = <-buildErrCh:
					default:
						buildErr = nil
					}
					if opts != nil && opts.OnComplete != nil {
						opts.OnComplete(buildErr)
					}
					done <- buildErr
					close(done)
					return nil
				case <-ctx.Done():
					if opts != nil && opts.OnComplete != nil {
						opts.OnComplete(ctx.Err())
					}
					done <- ctx.Err()
					close(done)
					return nil
				case <-ticker.C:
					if opts != nil && opts.OnHeartbeat != nil {
						opts.OnHeartbeat(ConstStreamBuildingPathCacheellipsis)
					}
				}
			}
		})
	return done
}

// RefreshPathCacheForProjectAsync starts a fire-and-forget refresh with context.Background() and no callbacks.
// For context cancellation, heartbeat, or OnComplete notification, use RefreshPathCacheForProject instead.
func RefreshPathCacheForProjectAsync(projectRoot string, provider ObjectStorageProvider) {
	RefreshPathCacheForProject(context.Background(), projectRoot, provider, nil)
}

// BuildStreamPathCache populates the path alias cache for projectRoot (including stream segment
// aliases). Kept for backward compatibility; prefer BuildPathAliasCacheForProject.
func BuildStreamPathCache(projectRoot string) {
	BuildPathAliasCacheForProject(projectRoot)
}

// GetStreamSegmentDir returns the absolute directory containing segment files for the kind.
// Resolves via path alias cache (prefix:streams/<kind>). Returns ErrPathAliasNotInCache when cache
// is not built—callers should run path-cache check or ensure pre-warm has run; do not fall back silently.
func GetStreamSegmentDir(projectRoot, kind string) (string, error) {
	if projectRoot == emptyValue || kind == emptyValue {
		return "", nil
	}
	return paths.ResolvePathStrict(projectRoot, "prefix:streams/"+kind)
}
