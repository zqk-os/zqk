// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package testrunner

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestCycle229_Contamination_EdgeCases validates SnapshotPlane and RunWithContaminationCheck edge cases.
func TestCycle229_Contamination_EdgeCases(t *testing.T) {
	t.Parallel()

	// 1. DiffSnapshots with deleted file
	before := PlaneSnapshot{"file1.yaml": "hash1", "deleted.yaml": "hash2"}
	after := PlaneSnapshot{"file1.yaml": "hash1"}
	diff := DiffSnapshots(before, after)
	if !diff.HasContamination() {
		t.Fatalf("expected contamination on file deletion")
	}
	if len(diff.Deleted) != 1 || diff.Deleted[0] != "deleted.yaml" {
		t.Fatalf("expected deleted.yaml in diff.Deleted, got %v", diff.Deleted)
	}

	// 2. RunWithContaminationCheck with non-zero exit code
	tempDir := t.TempDir()
	opts := ContaminationCheckOptions{
		ProjectRoot: tempDir,
		PlanePath:   ".",
	}
	var stdout, stderr bytes.Buffer
	exitCode, _, runErr := RunWithContaminationCheck(
		context.Background(),
		opts,
		[]string{"sh", "-c", "exit 42"},
		&stdout,
		&stderr,
	)
	if runErr == nil || exitCode != 42 {
		t.Fatalf("expected exit code 42 and error, got %d, err %v", exitCode, runErr)
	}

	// 3. RunWithContaminationCheck with invalid binary command
	exitCode2, _, runErr2 := RunWithContaminationCheck(
		context.Background(),
		opts,
		[]string{"/nonexistent/binary/path/that/cannot/exist"},
		&stdout,
		&stderr,
	)
	if runErr2 == nil || exitCode2 != 1 {
		t.Fatalf("expected exit code 1 and error, got %d, err %v", exitCode2, runErr2)
	}

	// 4. RunWithContaminationCheck where plane directory is removed during command
	planeSubdir := filepath.Join(tempDir, "ephemeral_plane")
	if err := fileutil.MkdirAll(planeSubdir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir ephemeral plane: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(planeSubdir, "data.yaml"), []byte("a: 1"), paths.FilePerm644); err != nil {
		t.Fatalf("write data.yaml: %v", err)
	}

	subOpts := ContaminationCheckOptions{
		ProjectRoot: tempDir,
		PlanePath:   "ephemeral_plane",
	}
	exitCode3, _, postErr := RunWithContaminationCheck(
		context.Background(),
		subOpts,
		[]string{"sh", "-c", fmt.Sprintf("rm -rf %q", planeSubdir)},
		&stdout,
		&stderr,
	)
	if postErr == nil {
		t.Fatalf("expected error when post-run snapshot fails, got nil (exitCode=%d)", exitCode3)
	}

	// 5. RunWithContaminationCheck with invalid pre-run plane path
	badPlaneOpts := ContaminationCheckOptions{
		ProjectRoot: tempDir,
		PlanePath:   "nonexistent_subpath_xyz",
	}
	exitCode4, _, preErr := RunWithContaminationCheck(
		context.Background(),
		badPlaneOpts,
		[]string{"true"},
		&stdout,
		&stderr,
	)
	if preErr == nil || exitCode4 != 2 {
		t.Fatalf("expected exit code 2 on bad pre-run plane, got %d, err %v", exitCode4, preErr)
	}
}

