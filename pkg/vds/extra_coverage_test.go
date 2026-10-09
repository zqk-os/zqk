package vds

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestChunk_SaveAndResolveChunksPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nested", "chunks.yaml")

	// Empty chunks slice should error
	if err := SaveChunks(p, nil); err == nil {
		t.Fatal("expected error saving empty chunks")
	}

	chunks := []Chunk{
		{
			ChunkID:           "CHK-1",
			Stage:             "design",
			Claim:             "test claim",
			RubricRef:         "CRIT-1",
			DSLChecks:         []string{"object_exists:BLI-1"},
			EvidenceRefs:      []string{"REF-1"},
			IndependentVerify: "yes",
		},
	}
	if err := SaveChunks(p, chunks); err != nil {
		t.Fatalf("SaveChunks failed: %v", err)
	}

	loaded, err := LoadChunks(p)
	if err != nil {
		t.Fatalf("LoadChunks failed: %v", err)
	}
	if len(loaded) != 1 || loaded[0].ChunkID != "CHK-1" {
		t.Fatalf("unexpected loaded chunks: %+v", loaded)
	}

	// ResolveChunksPath tests
	if got := ResolveChunksPath("/proj", ""); got != filepath.Join("/proj", DefaultChunksRel) {
		t.Fatalf("got %s, want default", got)
	}
	if got := ResolveChunksPath("/proj", "/abs/chunks.yaml"); got != "/abs/chunks.yaml" {
		t.Fatalf("got %s, want /abs/chunks.yaml", got)
	}
	if got := ResolveChunksPath("/proj", "custom/chunks.yaml"); got != filepath.Join("/proj", "custom/chunks.yaml") {
		t.Fatalf("got %s, want relative joined", got)
	}

	// KnownStage
	if !KnownStage("design") {
		t.Fatal("expected KnownStage(design) to be true")
	}
	if KnownStage("nonexistent_stage") {
		t.Fatal("expected KnownStage(nonexistent_stage) to be false")
	}

	// IsDoneValue
	if !IsDoneValue("YES", nil) || !IsDoneValue("na", nil) {
		t.Fatal("expected yes/na to be true with nil done list")
	}
	if IsDoneValue("pending", nil) {
		t.Fatal("expected pending to be false")
	}
	if !IsDoneValue("finished", []string{"finished", "approved"}) {
		t.Fatal("expected finished to match custom done list")
	}
	if IsDoneValue("", nil) {
		t.Fatal("expected empty string to be false")
	}
}

func TestReport_PassedAndPassedChunkIDs(t *testing.T) {
	var nilRep *Report
	if nilRep.Passed() {
		t.Fatal("nil report should not pass")
	}
	if ids := nilRep.PassedChunkIDs(); ids != nil {
		t.Fatalf("nil report should return nil chunk ids, got %v", ids)
	}

	rep := &Report{
		Verdict: "PASS",
		Chunks: []ChunkResult{
			{ChunkID: "c1", Verdict: "PASS"},
			{ChunkID: "c2", Verdict: "FAIL"},
			{ChunkID: "c3", Verdict: "WAIVED"},
		},
	}
	if !rep.Passed() {
		t.Fatal("rep should pass")
	}
	passIDs := rep.PassedChunkIDs()
	if len(passIDs) != 2 || passIDs[0] != "c1" || passIDs[1] != "c3" {
		t.Fatalf("unexpected passed chunk ids: %v", passIDs)
	}
}

func TestGlossary_IDFromObject(t *testing.T) {
	if got := IDFromObject(nil); got != "" {
		t.Fatalf("expected empty for nil, got %q", got)
	}
	if got := IDFromObject(map[string]any{"other": 123}); got != "" {
		t.Fatalf("expected empty for missing id, got %q", got)
	}
	if got := IDFromObject(map[string]any{objects.FieldKeyID: "GLS-100 "}); got != "GLS-100" {
		t.Fatalf("expected GLS-100, got %q", got)
	}
}

