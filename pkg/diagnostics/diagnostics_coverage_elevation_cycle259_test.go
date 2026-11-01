package diagnostics

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type mockExtractor struct {
	name      string
	artifact  DiagnosticArtifact
	err       error
	extracted bool
}

func (m *mockExtractor) Name() string {
	return m.name
}

func (m *mockExtractor) Extract(ctx context.Context) (DiagnosticArtifact, error) {
	m.extracted = true
	if m.err != nil {
		return DiagnosticArtifact{}, m.err
	}
	return m.artifact, nil
}

func TestStagnation_EdgeCases(t *testing.T) {
	// BuildCheckpoints with empty/nil
	if cp := BuildCheckpoints(nil); cp != nil {
		t.Fatalf("expected nil checkpoints for nil history, got %+v", cp)
	}
	if cp := BuildCheckpoints([]FailureEnvelope{}); cp != nil {
		t.Fatalf("expected nil checkpoints for empty history, got %+v", cp)
	}

	// EvaluateStagnation with default threshold overrides (<= 0)
	policy := StagnationPolicy{
		MaxStagnantAttempts:    0,
		MaxIdenticalSignatures: -1,
	}
	if assessment := EvaluateStagnation(nil, policy); assessment.IsStagnant {
		t.Fatalf("expected nil history to not be stagnant")
	}
	if assessment := EvaluateStagnation([]FailureEnvelope{{Attempt: 1}}, policy); assessment.IsStagnant {
		t.Fatalf("expected single attempt to not be stagnant")
	}

	// Non-consecutive identical signature frequency
	history := []FailureEnvelope{
		{Attempt: 1, Signature: "sig-alpha"},
		{Attempt: 2, Signature: "sig-beta"},
		{Attempt: 3, Signature: "sig-alpha"},
		{Attempt: 4, Signature: "sig-gamma"},
		{Attempt: 5, Signature: "sig-alpha"},
	}
	policy = StagnationPolicy{
		MaxStagnantAttempts:    5,
		MaxIdenticalSignatures: 3,
		ZeroDeltaFailFast:      false,
	}
	assessment := EvaluateStagnation(history, policy)
	if !assessment.IsStagnant {
		t.Fatalf("expected stagnation due to identical sig frequency, got %+v", assessment)
	}
	if !assessment.Escalate {
		t.Fatalf("expected escalation for repeated signature threshold")
	}

	// StagnationError helper
	nonStagnant := StagnationAssessment{IsStagnant: false}
	if err := StagnationError(nonStagnant); err != nil {
		t.Fatalf("expected nil error for non-stagnant assessment, got: %v", err)
	}
	stagnant := StagnationAssessment{
		IsStagnant:     true,
		Reason:         "too many loops",
		Recommendation: RecommendationPivot,
	}
	if err := StagnationError(stagnant); err == nil || !strings.Contains(err.Error(), "too many loops") {
		t.Fatalf("expected stagnation error, got: %v", err)
	}
}

