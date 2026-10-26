// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package testing

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storagetesting"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type cycle226MockAuditBuffer struct {
	enabled bool
	flushed bool
}

func (m *cycle226MockAuditBuffer) SetEnabled(enabled bool) {
	m.enabled = enabled
}

func (m *cycle226MockAuditBuffer) Flush() error {
	m.flushed = true
	return nil
}

type cycle226MockStorage struct {
	cleanedUp bool
}

func (m *cycle226MockStorage) GetTestCleanup() func() {
	return func() {
		m.cleanedUp = true
	}
}

func TestCallbackWaiterLifecycle(t *testing.T) {
	t.Run("new waiter initial count", func(t *testing.T) {
		cw := NewCallbackWaiter()
		assert.Equal(t, int64(0), cw.Count())
	})

	t.Run("fast path already invoked", func(t *testing.T) {
		cw := NewCallbackWaiter()
		cw.Invoke()
		assert.Equal(t, int64(1), cw.Count())
		ctx := context.Background()
		assert.True(t, cw.WaitForInvocation(ctx, 100*time.Millisecond))
	})

	t.Run("wait path concurrent invoke", func(t *testing.T) {
		cw := NewCallbackWaiter()
		ctx := context.Background()
		goroutinelabels.NewGoroutine("testing", "delayed invoke").StartSimple(func() {
			time.Sleep(20 * time.Millisecond)
			cw.Invoke()
		})
		assert.True(t, cw.WaitForInvocation(ctx, 500*time.Millisecond))
		assert.Equal(t, int64(1), cw.Count())
	})

	t.Run("wait path timeout", func(t *testing.T) {
		cw := NewCallbackWaiter()
		ctx := context.Background()
		assert.False(t, cw.WaitForInvocation(ctx, 30*time.Millisecond))
	})

	t.Run("wait for count already reached", func(t *testing.T) {
		cw := NewCallbackWaiter()
		cw.Invoke()
		cw.Invoke()
		ctx := context.Background()
		assert.True(t, cw.WaitForCount(ctx, 2, 100*time.Millisecond))
	})

	t.Run("wait for count concurrent and timeout", func(t *testing.T) {
		cw := NewCallbackWaiter()
		ctx := context.Background()
		goroutinelabels.NewGoroutine("testing", "delayed count invoke").StartSimple(func() {
			time.Sleep(15 * time.Millisecond)
			cw.Invoke()
			cw.Invoke()
		})
		assert.True(t, cw.WaitForCount(ctx, 2, 500*time.Millisecond))
		assert.False(t, cw.WaitForCount(ctx, 10, 30*time.Millisecond))
	})
}

func TestAtomicCounterLifecycle(t *testing.T) {
	ac := NewAtomicCounter()
	assert.Equal(t, int64(0), ac.Get())

	ac.Increment()
	ac.Increment()
	assert.Equal(t, int64(2), ac.Get())

	ctx := context.Background()
	assert.True(t, ac.WaitForValue(ctx, 2, 100*time.Millisecond))

	goroutinelabels.NewGoroutine("testing", "delayed increment").StartSimple(func() {
		time.Sleep(15 * time.Millisecond)
		ac.Increment()
	})
	assert.True(t, ac.WaitForValue(ctx, 3, 500*time.Millisecond))
	assert.False(t, ac.WaitForValue(ctx, 100, 30*time.Millisecond))
}

func TestEventCollectorLifecycle(t *testing.T) {
	ec := NewEventCollector()
	assert.Empty(t, ec.GetEvents())

	ec.Add("event1")
	ec.Add(42)
	events := ec.GetEvents()
	assert.Len(t, events, 2)
	assert.Equal(t, "event1", events[0])
	assert.Equal(t, 42, events[1])

	ctx := context.Background()
	assert.True(t, ec.WaitForEvents(ctx, 2, 100*time.Millisecond))

	goroutinelabels.NewGoroutine("testing", "delayed event").StartSimple(func() {
		time.Sleep(20 * time.Millisecond)
		ec.Add("event3")
	})
	assert.True(t, ec.WaitForEvents(ctx, 3, 500*time.Millisecond))
	assert.False(t, ec.WaitForEvents(ctx, 10, 30*time.Millisecond))
}

