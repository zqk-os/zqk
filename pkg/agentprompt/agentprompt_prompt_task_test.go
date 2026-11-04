package agentprompt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/observer"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

// TestCycle165_ExtractObserverContext verifies AST context extraction and error branches.
func TestCycle165_ExtractObserverContext(t *testing.T) {
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "pkg", "sample")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create sample dir: %v", err)
	}

	sampleGo := filepath.Join(subDir, "sample.go")
	code := []byte("package sample\n\ntype SampleService struct{}\nfunc (s *SampleService) Serve() error { return nil }\n")
	if err := os.WriteFile(sampleGo, code, 0644); err != nil {
		t.Fatalf("failed to write sample go file: %v", err)
	}

	ctx := context.Background()
	obs, err := ExtractObserverContext(ctx, tempDir)
	if err != nil {
		t.Fatalf("ExtractObserverContext failed on valid dir: %v", err)
	}
	if obs == nil || obs.Result == nil {
		t.Fatal("expected non-nil ObserverContext with Result")
	}

	found := false
	for _, ent := range obs.Result.Entities {
		if ent.Name == "SampleService" || ent.Name == "Serve" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected extracted entities to contain SampleService or Serve, got: %+v", obs.Result.Entities)
	}

	// Verify RelevantObserverSection with empty project root
	emptyRootSec := RelevantObserverSection(ctx, "", "title", "extra")
	if !strings.Contains(emptyRootSec, "Codebase Structural Context") {
		t.Fatalf("expected Codebase Structural Context in empty root section: %s", emptyRootSec)
	}

	// Verify RelevantObserverSection with valid project root and sample hits
	relSec := RelevantObserverSection(ctx, tempDir, "SampleService", "pkg/sample")
	if !strings.Contains(relSec, "Codebase Structural Context") {
		t.Fatalf("expected tool access in section: %s", relSec)
	}
	if !strings.Contains(relSec, "Task-relevant AST hits") {
		t.Fatalf("expected live AST hits in section: %s", relSec)
	}

	// Verify timeout / cancelled context in RelevantObserverSection
	cancCtx, cancel := context.WithCancel(ctx)
	cancel()
	cancSec := RelevantObserverSection(cancCtx, tempDir, "SampleService", "pkg/sample")
	if !strings.Contains(cancSec, "Codebase Structural Context") {
		t.Fatalf("expected tool access section on cancelled context: %s", cancSec)
	}
}

// TestCycle165_AppendUniqueHitsLimit verifies unique hit accumulation up to the limit.
func TestCycle165_AppendUniqueHitsLimit(t *testing.T) {
	seen := make(map[string]struct{})
	var hits []observer.Entity

	src := make([]observer.Entity, observerHintLimit+5)
	for i := range src {
		src[i] = observer.Entity{
			File: "file.go",
			Name: string(rune('A' + i)),
			Kind: "func",
		}
	}

	hits = appendUniqueHits(hits, src, seen)
	if len(hits) != observerHintLimit {
		t.Fatalf("expected hits clamped to limit %d, got %d", observerHintLimit, len(hits))
	}

	// Duplicate entries (already seen) should be skipped
	hits2 := appendUniqueHits(hits, src[:observerHintLimit], seen)
	if len(hits2) != observerHintLimit {
		t.Fatalf("expected no duplicates added, got %d", len(hits2))
	}
}

