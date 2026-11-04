// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package supervision

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestExtractFailureAnchor_ExhaustiveVariants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		stderr   string
		stdout   string
		expected string
	}{
		{
			name:     "pytest assertion error",
			stderr:   "AssertionError: assert_condition_failed",
			stdout:   "",
			expected: "assert_condition_failed",
		},
		{
			name:     "jest failed test suite",
			stderr:   "FAILED UserAuthSuite",
			stdout:   "",
			expected: "UserAuthSuite",
		},
		{
			name:     "bare FAIL symbol",
			stderr:   "FAIL: TestDatabaseMigration",
			stdout:   "",
			expected: "TestDatabaseMigration",
		},
		{
			name:     "generic exception pattern",
			stderr:   "Exception: NullPointerException in Handler",
			stdout:   "",
			expected: "NullPointerException in Handler",
		},
		{
			name:     "generic error pattern",
			stderr:   "Error: unable to bind port 8080",
			stdout:   "",
			expected: "unable to bind port 8080",
		},
		{
			name:     "file line column anchor",
			stderr:   "pkg/supervision/extractor.go:55:12: syntax error",
			stdout:   "",
			expected: "pkg/supervision/extractor.go:55:12",
		},
		{
			name:     "file line anchor without column",
			stderr:   "src/app/main.rs:102: mismatched types",
			stdout:   "",
			expected: "src/app/main.rs:102",
		},
		{
			name:     "relative path with backslash",
			stderr:   "sub\\module\\main.go:88: cannot compile",
			stdout:   "",
			expected: "sub\\module\\main.go:88",
		},
		{
			name:     "no matching anchor in output",
			stderr:   "warning: unused parameter x\nall green otherwise",
			stdout:   "process terminated normally",
			expected: "",
		},
		{
			name:     "empty stderr and stdout",
			stderr:   "",
			stdout:   "",
			expected: "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractFailureAnchor(tc.stderr, tc.stdout)
			if got != tc.expected {
				t.Fatalf("ExtractFailureAnchor() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestExtractFocusedDiagnostic_EmptyInputs(t *testing.T) {
	t.Parallel()

	// Both empty
	res := ExtractFocusedDiagnostic("", "", 127, 20)
	expected := "Process exited with status code 127 without diagnostic stderr/stdout output."
	if res != expected {
		t.Fatalf("expected empty diagnostic message %q, got %q", expected, res)
	}

	// Stderr empty, fallback to stdout
	resStdout := ExtractFocusedDiagnostic("", "Build output with FAIL: TestFoo", 1, 10)
	if !strings.Contains(resStdout, "FAIL: TestFoo") {
		t.Fatalf("expected stdout fallback, got %q", resStdout)
	}

	// MaxLines <= 0 fallback to 30
	resDefaultMax := ExtractFocusedDiagnostic("error: something failed", "", 1, -5)
	if !strings.Contains(resDefaultMax, "error: something failed") {
		t.Fatalf("expected error line with default maxLines, got %q", resDefaultMax)
	}
}

func TestExtractFocusedDiagnostic_SignalFilteringAndContext(t *testing.T) {
	t.Parallel()

	raw := `
[BUILD] Compiling modules...

[DEBUG] Loading plugins
undefined: MissingService
  at service.Init()
  at service.Start()
  at runtime.Goexit()
[INFO] Next phase
cannot use x as type y
not found: resource.json
expected integer but got float
validation errors encountered: field required
`
	// Tests undefined, cannot use, not found, expected, validation errors, and context capture
	got := ExtractFocusedDiagnostic(raw, "", 1, 25)
	if !strings.Contains(got, "undefined: MissingService") {
		t.Errorf("missing undefined signal in: %s", got)
	}
	if !strings.Contains(got, "at service.Init()") {
		t.Errorf("missing context line in: %s", got)
	}
	if !strings.Contains(got, "cannot use x as type y") {
		t.Errorf("missing 'cannot use' signal in: %s", got)
	}
	if !strings.Contains(got, "not found: resource.json") {
		t.Errorf("missing 'not found' signal in: %s", got)
	}
	if !strings.Contains(got, "expected integer but got float") {
		t.Errorf("missing 'expected' signal in: %s", got)
	}
	if !strings.Contains(got, "validation errors encountered") {
		t.Errorf("missing 'validation errors' signal in: %s", got)
	}
}

func TestExtractFocusedDiagnostic_SignalCountFallbackToTail(t *testing.T) {
	t.Parallel()

	// Less than 3 signal lines: should fall back to tail of output
	// Case 1: Total lines <= maxLines
	shortNoise := "Line 1\nLine 2\nLine 3\nLine 4"
	resShort := ExtractFocusedDiagnostic(shortNoise, "", 1, 10)
	if resShort != shortNoise {
		t.Fatalf("expected entire short noise output, got %q", resShort)
	}

	// Case 2: Total lines > maxLines
	var manyLines []string
	for i := 1; i <= 20; i++ {
		manyLines = append(manyLines, fmt.Sprintf("Noise step %d", i))
	}
	resTail := ExtractFocusedDiagnostic(strings.Join(manyLines, "\n"), "", 1, 5)
	lines := strings.Split(resTail, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected exactly 5 tail lines, got %d (%v)", len(lines), lines)
	}
	if lines[0] != "Noise step 16" || lines[4] != "Noise step 20" {
		t.Fatalf("unexpected tail lines content: %v", lines)
	}
}

func TestExtractFocusedDiagnostic_CappingSignalsToMaxLines(t *testing.T) {
	t.Parallel()

	var signalLines []string
	for i := 1; i <= 20; i++ {
		signalLines = append(signalLines, fmt.Sprintf("FAIL: TestSubCase_%02d", i))
	}
	raw := strings.Join(signalLines, "\n")
	res := ExtractFocusedDiagnostic(raw, "", 1, 5)
	lines := strings.Split(res, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 capped signal lines, got %d", len(lines))
	}
	if lines[0] != "FAIL: TestSubCase_16" || lines[4] != "FAIL: TestSubCase_20" {
		t.Fatalf("unexpected capped signals: %v", lines)
	}
}

func TestBuildFailureDiagnostic_PhaseDefaulting(t *testing.T) {
	t.Parallel()

	// Phase empty with exitCode == 0 -> PhaseValidation
	diagZero := BuildFailureDiagnostic(1, 0, "", "all good", "stdout", "none")
	if diagZero.Phase != PhaseValidation {
		t.Fatalf("expected PhaseValidation for exitCode 0, got %s", diagZero.Phase)
	}

	// Phase empty with exitCode != 0 -> PhaseExecution
	diagNonZero := BuildFailureDiagnostic(2, 2, "", "fatal failure", "", "crash")
	if diagNonZero.Phase != PhaseExecution {
		t.Fatalf("expected PhaseExecution for non-zero exitCode, got %s", diagNonZero.Phase)
	}

	// Explicit phase is preserved
	diagTimeout := BuildFailureDiagnostic(3, 124, PhaseTimeout, "timed out", "", "watchdog")
	if diagTimeout.Phase != PhaseTimeout {
		t.Fatalf("expected PhaseTimeout, got %s", diagTimeout.Phase)
	}

	// Timestamp is non-zero UTC
	if diagZero.Timestamp.IsZero() {
		t.Fatalf("expected non-zero diagnostic timestamp")
	}
}

func TestSynthesizeNegativeHypothesis_BranchCoverage(t *testing.T) {
	t.Parallel()

	// 1. triedApproach provided
	hypExplicit := SynthesizeNegativeHypothesis(1, nil, "Attempted unsafe pointer cast")
	if hypExplicit.InvalidatedApproach != "Attempted unsafe pointer cast" {
		t.Fatalf("unexpected invalidated approach: %s", hypExplicit.InvalidatedApproach)
	}
	if len(hypExplicit.ForbiddenActions) != 0 {
		t.Fatalf("expected no forbidden actions for nil diag")
	}

	// 2. triedApproach empty, diag == nil
	hypNilDiag := SynthesizeNegativeHypothesis(2, nil, "")
	if hypNilDiag.HypothesisSummary != "Prior attempt 2 failed verification." {
		t.Fatalf("unexpected summary for nil diag: %s", hypNilDiag.HypothesisSummary)
	}
	if !strings.Contains(hypNilDiag.InvalidatedApproach, "preceding implementation strategy") {
		t.Fatalf("unexpected approach for nil diag: %s", hypNilDiag.InvalidatedApproach)
	}

	// 3. triedApproach empty, diag has no FailureAnchor, exitCode == 0
	diagNoAnchorZeroExit := &FailureDiagnostic{
		Attempt:       3,
		ExitCode:      0,
		Phase:         PhaseValidation,
		FailureAnchor: "",
	}
	hypNoAnchor := SynthesizeNegativeHypothesis(3, diagNoAnchorZeroExit, "")
	if hypNoAnchor.HypothesisSummary != "Prior attempt 3 failed verification." {
		t.Fatalf("unexpected summary: %s", hypNoAnchor.HypothesisSummary)
	}
	if len(hypNoAnchor.ForbiddenActions) != 0 {
		t.Fatalf("expected zero forbidden actions when exitCode is 0 and anchor is empty")
	}

	// 4. diag has FailureAnchor and exitCode != 0
	diagWithAnchor := &FailureDiagnostic{
		Attempt:       4,
		ExitCode:      1,
		Phase:         PhaseCompile,
		FailureAnchor: "pkg/core/handler.go:42",
	}
	hypWithAnchor := SynthesizeNegativeHypothesis(4, diagWithAnchor, "")
	if !strings.Contains(hypWithAnchor.HypothesisSummary, "pkg/core/handler.go:42") {
		t.Fatalf("unexpected summary with anchor: %s", hypWithAnchor.HypothesisSummary)
	}
	if len(hypWithAnchor.ForbiddenActions) != 2 {
		t.Fatalf("expected 2 forbidden actions (exit code + targeted test), got %d", len(hypWithAnchor.ForbiddenActions))
	}
}

func TestFormatContrastiveRetryPrompt_MinimalAndEmptySections(t *testing.T) {
	t.Parallel()

	// Base envelope with minimal/empty fields
	envMinimal := SupervisionEnvelope{
		TaskID:      "TASK-MIN-001",
		Attempt:     1,
		MaxAttempts: 1,
	}

	resMinimal := FormatContrastiveRetryPrompt("Base task description", envMinimal)
	if !strings.Contains(resMinimal, "Base task description") {
		t.Errorf("missing base task prompt")
	}
	if !strings.Contains(resMinimal, "Attempt 1 of 1") {
		t.Errorf("missing attempt header")
	}
	if strings.Contains(resMinimal, "1. Focused Diagnostic Delta") {
		t.Errorf("diagnostic section should be absent when Diagnostic is nil")
	}
	if strings.Contains(resMinimal, "2. Negative Hypotheses") {
		t.Errorf("negative hypotheses section should be absent when empty")
	}
	if strings.Contains(resMinimal, "3. Canonical Reference Exemplars") {
		t.Errorf("exemplars section should be absent when empty")
	}
	if strings.Contains(resMinimal, "4. Progressive Scope Narrowing") {
		t.Errorf("progressive scope section should be absent when nil")
	}

	// Envelope with partial diagnostic (empty anchor and empty anomaly)
	envPartialDiag := SupervisionEnvelope{
		TaskID:      "TASK-PARTIAL",
		Attempt:     2,
		MaxAttempts: 5,
		Diagnostic: &FailureDiagnostic{
			Attempt:           1,
			ExitCode:          2,
			Phase:             PhaseUnknown,
			FocusedDiagnostic: "unknown error",
		},
	}
	resPartial := FormatContrastiveRetryPrompt("Refactor prompt", envPartialDiag)
	if strings.Contains(resPartial, "Failure Anchor / Symbol") {
		t.Errorf("should not print anchor line if empty")
	}
	if strings.Contains(resPartial, "Observed Anomaly") {
		t.Errorf("should not print anomaly line if empty")
	}
}

func TestFormatContrastiveRetryPrompt_PartialExemplarsAndScope(t *testing.T) {
	t.Parallel()

	env := SupervisionEnvelope{
		TaskID:      "TASK-PARTIAL-SUB",
		Attempt:     2,
		MaxAttempts: 4,
		NegativeBoundaries: []NegativeHypothesis{
			{
				Attempt:             1,
				InvalidatedApproach: "Direct slice allocation without bounds check",
				// ForbiddenActions empty
			},
		},
		Exemplars: []CanonicalExemplar{
			{
				Category:        "concurrency",
				Title:           "Worker Pool Pattern",
				Description:     "", // empty description
				PatternCode:     "", // empty pattern code
				AntiPatternCode: "go func() {}() // unmanaged goroutine",
			},
			{
				Category:        "concurrency",
				Title:           "Mutex Protection",
				Description:     "Always lock before read/write",
				PatternCode:     "mu.Lock()\ndefer mu.Unlock()",
				AntiPatternCode: "", // empty anti-pattern
			},
		},
		ProgressiveScope: &ProgressiveScope{
			// FocusedTarget empty
			SuggestedSubGateCommand: "", // empty command
			IterationRationale:      "Isolate unit test failures",
		},
	}

	res := FormatContrastiveRetryPrompt("Run tests", env)
	if !strings.Contains(res, "Direct slice allocation without bounds check") {
		t.Errorf("missing negative hypothesis text")
	}
	if !strings.Contains(res, "Worker Pool Pattern") {
		t.Errorf("missing worker pool exemplar")
	}
	if !strings.Contains(res, "Avoid (Anti-Pattern):") {
		t.Errorf("missing anti-pattern block")
	}
	if !strings.Contains(res, "Always lock before read/write") {
		t.Errorf("missing description for mutex exemplar")
	}
	if !strings.Contains(res, "Isolate unit test failures") {
		t.Errorf("missing rationale for progressive scope")
	}
	if strings.Contains(res, "Target Scope:") {
		t.Errorf("Target Scope should be omitted when empty")
	}
	if strings.Contains(res, "Suggested Sub-Gate Verification Command:") {
		t.Errorf("Suggested Sub-Gate Verification Command should be omitted when empty")
	}
}

func TestExemplarRegistry_DeduplicationAndWhitespaceNormalization(t *testing.T) {
	t.Parallel()

	reg := NewExemplarRegistry()

	// Register with whitespace and mixed casing
	reg.Register("  DATA_ACCESS  ", CanonicalExemplar{
		Category: "data_access",
		Title:    "Transaction Context Pattern",
	})

	// Register another with the exact same Title in same category
	reg.Register("data_access", CanonicalExemplar{
		Category: "data_access",
		Title:    "Transaction Context Pattern",
	})

	// Register same title in another category
	reg.Register("tx_handling", CanonicalExemplar{
		Category: "tx_handling",
		Title:    "Transaction Context Pattern",
	})

	// Resolve with whitespace and casing
	resolved := reg.Resolve(" Data_Access ", "TX_HANDLING")
	if len(resolved) != 1 {
		t.Fatalf("expected deduplication to result in 1 exemplar, got %d", len(resolved))
	}
	if resolved[0].Title != "Transaction Context Pattern" {
		t.Fatalf("unexpected exemplar title: %s", resolved[0].Title)
	}

	// Resolve unknown category
	unknown := reg.Resolve("non_existent_category")
	if len(unknown) != 0 {
		t.Fatalf("expected empty resolution for unknown category, got %d", len(unknown))
	}
}

func TestExemplarRegistry_ConcurrentRegistrationAndResolution(t *testing.T) {
	t.Parallel()

	reg := NewExemplarRegistry()
	const workerCount = 10
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(workerCount * 2)

	for i := 0; i < workerCount; i++ {
		workerID := i
		// Writer goroutines
		goroutinelabels.NewGoroutine("registry_test_writer", fmt.Sprintf("writer worker %d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					cat := fmt.Sprintf("category_%d", j%5)
					reg.Register(cat, CanonicalExemplar{
						Category: cat,
						Title:    fmt.Sprintf("Pattern-%d-%d", workerID, j),
					})
				}
			})

		// Reader goroutines
		goroutinelabels.NewGoroutine("registry_test_reader", fmt.Sprintf("reader worker %d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					cat := fmt.Sprintf("category_%d", j%5)
					res := reg.Resolve(cat, "category_unknown")
					if len(res) < 0 {
						t.Errorf("unexpected negative length for resolved exemplars")
					}
					time.Sleep(100 * time.Microsecond)
				}
			})
	}

	wg.Wait()

	// Verify all registered items are retrievable
	allResolved := reg.Resolve("category_0", "category_1", "category_2", "category_3", "category_4")
	if len(allResolved) == 0 {
		t.Fatalf("expected non-empty resolved catalog after concurrent execution")
	}
}