func TestChecklist_BuildAndRender(t *testing.T) {
	gls := GlossaryRefs{
		TermRef:    "GLS-1",
		AcronymRef: "GLS-2",
		ResolvedBy: "title",
	}
	cust := &Customization{
		ProjectID: "proj-test",
		TestExecution: TestExecutionPrefs{
			Mode: "scheduler",
		},
		ProcessData: ProcessDataPrefs{
			Mutation: "cli_intake_only",
		},
		CodeStyle: CodeStylePrefs{
			LintCommands: []string{"golangci-lint run"},
		},
	}

	// With spine nil
	repNilSpine := BuildChecklist(nil, cust, gls)
	if repNilSpine == nil || len(repNilSpine.Stages) == 0 {
		t.Fatal("expected non-empty stages with nil spine")
	}

	// With spine present
	spine := &SpineProfile{
		ProfileID: "spine-v1",
		Stages: []StageDef{
			{ID: "intent_capture", Label: "Intent", Purpose: "Capture intent"},
			{ID: "design", Label: "Design", Purpose: "System design"},
		},
	}
	rep := BuildChecklist(spine, cust, gls)
	if rep == nil {
		t.Fatal("expected non-nil checklist report")
	}
	brief := RenderChecklistBrief(rep)
	if !strings.Contains(brief, "proj-test") && !strings.Contains(brief, "VDS checklist") {
		t.Fatalf("unexpected brief: %s", brief)
	}
	if emptyBrief := RenderChecklistBrief(nil); emptyBrief != "" {
		t.Fatalf("expected empty brief for nil report, got %q", emptyBrief)
	}
	if emptyAgent := RenderAgentBrief(nil); emptyAgent != "" {
		t.Fatalf("expected empty agent brief for nil report, got %q", emptyAgent)
	}
}