// TestCycle165_StandingPolicyDiscoveryAndIntent verifies discovery and intent resolution.
func TestCycle165_StandingPolicyDiscoveryAndIntent(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Nil storage provider checks
	if pols := DiscoverStandingPolicies(ctx, nil, secCtx); pols != nil {
		t.Fatalf("expected nil policies from nil storage, got %v", pols)
	}
	if id, title := ResolvePolicyByIntent(ctx, nil, secCtx, "testing", "TDD"); id != "" || title != "" {
		t.Fatalf("expected empty resolution from nil storage, got (%s, %s)", id, title)
	}

	// 2. Storage with active policies
	sp := &testMockSP{
		objects: map[string]map[string]any{
			"POL-WORKFLOW-001": {
				objects.FieldKeyKind:        objects.KindPolicy,
				objects.FieldKeyID:          "POL-WORKFLOW-001",
				objects.FieldKeyCategory:    "workflow",
				objects.FieldKeyTitle:       "Pull Request-Only Workflow Flywheel",
				objects.FieldKeyDescription: "Autonomous progression without idle pauses",
				objects.FieldKeyStatus:      "active",
			},
			"POL-TEST-002": {
				objects.FieldKeyKind:        objects.KindPolicy,
				objects.FieldKeyID:          "POL-TEST-002",
				objects.FieldKeyCategory:    "testing",
				objects.FieldKeyTitle:       "Test-Driven Development Mandate",
				objects.FieldKeyDescription: "Write tests before mutating source",
				objects.FieldKeyStatus:      "active",
			},
			"POL-GENERAL-003": {
				objects.FieldKeyKind:        objects.KindPolicy,
				objects.FieldKeyID:          "POL-GENERAL-003",
				objects.FieldKeyCategory:    "general",
				objects.FieldKeyTitle:       "General Quality Standards",
				objects.FieldKeyDescription: "Zero swallowed errors",
				objects.FieldKeyStatus:      "active",
			},
		},
	}

	discovered := DiscoverStandingPolicies(ctx, sp, secCtx)
	if len(discovered) != 3 {
		t.Fatalf("expected 3 discovered policies, got %d", len(discovered))
	}

	// Category and keyword matching
	id, title := ResolvePolicyByIntent(ctx, sp, secCtx, "testing", "TDD", "Development")
	if id != "POL-TEST-002" || !strings.Contains(title, "Test-Driven Development") {
		t.Fatalf("expected POL-TEST-002, got (%s, %s)", id, title)
	}

	// Keyword matching fallback across other categories
	id2, title2 := ResolvePolicyByIntent(ctx, sp, secCtx, "security", "Workflow", "Flywheel")
	if id2 != "POL-WORKFLOW-001" || !strings.Contains(title2, "Flywheel") {
		t.Fatalf("expected POL-WORKFLOW-001 via fallback category, got (%s, %s)", id2, title2)
	}

	// Non-matching keywords
	id3, title3 := ResolvePolicyByIntent(ctx, sp, secCtx, "nonexistent", "UnmatchedKeyword")
	if id3 != "" || title3 != "" {
		t.Fatalf("expected empty resolution for unmatched keywords, got (%s, %s)", id3, title3)
	}
}

// TestCycle165_CapStagePromptsWithStorage verifies storage-backed cap stage prompt hydration.
func TestCycle165_CapStagePromptsWithStorage(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	preambleID := CapStagePreambleTemplateID
	designID := CapStagePromptTemplateIDs["cap_stage_design"]

	sp := &testMockSP{
		objects: map[string]map[string]any{
			preambleID: {
				objects.FieldKeyID:         preambleID,
				objects.FieldKeyPromptBody: "Custom CAP Preamble from Kernel Storage.",
			},
			designID: {
				objects.FieldKeyID:         designID,
				objects.FieldKeyPromptBody: "Custom Design Stage Body from Kernel Storage.",
			},
		},
	}

	opts := CapStageWakeOptions{
		PlanID: "PRI-TEST-PLAN",
		CvsID:  "CVS-TEST-CONV",
	}

	prompt, err := BuildCapStageWakePrompt(ctx, sp, secCtx, "cap_stage_design", opts)
	if err != nil {
		t.Fatalf("BuildCapStageWakePrompt returned unexpected error: %v", err)
	}

	for _, want := range []string{
		"Custom CAP Preamble from Kernel Storage.",
		"Custom Design Stage Body from Kernel Storage.",
		"plan_id: PRI-TEST-PLAN",
		"parent_cvs_id: CVS-TEST-CONV",
		"cap_stage: cap_stage_design",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", want, prompt)
		}
	}

	// Verify readPromptBody helpers directly with empty and missing targets
	if got := readPromptBody(ctx, nil, secCtx, preambleID); got != "" {
		t.Fatalf("expected empty string from nil storage, got %q", got)
	}
	if got := readPromptBody(ctx, sp, secCtx, ""); got != "" {
		t.Fatalf("expected empty string for empty ID, got %q", got)
	}
	if got := readPromptBody(ctx, sp, secCtx, "NONEXISTENT-ID"); got != "" {
		t.Fatalf("expected empty string for missing ID, got %q", got)
	}

	// Test all canonical CapStages with empty fallback
	for _, stage := range whatsnext.CapStages {
		p, err := BuildCapStageWakePrompt(ctx, nil, secCtx, stage, CapStageWakeOptions{})
		if err != nil {
			t.Fatalf("failed to build prompt for stage %s: %v", stage, err)
		}
		if !strings.Contains(p, "STAGE: "+stage) {
			t.Fatalf("expected stage header for %s in prompt: %s", stage, p)
		}
	}
}

