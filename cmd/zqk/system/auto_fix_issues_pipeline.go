package system

import (
	stdcontext "context"
	"time"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

const pipelineKindAutoFixIssues = "system_check_auto_fix"

// Outcome keys for system_check_auto_fix pipeline observability (wire shape unchanged).
const (
	autoFixIssuesOutcomeKeyIssuesCount          = "issues_count"
	autoFixIssuesOutcomeKeyAutoFixRequested     = "auto_fix_requested"
	autoFixIssuesOutcomeKeyNormalizedInputCount = "normalized_input_count"
	autoFixIssuesOutcomeKeyFixedCount           = "fixed_count"
	autoFixIssuesOutcomeKeyFinalizeDone         = "finalize_done"
)

type autoFixIssuesPipelinePayload struct {
	fixed []string
}

// noopMetricsSink avoids per-stage Info logs for a high-frequency (per-object) pipeline.
type noopMetricsSink struct{}

func (noopMetricsSink) RecordStage(ctx stdcontext.Context, kind, stage string, duration time.Duration, err error) {
}

func (noopMetricsSink) RecordStageWithBuckets(ctx stdcontext.Context, kind, stage string, duration time.Duration, err error, _ map[string]string) {
}

// RunAutoFixIssuesViaPipeline runs system-check auto-fix for a single object using the
// standardized pipeline lifecycle (INGEST → NORMALIZE → COMMIT → FINALIZE).
//
// NOTE: We keep behavior parity by calling the existing executeAutoFixIssuesCore in the COMMIT stage.
func RunAutoFixIssuesViaPipeline(
	ctx *cli.Context,
	cmd *cobra.Command,
	obj *parser.ParsedObject,
	filePath, kind string,
	issues []Issue,
	registry storage.HashRegistryProvider,
	hashRegistryCache *HashRegistryCacheType,
	objectIDCache *ObjectIDCache,
	storageProvider storage.ObjectStorageProvider,
) ([]string, error) {
	if ctx == nil {
		return nil, errfmt.Errorf("auto_fix: ctx required")
	}
	if cmd == nil {
		return nil, errfmt.Errorf("auto_fix: cmd required")
	}
	if obj == nil {
		return nil, errfmt.Errorf("auto_fix: object required")
	}

	logger := logging.GetLoggerFromProfile(ctx.Profile)
	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = pkgctx.NewSystemContext()
	}

	pl := pipeline.NewBuilder(pipelineKindAutoFixIssues, logger).
		WithProfile(string(pkgctx.ProfileSystem)).
		WithMetricsConfig(&pipeline.MetricsConfig{Sink: noopMetricsSink{}, Strategy: pipeline.NoopBucketing{}}).
		AddStage("INGEST", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*autoFixIssuesPipelinePayload](payload)
			if !ok {
				in = &autoFixIssuesPipelinePayload{}
			}
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			// Pipeline observability only; actual auto-fix behavior remains in the core.
			pctx.Outcome[autoFixIssuesOutcomeKeyIssuesCount] = len(issues)
			pctx.Outcome[autoFixIssuesOutcomeKeyAutoFixRequested] = in != nil // placeholder; actual decision is inside core
			return in, nil
		}).
		AddStage("NORMALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[autoFixIssuesOutcomeKeyNormalizedInputCount] = len(issues)
			return payload, nil
		}).
		AddStage("COMMIT", func(pctx *pipeline.Context, payload any) (any, error) {
			in, ok := nildecode.DecodeNonNilPayload[*autoFixIssuesPipelinePayload](payload)
			if !ok {
				return nil, errfmt.Errorf("COMMIT expected *autoFixIssuesPipelinePayload, got %T", payload)
			}

			fixed := executeAutoFixIssuesCore(ctx, cmd, obj, filePath, kind, issues, registry, hashRegistryCache, objectIDCache, storageProvider)
			in.fixed = fixed
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[autoFixIssuesOutcomeKeyFixedCount] = len(fixed)
			return in, nil
		}).
		AddStage("FINALIZE", func(pctx *pipeline.Context, payload any) (any, error) {
			if pctx.Outcome == nil {
				pctx.Outcome = make(map[string]any)
			}
			pctx.Outcome[autoFixIssuesOutcomeKeyFinalizeDone] = true
			return payload, nil
		}).
		Build()

	pctx := &pipeline.Context{Ctx: baseCtx, Outcome: make(map[string]any)}
	payload := &autoFixIssuesPipelinePayload{}
	out, err := pl.Run(pctx, payload)
	if err != nil {
		return nil, err
	}
	res, ok := nildecode.DecodeNonNilPayload[*autoFixIssuesPipelinePayload](out)
	if !ok {
		return nil, errfmt.Errorf("auto_fix: unexpected pipeline payload type %T", out)
	}
	return res.fixed, nil
}
