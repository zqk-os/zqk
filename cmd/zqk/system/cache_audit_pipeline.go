package system

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

const pipelineKindCacheAudit = "system_cache_audit"

// Outcome keys for system_cache_audit pipeline observability (wire shape unchanged).
const (
	cacheAuditOutcomeKeyIngestDone     = "ingest_done"
	cacheAuditOutcomeKeyCleanRequested = "clean_requested"
	cacheAuditOutcomeKeyTotalEntries   = "total_entries"
	cacheAuditOutcomeKeyStaleEntries   = "stale_entries"
	cacheAuditOutcomeKeyNormalizeDone  = "normalize_done"
	cacheAuditOutcomeKeyShouldClean    = "should_clean"
	cacheAuditOutcomeKeyDecideDone     = "decide_done"
	cacheAuditOutcomeKeyCommitDone     = "commit_done"
	cacheAuditOutcomeKeyFinalizeDone   = "finalize_done"
)

type cacheAuditPipelinePayload struct {
	cmd         *cobra.Command
	projectRoot string

	format string
	clean  bool

	logger *logging.EventLogger

	// Loaded from cache file
	byKind   map[string][]KindBucketEntry
	idToKind map[string]string
	metadata *ObjectIDCacheMetadata

	// Derived
	staleEntries []StaleEntry
	result       CacheAuditResult
	shouldClean  bool

	cache *ObjectIDCache
}

func detectStaleEntriesFromByKind(
	byKind map[string][]KindBucketEntry,
	projectRoot string,
	processDir string,
	statFn func(string) (fileutil.FileInfo, error),
	gitInfoFn func(projectRoot, filePath string) *GitDeletionInfo,
) []StaleEntry {
	staleEntries := make([]StaleEntry, 0)
	for kind, list := range byKind {
		kindDir := objects.GetDirectoryFromKind(kind)
		for i := range list {
			e := &list[i]
			if e.ID == emptyValue {
				continue
			}

			fullPath := e.Path
			if processDir != emptyValue && !filepath.IsAbs(e.Path) {
				if kindDir != emptyValue {
					fullPath = filepath.Join(processDir, kindDir, e.Path)
				} else {
					fullPath = filepath.Join(processDir, e.Path)
				}
			}

			_, err := statFn(fullPath)
			if err != nil {
				if fileutil.IsNotExist(err) {
					staleEntry := StaleEntry{
						ID:          e.ID,
						Kind:        kind,
						FilePath:    fullPath,
						CachedMTime: e.MTime,
					}
					if gitInfoFn != nil {
						if gitInfo := gitInfoFn(projectRoot, fullPath); gitInfo != nil {
							staleEntry.DeletedBy = gitInfo.Author
							staleEntry.DeletedAt = gitInfo.Date
							staleEntry.CommitHash = gitInfo.CommitHash
							staleEntry.CommitMsg = gitInfo.CommitMsg
						}
					}
					staleEntries = append(staleEntries, staleEntry)
				}
			}
		}
	}
	return staleEntries
}