func TestEnvelope_EdgeCases(t *testing.T) {
	// ComputeSignature with error message longer than DefaultMaxErrorLength
	longMsg := strings.Repeat("A", DefaultMaxErrorLength+500)
	sig1 := ComputeSignature(CategoryExecutionFailure, 1, longMsg, nil, nil)
	sig2 := ComputeSignature(CategoryExecutionFailure, 1, longMsg[:DefaultMaxErrorLength], nil, nil)
	if sig1 != sig2 {
		t.Fatalf("expected signature to truncate error message identically: %s vs %s", sig1, sig2)
	}

	// ClassifyError branch with err == nil and exitCode != 0
	cat := ClassifyError(nil, 42, nil)
	if cat != CategoryExecutionFailure {
		t.Fatalf("expected CategoryExecutionFailure for nil err with non-zero exit code, got %s", cat)
	}

	// ClassifyError with constraints present takes precedence
	cat = ClassifyError(nil, 0, []string{"invariant violated"})
	if cat != CategoryConstraintViolation {
		t.Fatalf("expected CategoryConstraintViolation, got %s", cat)
	}

	// ExtractFailureEnvelope bounds limits
	manyConstraints := make([]string, DefaultMaxConstraints+20)
	for i := range manyConstraints {
		manyConstraints[i] = fmt.Sprintf("constraint-%d", i)
	}
	manyActions := make([]string, DefaultMaxActionsLimit+20)
	for i := range manyActions {
		manyActions[i] = fmt.Sprintf("action-%d", i)
	}

	// Build report with more than DefaultMaxArtifactCount results
	report := &DiagnosticReport{}
	for i := 0; i < DefaultMaxArtifactCount+10; i++ {
		report.Results = append(report.Results, ResultRecord{
			OK: true,
			Artifact: &DiagnosticArtifact{
				Name: fmt.Sprintf("art-%d", i),
			},
		})
	}

	env := ExtractFailureEnvelope(0, 1, errors.New(longMsg), manyActions, manyConstraints, report)
	if env.Attempt != 1 {
		t.Fatalf("expected attempt to normalize to 1, got %d", env.Attempt)
	}
	if len(env.FailingConstraints) != DefaultMaxConstraints {
		t.Fatalf("expected constraints bounded to %d, got %d", DefaultMaxConstraints, len(env.FailingConstraints))
	}
	if len(env.ExecutedActions) != DefaultMaxActionsLimit {
		t.Fatalf("expected actions bounded to %d, got %d", DefaultMaxActionsLimit, len(env.ExecutedActions))
	}
	if len(env.DiagnosticReports) != DefaultMaxArtifactCount {
		t.Fatalf("expected artifacts bounded to %d, got %d", DefaultMaxArtifactCount, len(env.DiagnosticReports))
	}
	if len(env.ErrorMessage) != DefaultMaxErrorLength {
		t.Fatalf("expected error message bounded to %d, got %d", DefaultMaxErrorLength, len(env.ErrorMessage))
	}

	// HasSignatureMatch verification
	envA := FailureEnvelope{Signature: "sig-xyz"}
	envB := FailureEnvelope{Signature: "sig-xyz"}
	envC := FailureEnvelope{Signature: "sig-other"}
	envEmpty1 := FailureEnvelope{Signature: ""}
	envEmpty2 := FailureEnvelope{Signature: ""}

	if !envA.HasSignatureMatch(envB) {
		t.Fatalf("expected matching signatures to return true")
	}
	if envA.HasSignatureMatch(envC) {
		t.Fatalf("expected differing signatures to return false")
	}
	if envEmpty1.HasSignatureMatch(envEmpty2) {
		t.Fatalf("expected empty signatures to return false")
	}
	if envA.HasSignatureMatch(envEmpty1) {
		t.Fatalf("expected comparison with empty signature to return false")
	}
}

func TestExtractor_EdgeCases(t *testing.T) {
	reg := NewDiagnosticExtractorRegistry()

	// ByName with invalid name
	if _, err := reg.ByName("INVALID_UPPERCASE"); err == nil {
		t.Fatalf("expected error querying invalid name from ByName")
	}
	if _, err := reg.ByName(""); err == nil {
		t.Fatalf("expected error querying empty name from ByName")
	}

	// timeoutExtractor.Name delegation
	inner := &mockExtractor{name: "test-timeout-ext"}
	wrapped := WithTimeout(inner, 2*time.Second)
	if wrapped.Name() != "test-timeout-ext" {
		t.Fatalf("expected Name delegation to match inner, got %s", wrapped.Name())
	}

	// WithTimeout with non-positive timeout (timeout <= 0 bypasses derived deadline)
	wrappedNoTimeout := WithTimeout(inner, 0)
	art, err := wrappedNoTimeout.Extract(context.Background())
	if err != nil {
		t.Fatalf("unexpected error from zero timeout extraction: %v", err)
	}
	if !inner.extracted {
		t.Fatalf("expected inner extractor to be invoked")
	}
	_ = art

	wrappedNegTimeout := WithTimeout(inner, -5*time.Second)
	if _, err := wrappedNegTimeout.Extract(context.Background()); err != nil {
		t.Fatalf("unexpected error from negative timeout extraction: %v", err)
	}

	// Extractor returning artifact with empty Name gets assigned registered name
	reg2 := NewDiagnosticExtractorRegistry()
	unnamedArtExtractor := &mockExtractor{
		name: "auto-named",
		artifact: DiagnosticArtifact{
			Name:      "",
			SizeBytes: 128,
			MIMEType:  "text/plain",
		},
	}
	if err := reg2.Register(unnamedArtExtractor); err != nil {
		t.Fatalf("failed to register: %v", err)
	}
	rep := reg2.Run(context.Background(), nil)
	if rep.OKCount() != 1 {
		t.Fatalf("expected 1 ok count, got %d", rep.OKCount())
	}
	if rep.Results[0].Artifact.Name != "auto-named" {
		t.Fatalf("expected artifact name to default to extractor name, got %s", rep.Results[0].Artifact.Name)
	}

	// Whitebox test: invalid name or missing extractor in registry during Run
	reg3 := NewDiagnosticExtractorRegistry()
	reg3.mu.Lock()
	reg3.extractors["INVALID-UPPER!"] = inner
	reg3.mu.Unlock()

	rep3 := reg3.Run(context.Background(), nil)
	if rep3.FailedCount() < 1 {
		t.Fatalf("expected failure for invalid extractor name in registry map")
	}

	// Whitebox test: extractor deleted before ByName resolves
	reg4 := NewDiagnosticExtractorRegistry()
	if err := reg4.Register(&mockExtractor{name: "transient"}); err != nil {
		t.Fatalf("failed to register transient extractor: %v", err)
	}
	rep4 := reg4.Run(context.Background(), func(name string) bool {
		reg4.mu.Lock()
		delete(reg4.extractors, name)
		reg4.mu.Unlock()
		return true
	})
	if rep4.FailedCount() != 1 {
		t.Fatalf("expected failure when extractor deleted mid-run, got %d", rep4.FailedCount())
	}
}