// TestCycle165_TruncateRunes verifies boundary conditions on string truncation.
func TestCycle165_TruncateRunes(t *testing.T) {
	if got := truncateRunes("", 10); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
	if got := truncateRunes("hello", 0); got != "hello" {
		t.Fatalf("expected untruncated string for non-positive max, got %q", got)
	}
	if got := truncateRunes("hello", -5); got != "hello" {
		t.Fatalf("expected untruncated string for negative max, got %q", got)
	}
	if got := truncateRunes("short", 10); got != "short" {
		t.Fatalf("expected untruncated string when length <= max, got %q", got)
	}

	longStr := "The quick brown fox jumps over the lazy dog."
	truncated := truncateRunes(longStr, 15)
	if !strings.HasPrefix(truncated, "The quick brown") {
		t.Fatalf("expected prefix 'The quick brown', got %q", truncated)
	}
	if !strings.Contains(truncated, "instructions truncated for prompt budget") {
		t.Fatalf("expected budget truncation notice, got %q", truncated)
	}

	// Multi-byte Unicode runes
	unicodeStr := "🌟🚀🎯🔥💡✨🌈🎉"
	if got := truncateRunes(unicodeStr, 4); !strings.HasPrefix(got, "🌟🚀🎯🔥") {
		t.Fatalf("expected 4 unicode runes preserved, got %q", got)
	}
}

// TestCycle165_PolicyExistsInStorage verifies policy storage existence checks.
func TestCycle165_PolicyExistsInStorage(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	if policyExistsInStorage(ctx, nil, secCtx, "POL-1") {
		t.Fatal("expected false for nil storage provider")
	}

	sp := &testMockSP{
		objects: map[string]map[string]any{
			"POL-EXISTS": {
				objects.FieldKeyKind: objects.KindPolicy,
				objects.FieldKeyID:   "POL-EXISTS",
			},
		},
	}

	if !policyExistsInStorage(ctx, sp, secCtx, "POL-EXISTS") {
		t.Fatal("expected true for existing policy in storage")
	}
	if policyExistsInStorage(ctx, sp, secCtx, "POL-MISSING") {
		t.Fatal("expected false for missing policy")
	}
}