func TestWorkerStateTrackerLifecycle(t *testing.T) {
	wst := NewWorkerStateTracker()
	workerID := "worker-alpha"

	assert.False(t, wst.IsRunning(workerID))

	ctx := context.Background()
	assert.True(t, wst.WaitForStopped(ctx, workerID, 50*time.Millisecond))

	goroutinelabels.NewGoroutine("testing", "start worker").StartSimple(func() {
		time.Sleep(15 * time.Millisecond)
		wst.SetState(workerID, true)
	})
	assert.True(t, wst.WaitForRunning(ctx, workerID, 500*time.Millisecond))
	assert.True(t, wst.IsRunning(workerID))

	goroutinelabels.NewGoroutine("testing", "stop worker").StartSimple(func() {
		time.Sleep(15 * time.Millisecond)
		wst.SetState(workerID, false)
	})
	assert.True(t, wst.WaitForStopped(ctx, workerID, 500*time.Millisecond))
	assert.False(t, wst.IsRunning(workerID))

	assert.False(t, wst.WaitForRunning(ctx, "nonexistent-worker", 30*time.Millisecond))
}

func TestDeterministicWaitConditions(t *testing.T) {
	ctx := context.Background()

	t.Run("immediate condition true", func(t *testing.T) {
		called := false
		res := WaitForCondition(ctx, func() bool {
			called = true
			return true
		}, 5*time.Millisecond)
		assert.True(t, res)
		assert.True(t, called)
	})

	t.Run("delayed condition true", func(t *testing.T) {
		count := 0
		res := WaitForCondition(ctx, func() bool {
			count++
			return count >= 3
		}, 5*time.Millisecond)
		assert.True(t, res)
		assert.GreaterOrEqual(t, count, 3)
	})

	t.Run("cancelled context", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel()
		res := WaitForCondition(cancelCtx, func() bool {
			return false
		}, 5*time.Millisecond)
		assert.False(t, res)
	})

	t.Run("wait with timeout helper", func(t *testing.T) {
		flag := false
		goroutinelabels.NewGoroutine("testing", "delayed flag").StartSimple(func() {
			time.Sleep(15 * time.Millisecond)
			flag = true
		})
		res := WaitForConditionWithTimeout(ctx, func() bool {
			return flag
		}, 500*time.Millisecond, 5*time.Millisecond)
		assert.True(t, res)

		resTimeout := WaitForConditionWithTimeout(ctx, func() bool {
			return false
		}, 30*time.Millisecond, 5*time.Millisecond)
		assert.False(t, resTimeout)
	})
}

func TestFixtureObjectTitleFormatting(t *testing.T) {
	tests := []struct {
		kind     string
		seq      int
		expected string
	}{
		{"goal", 1, "Fixture: goal #1"},
		{"milestone", 42, "Fixture: milestone #42"},
		{"priority_plan", 0, "Fixture: priority_plan #0"},
		{"custom_kind-test", -5, "Fixture: custom_kind-test #-5"},
		{"", 99, "Fixture:  #99"},
	}

	for _, tc := range tests {
		title := FixtureObjectTitle(tc.kind, tc.seq)
		assert.Equal(t, tc.expected, title)
	}
}

func TestMustMkdirAllScaffolding(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "sub1", "sub2", "sub3")

	MustMkdirAll(t, nestedPath, 0o755)

	fi, err := fileutil.Stat(nestedPath)
	require.NoError(t, err)
	assert.True(t, fi.IsDir())
}

func TestParseGoTestOutputResults(t *testing.T) {
	passingOutput := `=== RUN   TestSamplePass
--- PASS: TestSamplePass (0.01s)
PASS
ok  	github.com/zqk-os/zqk/pkg/testing	0.050s
`
	summary, err := ParseGoTestOutput(passingOutput)
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.Equal(t, 1, summary.PassedCount)
	assert.Equal(t, 0, summary.FailedCount)
	assert.Len(t, summary.PassedTestList, 1)
	assert.Equal(t, "TestSamplePass", summary.PassedTestList[0].TestName)
	assert.Equal(t, "PASS", summary.PassedTestList[0].Status)

	failingOutput := `=== RUN   TestSampleFail
--- FAIL: TestSampleFail (0.02s)
FAIL
FAIL	github.com/zqk-os/zqk/pkg/testing	0.060s
`
	failSummary, err := ParseGoTestOutput(failingOutput)
	require.NoError(t, err)
	require.NotNil(t, failSummary)
	assert.Equal(t, 1, failSummary.FailedCount)
	assert.Contains(t, failSummary.GetFailedTestNames(), "TestSampleFail")
	failedByPkg := failSummary.GetFailedTestsByPackage()
	assert.NotEmpty(t, failedByPkg)

	emptySummary, err := ParseGoTestOutput("")
	require.NoError(t, err)
	require.NotNil(t, emptySummary)
	assert.Equal(t, 0, emptySummary.TotalTests)
}

