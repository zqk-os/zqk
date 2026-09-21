package vds

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestEvaluate_passStructuralAndObjectExists(t *testing.T) {
	lookup := func(_ context.Context, id string) (map[string]any, error) {
		if id == "GOAL-1" {
			return map[string]any{objects.FieldKeyID: "GOAL-1", objects.FieldKeyTitle: "Mission"}, nil
		}
		return nil, fileutil.ErrNotExist
	}
	chunks := []Chunk{{
		ChunkID:           "c1",
		Stage:             "intent_capture",
		Claim:             "Goal captures mission",
		WorkObjectRef:     "GOAL-1",
		RubricRef:         "POL-WORKFLOW-VDS#intent",
		DSLChecks:         []string{"object_exists:GOAL-1", "field_nonempty:GOAL-1:title"},
		EvidenceRefs:      []string{"GOAL-1"},
		GateIntent:        "yes",
		IndependentVerify: "pending",
	}}
	rep := Evaluate(context.Background(), chunks, &SpineProfile{ProfileID: "test"}, &Customization{}, EvalOptions{
		Lookup: lookup,
	})
	if rep.Verdict != "PASS" {
		t.Fatalf("verdict=%s brief=%s", rep.Verdict, rep.AgentBrief)
	}
	if len(rep.Chunks) != 1 || rep.Chunks[0].Verdict != "PASS" {
		t.Fatalf("chunk: %+v", rep.Chunks)
	}
}

func TestEvaluate_failMissingEvidence(t *testing.T) {
	chunks := []Chunk{{
		ChunkID:           "c2",
		Stage:             "implement",
		Claim:             "shipped feature",
		RubricRef:         "CRIT-1",
		DSLChecks:         []string{"tests_ok_per_customization"},
		EvidenceRefs:      nil,
		GateImplement:     "yes",
		IndependentVerify: "yes",
	}}
	rep := Evaluate(context.Background(), chunks, nil, &Customization{
		TestExecution: TestExecutionPrefs{Mode: "scheduler"},
	}, EvalOptions{})
	if rep.Passed() {
		t.Fatal("expected fail without evidence_refs")
	}
}

func TestEvaluate_schedulerTestsNeedJobAndLog(t *testing.T) {
	chunks := []Chunk{{
		ChunkID:           "c3",
		Stage:             "implement",
		Claim:             "tests green",
		RubricRef:         "CRIT-1",
		DSLChecks:         []string{"tests_ok_per_customization"},
		EvidenceRefs:      []string{"SCH-run-foo", filepath.Join(paths.ProjectDataDir, paths.LogsDir, paths.SchedulerSubdir, "test-bundles", "foo.log")},
		GateImplement:     "yes",
		IndependentVerify: "yes",
	}}
	rep := Evaluate(context.Background(), chunks, nil, &Customization{
		TestExecution: TestExecutionPrefs{Mode: "scheduler"},
	}, EvalOptions{})
	if !rep.Passed() {
		t.Fatalf("expected pass, got %s preds=%+v", rep.Verdict, rep.Chunks[0].Predicates)
	}
}

func TestEvaluate_foregroundTestsAcceptShellTestCommand(t *testing.T) {
	chunks := []Chunk{{
		ChunkID:           "shell-test",
		Stage:             "integrate_verify",
		Claim:             "payload gate passes",
		RubricRef:         "CRIT-1",
		DSLChecks:         []string{"tests_ok_per_customization"},
		EvidenceRefs:      []string{"sh scripts/open-core/test-public-release-gates.sh --payload-only"},
		GateIntegrate:     "yes",
		IndependentVerify: "yes",
	}}
	rep := Evaluate(context.Background(), chunks, nil, &Customization{
		TestExecution: TestExecutionPrefs{Mode: "foreground"},
	}, EvalOptions{})
	if !rep.Passed() {
		t.Fatalf("expected pass, got %s preds=%+v", rep.Verdict, rep.Chunks[0].Predicates)
	}
}

func TestLoadSpineFromRepo(t *testing.T) {
	root := findRepoRoot(t)
	spine, err := LoadSpine(root, DefaultSpineProfileRel)
	if err != nil {
		t.Fatal(err)
	}
	if spine.ProfileID == "" || len(spine.Stages) != 5 {
		t.Fatalf("spine: %+v", spine)
	}
}

func TestEvaluate_includesGlossaryFromPins(t *testing.T) {
	cust := &Customization{Glossary: GlossaryBinding{TermRef: "GLS-PIN"}}
	chunks := []Chunk{{
		ChunkID: "c", Stage: "design", Claim: "x", RubricRef: "r",
		DSLChecks:    []string{"smoke_or_integration_evidence_present"},
		EvidenceRefs: []string{"smoke-log"}, GateDesign: "yes", IndependentVerify: "yes",
	}}
	rep := Evaluate(context.Background(), chunks, nil, cust, EvalOptions{})
	if rep.GlossaryTermRef != "GLS-PIN" || rep.GlossaryResolvedBy != "customization" {
		t.Fatalf("%+v", rep)
	}
	if rep.GlossaryCanonicalTitle != GlossaryCanonicalTitle {
		t.Fatalf("title=%s", rep.GlossaryCanonicalTitle)
	}
}

func TestInitChunksFile(t *testing.T) {
	dir := t.TempDir()
	path, created, err := InitChunksFile(dir, false)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if _, err := fileutil.Stat(path); err != nil {
		t.Fatal(err)
	}
	_, created2, err := InitChunksFile(dir, false)
	if err != nil || created2 {
		t.Fatalf("second init should not create: created=%v err=%v", created2, err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := fileutil.Stat(filepath.Join(dir, DefaultSpineProfileRel)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Skip("repo spine profile not found from test cwd")
	return ""
}