// TestCycle165_SyncASKTwinsAndMarkdownPaths verifies skill twin synchronization and path resolution.
func TestCycle165_SyncASKTwinsAndMarkdownPaths(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	rootDir := t.TempDir()

	skillsDir := filepath.Join(rootDir, ".zqk", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("failed to create skills dir: %v", err)
	}

	// Create test files: SKILL.md (should be ignored), .hidden (should be ignored), valid.skill
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("root skill"), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, ".hidden"), []byte("hidden"), 0644); err != nil {
		t.Fatalf("failed to write .hidden: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "craftsman.skill"), []byte("craftsman body"), 0644); err != nil {
		t.Fatalf("failed to write craftsman.skill: %v", err)
	}

	sp := &testMockSP{
		objects: make(map[string]map[string]any),
	}

	if err := SyncASKTwins(ctx, sp, secCtx, rootDir); err != nil {
		t.Fatalf("SyncASKTwins failed: %v", err)
	}

	// Verify craftsman twin was created
	created := false
	for _, obj := range sp.objects {
		if title, _ := obj[objects.FieldKeyTitle].(string); title == "craftsman" {
			created = true
			break
		}
	}
	if !created {
		t.Fatal("expected craftsman skill twin created in storage")
	}

	// Verify non-existent directory succeeds silently
	if err := SyncASKTwins(ctx, sp, secCtx, filepath.Join(rootDir, "nonexistent")); err != nil {
		t.Fatalf("expected nil error for missing directory, got: %v", err)
	}

	// Test resolveSkillMarkdownPath variants
	if p := resolveSkillMarkdownPath(rootDir, ""); p != "" {
		t.Fatalf("expected empty path, got %q", p)
	}
	absPath := filepath.Join(rootDir, "docs", "skill.md")
	if p := resolveSkillMarkdownPath(rootDir, absPath); p != absPath {
		t.Fatalf("expected absPath %q, got %q", absPath, p)
	}

	// Relative .md path
	relMd := filepath.Join(".agent", "skills", "test.md")
	expectedRelMd := filepath.Join(rootDir, relMd)
	if p := resolveSkillMarkdownPath(rootDir, relMd); p != expectedRelMd {
		t.Fatalf("expected %q, got %q", expectedRelMd, p)
	}

	// Directory resolution to SKILL.md
	skillSubDir := filepath.Join(rootDir, "custom-skill")
	if err := os.MkdirAll(skillSubDir, 0755); err != nil {
		t.Fatalf("failed to make skillSubDir: %v", err)
	}
	expectedSkillMd := filepath.Join(skillSubDir, "SKILL.md")
	if p := resolveSkillMarkdownPath(rootDir, "custom-skill"); p != expectedSkillMd {
		t.Fatalf("expected %q, got %q", expectedSkillMd, p)
	}
}

// TestCycle165_SkillMatchingAndPromptSectionOpts verifies skill query matching and prompt section rendering.
func TestCycle165_SkillMatchingAndPromptSectionOpts(t *testing.T) {
	// Query matching tests
	if !skillMatchesQuery("software craftsmanship and engineering", "craftsman", "", "") {
		t.Fatal("expected match on title substring 'craftsman'")
	}
	if !skillMatchesQuery("optimizing test flakiness in suites", "tdd", "flakiness", "") {
		t.Fatal("expected match on description 'flakiness'")
	}
	if !skillMatchesQuery("ensuring architectural integrity across boundaries", "arch", "", "integrity") {
		t.Fatal("expected match on summary 'integrity'")
	}
	if skillMatchesQuery("unrelated query", "abc", "def", "ghi") {
		t.Fatal("expected no match on unrelated query")
	}

	// SkillEnforcement.GeneratePromptSectionOpts tests
	emptyEnf := &SkillEnforcement{}
	emptySec := emptyEnf.GeneratePromptSectionOpts(100)
	if !strings.Contains(emptySec, "No specialized skills discovered") {
		t.Fatalf("expected no specialized skills notice, got: %s", emptySec)
	}

	longInstructions := strings.Repeat("Follow all architectural invariants strictly. ", 50)
	enf := &SkillEnforcement{
		RelevantSkills: []map[string]any{
			{
				objects.FieldKeyID:                  "ASK-001",
				objects.FieldKeyTitle:               "Quality Engineer",
				objects.FieldKeyFilePath:            "skills/qa/SKILL.md",
				objects.FieldKeyInstructionsSummary: "Ensures comprehensive test coverage",
				objects.FieldKeyDescription:         "Verification discipline",
				objects.FieldKeyInstructions:        longInstructions,
			},
			{
				objects.FieldKeyID:    "ASK-002",
				objects.FieldKeyTitle: "Empty Instructions Skill",
			},
		},
	}

	sec := enf.GeneratePromptSectionOpts(120)
	for _, want := range []string{
		"Quality Engineer (ASK-001)",
		"skills/qa/SKILL.md",
		"Ensures comprehensive test coverage",
		"Verification discipline",
		"instructions truncated for prompt budget",
		"Empty Instructions Skill (ASK-002)",
		"No instructions body on this agent_skill",
	} {
		if !strings.Contains(sec, want) {
			t.Fatalf("expected section to contain %q, got:\n%s", want, sec)
		}
	}
}