// TestCycle229_ParallelSafety_EdgeCases verifies AST scanner edge cases, skip dirs, and env heuristics.
func TestCycle229_ParallelSafety_EdgeCases(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// 1. Create ignored directories
	for _, dirName := range []string{".git", "vendor", ".sandbox", "scratch"} {
		dirPath := filepath.Join(tempDir, dirName)
		if err := fileutil.MkdirAll(dirPath, paths.DirPerm755); err != nil {
			t.Fatalf("mkdir %s: %v", dirName, err)
		}
		dummyFile := filepath.Join(dirPath, "dummy_test.go")
		if err := fileutil.WriteFile(dummyFile, []byte("package dummy\nfunc TestIgnored(t *testing.T){}\n"), paths.FilePerm644); err != nil {
			t.Fatalf("write dummy: %v", err)
		}
	}

	// 2. Non-test go file
	if err := fileutil.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\nfunc main(){}\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	// 3. Syntax error test file (should be skipped)
	if err := fileutil.WriteFile(filepath.Join(tempDir, "corrupted_test.go"), []byte("this is not valid go code"), paths.FilePerm644); err != nil {
		t.Fatalf("write corrupted_test.go: %v", err)
	}

	// 4. Valid test file with sorting, non-test funcs, os.Setenv heuristics
	sampleCode := `package sample_test

import (
	"os"
	"testing"
)

func BenchmarkSpeed(b *testing.B) {}
func helperDoWork() {}

func TestZulu(t *testing.T) {
	os.Setenv("ENV_ONLY", "1")
	println("non-selector call")
}

func TestAlpha(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("TEMP_AND_ENV", tmp)
}
`
	if err := fileutil.WriteFile(filepath.Join(tempDir, "sample_test.go"), []byte(sampleCode), paths.FilePerm644); err != nil {
		t.Fatalf("write sample_test.go: %v", err)
	}

	// 5. Separate file to exercise File comparison sorting
	otherDir := filepath.Join(tempDir, "subpkg")
	if err := fileutil.MkdirAll(otherDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir subpkg: %v", err)
	}
	otherCode := `package subpkg_test
import "testing"
func TestBeta(t *testing.T) {
	t.Parallel()
}
`
	if err := fileutil.WriteFile(filepath.Join(otherDir, "other_test.go"), []byte(otherCode), paths.FilePerm644); err != nil {
		t.Fatalf("write other_test.go: %v", err)
	}

	results, err := ScanParallelSafety(tempDir, ".")
	if err != nil {
		t.Fatalf("ScanParallelSafety failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d: %+v", len(results), results)
	}

	// Verify sorting order: TestAlpha must precede TestZulu
	if results[0].FuncName != "TestAlpha" || results[1].FuncName != "TestZulu" {
		t.Fatalf("expected sorted order TestAlpha then TestZulu, got %s, %s", results[0].FuncName, results[1].FuncName)
	}

	// Verify TestAlpha has TempDir and os.Setenv reasons
	if results[0].Level != SafetyLevelNeedsReview {
		t.Fatalf("expected TestAlpha to be needs_review, got %s", results[0].Level)
	}
	hasTempDirReason := false
	hasOSSetenvReason := false
	for _, r := range results[0].Reasons {
		if strings.Contains(r, "t.TempDir()") {
			hasTempDirReason = true
		}
		if strings.Contains(r, "os.Setenv()") {
			hasOSSetenvReason = true
		}
	}
	if !hasTempDirReason || !hasOSSetenvReason {
		t.Fatalf("expected TestAlpha to note both tempDir and os.Setenv, got %v", results[0].Reasons)
	}

	// Verify TestZulu has os.Setenv reason without t.Setenv
	if results[1].Level != SafetyLevelNeedsReview {
		t.Fatalf("expected TestZulu to be needs_review, got %s", results[1].Level)
	}

	// Verify TestBeta has parallel level
	if results[2].FuncName != "TestBeta" || results[2].Level != SafetyLevelParallelAlready {
		t.Fatalf("expected results[2] to be TestBeta with parallel level, got %+v", results[2])
	}

	// 6. Non-existent path returns error
	_, scanErr := ScanParallelSafety(tempDir, "nonexistent_subpath_xyz")
	if scanErr == nil {
		t.Fatalf("expected error for non-existent path")
	}
}

