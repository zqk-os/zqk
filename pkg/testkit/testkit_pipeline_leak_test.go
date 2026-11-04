// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package testkit

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type cycle227RecordingTB struct {
	testing.TB
	fatalCalled bool
	fatalMsg    string
	logs        []string
	exitOnFatal bool
	cleanups    []func()
}

func (r *cycle227RecordingTB) Helper() {}

func (r *cycle227RecordingTB) Cleanup(fn func()) {
	r.cleanups = append(r.cleanups, fn)
}

func (r *cycle227RecordingTB) RunCleanups() {
	for i := len(r.cleanups) - 1; i >= 0; i-- {
		r.cleanups[i]()
	}
	r.cleanups = nil
}

func (r *cycle227RecordingTB) Fatal(args ...any) {
	r.fatalCalled = true
	r.fatalMsg = fmt.Sprint(args...)
	if r.exitOnFatal {
		runtime.Goexit()
	}
}

func (r *cycle227RecordingTB) Fatalf(format string, args ...any) {
	r.fatalCalled = true
	r.fatalMsg = fmt.Sprintf(format, args...)
	if r.exitOnFatal {
		runtime.Goexit()
	}
}

func (r *cycle227RecordingTB) Logf(format string, args ...any) {
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}

type dummyNilCleanupObj struct{}

func (d dummyNilCleanupObj) GetTestCleanup() func() {
	return nil
}