// TestCycle165_BuildTaskPrompt_PersistLayer verifies BuildTaskPrompt with PromptLayerPersist.
func TestCycle165_BuildTaskPrompt_PersistLayer(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	rootDir := t.TempDir()

	sp := &testMockSP{
		objects: map[string]map[string]any{
			"PER-ENGINEER": {
				objects.FieldKeyKind:                  objects.KindPersona,
				objects.FieldKeyID:                    "PER-ENGINEER",
				objects.FieldKeyRole:                  "IC Craftsman",
				objects.FieldKeyDescription:           "Delivers tested code",
				"system_prompt":                       "Adhere to continuous loop discipline",
				objects.FieldKeyInteractionPolicyRefs: []any{"POL-TEST-001"},
			},
			"POL-TEST-001": {
				objects.FieldKeyKind:        objects.KindPolicy,
				objects.FieldKeyID:          "POL-TEST-001",
				objects.FieldKeyCategory:    "testing",
				objects.FieldKeyTitle:       "Test-Driven Development",
				objects.FieldKeyDescription: "Write test before source edits",
				objects.FieldKeyStatus:      "active",
			},
		},
	}

	persistOpts := TaskPromptOptions{
		PlanTitle:   "Cycle 165 Plan",
		PlanID:      "PRI-165",
		TaskTitle:   "Elevation Task",
		PersonaID:   "PER-ENGINEER",
		Layer:       PromptLayerPersist,
		IncludeTDD:  true,
		TokenBudget: 4096,
	}
	persistPrompt, err := BuildTaskPrompt(ctx, sp, secCtx, rootDir, persistOpts)
	if err != nil {
		t.Fatalf("BuildTaskPrompt (Persist) returned error: %v", err)
	}
	if !strings.Contains(persistPrompt, TaskEnvelopeMarker) {
		t.Fatalf("expected TaskEnvelopeMarker in persist prompt: %s", persistPrompt)
	}
}

// TestCycle165_BuildTaskPrompt_ExecuteLayer verifies BuildTaskPrompt with full context options.
func TestCycle165_BuildTaskPrompt_ExecuteLayer(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	rootDir := t.TempDir()

	sp := &testMockSP{
		objects: map[string]map[string]any{
			"PER-ENGINEER": {
				objects.FieldKeyKind:                  objects.KindPersona,
				objects.FieldKeyID:                    "PER-ENGINEER",
				objects.FieldKeyRole:                  "IC Craftsman",
				objects.FieldKeyDescription:           "Delivers tested code",
				"system_prompt":                       "Adhere to continuous loop discipline",
				objects.FieldKeyInteractionPolicyRefs: []any{"POL-TEST-001"},
			},
			"POL-TEST-001": {
				objects.FieldKeyKind:        objects.KindPolicy,
				objects.FieldKeyID:          "POL-TEST-001",
				objects.FieldKeyCategory:    "testing",
				objects.FieldKeyTitle:       "Test-Driven Development",
				objects.FieldKeyDescription: "Write test before source edits",
				objects.FieldKeyStatus:      "active",
			},
			"POL-WORKFLOW-001": {
				objects.FieldKeyKind:        objects.KindPolicy,
				objects.FieldKeyID:          "POL-WORKFLOW-001",
				objects.FieldKeyCategory:    "workflow",
				objects.FieldKeyTitle:       "Pull Request-Only Flywheel",
				objects.FieldKeyDescription: "Anti-idleness progression",
				objects.FieldKeyStatus:      "active",
			},
		},
	}

	execOpts := TaskPromptOptions{
		PlanTitle:            "Cycle 165 Plan",
		PlanID:               "PRI-165",
		SessionID:            "SES-165",
		TargetAgent:          "PER-ENGINEER",
		PersonaID:            "PER-ENGINEER",
		Capability:           "IC Craftsman",
		TaskTitle:            "Elevation Implementation",
		TaskContext:          "Focus on pkg/agentprompt elevation",
		ValidationDSL:        "verify: test pass rate >= 85%",
		IncludeTDD:           true,
		InlineBodies:         true,
		IncludeObserver:      true,
		UpstreamDeliverables: "## Upstream Deliverables\n- Verified BLI-100 AST models",
		TaskSteps:            "1. Run tests\n2. Mutate source\n3. Verify coverage",
		TokenBudget:          8192,
	}

	execPrompt, err := BuildTaskPrompt(ctx, sp, secCtx, rootDir, execOpts)
	if err != nil {
		t.Fatalf("BuildTaskPrompt (Execute) returned error: %v", err)
	}

	for _, want := range []string{
		"Cycle 165 Plan",
		"SES-165",
		"Target Agent/Pod: PER-ENGINEER",
		"IC Craftsman",
		"Delivers tested code",
		"Adhere to continuous loop discipline",
		"Test-Driven Development",
		"Pull Request-Only Flywheel",
		"Upstream Deliverables",
		"Validation DSL",
		"verify: test pass rate >= 85%",
		"Elevation Implementation",
		"pkg/agentprompt elevation",
	} {
		if !strings.Contains(execPrompt, want) {
			t.Fatalf("expected execPrompt to contain %q, got:\n%s", want, execPrompt)
		}
	}
}

