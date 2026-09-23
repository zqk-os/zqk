package testkit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/storage"
)

type dummyCleanupObj struct {
	cleaned bool
}

func (d *dummyCleanupObj) GetTestCleanup() func() {
	return func() {
		d.cleaned = true
	}
}

type fakeTB struct {
	testing.TB
	fatalCalled bool
}

func (f *fakeTB) Helper() {}
func (f *fakeTB) Fatal(args ...any) {
	f.fatalCalled = true
}
func (f *fakeTB) Fatalf(format string, args ...any) {
	f.fatalCalled = true
}

func TestCLISubprocess_Wire(t *testing.T) {
	tmp := t.TempDir()
	cmd := exec.Command("echo", "hello")
	WireCLISubprocessForIsolatedProject(cmd, tmp)
	if cmd.Dir != tmp {
		t.Fatalf("expected cmd.Dir %s, got %s", tmp, cmd.Dir)
	}
}

func TestGitEvidence_ProductCommitMentioningBacklog(t *testing.T) {
	tmp := t.TempDir()
	relPath := "test_file.txt"
	if err := os.WriteFile(filepath.Join(tmp, relPath), []byte("hello backlog"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	commitHash := ProductCommitMentioningBacklog(t, tmp, "BLI-1234", relPath)
	if commitHash == "" {
		t.Fatal("expected non-empty commit hash")
	}

	// Verify commit message
	cmd := exec.Command("git", "log", "-1", "--pretty=%B")
	cmd.Dir = tmp
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to get git log: %v", err)
	}
	if !strings.Contains(string(out), "BLI-1234") {
		t.Fatalf("expected commit log to contain BLI-1234, got: %s", string(out))
	}
}

func TestPrepareGraphConnectionForTest_SkipsWhenDisabled(t *testing.T) {
	t.Setenv("ZQK_GRAPH_ENABLED", "false")
	var skipped bool
	t.Run("subtest", func(subT *testing.T) {
		defer func() {
			if subT.Skipped() {
				skipped = true
			}
		}()
		PrepareGraphConnectionForTest(subT)
	})
	if !skipped {
		t.Fatal("expected test to skip when ZQK_GRAPH_ENABLED=false")
	}
}

func TestIsolatedTempProject_Benchmark(t *testing.T) {
	res := testing.Benchmark(func(b *testing.B) {
		proj := PrepareIsolatedTempProjectForBenchmark(b, &IsolatedTempProjectOptions{
			SkipFileStorage: true,
		})
		if proj.Root == "" {
			b.Fatal("expected non-empty project root")
		}
	})
	t.Logf("Benchmark runs: %d", res.N)
}

func TestObjectHelpers_WriteTestObject(t *testing.T) {
	proj := PrepareIsolatedTempProject(t, &IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	fs := proj.FileStorage

	yamlContent := `
id: "BLI-9999"
kind: "backlog_item"
title: "Test Task"
status: "originated"
properties:
  owner: "tester"
  priority: 1
`
	filePath := WriteTestObject(t, fs, yamlContent)
	if filePath == "" {
		t.Fatal("expected non-empty filePath from WriteTestObject")
	}

	secCtx := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
	readObj, err := fs.Read(context.Background(), secCtx, "BLI-9999")
	if err != nil {
		t.Fatalf("failed to read test object: %v", err)
	}
	if readObj[objects.FieldKeyID] != "BLI-9999" {
		t.Fatalf("expected object ID BLI-9999, got %v", readObj[objects.FieldKeyID])
	}

	// Standalone write test object
	yamlStandalone := `
id: "TEST-OBJ-002"
kind: "epic"
title: "Test Epic"
status: "in_progress"
`
	standalonePath := WriteTestObjectStandalone(t, proj.Root, yamlStandalone)
	if _, err := os.Stat(standalonePath); os.IsNotExist(err) {
		t.Fatalf("expected standalone file to exist at %s", standalonePath)
	}
}

func TestPoll_Eventually(t *testing.T) {
	count := 0
	Eventually(t, 500*time.Millisecond, 10*time.Millisecond, func() bool {
		count++
		return count >= 2
	})

	// EventuallyWithContext success
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctxCount := 0
	err := EventuallyWithContext(ctx, 10*time.Millisecond, func(_ context.Context) (bool, error) {
		ctxCount++
		return ctxCount >= 2, nil
	})
	if err != nil {
		t.Fatalf("EventuallyWithContext failed: %v", err)
	}

	// EventuallyWithContext error return
	errExpected := errors.New("boom")
	err = EventuallyWithContext(ctx, 10*time.Millisecond, func(_ context.Context) (bool, error) {
		return false, errExpected
	})
	if !errors.Is(err, errExpected) {
		t.Fatalf("expected errExpected, got %v", err)
	}

	// EventuallyWithContext context cancel
	cancCtx, cancelFn := context.WithCancel(context.Background())
	cancelFn()
	err = EventuallyWithContext(cancCtx, 10*time.Millisecond, func(_ context.Context) (bool, error) {
		return false, nil
	})
	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
}

func TestRegister_CleanupVariants(t *testing.T) {
	tmp := t.TempDir()

	// Empty project root should return early
	RegisterTempProjectTeardown(t, "", nil)

	// RegisterAuditResetTeardown
	var dummyFS *storage.FileObjectStorage
	secCtx := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
	RegisterAuditResetTeardown(t, TeardownOptions{
		ProjectRoot: tmp,
		SecCtx:      secCtx,
	}, &dummyFS)

	// RegisterStorageTestCleanup with nil
	RegisterStorageTestCleanup(t, tmp, nil)
	RegisterStorageTestCleanup(t, "", nil)

	// RegisterStorageTestCleanup with cleanupGetter
	d := &dummyCleanupObj{}
	RegisterStorageTestCleanup(t, tmp, d)

	// RegisterStorageTestCleanup with unknown type
	RegisterStorageTestCleanup(t, tmp, "random_string")
}

func TestSchedulerDaemonBound_ValidationAndLimits(t *testing.T) {
	fb := &fakeTB{TB: t}
	StartBoundCLIScheduler(fb, BoundCLISchedulerOpts{
		CLIBinary:   "",
		ProjectRoot: "/tmp",
	})
	if !fb.fatalCalled {
		t.Fatal("expected fatal on missing CLIBinary")
	}

	fb2 := &fakeTB{TB: t}
	StartBoundCLIScheduler(fb2, BoundCLISchedulerOpts{
		CLIBinary:   "/bin/true",
		ProjectRoot: "",
	})
	if !fb2.fatalCalled {
		t.Fatal("expected fatal on missing ProjectRoot")
	}
}

func TestSchedulerDaemonBound_WaitChan(t *testing.T) {
	done := make(chan struct{})
	close(done)

	// Nil boundCtx
	if err := WaitChanOrBound(done, nil); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// Canceled boundCtx
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	unclosedDone := make(chan struct{})
	if err := WaitChanOrBound(unclosedDone, ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// WaitChanOrBoundWithCap with cap <= 0
	if err := WaitChanOrBoundWithCap(done, nil, 0); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// WaitChanOrBoundWithCap with nil boundCtx and done closed
	if err := WaitChanOrBoundWithCap(done, nil, 100*time.Millisecond); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// WaitChanOrBoundWithCap with nil boundCtx timing out
	start := time.Now()
	if err := WaitChanOrBoundWithCap(unclosedDone, nil, 20*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("took too long: %v", time.Since(start))
	}
}

func TestTeardown_Helpers(t *testing.T) {
	tmp := t.TempDir()

	opts := TempProjectTeardown(tmp, nil)
	if opts.ProjectRoot != tmp {
		t.Fatalf("expected ProjectRoot %s, got %s", tmp, opts.ProjectRoot)
	}

	pl := StandardTeardownPipeline(opts)
	if pl == nil {
		t.Fatal("expected non-nil pipeline")
	}

	ScrubProjectRootForTempCleanup(tmp, 1, time.Millisecond)

	_ = IsProbableGitWorktreeRoot(tmp)
}

func TestTestPipeline_MetricsAndBuilders(t *testing.T) {
	sink := noopTestPipelineMetrics{}
	ctx := context.Background()
	sink.RecordStage(ctx, "k", "s", time.Millisecond, nil)
	sink.RecordStageWithBuckets(ctx, "k", "s", time.Millisecond, nil, map[string]string{"foo": "bar"})

	// NewTestPipelineBuilder with various kinds
	b1 := NewTestPipelineBuilder("")
	if b1 == nil {
		t.Fatal("expected builder")
	}
	b2 := NewTestPipelineBuilder("test.already_prefixed")
	if b2 == nil {
		t.Fatal("expected builder")
	}

	// RunTestPipeline with nil ctx
	res, err := RunTestPipeline(nil, "mytest", 42, TestStage{
		Name: "step1",
		Fn: func(_ *pipeline.Context, payload any) (any, error) {
			return payload.(int) + 1, nil
		},
	})
	if err != nil || res.(int) != 43 {
		t.Fatalf("expected 43 and nil error, got %v, %v", res, err)
	}

	// RunNamedTestSteps with nil fn and error fn
	err = RunNamedTestSteps(ctx, "steps_test",
		NamedTestStep{
			Name: "nil_step",
			Fn:   nil,
		},
		NamedTestStep{
			Name: "ok_step",
			Fn: func() error {
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	stepErr := errors.New("step error")
	err = RunNamedTestSteps(ctx, "steps_err_test",
		NamedTestStep{
			Name: "err_step",
			Fn: func() error {
				return stepErr
			},
		},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestTiming_Comprehensive(t *testing.T) {
	// Isolate timing file to temp dir
	tmp := t.TempDir()
	origPath := TimingJSONRelativePath
	TimingJSONRelativePath = filepath.Join(tmp, "test_timings.json")
	defer func() {
		TimingJSONRelativePath = origPath
	}()

	// Initial timeout when none exists
	timeout := GetExpectedTimeout("NonExistentTest", "dummyPkg")
	if timeout != 5*time.Minute {
		t.Fatalf("expected default 5m timeout, got %v", timeout)
	}

	// First record
	RecordTestTiming(t, 100*time.Millisecond)

	// Second record to exercise update path
	RecordTestTiming(t, 200*time.Millisecond)

	// Third record with shorter duration to exercise MinDuration
	RecordTestTiming(t, 50*time.Millisecond)

	// Fourth record with longer duration to exercise MaxDuration
	RecordTestTiming(t, 500*time.Millisecond)

	// ReportTestTiming helper
	done := ReportTestTiming(t, time.Now().Add(-10*time.Millisecond))
	done()

	// GetExpectedTimeout after records
	pkgName := getPackageName()
	tName := t.Name()
	expected := GetExpectedTimeout(tName, pkgName)
	if expected < 30*time.Second {
		t.Fatalf("expected timeout >= 30s, got %v", expected)
	}

	// LoadTimingsIntoMap
	m := make(map[string]*TestTiming)
	LoadTimingsIntoMap(m)
	if len(m) == 0 {
		t.Fatal("expected at least one timing loaded into map")
	}

	// Force save & load
	saveTimings()
	loadTimings()
}
