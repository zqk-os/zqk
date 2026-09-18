package specorigination

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/pipeline"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func testModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
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

func TestYAMLConfigDocSchemaHeaderIsIdempotent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	const source = "$schema: schema.json\n\nversion: 1.0.0\n"
	if err := fileutil.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		doc, err := loadYAMLConfigDoc(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeYAMLConfigDoc(path, doc); err != nil {
			t.Fatal(err)
		}
	}

	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(data), "$schema:"); count != 1 {
		t.Fatalf("schema header count = %d, want 1:\n%s", count, data)
	}
}
