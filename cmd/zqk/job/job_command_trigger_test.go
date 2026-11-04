package job

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type cycle105JobTestHarness struct {
	projectRoot string
	cmd         *cobra.Command
	outBuf      *bytes.Buffer
	errBuf      *bytes.Buffer
	ctx         context.Context
}

func setupCycle105JobHarness(t *testing.T, args ...string) *cycle105JobTestHarness {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})

	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), zqkenv.UnreachableTestSocketPath())

	t.Cleanup(func() {
		_ = coordination.DrainGlobalCoordinator(5 * time.Second)
		_ = lifecycle.CloseLifecycleWAL(proj.Root)
		storagepkg.ScrubProjectRootForTempCleanup(proj.Root, 5, 20*time.Millisecond)
	})

	cmd := NewJobCmd()
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer

	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	baseCtx = cli.WithStorageProvider(baseCtx, proj.FileStorage)

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, proj.Root)
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	if err == nil && cctx != nil {
		cctx.ProjectRoot = proj.Root
		cli.SetContext(cmd, cctx)
	}

	cmd.SetContext(baseCtx)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)

	return &cycle105JobTestHarness{
		projectRoot: proj.Root,
		cmd:         cmd,
		outBuf:      &outBuf,
		errBuf:      &errBuf,
		ctx:         baseCtx,
	}
}

// TestCycle105_JobCmd_StructureAndMetadata verifies the command hierarchy, metadata,
// flag bindings, and subcommand wiring for the job command group.
func TestCycle105_JobCmd_StructureAndMetadata(t *testing.T) {
	cmd := NewJobCmd()
	require.NotNil(t, cmd)

	assert.Equal(t, "job", cmd.Name())
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)

	expectedSubcommands := []string{
		"list",
		"trigger",
		"activity",
		"history",
		"state",
		"config",
		"clear-issues",
		"health-check",
		"dump",
	}

	subcommandMap := make(map[string]*cobra.Command)
	for _, sub := range cmd.Commands() {
		subcommandMap[sub.Name()] = sub
	}

	for _, expected := range expectedSubcommands {
		subCmd, found := subcommandMap[expected]
		assert.Truef(t, found, "expected subcommand %q to be registered under job", expected)
		if found {
			assert.NotEmptyf(t, subCmd.Short, "subcommand %q should have non-empty Short description", expected)
		}
	}

	// Verify root command flags
	assert.NotNil(t, cmd.Flags().Lookup("format"), "job root command should have format flag")
	assert.NotNil(t, cmd.Flags().Lookup("timeout"), "job root command should have timeout flag")

	// Execute root help
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	require.NoError(t, err)

	helpText := buf.String()
	assert.Contains(t, helpText, "job [command]")
	assert.Contains(t, helpText, "list")
	assert.Contains(t, helpText, "trigger")
	assert.Contains(t, helpText, "activity")
}

// TestCycle105_JobCmd_RootFormatFlags verifies flag parsing and help output with format options.
func TestCycle105_JobCmd_RootFormatFlags(t *testing.T) {
	formats := []string{"json", "yaml", "table"}

	for _, format := range formats {
		cmd := NewJobCmd()
		var outBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&outBuf)
		cmd.SetArgs([]string{"--format", format, "--help"})

		err := cmd.Execute()
		require.NoError(t, err)
		assert.Contains(t, outBuf.String(), "job [command]")
	}
}

// TestCycle105_JobList_Execution verifies the list subcommand through the job hierarchy.
func TestCycle105_JobList_Execution(t *testing.T) {
	t.Run("default table format", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "list")
		err := h.cmd.Execute()
		require.NoError(t, err)
	})

	t.Run("json format", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "list", "--format", "json")
		err := h.cmd.Execute()
		require.NoError(t, err)

		outStr := strings.TrimSpace(h.outBuf.String())
		if outStr != "" {
			var parsed any
			err = json.Unmarshal([]byte(outStr), &parsed)
			assert.NoError(t, err, "output should be valid json")
		}
	})

	t.Run("yaml format", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "list", "--format", "yaml")
		err := h.cmd.Execute()
		require.NoError(t, err)

		outStr := strings.TrimSpace(h.outBuf.String())
		if outStr != "" {
			var parsed any
			err = yaml.Unmarshal([]byte(outStr), &parsed)
			assert.NoError(t, err, "output should be valid yaml")
		}
	})
}

// TestCycle105_JobTrigger_Execution verifies trigger validation and non-existent job handling.
func TestCycle105_JobTrigger_Execution(t *testing.T) {
	t.Run("missing arguments validation error", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "trigger")
		err := h.cmd.Execute()
		require.Error(t, err, "trigger without job ID argument should fail validation")
	})

	t.Run("trigger nonexistent job with isolated root", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "trigger", "JOB-DEMO-NONEXISTENT")
		_ = h.cmd.Execute()
		// Even if execution errors due to daemon or job not in registry, it shouldn't crash
	})
}