func TestDisambiguation_EdgeCases(t *testing.T) {
	// Options defaults overrides
	opts := DisambiguationOptions{
		MaxPromptBytes:   -1,
		MaxNegativeRules: 0,
		MaxGuidanceItems: -5,
	}

	// Success envelopes in history should be skipped
	history := []FailureEnvelope{
		{
			Attempt:  1,
			Category: CategorySuccess,
			ExitCode: 0,
		},
		{
			Attempt:            2,
			Category:           CategoryConstraintViolation,
			ExitCode:           1,
			FailingConstraints: []string{"constraint A", "constraint B"},
			ExecutedActions:    []string{"   ", "", "action-1", "action-2"},
		},
		{
			Attempt:         3,
			Category:        CategoryExecutionFailure,
			ExitCode:        2,
			ErrorMessage:    "", // empty error message, empty constraints -> category fallback
			ExecutedActions: []string{"action-1"},
		},
		{
			Attempt:         4,
			Category:        CategoryUnknown,
			ExitCode:        3,
			ErrorMessage:    "some failure reason",
			ExecutedActions: []string{"action-z", "action-a"}, // test equal count alphabetical sorting
		},
	}

	ctx := GenerateDisambiguation(history, opts)
	if ctx.Attempt != 5 {
		t.Fatalf("expected attempt 5, got %d", ctx.Attempt)
	}
	if len(ctx.NegativeConstraints) == 0 {
		t.Fatalf("expected negative constraints to be synthesized")
	}

	// Check stable sorting when counts are equal
	var foundZ, foundA int
	for i, nc := range ctx.NegativeConstraints {
		if nc.ActionPattern == "action-a" {
			foundA = i
		}
		if nc.ActionPattern == "action-z" {
			foundZ = i
		}
	}
	if foundA > 0 && foundZ > 0 && foundA > foundZ {
		t.Fatalf("expected action-a to sort before action-z when counts are equal")
	}

	// MaxNegativeRules bounding
	optsBounded := DisambiguationOptions{
		MaxPromptBytes:   1024,
		MaxNegativeRules: 1,
		MaxGuidanceItems: 2,
	}
	ctxBounded := GenerateDisambiguation(history, optsBounded)
	if len(ctxBounded.NegativeConstraints) > 1 {
		t.Fatalf("expected at most 1 negative constraint, got %d", len(ctxBounded.NegativeConstraints))
	}
}

func TestDisambiguation_EdgeCases_PromptFormatting(t *testing.T) {
	// Bounding of guidance items from failing constraints
	historyConstraints := []FailureEnvelope{
		{
			Attempt:            1,
			Signature:          "sig-1",
			FailingConstraints: []string{"fc1", "fc2", "fc3"},
		},
		{
			Attempt:            2,
			Signature:          "sig-2",
			FailingConstraints: []string{"fc1", "fc2", "fc3"},
		},
	}
	ctxBoundedGuidance := GenerateDisambiguation(historyConstraints, DisambiguationOptions{
		MaxGuidanceItems: 1,
	})
	if len(ctxBoundedGuidance.Guidance) != 1 {
		t.Fatalf("expected exactly 1 guidance item, got %d", len(ctxBoundedGuidance.Guidance))
	}

	ctx := DisambiguationContext{
		Attempt: 1,
		NegativeConstraints: []NegativeConstraint{
			{ActionPattern: "test-pattern", Reason: "test-reason"},
		},
	}

	// FormatDisambiguationPrompt with non-positive limit
	ctx.MaxPromptBytes = 0
	prompt := FormatDisambiguationPrompt(ctx)
	if prompt == "" {
		t.Fatalf("expected non-empty prompt")
	}

	// FormatDisambiguationPrompt truncation
	ctx.MaxPromptBytes = 50
	promptTrunc := FormatDisambiguationPrompt(ctx)
	if len(promptTrunc) > 50 {
		t.Fatalf("expected prompt to be truncated to 50 bytes, got %d", len(promptTrunc))
	}
}