func TestScenarioEnvironmentIsolation(t *testing.T) {
	t.Run("copy dir successfully", func(t *testing.T) {
		srcDir := filepath.Join(t.TempDir(), "src")
		dstDir := filepath.Join(t.TempDir(), "dst")
		require.NoError(t, fileutil.EnsureDir(filepath.Join(srcDir, "sub")))

		srcFile := filepath.Join(srcDir, "sub", "test.txt")
		require.NoError(t, fileutil.WriteFile(srcFile, []byte("payload"), 0o755))

		err := CopyDir(srcDir, dstDir)
		require.NoError(t, err)

		dstFile := filepath.Join(dstDir, "sub", "test.txt")
		fi, err := fileutil.Stat(dstFile)
		require.NoError(t, err)
		assert.Equal(t, fs.FileMode(0o755), fi.Mode().Perm())

		content, err := fileutil.ReadFile(dstFile)
		require.NoError(t, err)
		assert.Equal(t, "payload", string(content))
	})

	t.Run("copy dir nonexistent source error", func(t *testing.T) {
		err := CopyDir("/nonexistent/source/dir/path", filepath.Join(t.TempDir(), "dst"))
		assert.Error(t, err)
	})

	t.Run("setup scenario test environment with fixture", func(t *testing.T) {
		projRoot := findProjectRoot()
		require.NotEmpty(t, projRoot)

		scenarioName := "temp-cycle226-fixture"
		scenarioSrc := filepath.Join(projRoot, "test-scenarios", scenarioName)
		require.NoError(t, fileutil.EnsureDir(scenarioSrc))
		t.Cleanup(func() {
			if cleanErr := fileutil.RemoveAll(filepath.Join(projRoot, "test-scenarios")); cleanErr != nil {
				t.Logf("cleanup error: %v", cleanErr)
			}
		})

		dummyFile := filepath.Join(scenarioSrc, "scenario.txt")
		require.NoError(t, fileutil.WriteFile(dummyFile, []byte("scenario-data"), 0o644))

		env := SetupScenarioTestEnvironment(t, scenarioName)
		require.NotNil(t, env)
		assert.Equal(t, scenarioName, env.ScenarioName)
		assert.NotEmpty(t, env.TestRoot)
		assert.NotEmpty(t, env.ScenarioRoot)

		copiedFile := filepath.Join(env.ScenarioRoot, "scenario.txt")
		readData, err := fileutil.ReadFile(copiedFile)
		require.NoError(t, err)
		assert.Equal(t, "scenario-data", string(readData))
	})

	t.Run("setup scenario test environment nonexistent skips", func(t *testing.T) {
		subtestSkipped := false
		t.Run("skipped subtest", func(innerT *testing.T) {
			SetupScenarioTestEnvironment(innerT, "nonexistent-scenario-cycle226")
			subtestSkipped = true
		})
		assert.False(t, subtestSkipped)
	})
}