func TestProfile_ResolveProfilesAndQualityDir(t *testing.T) {
	dir := t.TempDir()
	qd := QualityDir(dir)
	if qd != filepath.Join(dir, paths.DocsQualityDir) {
		t.Fatalf("unexpected quality dir: %s", qd)
	}

	spinePath := filepath.Join(dir, DefaultSpineProfileRel)
	custPath := filepath.Join(dir, DefaultCustomizationProfileRel)

	if err := fileutil.MkdirAll(filepath.Dir(spinePath), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	spineYAML := `schema: zqk_vds_spine_v1
profile_id: spine-test
title: Spine Test
customization_profile_path: docs/quality/verifiable_decomposition_customization.yaml
stages:
  - id: intent_capture
    label: Intent
    purpose: Capture intent
`
	if err := fileutil.WriteFile(spinePath, []byte(spineYAML), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	custYAML := `schema: zqk_vds_project_customization_v1
project_id: cust-test
test_execution:
  mode: foreground
`
	if err := fileutil.WriteFile(custPath, []byte(custYAML), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	spine, cust, err := ResolveProfiles(dir, "", "")
	if err != nil {
		t.Fatalf("ResolveProfiles failed: %v", err)
	}
	if spine == nil || spine.ProfileID != "spine-test" {
		t.Fatalf("unexpected spine: %+v", spine)
	}
	if cust == nil || cust.ProjectID != "cust-test" {
		t.Fatalf("unexpected cust: %+v", cust)
	}
}

func TestPredicates_FullCoverage(t *testing.T) {
	ctx := context.Background()

	// Empty predicate
	res := EvalPredicate(ctx, "", Chunk{}, EvalOptions{})
	if res.OK || res.Detail != "empty predicate" {
		t.Fatalf("expected empty predicate error, got %+v", res)
	}

	// Unknown predicate
	res = EvalPredicate(ctx, "unknown_dsl_check", Chunk{}, EvalOptions{})
	if res.OK || !strings.Contains(res.Detail, "unknown predicate") {
		t.Fatalf("expected unknown predicate, got %+v", res)
	}

	// criteria_linked_or_acceptance_present
	res = EvalPredicate(ctx, "criteria_linked_or_acceptance_present", Chunk{}, EvalOptions{})
	if res.OK || res.Detail != "missing object id" {
		t.Fatalf("expected missing object id, got %+v", res)
	}

	res = EvalPredicate(ctx, "criteria_linked_or_acceptance_present:OBJ-1", Chunk{}, EvalOptions{})
	if res.OK || !res.Skipped || res.Detail != "no object lookup wired" {
		t.Fatalf("expected no lookup wired, got %+v", res)
	}

	lookupErr := func(_ context.Context, _ string) (map[string]any, error) {
		return nil, fileutil.ErrNotExist
	}
	res = EvalPredicate(ctx, "criteria_linked_or_acceptance_present:OBJ-1", Chunk{}, EvalOptions{Lookup: lookupErr})
	if res.OK || !strings.Contains(res.Detail, "file does not exist") {
		t.Fatalf("expected lookup error, got %+v", res)
	}

	lookupOKWithCriteria := func(_ context.Context, _ string) (map[string]any, error) {
		return map[string]any{
			objects.FieldKeyCriteriaRefs: []string{"CRIT-1"},
		}, nil
	}
	res = EvalPredicate(ctx, "criteria_linked_or_acceptance_present:OBJ-1", Chunk{}, EvalOptions{Lookup: lookupOKWithCriteria})
	if !res.OK || res.Detail != "criteria_refs present" {
		t.Fatalf("expected criteria_refs present, got %+v", res)
	}

	lookupOKNoCriteria := func(_ context.Context, _ string) (map[string]any, error) {
		return map[string]any{
			objects.FieldKeyCriteriaRefs: []string{},
		}, nil
	}
	res = EvalPredicate(ctx, "criteria_linked_or_acceptance_present:OBJ-1", Chunk{}, EvalOptions{Lookup: lookupOKNoCriteria})
	if res.OK || res.Detail != "no criteria_refs" {
		t.Fatalf("expected no criteria_refs, got %+v", res)
	}

	// git_diff_nonempty_or_waiver
	res = EvalPredicate(ctx, "git_diff_nonempty_or_waiver", Chunk{WaiverRef: "WAIVE-1"}, EvalOptions{})
	if !res.OK || res.Detail != "waiver_ref set" {
		t.Fatalf("expected waiver_ref set, got %+v", res)
	}

	res = EvalPredicate(ctx, "git_diff_nonempty_or_waiver", Chunk{}, EvalOptions{ProjectRoot: ""})
	if res.OK || res.Detail != "project root unknown" {
		t.Fatalf("expected project root unknown, got %+v", res)
	}

	// lint_ok_per_customization
	res = EvalPredicate(ctx, "lint_ok_per_customization", Chunk{}, EvalOptions{Customization: &Customization{}})
	if !res.OK || !res.Skipped {
		t.Fatalf("expected skipped when no lint commands, got %+v", res)
	}

	custWithLint := &Customization{
		CodeStyle: CodeStylePrefs{
			LintCommands: []string{"echo lint ok"},
		},
	}
	res = EvalPredicate(ctx, "lint_ok_per_customization", Chunk{}, EvalOptions{Customization: custWithLint, RunCommands: false})
	if res.OK || !res.Skipped {
		t.Fatalf("expected skipped when RunCommands=false, got %+v", res)
	}

	res = EvalPredicate(ctx, "lint_ok_per_customization", Chunk{}, EvalOptions{
		Customization: custWithLint,
		RunCommands:   true,
		ProjectRoot:   ".",
	})
	if !res.OK {
		t.Fatalf("expected lint commands to exit 0, got %+v", res)
	}

	custWithBadLint := &Customization{
		CodeStyle: CodeStylePrefs{
			LintCommands: []string{"nonexistent_command_12345"},
		},
	}
	res = EvalPredicate(ctx, "lint_ok_per_customization", Chunk{}, EvalOptions{
		Customization:  custWithBadLint,
		RunCommands:    true,
		ProjectRoot:    ".",
		CommandTimeout: 2 * time.Second,
	})
	if res.OK {
		t.Fatalf("expected bad lint command to fail, got %+v", res)
	}

	// ci_required_checks_green_or_na
	res = EvalPredicate(ctx, "ci_required_checks_green_or_na", Chunk{}, EvalOptions{Customization: &Customization{}})
	if !res.OK || !res.Skipped {
		t.Fatalf("expected skipped when no required CI checks, got %+v", res)
	}

	custWithCI := &Customization{
		CICD: CICDPrefs{
			RequiredChecks: []string{"test-suite", "lint-suite"},
		},
	}
	res = EvalPredicate(ctx, "ci_required_checks_green_or_na", Chunk{EvidenceRefs: []string{"test-suite"}}, EvalOptions{Customization: custWithCI})
	if res.OK || !strings.Contains(res.Detail, "missing CI evidence for lint-suite") {
		t.Fatalf("expected missing CI evidence, got %+v", res)
	}

	res = EvalPredicate(ctx, "ci_required_checks_green_or_na", Chunk{EvidenceRefs: []string{"test-suite", "lint-suite"}}, EvalOptions{Customization: custWithCI})
	if !res.OK || res.Detail != "required CI checks referenced in evidence" {
		t.Fatalf("expected CI checks referenced, got %+v", res)
	}

	// security_gate_ok_or_na & performance_gate_ok_or_na
	custWithSec := &Customization{
		Security: StageGatePrefs{
			RequiredForStages: []string{"operate_release"},
			ScanCommands:      []string{"echo sec ok"},
		},
		Performance: StageGatePrefs{
			RequiredForStages: []string{"operate_release"},
			ScanCommands:      []string{"echo perf ok"},
		},
	}
	res = EvalPredicate(ctx, "security_gate_ok_or_na", Chunk{Stage: "design"}, EvalOptions{Customization: custWithSec})
	if !res.OK || !res.Skipped {
		t.Fatalf("expected security gate skipped outside operate_release, got %+v", res)
	}

	res = EvalPredicate(ctx, "security_gate_ok_or_na", Chunk{Stage: "operate_release"}, EvalOptions{
		Customization: custWithSec,
		RunCommands:   true,
		ProjectRoot:   ".",
	})
	if !res.OK || res.Detail != "security commands exited 0" {
		t.Fatalf("expected security commands exited 0, got %+v", res)
	}

	res = EvalPredicate(ctx, "performance_gate_ok_or_na", Chunk{Stage: "operate_release"}, EvalOptions{
		Customization: custWithSec,
		RunCommands:   true,
		ProjectRoot:   ".",
	})
	if !res.OK || res.Detail != "performance commands exited 0" {
		t.Fatalf("expected perf commands exited 0, got %+v", res)
	}

	// publish_ack_present_if_public
	custNoAck := &Customization{
		CICD: CICDPrefs{
			PublishRequiresAck: false,
		},
	}
	res = EvalPredicate(ctx, "publish_ack_present_if_public", Chunk{}, EvalOptions{Customization: custNoAck})
	if !res.OK || !res.Skipped {
		t.Fatalf("expected skipped when PublishRequiresAck=false, got %+v", res)
	}

	custAck := &Customization{
		CICD: CICDPrefs{
			PublishRequiresAck: true,
		},
	}
	res = EvalPredicate(ctx, "publish_ack_present_if_public", Chunk{
		Stage:        "operate_release",
		EvidenceRefs: []string{"token public_push_ack"},
	}, EvalOptions{Customization: custAck})
	if !res.OK || !strings.Contains(res.Detail, "publish ack") {
		t.Fatalf("expected publish ack present, got %+v", res)
	}

	res = EvalPredicate(ctx, "publish_ack_present_if_public", Chunk{
		Stage:        "implement",
		EvidenceRefs: []string{},
	}, EvalOptions{Customization: custAck})
	if !res.OK || !res.Skipped {
		t.Fatalf("expected skipped for implement stage, got %+v", res)
	}

	res = EvalPredicate(ctx, "publish_ack_present_if_public", Chunk{
		Stage:        "operate_release",
		EvidenceRefs: []string{},
	}, EvalOptions{Customization: custAck})
	if res.OK || !strings.Contains(res.Detail, "operate_release with publish_requires_ack needs public_push_ack evidence") {
		t.Fatalf("expected fail when operate_release has no ack evidence, got %+v", res)
	}
}

func TestPredicatesExtended_Coverage(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// 1. predCommandExitCode
	res := EvalPredicate(ctx, "command_exit_code:", Chunk{}, EvalOptions{})
	if res.OK || res.Detail != "missing command string" {
		t.Fatalf("expected missing command string, got %+v", res)
	}

	res = EvalPredicate(ctx, "command_exit_code:echo ok", Chunk{}, EvalOptions{RunCommands: false})
	if !res.OK || !res.Skipped {
		t.Fatalf("expected skipped when RunCommands=false, got %+v", res)
	}

	res = EvalPredicate(ctx, "command_exit_code:exit 0", Chunk{}, EvalOptions{
		RunCommands: true,
		ProjectRoot: tempDir,
	})
	if !res.OK || res.Detail != "command exited 0" {
		t.Fatalf("expected exit 0 OK, got %+v", res)
	}

	res = EvalPredicate(ctx, "command_exit_code:exit 1", Chunk{}, EvalOptions{
		RunCommands: true,
		ProjectRoot: tempDir,
	})
	if res.OK || !strings.Contains(res.Detail, "command failed") {
		t.Fatalf("expected command failed, got %+v", res)
	}

	// 2. predContentHashMatches
	res = EvalPredicate(ctx, "content_hash_matches:", Chunk{}, EvalOptions{})
	if res.OK || res.Detail != "missing path for content_hash_matches" {
		t.Fatalf("expected missing path, got %+v", res)
	}

	res = EvalPredicate(ctx, "content_hash_matches:file.txt", Chunk{}, EvalOptions{ProjectRoot: ""})
	if res.OK || !res.Skipped || res.Detail != "no project root" {
		t.Fatalf("expected no project root, got %+v", res)
	}

	res = EvalPredicate(ctx, "content_hash_matches:nonexistent.txt", Chunk{}, EvalOptions{ProjectRoot: tempDir})
	if res.OK || !strings.Contains(res.Detail, "target file does not exist") {
		t.Fatalf("expected file does not exist, got %+v", res)
	}

	testFile := filepath.Join(tempDir, "sample.txt")
	_ = fileutil.WriteFile(testFile, []byte("content"), fileutil.StandardFilePerm)
	res = EvalPredicate(ctx, "content_hash_matches:sample.txt", Chunk{}, EvalOptions{ProjectRoot: tempDir})
	if !res.OK || !strings.Contains(res.Detail, "content_hash verified") {
		t.Fatalf("expected hash verified, got %+v", res)
	}

	// 3. predContentSizePositive
	res = EvalPredicate(ctx, "content_size_positive:", Chunk{}, EvalOptions{})
	if res.OK || res.Detail != "missing target for content_size_positive" {
		t.Fatalf("expected missing target, got %+v", res)
	}

	emptyFile := filepath.Join(tempDir, "empty.txt")
	_ = fileutil.WriteFile(emptyFile, []byte(""), fileutil.StandardFilePerm)
	res = EvalPredicate(ctx, "content_size_positive:empty.txt", Chunk{}, EvalOptions{ProjectRoot: tempDir})
	if res.OK || !strings.Contains(res.Detail, "size is 0 bytes") {
		t.Fatalf("expected 0 bytes fail, got %+v", res)
	}

	res = EvalPredicate(ctx, "content_size_positive:sample.txt", Chunk{}, EvalOptions{ProjectRoot: tempDir})
	if !res.OK || !strings.Contains(res.Detail, "file size") {
		t.Fatalf("expected positive file size, got %+v", res)
	}

	res = EvalPredicate(ctx, "content_size_positive:other.txt", Chunk{}, EvalOptions{ProjectRoot: ""})
	if !res.OK || res.Detail != "content size positive verified" {
		t.Fatalf("expected verified for non-path, got %+v", res)
	}

	// 4. predFieldMatches
	res = EvalPredicate(ctx, "field_matches:", Chunk{}, EvalOptions{})
	if res.OK || res.Detail != "want field_matches:{field}:{regex}" {
		t.Fatalf("expected invalid format, got %+v", res)
	}

	res = EvalPredicate(ctx, "field_matches:field:[invalid(regex", Chunk{}, EvalOptions{})
	if res.OK || !strings.Contains(res.Detail, "invalid regex") {
		t.Fatalf("expected regex error, got %+v", res)
	}

	res = EvalPredicate(ctx, "field_matches:status:^ready$", Chunk{}, EvalOptions{})
	if !res.OK || res.Detail != "field matches pattern" {
		t.Fatalf("expected field matches, got %+v", res)
	}

	// 5. predQueryMetric
	res = EvalPredicate(ctx, "query_metric:coverage >= 80", Chunk{}, EvalOptions{})
	if !res.OK || !strings.Contains(res.Detail, "coverage") {
		t.Fatalf("expected metric satisfied, got %+v", res)
	}

	res = EvalPredicate(ctx, "query_metric:bad_metric_syntax", Chunk{}, EvalOptions{})
	if res.OK {
		t.Fatalf("expected metric parse error, got %+v", res)
	}

	// 6. predASTSemanticMatch
	res = EvalPredicate(ctx, "ast_semantic_match:", Chunk{}, EvalOptions{})
	if res.OK || res.Detail != "want ast_semantic_match:{path}:{constraint}" {
		t.Fatalf("expected format error, got %+v", res)
	}

	res = EvalPredicate(ctx, "ast_semantic_match:pkg:no_raw_panics", Chunk{}, EvalOptions{ProjectRoot: ""})
	if res.OK || !res.Skipped || res.Detail != "no project root" {
		t.Fatalf("expected skipped with no project root, got %+v", res)
	}

	// Create test go files in tempDir
	srcDir := filepath.Join(tempDir, "asttest")
	_ = fileutil.MkdirAll(srcDir, fileutil.StandardDirPerm)
	panicSrc := `package asttest
func Bad() {
	panic("crash")
}
`
	_ = fileutil.WriteFile(filepath.Join(srcDir, "bad.go"), []byte(panicSrc), fileutil.StandardFilePerm)

	cleanSrc := `package asttest
type MyCustomService struct{}
func Good() string {
	return "ok"
}
`
	cleanDir := filepath.Join(tempDir, "astclean")
	_ = fileutil.MkdirAll(cleanDir, fileutil.StandardDirPerm)
	_ = fileutil.WriteFile(filepath.Join(cleanDir, "clean.go"), []byte(cleanSrc), fileutil.StandardFilePerm)

	// no_raw_panics on badDir -> fail
	res = EvalPredicate(ctx, "ast_semantic_match:asttest:no_raw_panics", Chunk{}, EvalOptions{ProjectRoot: tempDir})
	if res.OK || !strings.Contains(res.Detail, "detected raw panic") {
		t.Fatalf("expected panic detected, got %+v", res)
	}

	// no_raw_panics on cleanDir -> pass
	res = EvalPredicate(ctx, "ast_semantic_match:astclean:no_raw_panics", Chunk{}, EvalOptions{ProjectRoot: tempDir})
	if !res.OK || !strings.Contains(res.Detail, "zero raw panics") {
		t.Fatalf("expected zero raw panics, got %+v", res)
	}

	// symbol_present
	res = EvalPredicate(ctx, "ast_semantic_match:astclean:symbol_present:MyCustomService", Chunk{}, EvalOptions{ProjectRoot: tempDir})
	if !res.OK || !strings.Contains(res.Detail, "symbol \"MyCustomService\" verified present") {
		t.Fatalf("expected symbol present, got %+v", res)
	}

	res = EvalPredicate(ctx, "ast_semantic_match:astclean:symbol_present:NonExistentSymbol", Chunk{}, EvalOptions{ProjectRoot: tempDir})
	if res.OK || !strings.Contains(res.Detail, "expected symbol \"NonExistentSymbol\" not found") {
		t.Fatalf("expected symbol not found, got %+v", res)
	}

	// symbol_absent
	res = EvalPredicate(ctx, "ast_semantic_match:astclean:symbol_absent:NonExistentSymbol", Chunk{}, EvalOptions{ProjectRoot: tempDir})
	if !res.OK || !strings.Contains(res.Detail, "verified absent") {
		t.Fatalf("expected symbol absent, got %+v", res)
	}

	res = EvalPredicate(ctx, "ast_semantic_match:astclean:symbol_absent:MyCustomService", Chunk{}, EvalOptions{ProjectRoot: tempDir})
	if res.OK || !strings.Contains(res.Detail, "forbidden symbol \"MyCustomService\" found") {
		t.Fatalf("expected forbidden symbol found, got %+v", res)
	}

	// unknown constraint
	res = EvalPredicate(ctx, "ast_semantic_match:astclean:unknown_constraint", Chunk{}, EvalOptions{ProjectRoot: tempDir})
	if res.OK || !strings.Contains(res.Detail, "unknown ast constraint") {
		t.Fatalf("expected unknown constraint, got %+v", res)
	}

	// 7. Standard checks pass
	res = EvalPredicate(ctx, "standard_checks_pass", Chunk{}, EvalOptions{})
	if !res.OK || res.Detail != "standard checks pass" {
		t.Fatalf("expected standard checks pass, got %+v", res)
	}
}

func TestInitChunks_And_ApplyVerify(t *testing.T) {
	tempDir := t.TempDir()

	// InitChunksFile
	p, created, err := InitChunksFile(tempDir, false)
	if err != nil || !created {
		t.Fatalf("InitChunksFile failed: %v, created=%v", err, created)
	}

	// Calling InitChunksFile again without force should not recreate
	_, created, err = InitChunksFile(tempDir, false)
	if err != nil || created {
		t.Fatalf("expected created=false on existing chunks file, got created=%v, err=%v", created, err)
	}

	// Calling InitChunksFile with force should succeed and recreate
	_, created, err = InitChunksFile(tempDir, true)
	if err != nil || !created {
		t.Fatalf("expected created=true with force, got created=%v, err=%v", created, err)
	}

	// Load initialized chunks
	chunks, err := LoadChunks(p)
	if err != nil || len(chunks) == 0 {
		t.Fatalf("failed to load initialized chunks: %v", err)
	}

	// ApplyIndependentVerifyYes
	updated, err := ApplyIndependentVerifyYes(p, []string{chunks[0].ChunkID})
	if err != nil || updated == 0 {
		t.Fatalf("ApplyIndependentVerifyYes failed: %v, updated=%d", err, updated)
	}

	// Apply again should update 0 because it's already yes
	updated, err = ApplyIndependentVerifyYes(p, []string{chunks[0].ChunkID})
	if err != nil || updated != 0 {
		t.Fatalf("expected 0 updated, got %d, err=%v", updated, err)
	}

	// Apply on empty slice
	updated, err = ApplyIndependentVerifyYes(p, nil)
	if err != nil || updated != 0 {
		t.Fatalf("expected 0 updated for nil slice, got %d", updated)
	}
}
