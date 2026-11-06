package interceptor

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

// PipelineInterceptor manages the search interception pipeline lifecycle.
type PipelineInterceptor struct {
	pipeline *pipeline.Pipeline
	options  Options
}

// DefaultInterceptor is an alias for PipelineInterceptor.
type DefaultInterceptor = PipelineInterceptor

// BuildPipeline constructs the underlying [pipeline.Pipeline] for search command interception.
func BuildPipeline(logger logging.Logger, opts Options) (*pipeline.Pipeline, error) {
	b := pipeline.NewBuilder(PipelineKindSearchInterceptor, logger).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(StageFunnelFilter, stageFunnelFilter).
		AddStage(StageStreamFilterDetector, stageStreamFilterDetector).
		AddStage(StagePathBoundaryDetector, stagePathBoundaryDetector).
		AddStage(StageIntentClassifier, stageIntentClassifier).
		AddStage(StageRewriteTransformer, stageRewriteTransformer(opts))

	return b.Build(), nil
}

// NewInterceptor initializes a PipelineInterceptor with the given options.
func NewInterceptor(logger logging.Logger, opts ...Option) (*PipelineInterceptor, error) {
	var cfg Options
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	pl, err := BuildPipeline(logger, cfg)
	if err != nil {
		return nil, err
	}

	return &PipelineInterceptor{
		pipeline: pl,
		options:  cfg,
	}, nil
}

// NewDefaultInterceptor returns a default PipelineInterceptor without extra options.
func NewDefaultInterceptor() *PipelineInterceptor {
	in, _ := NewInterceptor(nil)
	return in
}

// Intercept executes the pipeline on the given request.
func (in *PipelineInterceptor) Intercept(ctx context.Context, req InterceptRequest) (*InterceptResult, error) {
	if in == nil || in.pipeline == nil {
		return nil, errfmt.Errorf("search interceptor: pipeline uninitialized")
	}

	pctx := pipeline.NewContext(ctx)
	initialState := &PipelineState{
		Req: req,
	}

	out, err := in.pipeline.Run(pctx, initialState)
	if err != nil {
		return nil, err
	}

	state, ok := out.(*PipelineState)
	if !ok {
		return nil, errfmt.Errorf("search interceptor: unexpected final state type %T", out)
	}

	return &state.Result, nil
}

// Intercept is a package-level helper that executes the search interception pipeline.
func Intercept(ctx context.Context, req InterceptRequest, opts ...Option) (*InterceptResult, error) {
	in, err := NewInterceptor(nil, opts...)
	if err != nil {
		return nil, err
	}
	return in.Intercept(ctx, req)
}