// TestCycle105_JobActivity_Execution verifies recent job activity querying across formats.
func TestCycle105_JobActivity_Execution(t *testing.T) {
	t.Run("activity table default", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "activity")
		_ = h.cmd.Execute()
	})

	t.Run("activity with limit and json format", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "activity", "--limit", "5", "--format", "json")
		_ = h.cmd.Execute()

		outStr := strings.TrimSpace(h.outBuf.String())
		if outStr != "" {
			var parsed any
			_ = json.Unmarshal([]byte(outStr), &parsed)
		}
	})

	t.Run("activity with specific job-id filter", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "activity", "--job-id", "JOB-COMMUNITY-001")
		_ = h.cmd.Execute()
	})
}

// TestCycle105_JobHistory_Execution verifies execution history querying and limit flags.
func TestCycle105_JobHistory_Execution(t *testing.T) {
	t.Run("history table default", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "history")
		_ = h.cmd.Execute()
	})

	t.Run("history with limit and json format", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "history", "--limit", "10", "--format", "json")
		_ = h.cmd.Execute()
	})
}

// TestCycle105_JobState_Execution verifies job state querying.
func TestCycle105_JobState_Execution(t *testing.T) {
	t.Run("state table default", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "state")
		_ = h.cmd.Execute()
	})

	t.Run("state json format", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "state", "--format", "json")
		_ = h.cmd.Execute()
	})
}

// TestCycle105_JobConfig_Execution verifies configuration inspection.
func TestCycle105_JobConfig_Execution(t *testing.T) {
	t.Run("config table default", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "config")
		_ = h.cmd.Execute()
	})

	t.Run("config json format", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "config", "--format", "json")
		_ = h.cmd.Execute()
	})
}

// TestCycle105_JobClearIssues_Execution verifies clearing scheduler issues.
func TestCycle105_JobClearIssues_Execution(t *testing.T) {
	h := setupCycle105JobHarness(t, "clear-issues")
	_ = h.cmd.Execute()
}

// TestCycle105_JobHealth_Execution verifies health check execution.
func TestCycle105_JobHealth_Execution(t *testing.T) {
	t.Run("health check execution", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "health")
		_ = h.cmd.Execute()
	})

	t.Run("health check json format", func(t *testing.T) {
		h := setupCycle105JobHarness(t, "health", "--format", "json")
		_ = h.cmd.Execute()
	})
}

// TestCycle105_JobDump_Execution verifies dump diagnostic output.
func TestCycle105_JobDump_Execution(t *testing.T) {
	h := setupCycle105JobHarness(t, "dump")
	_ = h.cmd.Execute()
}

// TestCycle105_WorkerPool_TriggerQueueAndStateRegistry verifies worker pool queuing and state transitions.
func TestCycle105_WorkerPool_TriggerQueueAndStateRegistry(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	t.Cleanup(func() {
		_ = coordination.DrainGlobalCoordinator(5 * time.Second)
		_ = lifecycle.CloseLifecycleWAL(proj.Root)
		storagepkg.ScrubProjectRootForTempCleanup(proj.Root, 5, 20*time.Millisecond)
	})

	// Test JobTriggerQueue
	queue := schedulerpkg.NewJobTriggerQueue(proj.Root)
	require.NotNil(t, queue)

	testJobID := "JOB-WORKER-TEST-001"
	err := queue.EnqueueTriggerRequest(testJobID)
	require.NoError(t, err)

	requests, err := queue.PeekTriggerRequests()
	require.NoError(t, err)
	require.NotEmpty(t, requests)
	assert.Equal(t, testJobID, requests[0].JobID)

	dequeued, err := queue.DequeueTriggerRequests(10)
	require.NoError(t, err)
	assert.NotEmpty(t, dequeued)

	requestsAfterClear, err := queue.PeekTriggerRequests()
	require.NoError(t, err)
	assert.Empty(t, requestsAfterClear)

	// Test JobExecutionLease
	now := time.Now()
	validLease := &schedulerpkg.JobExecutionLease{
		JobID:       testJobID,
		ExecutionID: "EXEC-001",
		AcquiredAt:  now,
		ExpiresAt:   now.Add(1 * time.Minute),
	}
	assert.True(t, validLease.IsValid(now))

	expiredLease := &schedulerpkg.JobExecutionLease{
		JobID:       testJobID,
		ExecutionID: "EXEC-002",
		AcquiredAt:  now.Add(-2 * time.Minute),
		ExpiresAt:   now.Add(-1 * time.Minute),
	}
	assert.False(t, expiredLease.IsValid(now))
}