// TestCycle229_RaceGate_EdgeCases validates RaceGateConfig defaults and execution branches.
func TestCycle229_RaceGate_EdgeCases(t *testing.T) {
	t.Parallel()

	// 1. Missing package path
	_, err := RunRaceGate(context.Background(), RaceGateConfig{})
	if err == nil {
		t.Fatalf("expected error when package path is empty")
	}

	// 2. Hermetic test project with clean passing test under default timeout
	tempDir := t.TempDir()
	modContent := "module github.com/zqk-os/zqk/ephemeralrace\n\ngo 1.24\n"
	if err := fileutil.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(modContent), paths.FilePerm644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	passDir := filepath.Join(tempDir, "clean")
	if err := fileutil.MkdirAll(passDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir clean: %v", err)
	}
	passCode := `package clean_test
import "testing"
func TestCleanPass(t *testing.T) {}
`
	if err := fileutil.WriteFile(filepath.Join(passDir, "clean_test.go"), []byte(passCode), paths.FilePerm644); err != nil {
		t.Fatalf("write clean_test.go: %v", err)
	}

	cfg := RaceGateConfig{
		PackagePath: "./clean",
		ProjectRoot: tempDir,
		Timeout:     0, // Exercises default 2-minute timeout branch
		ExtraArgs:   []string{"-count=1"},
		Env:         []string{"ZQK_TEST_RACEGATE=1"},
	}

	res, err := RunRaceGate(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunRaceGate failed: %v", err)
	}
	if !res.Passed {
		t.Fatalf("expected clean package to pass, got failed: %s", res.Output)
	}
	if res.HasDataRace {
		t.Fatalf("expected no data race")
	}

	// 3. Failing test without data race
	failDir := filepath.Join(tempDir, "failpkg")
	if err := fileutil.MkdirAll(failDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failpkg: %v", err)
	}
	failCode := `package failpkg_test
import "testing"
func TestForcedFail(t *testing.T) { t.Fatal("forced intentional fail") }
`
	if err := fileutil.WriteFile(filepath.Join(failDir, "fail_test.go"), []byte(failCode), paths.FilePerm644); err != nil {
		t.Fatalf("write fail_test.go: %v", err)
	}

	cfgFail := RaceGateConfig{
		PackagePath: "./failpkg",
		ProjectRoot: tempDir,
		Timeout:     30 * time.Second,
	}
	resFail, errFail := RunRaceGate(context.Background(), cfgFail)
	if errFail != nil {
		t.Fatalf("RunRaceGate error on failing test: %v", errFail)
	}
	if resFail.Passed {
		t.Fatalf("expected resFail.Passed to be false")
	}
	if resFail.ExitCode == 0 {
		t.Fatalf("expected non-zero exit code for failing test")
	}

	// 4. Test package with intentional data race under -race
	raceDir := filepath.Join(tempDir, "racepkg")
	if err := fileutil.MkdirAll(raceDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir racepkg: %v", err)
	}
	raceCode := strings.Join([]string{
		"package racepkg_test",
		"import (",
		"\t\"sync\"",
		"\t\"testing\"",
		")",
		"var sharedCount int",
		"func TestDataRace(t *testing.T) {",
		"\tvar wg sync.WaitGroup",
		"\twg.Add(2)",
		"\tgo" + " func() {",
		"\t\tdefer wg.Done()",
		"\t\tsharedCount = 1",
		"\t}()",
		"\tgo" + " func() {",
		"\t\tdefer wg.Done()",
		"\t\tsharedCount = 2",
		"\t}()",
		"\twg.Wait()",
		"}",
	}, "\n")
	if err := fileutil.WriteFile(filepath.Join(raceDir, "race_test.go"), []byte(raceCode), paths.FilePerm644); err != nil {
		t.Fatalf("write race_test.go: %v", err)
	}

	cfgRace := RaceGateConfig{
		PackagePath: "./racepkg",
		ProjectRoot: tempDir,
		Timeout:     30 * time.Second,
	}
	resRace, errRace := RunRaceGate(context.Background(), cfgRace)
	if errRace != nil {
		t.Fatalf("RunRaceGate on racepkg error: %v", errRace)
	}
	if resRace.Passed {
		t.Fatalf("expected resRace.Passed to be false on data race")
	}
	if !resRace.HasDataRace {
		t.Fatalf("expected resRace.HasDataRace to be true, got false. Output:\n%s", resRace.Output)
	}
	if resRace.Error != "data race detected" {
		t.Fatalf("expected error 'data race detected', got %q", resRace.Error)
	}
}

