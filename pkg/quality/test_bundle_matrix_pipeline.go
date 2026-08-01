// Package quality implements test-bundle matrix workflows using the native pipeline API
// (pkg/pipeline), aligned with docs/architecture/data-pipeline-lifecycle.md.
package quality

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/contextevents"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/zqktime"
)

const pipelineKindTestBundleMatrix = "quality.test_bundle_matrix"

// TestBundleMatrixOptions configures RunTestBundleMatrixPipeline (regenerate CSV + optional verify + optional convergence).
type TestBundleMatrixOptions struct {
	ProjectRoot string
	// BundlePrefix limits rows to .zqk/test-bundles/<prefix>*.json (empty = all bundles).
	BundlePrefix string
	// OutputPath is the CSV path (default docs/quality/TEST_BUNDLE_MATRIX.csv under project root).
	OutputPath string
	// ProfilePath overrides docs/quality/test_bundle_matrix_profile.yaml when set.
	ProfilePath string
	SkipVerify  bool
	// StrictVerify sets VERIFY_TEST_BUNDLE_STRICT=1 for verify-test-bundle-matrix.sh.
	StrictVerify bool
	// Convergence runs zqk scheduler convergence measure --format json (non-fatal parse errors logged).
	Convergence bool
	// Python is the interpreter for generate script (default "python3").
	Python string
	// ZQKBin is the zqk binary for convergence (default: lookup or "zqk").
	ZQKBin string
}

type testBundleMatrixPayload struct {
	opts *TestBundleMatrixOptions
	err  error
}

// testBundleMatrixPayloadFrom returns the payload when the type assertion and options are present.
func testBundleMatrixPayloadFrom(payload any) (*testBundleMatrixPayload, bool) {
	p, ok := nildecode.DecodeNonNilPayload[*testBundleMatrixPayload](payload)
	if !ok || p.opts == nil {
		return nil, false
	}
	return p, true
}

// RunTestBundleMatrixPipeline runs INGEST → NORMALIZE (generate) → DECIDE (verify) → TRIGGER? → FINALIZE
// using pkg/pipeline. Generate/verify delegate to repo scripts (Python + bash) for parity with scripts/test_bundle_matrix_pipeline.py.
func RunTestBundleMatrixPipeline(ctx context.Context, opts *TestBundleMatrixOptions) error {
	if opts == nil {
		return errfmt.Errorf("quality: options required")
	}
	root := strings.TrimSpace(opts.ProjectRoot)
	if root == "" {
		return errfmt.Errorf("quality: ProjectRoot required")
	}
	if opts.Python == "" {
		opts.Python = "python3"
	}
	out := strings.TrimSpace(opts.OutputPath)
	if out == "" {
		out = filepath.Join(root, paths.DocsQualityDir, "TEST_BUNDLE_MATRIX.csv")
		opts.OutputPath = out
	} else if !filepath.IsAbs(out) {
		opts.OutputPath = filepath.Join(root, out)
	}
	if p := strings.TrimSpace(opts.ProfilePath); p != "" && !filepath.IsAbs(p) {
		opts.ProfilePath = filepath.Join(root, p)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}

	pl := pipeline.NewBuilder(pipelineKindTestBundleMatrix, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, ingestTestBundleMatrix).
		AddStage(pipeline.StageNormalize, normalizeGenerateCSV).
		AddStage(pipeline.StageDecide, decideVerifyMatrix).
		AddStage("TRIGGER", triggerConvergenceOptional).
		AddStage(pipeline.StageFinalize, finalizeTestBundleMatrix).
		Build()

	pctx := &pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}
	payload := &testBundleMatrixPayload{opts: opts}
	_, err := pl.Run(pctx, payload)
	if err != nil {
		return err
	}
	if payload.err != nil {
		return payload.err
	}
	return nil
}

func ingestTestBundleMatrix(pctx *pipeline.Context, payload any) (any, error) {
	p, ok := testBundleMatrixPayloadFrom(payload)
	if !ok {
		return nil, errfmt.Errorf("INGEST: expected *testBundleMatrixPayload")
	}
	root := p.opts.ProjectRoot
	gen := filepath.Join(root, "scripts", "generate-test-bundle-matrix.py")
	if st, err := os.Stat(gen); err != nil || st.IsDir() {
		return nil, errfmt.Errorf("INGEST: missing script %s", gen)
	}
	if pctx.Outcome != nil {
		pctx.Outcome[pipeline.OutcomeKeyProjectRoot] = root
		pctx.Outcome[pipeline.OutcomeKeyOutputCSV] = p.opts.OutputPath
	}
	return payload, nil
}

func normalizeGenerateCSV(pctx *pipeline.Context, payload any) (any, error) {
	p, ok := testBundleMatrixPayloadFrom(payload)
	if !ok {
		return nil, errfmt.Errorf("NORMALIZE: expected *testBundleMatrixPayload")
	}
	o := p.opts
	args := []string{filepath.Join(o.ProjectRoot, "scripts", "generate-test-bundle-matrix.py"), "--output", o.OutputPath}
	if strings.TrimSpace(o.BundlePrefix) != "" {
		args = append(args, "--bundle-prefix", strings.TrimSpace(o.BundlePrefix))
	}
	if strings.TrimSpace(o.ProfilePath) != "" {
		args = append(args, "--profile", strings.TrimSpace(o.ProfilePath))
	}
	cmd := exec.CommandContext(pctx.Ctx, o.Python, args...)
	cmd.Dir = o.ProjectRoot
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")
	out, err := cmd.CombinedOutput()
	if pctx.Outcome != nil {
		pctx.Outcome[pipeline.OutcomeKeyGenerateExit] = err == nil
		if len(out) > 0 {
			trim := strings.TrimSpace(string(out))
			if len(trim) > 800 {
				trim = trim[:800] + "..."
			}
			pctx.Outcome[pipeline.OutcomeKeyGenerateLogTail] = trim
		}
	}
	if err != nil {
		p.err = errfmt.Errorf("generate-test-bundle-matrix: %w: %s", err, strings.TrimSpace(string(out)))
		return payload, p.err
	}
	return payload, nil
}