func TestSnapshotTestHelpersSuite(t *testing.T) {
	projRoot := findProjectRoot()
	require.NotEmpty(t, projRoot)

	dummySnapshotPath := filepath.Join(projRoot, "test-scenarios", "mock-scenario", "snapshot.tar")
	helper, err := SetupSnapshotTestEnvironment(t, dummySnapshotPath)
	require.NoError(t, err)
	require.NotNil(t, helper)
	assert.NotEmpty(t, helper.TestRoot)
	assert.Equal(t, dummySnapshotPath, helper.SnapshotPath)
	assert.False(t, helper.IsRestored)

	err = helper.RestoreSnapshot(t)
	require.NoError(t, err)
	assert.True(t, helper.IsRestored)

	assert.Equal(t, helper.TestRoot, helper.GetProjectRoot())
	helper.TestRoot = ""
	assert.Equal(t, helper.OriginalRoot, helper.GetProjectRoot())

	pattern := NewSnapshotAgnosticTest(t, true, "/path/to/snap.tar")
	require.NotNil(t, pattern)
	assert.True(t, pattern.UseSnapshot)
	assert.Equal(t, "/path/to/snap.tar", pattern.SnapshotPath)
	assert.NotEmpty(t, pattern.GetTestRoot())

	assert.True(t, contains("alpha-beta-gamma", "beta"))
	assert.True(t, contains("exact", "exact"))
	assert.False(t, contains("short", "longer-substring"))
	assert.False(t, contains("abcdef", "xyz"))
	assert.True(t, containsMiddle("abcdef", "cd"))
	assert.False(t, containsMiddle("abcdef", "gh"))

	t.Run("find original project root paths", func(t *testing.T) {
		p1 := filepath.Join(projRoot, "test-scenarios", "scen1", "snap.tar")
		assert.Equal(t, projRoot, findOriginalProjectRoot(p1))

		p2 := filepath.Join(t.TempDir(), "test-scenarios", "scen1", "snap.tar")
		assert.Equal(t, projRoot, findOriginalProjectRoot(p2))

		p3 := filepath.Join(t.TempDir(), "random", "snap.tar")
		assert.Equal(t, projRoot, findOriginalProjectRoot(p3))
	})
}

func TestTimingAndServiceHelpers(t *testing.T) {
	tmpDir := t.TempDir()
	origTimingPath := testkit.TimingJSONRelativePath
	testkit.TimingJSONRelativePath = filepath.Join(tmpDir, "test_timings.json")
	t.Cleanup(func() {
		testkit.TimingJSONRelativePath = origTimingPath
	})
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	RecordTestTiming(t, 25*time.Millisecond)

	duration := GetExpectedTimeout("TestTimingAndServiceHelpers", "pkg/testing")
	assert.Greater(t, duration, time.Duration(0))

	finishTiming := ReportTestTiming(t, time.Now().Add(-10*time.Millisecond))
	require.NotNil(t, finishTiming)
	finishTiming()

	mgr := NewServiceManager()
	require.NotNil(t, mgr)

	cfg := GetMemGraphConfig()
	assert.NotEmpty(t, cfg.ContainerName)
}

func TestTestSetupAndConfigElevated(t *testing.T) {
	t.Run("write test config", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := WriteTestConfig(tmpDir, nil)
		require.NoError(t, err)
		dataDir := filepath.Join(tmpDir, paths.ProjectDataDir)
		fi, statErr := fileutil.Stat(dataDir)
		require.NoError(t, statErr)
		assert.True(t, fi.IsDir())

		// Trigger WriteTestConfig error
		badDir := t.TempDir()
		require.NoError(t, fileutil.WriteFile(filepath.Join(badDir, paths.ProjectDataDir), []byte("blocker"), 0o644))
		errBad := WriteTestConfig(badDir, nil)
		assert.Error(t, errBad)
	})

	t.Run("setup test environment error path", func(t *testing.T) {
		badDir := t.TempDir()
		require.NoError(t, fileutil.WriteFile(filepath.Join(badDir, paths.ConfigDir), []byte("blocker"), 0o644))
		_, err := SetupTestEnvironment(badDir)
		assert.Error(t, err)
	})

	t.Run("get test config relative path", func(t *testing.T) {
		t.Setenv(zqkenv.TestRoot().Name(), "custom/rel/path")
		cfg := GetTestConfig()
		require.NotNil(t, cfg)
		assert.True(t, cfg.UseTestConfig)
		assert.True(t, filepath.IsAbs(cfg.TestProjectRoot))
	})
}