func TestCapturePrune_AdditionalBranches(t *testing.T) {
	// pruneDiagnosticsCaptures on non-existent directory
	pruneDiagnosticsCaptures("/nonexistent/path/that/cannot/exist/12345", "test")

	dir := t.TempDir()

	// Create subdirectory that ent.IsDir() skips
	subDir := filepath.Join(dir, "sub_directory")
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	// Create files with unknown suffixes or invalid timestamps
	_ = fileutil.WriteFile(filepath.Join(dir, "test_20260101-120000_unknown.txt"), []byte("data"), paths.FilePerm644)
	_ = fileutil.WriteFile(filepath.Join(dir, "test_invalidts_goroutines.txt"), []byte("data"), paths.FilePerm644)
	_ = fileutil.WriteFile(filepath.Join(dir, "unrelated_file.log"), []byte("data"), paths.FilePerm644)

	pruneDiagnosticsCaptures(dir, "test")

	// Ensure files are still there
	if _, err := fileutil.Stat(filepath.Join(dir, "test_20260101-120000_unknown.txt")); err != nil {
		t.Fatalf("expected unknown suffix file to be untouched: %v", err)
	}
}

func TestCaptureProcessInfo_ExecError(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "process_exec_err.txt")

	// Set PATH to empty to trigger exec error in ps/top/lsof
	t.Setenv("PATH", "")

	if err := captureProcessInfo(outFile); err != nil {
		t.Fatalf("captureProcessInfo should handle exec errors gracefully, got: %v", err)
	}

	content, err := fileutil.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte("=== All Processes (ps aux) (error) ===")) {
		t.Fatalf("expected error section in process info file, got: %s", string(content))
	}
}

func TestCPUProfileSignal_DirectExecutionEdges(t *testing.T) {
	// runCPUProfileDump with empty cpuProfileDir (falls back to TempDir)
	cpuProfileDirMu.Lock()
	cpuProfileDir = ""
	cpuProfileDirMu.Unlock()

	// Shorten duration for quick test execution
	cpuProfileDuration = 10 * time.Millisecond
	defer func() {
		cpuProfileDuration = 30 * time.Second
	}()

	runCPUProfileDump()

	// runCPUProfileDump when fileutil.Create fails (target file is an existing directory)
	dir := t.TempDir()
	ts := time.Now().Format("20060102-150405")
	name := fmt.Sprintf("cpu-%s.prof", ts)
	conflictDir := filepath.Join(dir, name)
	if err := fileutil.MkdirAll(conflictDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	cpuProfileDirMu.Lock()
	cpuProfileDir = dir
	cpuProfileDirMu.Unlock()

	runCPUProfileDump()

	// runCPUProfileDump when pprof.StartCPUProfile fails because profiling is already active
	dirClean := t.TempDir()
	cpuProfileDirMu.Lock()
	cpuProfileDir = dirClean
	cpuProfileDirMu.Unlock()

	var buf bytes.Buffer
	if err := pprof.StartCPUProfile(&buf); err == nil {
		runCPUProfileDump()
		pprof.StopCPUProfile()
	}
}

func TestSetupSignalHandler_CaptureErrorPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// SetupSignalHandler with a root where paths.ProjectDataDir cannot be created (regular file)
	tempDir := t.TempDir()
	badRoot := filepath.Join(tempDir, "file_as_root")
	if err := fileutil.WriteFile(badRoot, []byte("blocker"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	SetupSignalHandler(ctx, badRoot, "test_fail_prefix")

	// Trigger SIGUSR1 - this exercises the capture failure logging path
	_ = syscall.Kill(os.Getpid(), syscall.SIGUSR1)

	// Wait briefly for background goroutine to execute
	time.Sleep(100 * time.Millisecond)

	// Test context cancellation exits handler
	cancel()
	time.Sleep(50 * time.Millisecond)

	// Also test successful capture path to exercise logging success branch
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	goodRoot := t.TempDir()
	SetupSignalHandler(ctx2, goodRoot, "test_success_prefix")
	_ = syscall.Kill(os.Getpid(), syscall.SIGUSR1)

	// Wait until files are written or timeout
	outDir := filepath.Join(goodRoot, paths.ProjectDataDir, diagnosticsSubdir, "test_success_prefix")
	for i := 0; i < 40; i++ {
		if entries, err := fileutil.ReadDir(outDir); err == nil && len(entries) >= 4 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}