func decideVerifyMatrix(pctx *pipeline.Context, payload any) (any, error) {
	p, ok := testBundleMatrixPayloadFrom(payload)
	if !ok {
		return nil, errfmt.Errorf("DECIDE: expected *testBundleMatrixPayload")
	}
	if p.opts.SkipVerify {
		if pctx.Outcome != nil {
			pctx.Outcome[pipeline.OutcomeKeyVerifySkipped] = true
		}
		return payload, nil
	}
	sh := filepath.Join(p.opts.ProjectRoot, "scripts", "verify-test-bundle-matrix.sh")
	cmd := exec.CommandContext(pctx.Ctx, "/bin/bash", sh)
	cmd.Dir = p.opts.ProjectRoot
	cmd.Env = append(os.Environ(), "TEST_BUNDLE_MATRIX_CSV="+p.opts.OutputPath)
	if p.opts.StrictVerify {
		cmd.Env = append(cmd.Env, "VERIFY_TEST_BUNDLE_STRICT=1")
	}
	out, err := cmd.CombinedOutput()
	if pctx.Outcome != nil {
		pctx.Outcome[pipeline.OutcomeKeyVerifyExit] = err == nil
		if len(out) > 0 {
			pctx.Outcome[pipeline.OutcomeKeyVerifyLog] = strings.TrimSpace(string(out))
		}
	}
	if err != nil {
		p.err = errfmt.Errorf("verify-test-bundle-matrix: %w: %s", err, strings.TrimSpace(string(out)))
		return payload, p.err
	}
	return payload, nil
}

func triggerConvergenceOptional(pctx *pipeline.Context, payload any) (any, error) {
	p, ok := testBundleMatrixPayloadFrom(payload)
	if !ok {
		return nil, errfmt.Errorf("TRIGGER: expected *testBundleMatrixPayload")
	}
	if !p.opts.Convergence {
		return payload, nil
	}
	zqk := strings.TrimSpace(p.opts.ZQKBin)
	if zqk == "" {
		cand := filepath.Join(p.opts.ProjectRoot, "bin", "zqk")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			zqk = cand
		} else {
			zqk = "zqk"
		}
	}
	cmd := exec.CommandContext(pctx.Ctx, zqk, "scheduler", "convergence", "snapshot", "--format", "json")
	cmd.Dir = p.opts.ProjectRoot
	out, err := cmd.CombinedOutput()
	if pctx.Outcome != nil {
		pctx.Outcome[pipeline.OutcomeKeyConvergenceRan] = true
		pctx.Outcome[pipeline.OutcomeKeyConvergenceExit] = err == nil
		s := strings.TrimSpace(string(out))
		if len(s) > 4000 {
			s = s[:4000] + "..."
		}
		pctx.Outcome[pipeline.OutcomeKeyConvergenceJSON] = s
	}
	// Non-fatal: convergence is observability; matrix verify already gated bad rows.
	if err != nil {
		if pctx.Outcome != nil {
			pctx.Outcome[pipeline.OutcomeKeyConvergenceError] = err.Error()
		}
	}
	return payload, nil
}

func finalizeTestBundleMatrix(pctx *pipeline.Context, payload any) (any, error) {
	p, ok := testBundleMatrixPayloadFrom(payload)
	if ok && p != nil {
		maybeAppendTestBundleMatrixVerifyContextEvent(pctx, p)
	}
	if pctx.Outcome != nil {
		pctx.Outcome[pipeline.OutcomeKeyFinalizeDone] = true
		pctx.Outcome[pipeline.OutcomeKeyFinalizeAt] = zqktime.NowRFC3339UTC()
	}
	return payload, nil
}

func maybeAppendTestBundleMatrixVerifyContextEvent(pctx *pipeline.Context, p *testBundleMatrixPayload) {
	if pctx == nil || p == nil || p.opts == nil || pctx.Outcome == nil || !matrixVerifySucceededForContextEvent(pctx.Outcome) {
		return
	}
	root := strings.TrimSpace(p.opts.ProjectRoot)
	if root == "" {
		return
	}
	csvPath := strings.TrimSpace(p.opts.OutputPath)
	if csvPath == "" {
		return
	}
	rel := csvPath
	if filepath.IsAbs(csvPath) {
		if r, err := filepath.Rel(root, csvPath); err == nil {
			rel = r
		}
	} else {
		rel = filepath.Clean(csvPath)
	}
	payload := map[string]any{
		objects.FieldKeyPath: rel,
	}
	if bp := strings.TrimSpace(p.opts.BundlePrefix); bp != "" {
		payload[contextevents.WirePayloadBundlePrefix] = bp
	}
	payload[contextevents.WirePayloadStrictVerify] = p.opts.StrictVerify
	rec := &contextevents.Record{
		EventType: contextevents.EventTypeTestBundleMatrixVerifyOK,
		Source:    contextevents.SourceTestBundleMatrixPipeline,
		Payload:   payload,
	}
	_ = contextevents.Append(root, rec)
}

func matrixVerifySucceededForContextEvent(outcome map[string]any) bool {
	if outcome == nil {
		return false
	}
	if skipped, ok := outcome[pipeline.OutcomeKeyVerifySkipped].(bool); ok && skipped {
		return false
	}
	ok, hit := outcome[pipeline.OutcomeKeyVerifyExit].(bool)
	return hit && ok
}