// RunCacheAuditViaPipeline wraps `system cache-audit` in the canonical pipeline lifecycle.
func RunCacheAuditViaPipeline(cmd *cobra.Command, _ []string) error {
	if cmd == nil {
		return errfmt.Errorf("cache-audit: cmd required")
	}

	stageLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = pkgctx.NewSystemContext()
	}

	pl := pipeline.NewBuilder(pipelineKindCacheAudit, stageLogger).
		WithProfile(string(pkgctx.ProfileSystem)).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: noopMetricsSink{}, Strategy: pipeline.NoopBucketing{}}).
		AddStage("INGEST", func(pctx *pipeline.Context, _ any) (any, error) {
			proc, err := cli.NewProcessor(cmd)
			if err != nil {
				return nil, errfmt.Newf("failed to create processor").Wrap(err)
			}

			projectRoot := proc.Context().ProjectRoot
			if projectRoot == emptyValue {
				return nil, errfmt.Errorf("project root not found")
			}

			format, _ := cmd.Flags().GetString("format")
			clean, _ := cmd.Flags().GetBool("clean")

			cache := GetGlobalObjectIDCache()

			out := &cacheAuditPipelinePayload{
				cmd:         cmd,
				projectRoot: projectRoot,
				format:      format,
				clean:       clean,
				logger:      proc.Logger(),
				cache:       cache,
			}
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[cacheAuditOutcomeKeyIngestDone] = true
			pctx.Outcome[objects.FieldKeyFormat] = format
			pctx.Outcome[cacheAuditOutcomeKeyCleanRequested] = clean
			return out, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*cacheAuditPipelinePayload](payload)
			if !ok {
				return nil, errfmt.Errorf("NORMALIZE expected *cacheAuditPipelinePayload, got %T", payload)
			}

			cachePath := filepath.Join(in.projectRoot, paths.ProjectDataDir, paths.CacheDir, paths.ObjectIDCacheFile)
			data, err := fileutil.ReadFile(cachePath)
			if err != nil {
				return nil, errfmt.Newf("failed to read cache file").Wrap(err)
			}

			byKind, idToKind, metadata, _, err := ParseObjectIDCacheFile(data)
			if err != nil {
				return nil, errfmt.Newf("failed to parse cache file").Wrap(err)
			}
			if metadata == nil {
				return nil, errfmt.Errorf("cache file missing metadata")
			}

			in.byKind = byKind
			in.idToKind = idToKind
			in.metadata = metadata

			processDir := datacell.ProcessPrimaryDir(metadata.ProjectRoot)
			in.staleEntries = detectStaleEntriesFromByKind(
				in.byKind,
				in.projectRoot,
				processDir,
				fileutil.Stat,
				findGitDeletionInfo,
			)

			in.result = CacheAuditResult{
				TotalEntries:   len(in.idToKind),
				StaleEntries:   len(in.staleEntries),
				StaleEntryList: in.staleEntries,
				AuditTime:      time.Now(),
				ProjectRoot:    in.projectRoot,
			}

			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[cacheAuditOutcomeKeyTotalEntries] = in.result.TotalEntries
			pctx.Outcome[cacheAuditOutcomeKeyStaleEntries] = in.result.StaleEntries
			pctx.Outcome[cacheAuditOutcomeKeyNormalizeDone] = true
			return in, nil
		}).
		AddStage("DECIDE", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*cacheAuditPipelinePayload](payload)
			if !ok {
				return nil, errfmt.Errorf("DECIDE expected *cacheAuditPipelinePayload, got %T", payload)
			}

			in.shouldClean = in.clean && len(in.staleEntries) > 0

			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[cacheAuditOutcomeKeyShouldClean] = in.shouldClean
			pctx.Outcome[cacheAuditOutcomeKeyDecideDone] = true
			return in, nil
		}).
		AddStage("COMMIT", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*cacheAuditPipelinePayload](payload)
			if !ok {
				return nil, errfmt.Errorf("COMMIT expected *cacheAuditPipelinePayload, got %T", payload)
			}

			// Output is intentionally before clean to preserve CLI UX ordering.
			if err := outputCacheAuditResults(in.cmd, &in.result, in.format); err != nil {
				return nil, err
			}

			if in.shouldClean {
				logging.FluentEvent(in.logger).Info("Cleaning stale cache entries").
					Int("count", len(in.staleEntries)).
					Log()
				staleCount := in.cache.ValidateAndCleanStale()
				if staleCount > 0 {
					if err := in.cache.SaveCache(in.projectRoot); err != nil {
						logging.FluentEvent(in.logger).Warn("Failed to save cleaned cache").
							WithError(err).
							Log()
					} else {
						logging.FluentEvent(in.logger).Info("Cache cleaned and saved").
							Int("removed", staleCount).
							Log()
					}
				}
			}

			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[cacheAuditOutcomeKeyCommitDone] = true
			return in, nil
		}).
		AddStage("FINALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[cacheAuditOutcomeKeyFinalizeDone] = true
			return payload, nil
		}).
		Build()

	pctx := &pipeline.Context{Ctx: baseCtx, Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, struct{}{})
	return err
}
