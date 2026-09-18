package projecttemp

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"io"
	"path/filepath"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

func discardLogger() logging.Logger {
	return logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewJSONFormatter(pkgctx.NewSystemContext()))
}

// NewIsolatedRootStripPipeline builds the standard two-stage strip: guard (empty / git) then layout removal.
// Metrics are disabled (discard logger, no metrics sink) so nested use inside storage's larger teardown pipeline
// does not double-count.
func NewIsolatedRootStripPipeline() *pipeline.Pipeline {
	return pipeline.NewBuilder(PipelineKindIsolatedRootStrip, discardLogger()).
		AddStage(StageGuardStripPreconditions, stageGuardStripPreconditions()).
		AddStage(StageStripZQKLayout, stageStripZQKLayout()).
		Build()
}

func stageGuardStripPreconditions() pipeline.StageFunc {
	return func(pctx *pipeline.Context, payload any) (any, error) {
		root, _ := payload.(string)
		if root == emptyValue {
			pctx.Outcome[OutcomeStripSkippedEmpty] = true
			return payload, nil
		}
		if IsProbableGitWorktreeRoot(root) {
			pctx.Outcome[OutcomeStripSkippedGitWorktree] = true
		}
		return payload, nil
	}
}

func outcomeSkipStrip(pctx *pipeline.Context) bool {
	if pctx == nil || pctx.Outcome == nil {
		return false
	}
	if v, ok := pctx.Outcome[OutcomeStripSkippedEmpty].(bool); ok && v {
		return true
	}
	if v, ok := pctx.Outcome[OutcomeStripSkippedGitWorktree].(bool); ok && v {
		return true
	}
	return false
}

func stageStripZQKLayout() pipeline.StageFunc {
	return func(pctx *pipeline.Context, payload any) (any, error) {
		if outcomeSkipStrip(pctx) {
			return nil, nil
		}
		root, ok := payload.(string)
		if !ok || root == emptyValue {
			return nil, nil
		}
		_ = fileutil.RemoveAll(datacell.ProcessPrimaryDir(root))          //nolint:errcheck // best-effort teardown
		_ = fileutil.RemoveAll(filepath.Join(root, paths.ProjectDataDir)) //nolint:errcheck // best-effort teardown
		return nil, nil
	}
}

// RunIsolatedRootStrip runs [NewIsolatedRootStripPipeline] for projectRoot. It is safe for import from
// pkg/validation and other packages that cannot depend on pkg/storage.
func RunIsolatedRootStrip(projectRoot string) error {
	pl := NewIsolatedRootStripPipeline()
	pctx := &pipeline.Context{Ctx: pkgctx.NewSystemContext(), Outcome: make(map[string]any)}
	_, err := pl.Run(pctx, projectRoot)
	return err
}