// TestCycle229_StreamTests_EdgeCases exercises StreamTests streaming patterns and summary aggregation.
func TestCycle229_StreamTests_EdgeCases(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	modContent := "module github.com/zqk-os/zqk\n\ngo 1.24\n"
	if err := fileutil.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(modContent), paths.FilePerm644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	// Package 1: skipped package (no test files)
	skipDir := filepath.Join(tempDir, "pkg", "skippkg")
	if err := fileutil.MkdirAll(skipDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir skippkg: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(skipDir, "lib.go"), []byte("package skippkg\nfunc Lib(){}\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write lib.go: %v", err)
	}

	// Package 2: passing package
	passDir := filepath.Join(tempDir, "pkg", "passpkg")
	if err := fileutil.MkdirAll(passDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir passpkg: %v", err)
	}
	passCode := "package passpkg\nimport \"testing\"\nfunc TestPass(t *testing.T){}\n"
	if err := fileutil.WriteFile(filepath.Join(passDir, "pass_test.go"), []byte(passCode), paths.FilePerm644); err != nil {
		t.Fatalf("write pass_test.go: %v", err)
	}

	// Package 3: failing package
	failDir := filepath.Join(tempDir, "pkg", "failpkg")
	if err := fileutil.MkdirAll(failDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failpkg: %v", err)
	}
	failCode := "package failpkg\nimport \"testing\"\nfunc TestFail(t *testing.T){ t.Fatal(\"intentional failure\") }\n"
	if err := fileutil.WriteFile(filepath.Join(failDir, "fail_test.go"), []byte(failCode), paths.FilePerm644); err != nil {
		t.Fatalf("write fail_test.go: %v", err)
	}

	var buf bytes.Buffer
	// Test with zero-value options to exercise default values
	opts := StreamOptions{
		ProjectRoot: tempDir,
		Parallel:    0,
		Timeout:     0,
		Pkg:         "", // Empty exercises default "./..."
	}

	summary, runErr := StreamTests(context.Background(), opts, &buf)
	if runErr == nil {
		t.Fatalf("expected error from StreamTests with failing package")
	}

	out := buf.String()
	if !strings.Contains(out, "SKIP: pkg/skippkg") {
		t.Errorf("expected skip notification in output, got:\n%s", out)
	}
	if !strings.Contains(out, "PASS: pkg/passpkg") {
		t.Errorf("expected pass notification in output, got:\n%s", out)
	}
	if !strings.Contains(out, "FAIL: pkg/failpkg") {
		t.Errorf("expected fail notification in output, got:\n%s", out)
	}
	if summary.Passed < 1 || summary.Failed < 1 || summary.Skipped < 1 {
		t.Fatalf("expected passed, failed, and skipped packages, got %+v", summary)
	}
	if summary.ExitCode == 0 {
		t.Fatalf("expected non-zero exit code in summary, got %d", summary.ExitCode)
	}
}