// TestCycle165_CompactStateJSONAndSentinelPrompt verifies state JSON compaction and sentinel prompt branches.
func TestCycle165_CompactStateJSONAndSentinelPrompt(t *testing.T) {
	// CompactStateJSON branches
	if empty := compactStateJSON(""); empty != "{}" {
		t.Fatalf("expected '{}' for empty input, got %q", empty)
	}
	if invalid := compactStateJSON("invalid-json"); invalid != "invalid-json" {
		t.Fatalf("expected raw output for invalid JSON, got %q", invalid)
	}

	rawState := `{
		"agent_instruction": "Continue execution",
		"priority_plan": {"id": "PRI-1", "title": "Lead Plan"},
		"backlog_counts_by_status": {"todo": 1, "done": 5},
		"measure_skip_reason": "No drift detected",
		"convergence_sessions_active": ["cvs-1", "cvs-2"],
		"unwanted_verbose_field": "drop me"
	}`

	compacted := compactStateJSON(rawState)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(compacted), &parsed); err != nil {
		t.Fatalf("failed to parse compacted JSON: %v", err)
	}

	if _, ok := parsed["unwanted_verbose_field"]; ok {
		t.Fatal("expected unwanted_verbose_field to be stripped")
	}
	if count, ok := parsed["convergence_sessions_active_count"].(float64); !ok || int(count) != 2 {
		t.Fatalf("expected convergence_sessions_active_count = 2, got %v", parsed["convergence_sessions_active_count"])
	}
	if inst, ok := parsed["agent_instruction"].(string); !ok || inst != "Continue execution" {
		t.Fatalf("expected agent_instruction preserved, got %v", parsed["agent_instruction"])
	}

	// BuildSentinelPrompt with health signals and fallback template
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	sp := &testMockSP{
		objects: map[string]map[string]any{
			"MIS-001": {
				objects.FieldKeyKind:             objects.KindMission,
				objects.FieldKeyID:               "MIS-001",
				objects.FieldKeyMissionStatement: "Deliver autonomous resilience",
				objects.FieldKeyStatus:           "active",
			},
			"MIL-001": {
				objects.FieldKeyKind:   objects.KindMilestone,
				objects.FieldKeyID:     "MIL-001",
				objects.FieldKeyTitle:  "Alpha Release",
				objects.FieldKeyStatus: "in_progress",
			},
		},
	}

	sentinelOpts := SentinelPromptOptions{
		StateJSON:   rawState,
		TokenBudget: 4096,
		HealthSignals: KernelHealthSignals{
			FailingTestCount:   2,
			PolicyViolations:   1,
			SchedulerErrorRate: 0.05,
			DriftIndicators:    []string{"Test flake rate spike in scheduler"},
		},
	}

	prompt, err := BuildSentinelPrompt(ctx, sp, secCtx, "", sentinelOpts)
	if err != nil {
		t.Fatalf("BuildSentinelPrompt returned error: %v", err)
	}

	for _, want := range []string{
		"CAP Kernel Steward",
		"DIRECTIVE REQUIRED",
		"Kernel Health Signals",
		"Failing tests: 2",
		"Policy violations (POL-CODE-007): 1",
		"Scheduler error rate: 5.0%",
		"Drift indicator: Test flake rate spike",
		"Deliver autonomous resilience",
		"Alpha Release",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected sentinel prompt to contain %q, got:\n%s", want, prompt)
		}
	}
}