func TestTestEnvironmentSetupLifecycle(t *testing.T) {
	t.Run("default test env options", func(t *testing.T) {
		opts := DefaultTestEnvironmentOptions()
		require.NotNil(t, opts)
		assert.True(t, opts.CopySpecs)
		assert.True(t, opts.GenerateSpecs)
		assert.True(t, opts.DisableAuditBuffer)
		assert.True(t, opts.NoopStorageMetrics)
		assert.False(t, opts.SkipStorageCreation)
	})

	t.Run("setup complete test environment with mock factory", func(t *testing.T) {
		mockBuf := &cycle226MockAuditBuffer{}
		mockStorage := &cycle226MockStorage{}

		factory := storagetesting.NewMockIsolationFactory().
			WithAuditBuffer(mockBuf).
			WithCreateFileStorage(func(testRoot string) (any, error) {
				return mockStorage, nil
			})

		opts := DefaultTestEnvironmentOptions()
		opts.ProjectRoot = findProjectRoot()
		opts.NoopStorageMetrics = false
		env := SetupCompleteTestEnvironment(t, opts, factory)
		require.NotNil(t, env)
		assert.NotEmpty(t, env.TestRoot)
		assert.NotNil(t, env.SpecLoader)
		assert.NotNil(t, env.LifecycleLoader)
		assert.NotNil(t, env.SecurityContext)

		assert.False(t, mockBuf.enabled)
		assert.True(t, mockBuf.flushed)

		env.Cleanup()
		assert.True(t, mockStorage.cleanedUp)
	})

	t.Run("setup complete test environment default project root and nil audit buffer", func(t *testing.T) {
		factory := storagetesting.NewMockIsolationFactory().
			WithAuditBuffer(nil).
			WithCreateFileStorage(func(testRoot string) (any, error) {
				return struct{}{}, nil
			})

		opts := DefaultTestEnvironmentOptions()
		opts.ProjectRoot = ""
		env := SetupCompleteTestEnvironment(t, opts, factory)
		require.NotNil(t, env)
		env.Cleanup()
	})

	t.Run("setup complete test environment skip storage", func(t *testing.T) {
		opts := &TestEnvironmentOptions{
			SkipStorageCreation: true,
			CopySpecs:           false,
			GenerateSpecs:       false,
			DisableAuditBuffer:  false,
		}
		env := SetupCompleteTestEnvironment(t, opts, nil)
		require.NotNil(t, env)
		assert.Nil(t, env.Storage)
		env.Cleanup()
	})
}

func TestCopySpecFilesAndSynonyms(t *testing.T) {
	t.Run("copy spec files to test custom folder", func(t *testing.T) {
		fakeProj := t.TempDir()
		sourceDir := filepath.Join(fakeProj, paths.ProcessInternalObjectSpecsDir)
		require.NoError(t, fileutil.EnsureDir(sourceDir))

		f1 := filepath.Join(sourceDir, "spec1.yaml")
		require.NoError(t, fileutil.WriteFile(f1, []byte("name: spec1"), 0o644))
		f2 := filepath.Join(sourceDir, "spec2.yml")
		require.NoError(t, fileutil.WriteFile(f2, []byte("name: spec2"), 0o644))
		ignoreFile := filepath.Join(sourceDir, "ignore.txt")
		require.NoError(t, fileutil.WriteFile(ignoreFile, []byte("ignore"), 0o644))

		subDir := filepath.Join(sourceDir, "nested")
		require.NoError(t, fileutil.EnsureDir(subDir))

		fakeTestRoot := t.TempDir()
		err := copySpecFilesToTest(fakeTestRoot, fakeProj)
		require.NoError(t, err)

		targetDir := filepath.Join(fakeTestRoot, paths.ProcessInternalObjectSpecsDir)
		copiedF1 := filepath.Join(targetDir, "spec1.yaml")
		data1, err := fileutil.ReadFile(copiedF1)
		require.NoError(t, err)
		assert.Equal(t, "name: spec1", string(data1))

		copiedF2 := filepath.Join(targetDir, "spec2.yml")
		data2, err := fileutil.ReadFile(copiedF2)
		require.NoError(t, err)
		assert.Equal(t, "name: spec2", string(data2))

		errNonexistent := copySpecFilesToTest(fakeTestRoot, "/nonexistent/path/root")
		assert.Error(t, errNonexistent)

		badTestRoot := t.TempDir()
		require.NoError(t, fileutil.EnsureDir(filepath.Join(badTestRoot, paths.ProcessInternalDir)))
		require.NoError(t, fileutil.WriteFile(filepath.Join(badTestRoot, paths.ProcessInternalObjectSpecsDir), []byte("blocker"), 0o644))
		errTargetBlocked := copySpecFilesToTest(badTestRoot, fakeProj)
		assert.Error(t, errTargetBlocked)

		// Source specs path is a file instead of directory -> ReadDir error
		badSourceProj := t.TempDir()
		require.NoError(t, fileutil.EnsureDir(filepath.Join(badSourceProj, paths.ProcessInternalDir)))
		require.NoError(t, fileutil.WriteFile(filepath.Join(badSourceProj, paths.ProcessInternalObjectSpecsDir), []byte("file"), 0o644))
		errSourceRead := copySpecFilesToTest(fakeTestRoot, badSourceProj)
		assert.Error(t, errSourceRead)
	})

	t.Run("yaml extension checker", func(t *testing.T) {
		assert.True(t, hasYAMLExtension("file.yaml"))
		assert.True(t, hasYAMLExtension("file.yml"))
		assert.False(t, hasYAMLExtension("file.json"))
		assert.False(t, hasYAMLExtension("file.go"))
	})

	t.Run("copy specs to test root", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := CopySpecsToTestRoot(tmpDir)
		require.NoError(t, err)
	})

	t.Run("copy kind synonyms missing source", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := CopyKindSynonymsToTestRoot(tmpDir)
		assert.Error(t, err)
	})
}