// TestCycle229_TestRunner_ResolveAndBuildCommand validates invocation mapping and command construction.
func TestCycle229_TestRunner_ResolveAndBuildCommand(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// 1. Nil test case error
	_, err := ResolveInvocations(nil, tempDir)
	if err == nil {
		t.Fatalf("expected error on nil test case")
	}

	// 2. Partial verification_suites mapping: one criterion mapped, one falls back to buildDefaultCommand
	tcPartial := map[string]any{
		objects.FieldKeyCriteriaRefs: []string{"CRIT-A", "CRIT-B"},
		objects.FieldKeyVerificationSuites: []string{
			"CRIT-A: echo 'Mapped A'",
		},
		objects.FieldKeyPathOrID: "pkg/testrunner/sample.go",
	}
	invs, err := ResolveInvocations(tcPartial, tempDir)
	if err != nil {
		t.Fatalf("ResolveInvocations failed: %v", err)
	}
	if len(invs) != 2 {
		t.Fatalf("expected 2 invocations, got %d", len(invs))
	}
	if invs[0].CriterionID != "CRIT-A" || invs[0].Command != "echo 'Mapped A'" {
		t.Errorf("unexpected invocation 0: %+v", invs[0])
	}
	if invs[1].CriterionID != "CRIT-B" || !strings.Contains(invs[1].Command, "go test") {
		t.Errorf("expected invocation 1 to fallback to go test, got %+v", invs[1])
	}

	// 3. Fallback when pathOrID and verification_suites are empty
	tcEmpty := map[string]any{
		objects.FieldKeyCriteriaRefs: []string{"CRIT-DEFAULT"},
	}
	invsEmpty, err := ResolveInvocations(tcEmpty, tempDir)
	if err != nil {
		t.Fatalf("ResolveInvocations empty failed: %v", err)
	}
	if len(invsEmpty) != 1 || invsEmpty[0].Command != "echo 'Verifying CRIT-DEFAULT'" {
		t.Fatalf("expected default echo command, got %+v", invsEmpty)
	}

	// 4. buildDefaultCommand edge cases
	cmdEmpty := buildDefaultCommand("", "CRIT-1", tempDir)
	if cmdEmpty != "echo 'Checking CRIT-1'" {
		t.Errorf("expected checking echo for empty path, got %s", cmdEmpty)
	}

	// .go file (not _test.go)
	cmdGo := buildDefaultCommand("pkg/sub/worker.go", "CRIT-1", tempDir)
	if cmdGo != "go test -v ./pkg/sub" {
		t.Errorf("expected go test ./pkg/sub, got %s", cmdGo)
	}

	// _test.go without func Test
	noTestFile := filepath.Join(tempDir, "no_test.go")
	if err := fileutil.WriteFile(noTestFile, []byte("package sub\nfunc Helper(){}\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write no_test.go: %v", err)
	}
	cmdNoTest := buildDefaultCommand(noTestFile, "CRIT-1", tempDir)
	if !strings.HasPrefix(cmdNoTest, "go test -v") {
		t.Errorf("expected go test -v for test file without funcs, got %s", cmdNoTest)
	}

	// Raw shell command fallback
	cmdRaw := buildDefaultCommand("npm test", "CRIT-1", tempDir)
	if cmdRaw != "npm test" {
		t.Errorf("expected raw command, got %s", cmdRaw)
	}
}

// TestCycle229_TestRunner_ExecuteSubprocess validates subprocess execution, error paths, and isolation.
func TestCycle229_TestRunner_ExecuteSubprocess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// 1. Invocation with custom environment variables
	inv := CriterionInvocation{
		CriterionID: "CRIT-ENV",
		Command:     "test -n \"$CUSTOM_VAR_XYZ\"",
		Env:         []string{"CUSTOM_VAR_XYZ=present"},
	}
	res := ExecuteSubprocess(ctx, inv)
	if !res.Passed {
		t.Fatalf("expected custom env execution to pass, got error: %s", res.Error)
	}

	// 2. Command cancellation via context timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()

	invSleep := CriterionInvocation{
		CriterionID: "CRIT-TIMEOUT",
		Command:     "sleep 2",
	}
	resTimeout := ExecuteSubprocess(timeoutCtx, invSleep)
	if resTimeout.Passed {
		t.Fatalf("expected timeout invocation to fail")
	}

	// 3. Command execution failure branch
	invFail := CriterionInvocation{
		CriterionID: "CRIT-EXIT-CODE",
		Command:     "exit 7",
	}
	resFail := ExecuteSubprocess(ctx, invFail)
	if resFail.Passed || resFail.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %d, passed=%v", resFail.ExitCode, resFail.Passed)
	}
}

// TestCycle229_TestRunner_ExecuteSubprocess_InvalidShell exercises subprocess startup failure.
func TestCycle229_TestRunner_ExecuteSubprocess_InvalidShell(t *testing.T) {
	orig := zqkenv.OSShell().Get()
	zqkenv.OSShell().Set("/nonexistent/bin/invalid_shell_c229")
	defer zqkenv.OSShell().Set(orig)

	res := ExecuteSubprocess(context.Background(), CriterionInvocation{
		CriterionID: "CRIT-FAIL-SHELL",
		Command:     "echo hello",
	})
	if res.Passed || res.ExitCode != 1 || res.Error == "" {
		t.Fatalf("expected startup failure with exit code 1, got %+v", res)
	}
}

// failingStorageProvider intercepts Update calls to simulate a database or storage error.
type failingStorageProvider struct {
	storage.ObjectStorageProvider
	failID string
}

func (f *failingStorageProvider) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, data map[string]any) error {
	if id == f.failID {
		return fmt.Errorf("simulated storage failure for %s", id)
	}
	return f.ObjectStorageProvider.Update(ctx, secCtx, id, data)
}