// TestCycle165_EnvelopeMethodsAndParsers verifies envelope helper methods and parser edge cases.
func TestCycle165_EnvelopeMethodsAndParsers(t *testing.T) {
	// EnvelopeFromAttentionMode invalid mode
	if _, err := EnvelopeFromAttentionMode("unknown_mode"); err == nil {
		t.Fatal("expected error for unknown attention mode")
	}

	// ParseLeadingEnvelope unclosed comment
	unclosed := "<!-- zqk:agent_prompt {\"schema\":\"v1\"}"
	if _, _, err := ParseLeadingEnvelope(unclosed); err == nil {
		t.Fatal("expected error for unclosed comment")
	}

	// ParseLeadingEnvelope invalid JSON
	badJSON := "<!-- zqk:agent_prompt {not-json} -->\nBody"
	if _, _, err := ParseLeadingEnvelope(badJSON); err == nil {
		t.Fatal("expected error for malformed JSON in envelope")
	}

	// ParseLeadingEnvelope unsupported schema
	badSchema := "<!-- zqk:agent_prompt {\"schema\":\"v999\"} -->\nBody"
	if _, _, err := ParseLeadingEnvelope(badSchema); err == nil {
		t.Fatal("expected error for unsupported schema version")
	}

	// FormatHTMLCommentLine and ParseLeadingEnvelope roundtrip
	validEnv := Envelope{
		Schema:          SchemaV1,
		AttentionMode:   AttentionTestNonDirective,
		DirectiveScope:  DirectiveScopeNone,
		OperationalWork: false,
		QueuePolicy:     QueuePolicyIgnoreAsOperationalDirective,
	}

	commentLine, err := FormatHTMLCommentLine(validEnv)
	if err != nil {
		t.Fatalf("FormatHTMLCommentLine error: %v", err)
	}

	body := commentLine + "# Actual Markdown Task Body"
	parsedEnv, rem, err := ParseLeadingEnvelope(body)
	if err != nil {
		t.Fatalf("ParseLeadingEnvelope roundtrip error: %v", err)
	}
	if parsedEnv == nil || parsedEnv.AttentionMode != AttentionTestNonDirective {
		t.Fatalf("unexpected parsed envelope: %+v", parsedEnv)
	}
	if !strings.Contains(rem, "# Actual Markdown Task Body") {
		t.Fatalf("expected remainder markdown body, got: %s", rem)
	}

	// IgnoreAsOperationalDirective tests
	var nilEnv *Envelope
	if nilEnv.IgnoreAsOperationalDirective() {
		t.Fatal("expected false for nil envelope")
	}
	if !parsedEnv.IgnoreAsOperationalDirective() {
		t.Fatal("expected true for AttentionTestNonDirective")
	}
	otherEnv := &Envelope{QueuePolicy: "standard"}
	if otherEnv.IgnoreAsOperationalDirective() {
		t.Fatal("expected false for standard queue policy")
	}

	// InterruptPlumbingDryRun tests
	if nilEnv.InterruptPlumbingDryRun() {
		t.Fatal("expected false for nil envelope")
	}
	interruptEnv := &Envelope{
		AttentionMode: AttentionInterruptPlumbing,
		QueuePolicy:   QueuePolicyCriticalInterruptPathDryRun,
	}
	if !interruptEnv.InterruptPlumbingDryRun() {
		t.Fatal("expected true for critical interrupt envelope")
	}
	if otherEnv.InterruptPlumbingDryRun() {
		t.Fatal("expected false for standard envelope")
	}
}