func TestCycle227_Pipeline_MetricsAndBuilders(t *testing.T) {
	sink := noopTestPipelineMetrics{}
	ctx := context.Background()
	sink.RecordStage(ctx, "test_kind", "stage_name", 10*time.Millisecond, nil)
	sink.RecordStageWithBuckets(ctx, "test_kind", "stage_name", 10*time.Millisecond, nil, map[string]string{
		"bucket": "unit",
	})

	b1 := NewTestPipelineBuilder("   ")
	if b1 == nil {
		t.Fatal("expected non-nil builder for blank kind")
	}

	b2 := NewTestPipelineBuilder("test.already_scoped")
	if b2 == nil {
		t.Fatal("expected non-nil builder for prefixed kind")
	}

	b3 := NewTestPipelineBuilder("plain_kind")
	if b3 == nil {
		t.Fatal("expected non-nil builder for plain kind")
	}

	res, err := RunTestPipeline(nil, "sample", 100,
		TestStage{
			Name: "increment",
			Fn: func(_ *pipeline.Context, payload any) (any, error) {
				val, ok := payload.(int)
				if !ok {
					return nil, errors.New("invalid payload type")
				}
				return val + 5, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("RunTestPipeline failed: %v", err)
	}
	intRes, ok := res.(int)
	if !ok || intRes != 105 {
		t.Fatalf("expected result 105, got %v", res)
	}

	expectedErr := errors.New("pipeline fail deliberate")
	_, err = RunTestPipeline(ctx, "fail_sample", 0,
		TestStage{
			Name: "fail_stage",
			Fn: func(_ *pipeline.Context, _ any) (any, error) {
				return nil, expectedErr
			},
		},
	)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestCycle227_Pipeline_NamedSteps(t *testing.T) {
	ctx := context.Background()
	var executed bool

	err := RunNamedTestSteps(ctx, "steps_success",
		NamedTestStep{
			Name: "nil_step",
			Fn:   nil,
		},
		NamedTestStep{
			Name: "exec_step",
			Fn: func() error {
				executed = true
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("RunNamedTestSteps failed: %v", err)
	}
	if !executed {
		t.Fatal("expected step to be executed")
	}

	deliberateErr := errors.New("step aborted")
	err = RunNamedTestSteps(ctx, "steps_failure",
		NamedTestStep{
			Name: "failing_step",
			Fn: func() error {
				return deliberateErr
			},
		},
	)
	if !errors.Is(err, deliberateErr) {
		t.Fatalf("expected deliberate error, got: %v", err)
	}
}

func TestCycle227_LeakDetector_SubprocessAndUnix(t *testing.T) {
	procs, err := findChildProcesses(1)
	if err != nil {
		t.Logf("findChildProcesses returned: %v (allowed in restricted environments)", err)
	} else {
		t.Logf("findChildProcesses returned %d processes for pid 1", len(procs))
	}

	cmd := ManagedCommand(t, context.Background(), "sleep", "5")
	if startErr := cmd.Start(); startErr != nil {
		t.Fatalf("failed to start sleep process: %v", startErr)
	}

	sleepPID := cmd.Process.Pid
	if killErr := killProcessByPid(sleepPID); killErr != nil {
		t.Fatalf("killProcessByPid failed: %v", killErr)
	}

	waitErr := cmd.Wait()
	if waitErr != nil && !strings.Contains(waitErr.Error(), "signal: killed") {
		t.Logf("cmd.Wait result: %v", waitErr)
	}

	mockTB := &cycle227RecordingTB{TB: t}
	leakedCmd := ManagedCommand(mockTB, context.Background(), "sleep", "10")
	if startErr := leakedCmd.Start(); startErr != nil {
		t.Fatalf("failed to start leaked child: %v", startErr)
	}
	defer func() {
		if killErr := killProcessByPid(leakedCmd.Process.Pid); killErr != nil {
			t.Logf("cleanup killProcessByPid: %v", killErr)
		}
		waitCleanupErr := leakedCmd.Wait()
		if waitCleanupErr != nil && !strings.Contains(waitCleanupErr.Error(), "signal: killed") {
			t.Logf("cleanup leakedCmd.Wait: %v", waitCleanupErr)
		}
	}()

	VerifyNoSubprocessLeaks(mockTB)
	if !mockTB.fatalCalled {
		t.Fatal("expected VerifyNoSubprocessLeaks to flag leaked child process")
	}
	if !strings.Contains(mockTB.fatalMsg, "detected") {
		t.Fatalf("unexpected fatal message: %s", mockTB.fatalMsg)
	}
}

func TestCycle227_LeakDetector_Goroutines(t *testing.T) {
	VerifyNoGoroutineLeaks(t, "testkit", "testing")

	goroutinelabels.NewGoroutine("testkit-leak-test", "verify goroutine leak detection").StartSimple(func() {
		time.Sleep(10 * time.Millisecond)
	})

	VerifyNoGoroutineLeaks(t, "testkit-leak-test")
}

func TestCycle227_Register_CleanupEdgeCases(t *testing.T) {
	tmp := t.TempDir()

	RegisterTempProjectTeardown(t, "", nil)

	RegisterStorageTestCleanup(t, "", nil)
	RegisterStorageTestCleanup(t, tmp, nil)

	dummyNil := dummyNilCleanupObj{}
	RegisterStorageTestCleanup(t, tmp, dummyNil)

	RegisterStorageTestCleanup(t, tmp, "arbitrary_storage_handle")

	optsWithoutSec := TeardownOptions{
		ProjectRoot: tmp,
	}
	RegisterAuditResetTeardown(t, optsWithoutSec, nil)

	mockTB := &cycle227RecordingTB{TB: t}
	failingOpts := TeardownOptions{
		ProjectRoot: "/nonexistent/invalid/path/for/teardown",
	}
	RegisterStandardTeardown(mockTB, failingOpts)

	var dummyFS *storage.FileObjectStorage
	RegisterAuditResetTeardown(mockTB, TeardownOptions{
		ProjectRoot: "/nonexistent/path/for/audit/reset",
		SecCtx:      pkgctx.NewSystemSecurityContext(),
	}, &dummyFS)

	mockTB.RunCleanups()
}

func TestCycle227_SchedulerDaemonBound_Coverage(t *testing.T) {
	ctx1, cancel1 := BoundSchedulerStartContext(t, nil, 0)
	defer cancel1()
	if ctx1 == nil {
		t.Fatal("expected non-nil context from BoundSchedulerStartContext")
	}

	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	ctx2, cancel2 := BoundSchedulerStartContext(t, parentCtx, 10*time.Second)
	defer cancel2()
	if ctx2 == nil {
		t.Fatal("expected non-nil child context")
	}

	doneChan := make(chan struct{})
	close(doneChan)

	if err := WaitChanOrBound(doneChan, nil); err != nil {
		t.Fatalf("WaitChanOrBound failed on closed chan with nil ctx: %v", err)
	}

	activeCtx := context.Background()
	if err := WaitChanOrBound(doneChan, activeCtx); err != nil {
		t.Fatalf("WaitChanOrBound failed on closed chan with active ctx: %v", err)
	}

	cancCtx, cancFn := context.WithCancel(context.Background())
	cancFn()
	unclosed := make(chan struct{})
	if err := WaitChanOrBound(unclosed, cancCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if err := WaitChanOrBoundWithCap(doneChan, nil, -1); err != nil {
		t.Fatalf("WaitChanOrBoundWithCap failed on cap <= 0: %v", err)
	}

	if err := WaitChanOrBoundWithCap(doneChan, activeCtx, 50*time.Millisecond); err != nil {
		t.Fatalf("expected nil error when done is closed: %v", err)
	}

	if err := WaitChanOrBoundWithCap(unclosed, cancCtx, 100*time.Millisecond); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled from cap with canceled boundCtx: %v", err)
	}

	start := time.Now()
	if err := WaitChanOrBoundWithCap(unclosed, activeCtx, 20*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded on cap timeout: %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("cap timeout took unexpectedly long: %v", time.Since(start))
	}
}

func TestCycle227_SchedulerDaemonBound_Options(t *testing.T) {
	opts := BoundCLISchedulerOpts{
		CLIBinary:   "/bin/echo",
		ProjectRoot: t.TempDir(),
		Env:         []string{"CUSTOM_VAR=value"},
		TestID:      "test-id-custom",
	}

	stopCmd := buildSchedulerStopCmd(opts, "test-id-custom")
	if stopCmd == nil {
		t.Fatal("expected non-nil stopCmd")
	}
	if stopCmd.Dir != opts.ProjectRoot {
		t.Fatalf("expected Dir %s, got %s", opts.ProjectRoot, stopCmd.Dir)
	}
	foundEnv := false
	for _, envVar := range stopCmd.Env {
		if strings.HasPrefix(envVar, "CUSTOM_VAR=") {
			foundEnv = true
			break
		}
	}
	if !foundEnv {
		t.Fatal("expected custom env in stopCmd.Env")
	}

	optsNoEnv := BoundCLISchedulerOpts{
		CLIBinary:   "/bin/echo",
		ProjectRoot: t.TempDir(),
	}
	stopCmd2 := buildSchedulerStopCmd(optsNoEnv, "test-id-default")
	if stopCmd2 == nil {
		t.Fatal("expected non-nil stopCmd2")
	}
}

func TestCycle227_Timing_ClampingAndCalculations(t *testing.T) {
	tmpDir := t.TempDir()
	origRelPath := TimingJSONRelativePath
	TimingJSONRelativePath = filepath.Join(tmpDir, "cycle227_timings.json")
	defer func() {
		TimingJSONRelativePath = origRelPath
	}()

	mockTestKey := fmt.Sprintf("%s/%s", "TestHighAvg", "pkg_cycle227")
	timingMutex.Lock()
	testTimings[mockTestKey] = &TestTiming{
		TestName:        "TestHighAvg",
		Package:         "pkg_cycle227",
		Duration:        40 * time.Minute,
		AverageDuration: 40 * time.Minute,
		MinDuration:     40 * time.Minute,
		MaxDuration:     40 * time.Minute,
		RunCount:        5,
	}
	timingMutex.Unlock()

	timeoutHigh := GetExpectedTimeout("TestHighAvg", "pkg_cycle227")
	if timeoutHigh != 1*time.Hour {
		t.Fatalf("expected clamped timeout of 1 hour, got: %v", timeoutHigh)
	}

	mockTestKeyMax := fmt.Sprintf("%s/%s", "TestHighMax", "pkg_cycle227")
	timingMutex.Lock()
	testTimings[mockTestKeyMax] = &TestTiming{
		TestName:        "TestHighMax",
		Package:         "pkg_cycle227",
		Duration:        20 * time.Minute,
		AverageDuration: 10 * time.Minute,
		MinDuration:     5 * time.Minute,
		MaxDuration:     25 * time.Minute,
		RunCount:        2,
	}
	timingMutex.Unlock()

	timeoutMax := GetExpectedTimeout("TestHighMax", "pkg_cycle227")
	if timeoutMax != 50*time.Minute {
		t.Fatalf("expected 50m (MaxDuration*2), got: %v", timeoutMax)
	}

	saveTimings()

	corruptPath := filepath.Join(tmpDir, "corrupt_timings.json")
	if writeErr := fileutil.WriteFile(corruptPath, []byte("{invalid-json-content"), paths.FilePerm644); writeErr != nil {
		t.Fatalf("failed to write corrupt timings file: %v", writeErr)
	}

	TimingJSONRelativePath = corruptPath
	loadTimings()

	targetMap := make(map[string]*TestTiming)
	LoadTimingsIntoMap(targetMap)
	if len(targetMap) != 0 {
		t.Fatalf("expected empty map from corrupt json, got %d entries", len(targetMap))
	}

	TimingJSONRelativePath = filepath.Join(tmpDir, "nonexistent", "missing.json")
	LoadTimingsIntoMap(targetMap)

	TimingJSONRelativePath = filepath.Join("/dev/null/forbidden", "timings.json")
	saveTimings()
}

func TestCycle227_Timing_PackageName(t *testing.T) {
	pkgName := getPackageName()
	if pkgName == "" {
		t.Fatal("expected non-empty package name")
	}
}

func TestCycle227_Verification_ErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Setenv(zqkenv.ProjectRoot().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), "")
	t.Setenv(zqkenv.InTest().Name(), "false")

	err := SignTestCaseCompletion(ctx, "TEST-MISSING", []string{"foo.txt"})
	if err == nil || !strings.Contains(err.Error(), "project root not set") {
		t.Fatalf("expected project root not set error, got: %v", err)
	}

	proj := PrepareIsolatedTempProject(t, &IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.TestRoot().Name(), proj.Root)
	t.Setenv(zqkenv.InTest().Name(), "true")

	err = SignTestCaseCompletion(ctx, "TEST-MISSING-ARTIFACT", []string{"nonexistent_artifact.txt"})
	if err == nil || !strings.Contains(err.Error(), "failed to open artifact") {
		t.Fatalf("expected failed to open artifact error, got: %v", err)
	}

	artRel := "present_artifact.txt"
	artAbs := filepath.Join(proj.Root, artRel)
	if writeErr := fileutil.WriteStandardFile(artAbs, []byte("artifact content proof")); writeErr != nil {
		t.Fatalf("failed to write artifact file: %v", writeErr)
	}

	err = SignTestCaseCompletion(ctx, "TEST-NONEXISTENT-CASE", []string{artAbs})
	if err == nil {
		t.Fatal("expected failure updating non-existent test case")
	}
}

func TestCycle227_Verification_FullProof(t *testing.T) {
	proj := PrepareIsolatedTempProject(t, &IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	if copyErr := testenvroot.CopyObjectSpecsFromProject(proj.Root, "../.."); copyErr != nil {
		t.Logf("CopyObjectSpecsFromProject notice: %v", copyErr)
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	testCaseID := "TEST-CYCLE227-PROOF"
	tc := map[string]any{
		objects.FieldKeyKind:     objects.KindTestCase,
		objects.FieldKeyID:       testCaseID,
		objects.FieldKeyTitle:    "Cycle 227 Verification Case",
		objects.FieldKeyStatus:   objects.ObjectStatusDraft,
		objects.FieldKeyPathOrID: "cycle227_proof.go",
		objects.FieldKeyScope:    "unit",
	}
	storage.CreateCASVisible(t, proj.FileStorage, ctx, secCtx, tc, objects.ObjectStatusActive)

	artRel := "cycle227_proof.go"
	artPath := filepath.Join(proj.Root, artRel)
	if writeErr := fileutil.WriteStandardFile(artPath, []byte("// proof artifact content")); writeErr != nil {
		t.Fatalf("failed to write test artifact: %v", writeErr)
	}

	if signErr := SignTestCaseCompletion(ctx, testCaseID, []string{artRel}); signErr != nil {
		t.Fatalf("SignTestCaseCompletion failed: %v", signErr)
	}

	newStore, storeErr := storage.NewFileObjectStorageForTest(proj.Root)
	if storeErr != nil {
		t.Fatalf("failed to open store for verification: %v", storeErr)
	}
	RegisterStorageTestCleanup(t, proj.Root, newStore)

	obj, readErr := newStore.Read(ctx, secCtx, testCaseID)
	if readErr != nil {
		t.Fatalf("failed to read test case after sign: %v", readErr)
	}

	if status := objects.GetString(obj, objects.FieldKeyStatus); status != objects.ObjectStatusComplete {
		t.Fatalf("expected status complete, got %s", status)
	}
}

func TestCycle227_IsolatedTempProject_Options(t *testing.T) {
	proj := PrepareIsolatedTempProject(t, &IsolatedTempProjectOptions{
		SkipFileStorage:          true,
		ForceRemoveRootOnCleanup: true,
		Kind:                     "cycle227_plain",
	})
	if proj.Root == "" {
		t.Fatal("expected non-empty Root")
	}
	if proj.FileStorage != nil {
		t.Fatal("expected nil FileStorage when SkipFileStorage is true")
	}

	var customStageRan bool
	projCustom := PrepareIsolatedTempProject(t, &IsolatedTempProjectOptions{
		UsePlainFileObjectStorage: true,
		Kind:                      "cycle227_custom",
		AppendStagesBeforeStorage: func(root string) []NamedTestStep {
			return []NamedTestStep{
				{
					Name: "custom_verification_stage",
					Fn: func() error {
						customStageRan = true
						marker := filepath.Join(root, "custom_marker.txt")
						return fileutil.WriteFile(marker, []byte("marked"), paths.FilePerm644)
					},
				},
			}
		},
	})
	if !customStageRan {
		t.Fatal("expected AppendStagesBeforeStorage stage to run")
	}
	if projCustom.FileStorage == nil {
		t.Fatal("expected non-nil FileStorage from UsePlainFileObjectStorage")
	}

	modRoot, err := moduleRootFromWorkingDir()
	if err != nil {
		t.Fatalf("moduleRootFromWorkingDir returned error: %v", err)
	}
	if modRoot == "" {
		t.Fatal("expected non-empty module root")
	}
}

func TestCycle227_ObjectHelpers_Variations(t *testing.T) {
	proj := PrepareIsolatedTempProject(t, &IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})

	yamlUnknownKind := `
id: "CUSTOM-001"
kind: "nonexistent_domain_kind"
title: "Custom Domain Entity"
status: "draft"
`
	filePath := WriteTestObjectStandalone(t, proj.Root, yamlUnknownKind)
	if filePath == "" {
		t.Fatal("expected valid file path from WriteTestObjectStandalone")
	}
	if statInfo, statErr := fileutil.Stat(filePath); statErr != nil || statInfo.IsDir() {
		t.Fatalf("expected regular file at %s: %v", filePath, statErr)
	}

	mockTB := &cycle227RecordingTB{TB: t, exitOnFatal: true}
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("test_malformed_yaml", "testing malformed yaml failure").StartSimple(func() {
		defer close(done)
		invalidPath := WriteTestObjectStandalone(mockTB, proj.Root, "invalid: [unclosed sequence")
		if invalidPath != "" {
			t.Logf("WriteTestObjectStandalone path: %s", invalidPath)
		}
	})
	<-done
	if !mockTB.fatalCalled {
		t.Fatal("expected fatal on malformed YAML in WriteTestObjectStandalone")
	}
}

func TestCycle227_GitEvidence_FailureBranch(t *testing.T) {
	mockTB := &cycle227RecordingTB{TB: t}
	tmp := t.TempDir()

	gitOut := runGit(mockTB, tmp, "invalid-git-subcommand-deliberate")
	if gitOut != "" {
		t.Logf("runGit output: %s", gitOut)
	}
	if !mockTB.fatalCalled {
		t.Fatal("expected fatal on failing git command")
	}
	if !strings.Contains(mockTB.fatalMsg, "invalid-git-subcommand-deliberate") {
		t.Fatalf("unexpected fatal message: %s", mockTB.fatalMsg)
	}
}

func TestCycle227_Subprocess_ManagedCommandLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := ManagedCommand(t, ctx, "sh", "-c", "echo 'cycle227-managed' && exit 0")
	tmp := t.TempDir()
	WireCLISubprocessForIsolatedProject(cmd, tmp)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ManagedCommand failed: %v", err)
	}
	if !strings.Contains(string(out), "cycle227-managed") {
		t.Fatalf("unexpected command output: %s", string(out))
	}

	hangCmd := ManagedCommand(t, context.Background(), "sleep", "10")
	if startErr := hangCmd.Start(); startErr != nil {
		t.Fatalf("failed to start hangCmd: %v", startErr)
	}

	if killErr := KillManagedProcessGroup(hangCmd); killErr != nil {
		t.Logf("KillManagedProcessGroup notice: %v", killErr)
	}
	waitErr := hangCmd.Wait()
	if waitErr != nil && !strings.Contains(waitErr.Error(), "signal: killed") {
		t.Logf("hangCmd.Wait notice: %v", waitErr)
	}
}

func TestCycle227_Teardown_EdgeCases(t *testing.T) {
	tmp := t.TempDir()

	opts := TempProjectTeardown(tmp, nil)
	b := pipeline.NewBuilder("test.teardown_assembly", nil)
	appendRet := AppendStandardStorageTeardownStages(b, opts)
	if appendRet == nil {
		t.Fatal("expected non-nil builder from AppendStandardStorageTeardownStages")
	}
	pl := b.Build()
	if pl == nil {
		t.Fatal("expected non-nil teardown pipeline")
	}

	if err := RunStandardTeardown(TeardownOptions{
		ProjectRoot: tmp,
	}); err != nil {
		t.Logf("RunStandardTeardown notice: %v", err)
	}

	ScrubProjectRootForTempCleanup(tmp, 2, 5*time.Millisecond)

	isProbable := IsProbableGitWorktreeRoot(tmp)
	if isProbable {
		t.Fatal("expected temp dir not to be git worktree root")
	}
}

func TestCycle227_ConcurrencySafety(t *testing.T) {
	const workerCount = 8
	var wg sync.WaitGroup
	wg.Add(workerCount)

	tmp := t.TempDir()

	for i := 0; i < workerCount; i++ {
		workerID := i
		builder := goroutinelabels.NewGoroutine("testkit_cycle227_worker", fmt.Sprintf("worker-%d", workerID))
		builder.StartSimple(func() {
			defer wg.Done()

			timeoutVal := GetExpectedTimeout("ConcurrentTest", "testkit")
			if timeoutVal <= 0 {
				t.Errorf("unexpected non-positive timeout: %v", timeoutVal)
			}

			b := NewTestPipelineBuilder("test.concurrent")
			builtPipeline := b.Build()
			if builtPipeline == nil {
				t.Errorf("unexpected nil pipeline")
			}

			done := make(chan struct{})
			close(done)
			if waitErr := WaitChanOrBoundWithCap(done, context.Background(), 10*time.Millisecond); waitErr != nil {
				t.Errorf("WaitChanOrBoundWithCap failed: %v", waitErr)
			}

			isWorktree := IsProbableGitWorktreeRoot(tmp)
			if isWorktree {
				t.Errorf("temp dir should not be git worktree")
			}

			pollCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			if pollErr := EventuallyWithContext(pollCtx, 5*time.Millisecond, func(_ context.Context) (bool, error) {
				return true, nil
			}); pollErr != nil {
				t.Errorf("EventuallyWithContext failed: %v", pollErr)
			}
		})
	}

	wg.Wait()
}