// setupCycle229TestSuite seeds standard goal, req, criteria, and tc objects.
func setupCycle229TestSuite(
	t *testing.T,
	sp storage.ObjectStorageProvider,
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
) (string, string, string, string, string) {
	t.Helper()

	goalID := "GOAL-C229-001"
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          goalID,
		objects.FieldKeyKind:        objects.KindGoal,
		objects.FieldKeyTitle:       "Cycle 229 Goal",
		objects.FieldKeyDescription: "Cycle 229 Goal Description",
		objects.FieldKeyStatus:      objects.ObjectStatusActive,
	}); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	critID1 := "CRIT-C229-001"
	critID2 := "CRIT-C229-002"
	for _, cid := range []string{critID1, critID2} {
		if err := sp.Create(ctx, secCtx, map[string]any{
			objects.FieldKeyID:          cid,
			objects.FieldKeyKind:        objects.KindCriteria,
			objects.FieldKeyTitle:       "Criteria " + cid,
			objects.FieldKeyDescription: "Criteria Description " + cid,
			objects.FieldKeyStatus:      objects.ObjectStatusAwaitingVerification,
			objects.FieldKeyCategory:    "test",
		}); err != nil {
			t.Fatalf("create criteria %s: %v", cid, err)
		}
	}

	reqID := "REQ-C229-001"
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:           reqID,
		objects.FieldKeyKind:         objects.KindRequirement,
		objects.FieldKeyTitle:        "Cycle 229 Requirement",
		objects.FieldKeyDescription:  "Cycle 229 Requirement Description",
		objects.FieldKeyStatus:       objects.ObjectStatusActive,
		objects.FieldKeyGoalRefs:     []string{goalID},
		objects.FieldKeyCriteriaRefs: []string{critID1, critID2},
	}); err != nil {
		t.Fatalf("create req: %v", err)
	}

	tcID := "TST-C229-001"
	if err := sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:              tcID,
		objects.FieldKeyKind:            objects.KindTestCase,
		objects.FieldKeyTitle:           "Cycle 229 Test Suite",
		objects.FieldKeyDescription:     "Cycle 229 Test Suite Description",
		objects.FieldKeyStatus:          "originated", // Test originated->active transition
		objects.FieldKeyRequirementRefs: []string{reqID},
		objects.FieldKeyCriteriaRefs:    []string{critID1, critID2},
		objects.FieldKeyVerificationSuites: []string{
			fmt.Sprintf("%s: echo 'PASS-1'", critID1),
			fmt.Sprintf("%s: echo 'PASS-2'", critID2),
		},
	}); err != nil {
		t.Fatalf("create tc: %v", err)
	}

	return goalID, reqID, critID1, critID2, tcID
}

// TestCycle229_TestRunner_RunTestCase_ValidationAndFilters validates options and validations.
func TestCycle229_TestRunner_RunTestCase_ValidationAndFilters(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	sp := proj.FileStorage
	tempDir := proj.Root
	ctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. sp is nil error
	_, err := RunTestCase(ctx, nil, tempDir, "TST-001", RunOptions{})
	if err == nil {
		t.Fatalf("expected error on nil sp")
	}

	// 2. testCaseID is empty error
	_, err = RunTestCase(ctx, sp, tempDir, "", RunOptions{})
	if err == nil {
		t.Fatalf("expected error on empty testCaseID")
	}

	// 3. non-existent test case error
	_, err = RunTestCase(ctx, sp, tempDir, "TST-NONEXISTENT", RunOptions{})
	if err == nil {
		t.Fatalf("expected error on non-existent test case")
	}

	_, _, critID1, _, tcID := setupCycle229TestSuite(t, sp, ctx, secCtx)

	// 4. DryRun option test
	resDry, err := RunTestCase(ctx, sp, tempDir, tcID, RunOptions{DryRun: true})
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	if resDry.Status != "planned" || len(resDry.CriteriaResults) != 2 {
		t.Fatalf("expected planned status in DryRun, got %+v", resDry)
	}

	// 5. CriteriaFilter option test (run only critID1)
	resFilter, err := RunTestCase(ctx, sp, tempDir, tcID, RunOptions{
		CriteriaFilter: []string{critID1},
	})
	if err != nil {
		t.Fatalf("CriteriaFilter run failed: %v", err)
	}
	if resFilter.TotalCriteria != 1 || resFilter.PassedCriteria != 1 {
		t.Fatalf("expected 1 filtered criterion executed, got total=%d, passed=%d", resFilter.TotalCriteria, resFilter.PassedCriteria)
	}
}

