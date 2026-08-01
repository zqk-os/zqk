package specorigination

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/pipeline"
)

func testModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("go.mod not found")
		}
		dir = parent
	}
}

func TestRun_auditable_dryRun_softFinalize(t *testing.T) {
	t.Parallel()
	root := testModuleRoot(t)
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))
	pctx := &pipeline.Context{Ctx: pkgctx.NewSystemContext(), Outcome: make(map[string]any)}
	st, err := Run(pctx, logger, Options{
		ProjectRoot:            root,
		Ontology:               "auditable",
		DryRun:                 true,
		SkipFinalizeValidation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.SpecPath == "" || st.Spec == nil {
		t.Fatal("expected spec path and loaded spec")
	}
	if pctx.Outcome[pipeline.OutcomeKeyFinalizeValidationSoft] != true {
		t.Fatalf("expected soft finalize: %#v", pctx.Outcome)
	}
	if v, ok := pctx.Outcome[pipeline.OutcomeKeyTriggerExecuted]; ok && v == true {
		t.Fatalf("dry-run must not execute trigger subprocesses: %#v", pctx.Outcome)
	}
}

func TestBuildSpecOriginationPipeline_invalidOptions(t *testing.T) {
	t.Parallel()
	logger := logging.NewLogger(io.Discard, logging.InfoLevel, logging.NewTextFormatter(pkgctx.NewSystemContext()))
	_, err := BuildSpecOriginationPipeline(logger, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
}