// TestCycle165_TaskStepsFormattingEdgeCases verifies task steps formatting with verification commands and non-coding tasks.
func TestCycle165_TaskStepsFormattingEdgeCases(t *testing.T) {
	steps := []map[string]any{
		{
			objects.FieldKeyTitle:       "Check Baseline",
			objects.FieldKeyDescription: "Inspect existing unit tests",
			objects.FieldKeyStatus:      "pending",
			objects.FieldKeyCommand:     "go test -v ./pkg/sample",
		},
		{
			// Empty title should default to Step 2
			objects.FieldKeyDescription: "Run linters",
			objects.FieldKeyCommand:     "golangci-lint run",
		},
	}

	formatted := FormatTaskStepsPromptSection(steps, WorkClassCoding)
	for _, want := range []string{
		"1. **Check Baseline** [pending]",
		"Inspect existing unit tests",
		"Verification Command: `go test -v ./pkg/sample`",
		"2. **Step 2**",
		"Run linters",
		"Verification Command: `golangci-lint run`",
	} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("expected formatted steps to contain %q, got:\n%s", want, formatted)
		}
	}

	// Test slice of any containing mixed types
	anySlice := []any{
		map[string]any{
			objects.FieldKeyTitle: "Step from any slice",
		},
		"ignored non-map item",
	}
	anyFormatted := FormatTaskStepsPromptSection(anySlice, WorkClassCoding)
	if !strings.Contains(anyFormatted, "Step from any slice") {
		t.Fatalf("expected step from any slice in output: %s", anyFormatted)
	}

	// Non-coding task with empty steps returns empty string
	nonCodeFormatted := FormatTaskStepsPromptSection(nil, WorkClassDocsEval)
	if nonCodeFormatted != "" {
		t.Fatalf("expected empty steps for docs eval, got: %q", nonCodeFormatted)
	}
}

// TestCycle165_WorkClassClassifications verifies classification of work classes and prompt capabilities.
func TestCycle165_WorkClassClassifications(t *testing.T) {
	markers := []struct {
		text string
		want WorkClass
	}{
		{"", WorkClassCoding},
		{"regular software implementation", WorkClassCoding},
		{"docs_eval review task", WorkClassDocsEval},
		{"docs/quality/cef-runs evaluation", WorkClassDocsEval},
		{"pipeline_ref=pip-cef inspection", WorkClassDocsEval},
		{"forbid: source edits strictly enforced", WorkClassDocsEval},
		{"modify application source prohibited", WorkClassDocsEval},
		{"l-architecture specialist assessment", WorkClassDocsEval},
		{"w1_arch_specialist guidance", WorkClassDocsEval},
		{"finding.schema.json validation", WorkClassDocsEval},
	}

	for _, tc := range markers {
		got := ClassifyWorkClass(tc.text)
		if got != tc.want {
			t.Errorf("ClassifyWorkClass(%q) = %s, want %s", tc.text, got, tc.want)
		}
	}

	if !WorkClassDocsEval.IsDocsEval() {
		t.Fatal("expected IsDocsEval to be true for WorkClassDocsEval")
	}
	if WorkClassDocsEval.IsCode() {
		t.Fatal("expected IsCode to be false for WorkClassDocsEval")
	}
	if !WorkClassCoding.IsCoding() {
		t.Fatal("expected IsCoding to be true for WorkClassCoding")
	}

	// Prompt capabilities
	caps := WorkClassCoding.PromptCapabilities()
	if len(caps) == 0 || caps[0] != "coding" {
		t.Fatalf("expected coding capability in prompt capabilities: %v", caps)
	}
	emptyCaps := WorkClass("").PromptCapabilities()
	if len(emptyCaps) == 0 || emptyCaps[0] != "coding" {
		t.Fatalf("expected fallback coding capability for empty work class: %v", emptyCaps)
	}
}