// TestCycle229_TestRunner_RunTestCase_TransitionsAndCompletion validates state transitions and terminal gating.
func TestCycle229_TestRunner_RunTestCase_TransitionsAndCompletion(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	sp := proj.FileStorage
	tempDir := proj.Root
	ctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	_, _, _, critID2, tcID := setupCycle229TestSuite(t, sp, ctx, secCtx)

	// 1. Failing storage update on criterion transition (lines 382-388)
	failSp := &failingStorageProvider{
		ObjectStorageProvider: sp,
		failID:                critID2,
	}
	resFailUpdate, err := RunTestCase(ctx, failSp, tempDir, tcID, RunOptions{
		CriteriaFilter: []string{critID2},
	})
	if err != nil {
		t.Fatalf("run with failing criterion update failed: %v", err)
	}
	if resFailUpdate.PassedCriteria != 0 || resFailUpdate.FailedCriteria != 1 {
		t.Fatalf("expected failed criterion due to simulated storage update error, got %+v", resFailUpdate)
	}

	// 2. Full passing execution completing all criteria and test case
	tcCurrent, readErr := sp.Read(ctx, secCtx, tcID)
	if readErr != nil {
		t.Fatalf("read tc before full run: %v", readErr)
	}
	tcCurrent[objects.FieldKeyRemainingOpenCount] = 0
	if updateErr := sp.Update(ctx, secCtx, tcID, tcCurrent); updateErr != nil {
		t.Fatalf("update tc remaining_open_count: %v", updateErr)
	}

	resFull, err := RunTestCase(ctx, sp, tempDir, tcID, RunOptions{})
	if err != nil {
		t.Fatalf("full RunTestCase failed: %v", err)
	}
	if resFull.PassedCriteria != 2 || resFull.Status != "passed" {
		t.Fatalf("expected all 2 criteria passed, got %+v", resFull)
	}
	if !resFull.TestCaseCompleted {
		t.Fatalf("expected TestCaseCompleted to be true when all criteria passed and remaining count drained")
	}
}

// TestCycle229_EphemeralTmpDir_And_IsolatedEnv directly verifies internal isolation helpers.
func TestCycle229_EphemeralTmpDir_And_IsolatedEnv(t *testing.T) {
	t.Parallel()

	// 1. createEphemeralTmpDir creation and cleanup
	tmpDir, cleanup := createEphemeralTmpDir("zqk-cycle229-")
	if tmpDir == "" {
		t.Fatalf("expected non-empty tmpDir")
	}
	if !fileutil.Exists(tmpDir) {
		t.Fatalf("expected tmpDir to exist on disk: %s", tmpDir)
	}
	cleanup()
	if fileutil.Exists(tmpDir) {
		t.Fatalf("expected tmpDir to be cleaned up: %s", tmpDir)
	}

	// 2. setupIsolatedCommandEnv configuration
	cmd := testkit.ManagedCommand(t, context.Background(), "true")
	setupIsolatedCommandEnv(cmd, "/tmp/ephemeral-sandbox", []string{"BASE_VAR=base"}, []string{"EXTRA_VAR=extra"})

	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatalf("expected Setpgid to be configured on SysProcAttr")
	}

	hasBase := false
	hasExtra := false
	hasTmpDir := false
	for _, env := range cmd.Env {
		if env == "BASE_VAR=base" {
			hasBase = true
		}
		if env == "EXTRA_VAR=extra" {
			hasExtra = true
		}
		if env == "TMPDIR=/tmp/ephemeral-sandbox" {
			hasTmpDir = true
		}
	}

	if !hasBase || !hasExtra || !hasTmpDir {
		t.Fatalf("expected cmd.Env to include base, extra, and TMPDIR, got: %v", cmd.Env)
	}
}