func TestBootstrapAndIntegrationHelpers(t *testing.T) {
	projRoot := findProjectRoot()
	require.NotEmpty(t, projRoot)

	t.Run("bootstrap test root", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := BootstrapTestRoot(tmpDir, projRoot)
		require.NoError(t, err)
	})

	t.Run("must bootstrap scenario root for cli", func(t *testing.T) {
		tmpDir := t.TempDir()
		MustBootstrapScenarioRootForCLI(t, tmpDir)
		targetConfigFile := filepath.Join(tmpDir, paths.ProcessInternalConfigsDir, paths.IdPrefixesConfigFile)
		fi, err := fileutil.Stat(targetConfigFile)
		require.NoError(t, err)
		assert.False(t, fi.IsDir())
	})

	t.Run("module root from go env", func(t *testing.T) {
		mod := ModuleRootFromGoEnv(t)
		assert.NotEmpty(t, mod)
	})

	t.Run("capture stdout", func(t *testing.T) {
		output := CaptureStdout(t, func() {
			fmt.Print("cycle226 stdout verification")
		})
		assert.Equal(t, "cycle226 stdout verification", output)
	})

	t.Run("get project root for integration", func(t *testing.T) {
		root := GetProjectRootForIntegration(t)
		assert.NotEmpty(t, root)
	})

	t.Run("setup integration test with specs", func(t *testing.T) {
		testRoot, integrationProjRoot := SetupIntegrationTestWithSpecs(t, "scenario-cycle226")
		assert.NotEmpty(t, testRoot)
		assert.NotEmpty(t, integrationProjRoot)
		assert.Equal(t, testRoot, GetTestConfig().TestProjectRoot)
	})
}

func TestTestingConcurrencySafety(t *testing.T) {
	cw := NewCallbackWaiter()
	ac := NewAtomicCounter()
	ec := NewEventCollector()
	wst := NewWorkerStateTracker()

	var wg sync.WaitGroup
	numWorkers := 10

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		workerIdx := i
		workerID := fmt.Sprintf("worker-%d", workerIdx)
		goroutinelabels.NewGoroutine("testing", "concurrent worker").StartSimple(func() {
			defer wg.Done()
			cw.Invoke()
			ac.Increment()
			ec.Add(fmt.Sprintf("event-%d", workerIdx))
			wst.SetState(workerID, true)
			assert.True(t, wst.IsRunning(workerID))
			wst.SetState(workerID, false)
			assert.False(t, wst.IsRunning(workerID))
		})
	}

	wg.Wait()

	assert.Equal(t, int64(numWorkers), cw.Count())
	assert.Equal(t, int64(numWorkers), ac.Get())
	assert.Len(t, ec.GetEvents(), numWorkers)
}
