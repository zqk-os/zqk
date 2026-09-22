// BLI-STARTER-COMMUNITY-056 / PRI-STARTER-COMMUNITY-056 coverage elevation
package testing

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	stdtesting "testing"

	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraProcessorsAndRegistry(t *stdtesting.T) {
	t.Parallel()
	ok, err, cont := ResponseProcessorFunc(func(r any, e error) (any, error, bool) {
		return r, e, true
	}).ProcessResponse("x", nil)
	if ok != "x" || err != nil || !cont {
		t.Fatalf("func processor: %v %v %v", ok, err, cont)
	}

	el := &ElicitationToSuccessProcessor{}
	got, e2, c2 := el.ProcessResponse("ok", nil)
	if got != "ok" || e2 != nil || !c2 {
		t.Fatalf("elicitation nil err: %v %v", got, e2)
	}
	elErr := mcp.NewElicitationError("need input", nil)
	got, e2, c2 = el.ProcessResponse(nil, elErr)
	if e2 != nil || !c2 {
		t.Fatal("elicitation should succeed")
	}
	m, _ := got.(map[string]any)
	if m["elicitation"] != true {
		t.Fatalf("elicitation map: %#v", got)
	}
	got, e2, _ = el.ProcessResponse("keep", errors.New("other"))
	if e2 == nil || got != "keep" {
		t.Fatal("non-elicitation error should pass through")
	}

	ig := &IgnoreErrorsProcessor{}
	got, e2, _ = ig.ProcessResponse("ok", nil)
	if e2 != nil || got != "ok" {
		t.Fatal("ignore nil")
	}
	got, e2, _ = ig.ProcessResponse("orig", errors.New("boom"))
	if e2 != nil {
		t.Fatal("ignore should swallow")
	}

	tr := &ErrorTranslatorProcessor{TranslateFunc: func(e error) (any, error) {
		return map[string]any{"translated": e.Error()}, nil
	}}
	got, e2, _ = tr.ProcessResponse("x", nil)
	if got != "x" || e2 != nil {
		t.Fatal("translator nil")
	}
	got, e2, _ = tr.ProcessResponse(nil, errors.New("x"))
	if e2 != nil {
		t.Fatal("translator err")
	}

	skip := ResponseProcessorFunc(func(any, error) (any, error, bool) { return "skip", nil, false })
	chain := &ChainedResponseProcessor{Processors: []ResponseProcessor{skip, ig}}
	got, _, cont = chain.ProcessResponse(nil, errors.New("n"))
	if got != "skip" || cont {
		t.Fatalf("chain stop: %v %v", got, cont)
	}
	chain2 := &ChainedResponseProcessor{Processors: []ResponseProcessor{ig}}
	if _, e, c := chain2.ProcessResponse(nil, errors.New("n")); e != nil || !c {
		t.Fatal("chain continue")
	}

	cond := &ConditionalProcessor{
		Condition: func(_ any, err error) bool { return err != nil },
		Processor: ig,
		Otherwise: ResponseProcessorFunc(func(r any, e error) (any, error, bool) { return r, e, true }),
	}
	if _, e, _ := cond.ProcessResponse("a", errors.New("e")); e != nil {
		t.Fatal("cond err branch")
	}
	if got, _, _ := cond.ProcessResponse("b", nil); got != "b" {
		t.Fatal("cond otherwise")
	}
	condNil := &ConditionalProcessor{
		Condition: func(any, error) bool { return false },
		Processor: ig,
	}
	if got, _, _ := condNil.ProcessResponse("c", nil); got != "c" {
		t.Fatal("cond no otherwise")
	}

	cfg := NewDefaultResponseProcessorConfig()
	if cfg.GetProcessorForStep("missing") == nil {
		t.Fatal("default processor")
	}
	noop := NewNoOpResponseProcessorConfig()
	if noop.GetProcessorForStep("x") == nil {
		t.Fatal("noop processor")
	}
	stepCfg := &ResponseProcessorConfig{
		StepProcessors:  map[string]ResponseProcessor{"s1": ig},
		GlobalProcessor: el,
	}
	if stepCfg.GetProcessorForStep("s1") != ig {
		t.Fatal("step-specific")
	}
	if stepCfg.GetProcessorForStep("other") != el {
		t.Fatal("global fallback")
	}

	reg := GetGlobalProcessorRegistry()
	if _, err := reg.Get("no-such-processor"); err == nil {
		t.Fatal("unknown processor")
	}
	names := reg.List()
	if len(names) == 0 {
		t.Fatal("expected built-in processors")
	}
	RegisterProcessor("extra_cov_noop", func() ResponseProcessor {
		return ResponseProcessorFunc(func(r any, e error) (any, error, bool) { return r, e, true })
	})
	if _, err := GetProcessor("extra_cov_noop"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetProcessor("still-missing"); err == nil {
		t.Fatal("GetProcessor miss")
	}
}

func TestExtraExecutorSkipCleanupAndWriter(t *stdtesting.T) {
	t.Parallel()
	server := mcp.NewServer()
	mcp.RegisterCommonTools(server)
	ex := NewScenarioExecutor(server)
	ex.SetResponseProcessor(NewNoOpResponseProcessorConfig())
	ex2 := NewScenarioExecutorWithProcessor(server, NewDefaultResponseProcessorConfig())
	if ex2 == nil {
		t.Fatal("with processor")
	}

	fail := false
	res, err := ex.RunScenario(&TestScenario{
		Name: "skip-and-cleanup",
		Tests: []TestStep{
			{Name: "skipped", Skip: true},
			{Name: "dep-miss", Tool: "nope", DependsOn: []string{"never-stored"}},
		},
		Cleanup: []TestStep{{Name: "cleanup", Tool: "definitely-missing"}},
	})
	if err == nil {
		t.Fatal("expected dependency error")
	}
	if res == nil || !res.StepResults[0].Skipped {
		t.Fatalf("skip result: %#v", res)
	}

	if _, err := ex.RunScenario(&TestScenario{Name: "bad-proc", ResponseProcessor: "nope-proc"}); err == nil {
		t.Fatal("unknown scenario processor")
	}

	_, _ = ex.RunScenario(&TestScenario{
		Name: "expected-fail",
		Tests: []TestStep{{
			Name:     "fail-ok",
			Tool:     "definitely-missing",
			Expected: &TestExpectation{Success: &fail},
		}},
	})

	okTrue := true
	_, _ = ex.RunScenario(&TestScenario{
		Name: "legacy-prefix",
		Tests: []TestStep{{
			Name: "legacy",
			Tool: "zqk_nonexistent_tool",
			Skip: false,
		}},
	})
	_, _ = ex.RunScenario(&TestScenario{
		Name: "fields-skip",
		Tests: []TestStep{{
			Name: "echo-ish",
			Skip: true,
			Expected: &TestExpectation{
				Success:      &okTrue,
				HasFields:    []string{"id"},
				NotHasFields: []string{"secret"},
				Equals:       map[string]any{"id": "1"},
				HasField:     map[string]any{"id": "1"},
				Matches:      map[string]string{"id": "^1$"},
			},
		}},
	})

	w := NewScenarioWriter()
	sc := NewScenarioBuilder().Name("w").Description("d").ResponseProcessor("noop").
		AddImport("x.yaml").
		AddCleanupStep(SimpleStep("c", "tool")).
		AddTestStep(NewStepBuilder().Name("t").Tool("tool").Args(map[string]any{"a": 1}).ImportPath("args").Build()).
		Build()
	b, err := w.WriteToBytes(sc)
	if err != nil || len(b) == 0 {
		t.Fatal(err)
	}
	s, err := w.WriteString(sc)
	if err != nil || !strings.Contains(s, "w") {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := w.Write(sc, &buf); err != nil || buf.Len() == 0 {
		t.Fatal(err)
	}

	out := t.TempDir()
	gen := NewScenarioGenerator(out)
	if err := gen.GenerateFromSpecs([]ScenarioSpec{
		{Name: "Hello World!", Description: "d", ResponseProcessor: "noop", Imports: []string{"a.yaml"},
			Setup: []TestStep{{Name: "s", Tool: "t"}}, Tests: []TestStep{{Name: "t", Tool: "t"}}, Cleanup: []TestStep{{Name: "c", Tool: "t"}}},
		{Name: "!!!", Tests: []TestStep{{Name: "t", Tool: "t"}}},
	}); err != nil {
		t.Fatal(err)
	}

	spec := filepath.Join(t.TempDir(), "specs.yaml")
	if err := fileutil.WriteFile(spec, []byte("scenarios:\n  - name: from-file\n    tests:\n      - name: t\n        tool: x\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := GenerateScenariosFromSpecFile(spec, t.TempDir()); err != nil {
		t.Fatal(err)
	}

	_ = ExpectElicitation()
	_ = ExpectFailure()
	_ = NewErrorExpectationBuilder().Code(1).Message("m").Type("elicitation").Build()
	_ = NewStepBuilder().Description("d").Import("i.yaml").Skip(true).StoreResult("k").DependsOn("a").Build()
}

func TestExtraLoaderMergePath(t *stdtesting.T) {
	t.Parallel()
	dir := t.TempDir()
	imp := filepath.Join(dir, "data.yaml")
	if err := fileutil.WriteFile(imp, []byte("k: v\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	scPath := filepath.Join(dir, "sc.yaml")
	body := "name: n\ntests:\n  - name: t\n    tool: x\n    import: data.yaml\n    import_path: args\n"
	if err := fileutil.WriteFile(scPath, []byte(body), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	sc, err := NewScenarioLoader(dir).LoadScenario(scPath)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Tests[0].Args["k"] != "v" {
		t.Fatalf("import args: %#v", sc.Tests[0].Args)
	}

	bad := filepath.Join(dir, "badpath.yaml")
	if err := fileutil.WriteFile(bad, []byte("name: n\ntests:\n  - name: t\n    tool: x\n    import: data.yaml\n    import_path: nope.nested\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewScenarioLoader(dir).LoadScenario(bad); err == nil {
		t.Fatal("unsupported merge path")
	}
}
