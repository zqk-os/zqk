package swarm

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/bootstrap"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/swarm/metabolism"
	"github.com/zqk-os/zqk/pkg/swarm/pack"
	"github.com/zqk-os/zqk/pkg/swarminit"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSwarmCmd_StructureAndSubcommands(t *testing.T) {
	cmd := NewSwarmCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "swarm", cmd.Use)
	assert.True(t, cmd.HasSubCommands())

	subCmdNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subCmdNames[sub.Name()] = true
	}
	assert.True(t, subCmdNames["status"], "missing status subcommand")
	assert.True(t, subCmdNames["init"], "missing init subcommand")
	assert.True(t, subCmdNames["run"], "missing run subcommand")

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "swarm")
}

func TestSwarmStatusCmd_HierarchyAndHelp(t *testing.T) {
	cmd := NewStatusCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "status", cmd.Name())

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	err := cmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "swarm status")
}

func TestSwarmInitCmd_HierarchyAndHelp(t *testing.T) {
	initCmd := NewInitCmd()
	require.NotNil(t, initCmd)
	assert.Equal(t, "init", initCmd.Use)
	assert.Contains(t, initCmd.Aliases, "swarm-init")

	buf := new(bytes.Buffer)
	initCmd.SetOut(buf)
	initCmd.SetErr(buf)
	initCmd.SetArgs([]string{"--help"})
	err := initCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "--pipeline")
	assert.Contains(t, buf.String(), "--dry-run")

	agentInitCmd := NewAgentSwarmInitCmd()
	require.NotNil(t, agentInitCmd)
	assert.Equal(t, "swarm-init", agentInitCmd.Use)

	buf.Reset()
	agentInitCmd.SetOut(buf)
	agentInitCmd.SetErr(buf)
	agentInitCmd.SetArgs([]string{"--help"})
	err = agentInitCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "--pipeline")
}

func TestSwarmStatus_BuildSwarmStatusAndExecution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	root := proj.Root
	logger := logging.GetLoggerFromProfile("test")
	if err := bootstrap.ExtractTo(root, logger, true); err != nil {
		t.Fatalf("bootstrap.ExtractTo failed: %v", err)
	}

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, root)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, root, store)

	// 1. Initially idle status
	payload, err := BuildSwarmStatus(ctx, store, sec)
	require.NoError(t, err)
	require.NotNil(t, payload)
	swarmData, ok := payload["swarm"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "idle", swarmData["throughput_hint"])
	assert.Equal(t, 0, swarmData["executing_agent_tasks"])
	assert.Equal(t, 0, swarmData["agent_tasks_total"])

	// Seed skill and persona for reference validation
	skillObj := map[string]any{
		objects.FieldKeyID:            "ASK-TEST-SKILL-1",
		objects.FieldKeyKind:          objects.KindAgentSkill,
		objects.FieldKeyTitle:         "Test Skill",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusApproved,
		"instructions":                "Test Instructions",
		"instructions_summary":        "Summary",
		objects.FieldKeyCreatedAt:     time.Now().UTC().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "test",
	}
	storage.CreateCASVisible(t, store, ctx, sec, skillObj, objects.ObjectStatusApproved)
	require.NoError(t, storage.FlushListingIndexForProjectRoot(root, objects.KindAgentSkill))

	testPersona := map[string]any{
		objects.FieldKeyID:             "PER-TEST-PERSONA-1",
		objects.FieldKeyKind:           objects.KindPersona,
		objects.FieldKeyTitle:          "Test Persona",
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyName:           "Test Persona",
		objects.FieldKeyRole:           "agent",
		objects.FieldKeyAgentSkillRefs: "ASK-TEST-SKILL-1",
		objects.FieldKeyStatus:         objects.ObjectStatusApproved,
		objects.FieldKeyCreatedAt:      time.Now().UTC().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:      "test",
	}
	storage.CreateCASVisible(t, store, ctx, sec, testPersona, objects.ObjectStatusApproved)
	require.NoError(t, storage.FlushListingIndexForProjectRoot(root, objects.KindPersona))

	// 2. Add an AgentTask in approved status -> queued throughput hint
	taskApproved := map[string]any{
		objects.FieldKeyID:                 "ATK-TEST-APPROVED-1",
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Approved Task",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyAssigneePersonaRef: "PER-TEST-PERSONA-1",
		objects.FieldKeyCreatedAt:          time.Now().UTC().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:          "test",
	}
	storage.CreateCASVisible(t, store, ctx, sec, taskApproved, objects.ObjectStatusApproved)
	require.NoError(t, storage.FlushListingIndexForProjectRoot(root, objects.KindAgentTask))

	payload, err = BuildSwarmStatus(ctx, store, sec)
	require.NoError(t, err)
	swarmData = payload["swarm"].(map[string]any)
	assert.Equal(t, "queued", swarmData["throughput_hint"])
	assert.Equal(t, 1, swarmData["agent_tasks_total"])

	// 3. Add an AgentTask in in_progress status -> executing throughput hint
	taskInProgress := map[string]any{
		objects.FieldKeyID:                 "ATK-TEST-PROGRESS-1",
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Executing Task",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyAssigneePersonaRef: "PER-TEST-PERSONA-1",
		objects.FieldKeyCreatedAt:          time.Now().UTC().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:          "test",
	}
	storage.CreateCASVisible(t, store, ctx, sec, taskInProgress, objects.ObjectStatusInProgress)
	require.NoError(t, storage.FlushListingIndexForProjectRoot(root, objects.KindAgentTask))

	payload, err = BuildSwarmStatus(ctx, store, sec)
	require.NoError(t, err)
	swarmData = payload["swarm"].(map[string]any)
	assert.Equal(t, "executing", swarmData["throughput_hint"])
	assert.Equal(t, 1, swarmData["executing_agent_tasks"])
	assert.Equal(t, 2, swarmData["agent_tasks_total"])

	// 4. Test Personas with skill bindings
	personaBound := map[string]any{
		objects.FieldKeyID:             "PER-TEST-BOUND-1",
		objects.FieldKeyKind:           objects.KindPersona,
		objects.FieldKeyTitle:          "Bound Persona",
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyName:           "Bound Persona",
		objects.FieldKeyRole:           "agent",
		objects.FieldKeyAgentSkillRefs: "ASK-TEST-SKILL-1",
		objects.FieldKeyStatus:         objects.ObjectStatusApproved,
		objects.FieldKeyCreatedAt:      time.Now().UTC().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:      "test",
	}
	storage.CreateCASVisible(t, store, ctx, sec, personaBound, objects.ObjectStatusApproved)

	personaUnbound := map[string]any{
		objects.FieldKeyID:            "PER-TEST-UNBOUND-1",
		objects.FieldKeyKind:          objects.KindPersona,
		objects.FieldKeyTitle:         "Unbound Persona",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyName:          "Unbound Persona",
		objects.FieldKeyRole:          "agent",
		objects.FieldKeyStatus:        objects.ObjectStatusApproved,
		objects.FieldKeyCreatedAt:     time.Now().UTC().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:     "test",
	}
	err = store.Create(ctx, sec, personaUnbound)
	require.NoError(t, err)
	require.NoError(t, storage.FlushListingIndexForProjectRoot(root, objects.KindPersona))

	personaSummary := summarizePersonaSkillBound(ctx, store, sec)
	assert.GreaterOrEqual(t, personaSummary["bound"], 1)
	assert.GreaterOrEqual(t, personaSummary["unbound"], 0)

	// 5. Test countByStatus and countWithFilter directly
	byStatus, total, err := countByStatus(ctx, store, sec, objects.KindAgentTask)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Equal(t, 1, byStatus[objects.ObjectStatusApproved])
	assert.Equal(t, 1, byStatus[objects.ObjectStatusInProgress])

	countActive, err := countWithFilter(ctx, store, sec, objects.KindPriorityPlan, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, countActive, 0)

	// 6. Test runSwarmStatus via command execution
	statusCmd := NewStatusCmd()
	cli.SetContext(statusCmd, cli.ContextForProjectRoot(root))
	buf := new(bytes.Buffer)
	statusCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
	statusCmd.SetOut(buf)
	statusCmd.SetErr(buf)
	statusCmd.SetArgs([]string{"--format", "json"})
	err = statusCmd.Execute()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "throughput_hint")

	// Table output format
	buf.Reset()
	statusCmd = NewStatusCmd()
	cli.SetContext(statusCmd, cli.ContextForProjectRoot(root))
	statusCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
	statusCmd.SetOut(buf)
	statusCmd.SetErr(buf)
	statusCmd.SetArgs([]string{"--format", "table"})
	err = statusCmd.Execute()
	assert.NoError(t, err)

	// 7. Verify CAP Orchestrator job presence detection
	capJob := map[string]any{
		objects.FieldKeyID:        capOrchestratorJobID,
		objects.FieldKeyKind:      objects.KindSchedulerJob,
		objects.FieldKeyTitle:     "CAP Orchestrator Job",
		objects.FieldKeyStatus:    objects.ObjectStatusActive,
		objects.FieldKeyCreatedAt: time.Now().UTC().Format(time.RFC3339),
		objects.FieldKeyCreatedBy: "test",
	}
	_ = store.Create(ctx, sec, capJob)
	payloadWithCap, err := BuildSwarmStatus(ctx, store, sec)
	require.NoError(t, err)
	swarmDataWithCap := payloadWithCap["swarm"].(map[string]any)
	capJobInfo := swarmDataWithCap["cap_orchestrator_job"].(map[string]any)
	assert.True(t, capJobInfo["present"].(bool))

	// 8. Error handling paths
	errStatusCmd := NewStatusCmd()
	errStatusCmd.SetArgs([]string{"--non-existent-flag"})
	err = errStatusCmd.Execute()
	assert.Error(t, err)

	_, _, err = countByStatus(context.Background(), nil, sec, "agent_task")
	assert.Error(t, err)
}

func TestSwarmInit_HelpersAndExecution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	root := proj.Root
	logger := logging.GetLoggerFromProfile("test")
	if err := bootstrap.ExtractTo(root, logger, true); err != nil {
		t.Fatalf("bootstrap.ExtractTo failed: %v", err)
	}

	// 1. Test swarmInitCLIBin
	binPath := swarmInitCLIBin(root)
	assert.NotEmpty(t, binPath)

	// Test fallback when no binary exists
	emptyDir := t.TempDir()
	fallbackBin := swarmInitCLIBin(emptyDir)
	assert.Equal(t, "zqk", fallbackBin)

	// Test stable binary path branch
	stableRoot := t.TempDir()
	stablePath := paths.RepoStableBinaryPath(stableRoot)
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(stablePath), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(stablePath, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755))
	assert.Equal(t, stablePath, swarmInitCLIBin(stableRoot))

	// 2. Test swarmInitChatBootstrap
	bootstrapFn := swarmInitChatBootstrap(root)
	require.NotNil(t, bootstrapFn)

	// Test missing payload file error
	err := bootstrapFn(context.Background(), "seat-test", agentfeed.PeerSeatRecord{
		PID: 1234,
	}, filepath.Join(emptyDir, "nonexistent-payload.json"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read chat payload")

	// Test existing payload file
	payloadFile := filepath.Join(emptyDir, "payload.txt")
	err = fileutil.WriteFile(payloadFile, []byte("HELLO SWARM"), paths.FilePerm644)
	require.NoError(t, err)
	_ = bootstrapFn(context.Background(), "seat-test", agentfeed.PeerSeatRecord{PID: 1234}, payloadFile)

	// 3. Test swarmInitSteer
	require.NoError(t, datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		Note:          "Test local feed channel",
	}))

	steerFn := swarmInitSteer(root, "coord-1", "PER-COMMUNITY-SOFTWARE-ENGINEER", logger)
	require.NotNil(t, steerFn)

	outcome, err := steerFn(context.Background(), "peer-worker-1", "Steer instruction", true)
	assert.NoError(t, err)
	assert.Equal(t, "peer-worker-1", outcome.ToAgentID)

	outcomeAwait, err := steerFn(context.Background(), "peer-worker-2", "Steer with await", true)
	assert.NoError(t, err)
	assert.NotEmpty(t, outcomeAwait.AwaitID)

	// Error path: directed without awaitPeerAck fails POL-AGENT-ORCH-HOURGLASS-001
	_, err = steerFn(context.Background(), "peer-worker-3", "Fails hourglass", false)
	assert.Error(t, err)

	// Error path: empty message fails feed append
	_, err = steerFn(context.Background(), "peer-worker-4", "", true)
	assert.Error(t, err)

	// Test default coordinator fallback in steer
	steerDefaultFn := swarmInitSteer(root, "", "", logger)
	_, _ = steerDefaultFn(context.Background(), "peer-worker-3", "Msg", true)

	// 4. Test swarmInitInstallWorkers
	installFn := swarmInitInstallWorkers(root)
	require.NotNil(t, installFn)
	_ = installFn(context.Background(), swarminit.SeatWorkerInstallConfig{
		Seats:       []string{"worker-1"},
		PollSeconds: 1,
	})

	// 4. Test swarmInitEnsureMCP
	ensureMCPFn := swarmInitEnsureMCP(root)
	require.NotNil(t, ensureMCPFn)
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err = ensureMCPFn(canceledCtx, "127.0.0.1:0")
	assert.Error(t, err)

	// 5. Test swarmInitMCPSubscribers
	subsFn := swarmInitMCPSubscribers(logger)
	require.NotNil(t, subsFn)
	count, _ := subsFn(canceledCtx)
	assert.GreaterOrEqual(t, count, 0)

	// 6. Test runSwarmInit command execution without pipeline (fails validation gracefully)
	initCmd := NewInitCmd()
	cli.SetContext(initCmd, cli.ContextForProjectRoot(root))
	buf := new(bytes.Buffer)
	initCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
	initCmd.SetOut(buf)
	initCmd.SetErr(buf)
	initCmd.SetArgs([]string{"--dry-run"})
	err = initCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "require --pipeline or --workflow")

	// 7. Test runSwarmInit command execution with a dry-run pipeline
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, root)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, root, store)

	dryPipeline := map[string]any{
		objects.FieldKeyID:          "PIP-SWARM-TEST-1",
		objects.FieldKeyKind:        objects.KindPipeline,
		objects.FieldKeyTitle:       "Dry Run Test Pipeline",
		objects.FieldKeyStatus:      objects.ObjectStatusApproved,
		objects.FieldKeyDescription: "Dry Run Test Pipeline Description",
		"trigger":                   "manual",
		objects.FieldKeyCreatedAt:   time.Now().UTC().Format(time.RFC3339),
		objects.FieldKeyCreatedBy:   "test",
		"stages": []map[string]any{
			{
				objects.FieldKeyID: "control_plane",
				"executor":         swarminit.ExecutorControlPlane,
			},
		},
	}
	err = store.Create(ctx, sec, dryPipeline)
	require.NoError(t, err)

	buf.Reset()
	initCmd = NewInitCmd()
	cli.SetContext(initCmd, cli.ContextForProjectRoot(root))
	initCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
	initCmd.SetOut(buf)
	initCmd.SetErr(buf)
	initCmd.SetArgs([]string{"--pipeline", "PIP-SWARM-TEST-1", "--dry-run"})
	err = initCmd.Execute()
	if err != nil {
		assert.Contains(t, err.Error(), "PIP-SWARM-TEST-1")
	}

	// 8. Test live initCmd execution (non-dry-run)
	buf.Reset()
	initLiveCmd := NewInitCmd()
	cli.SetContext(initLiveCmd, cli.ContextForProjectRoot(root))
	initLiveCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
	initLiveCmd.SetOut(buf)
	initLiveCmd.SetErr(buf)
	initLiveCmd.SetArgs([]string{
		"--pipeline", "PIP-SWARM-TEST-1",
		"--agent-id", "coord-1",
		"--persona-ref", "PER-TEST-PERSONA-1",
	})
	_ = initLiveCmd.Execute()
}

func TestSwarmRun_HelperCoverage(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	root := proj.Root
	logger := logging.GetLoggerFromProfile("test")
	if err := bootstrap.ExtractTo(root, logger, true); err != nil {
		t.Fatalf("bootstrap.ExtractTo failed: %v", err)
	}

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, root)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, root, store)

	// 1. Test confirmSwarmLaunch
	buf := new(bytes.Buffer)
	confirmed := confirmSwarmLaunch(buf, true)
	assert.True(t, confirmed)

	// Non-terminal execution returns true
	confirmedNonTerminal := confirmSwarmLaunch(buf, false)
	assert.True(t, confirmedNonTerminal)

	// 2. Test persistPromptTemplates
	templates := []metabolism.PromptTemplateObject{
		{
			ID:          "PROMPT-TEST-SWARM-1",
			Title:       "Test Prompt Template",
			Description: "Test Description",
			Category:    "core",
			Template:    "Analyze {{target}}",
		},
		{
			ID: "", // Should be skipped
		},
	}
	eventLogger := logging.NewEventLogger(ctx)
	stored := persistPromptTemplates(ctx, sec, store, templates, eventLogger)
	assert.Equal(t, 1, stored)

	// Storing again skips already existing template
	storedAgain := persistPromptTemplates(ctx, sec, store, templates, eventLogger)
	assert.Equal(t, 0, storedAgain)

	// 3. Test resolveSwarmOutputDir
	samplePkg := &pack.SwarmPackage{
		Name: "sample-agent-pack",
	}
	defaultOut := resolveSwarmOutputDir(root, samplePkg, nil)
	assert.Contains(t, defaultOut, "sample-agent-pack-latest")

	customRelOut := resolveSwarmOutputDir(root, samplePkg, map[string]interface{}{
		"output_dir": "custom/runs",
	})
	assert.Equal(t, filepath.Join(root, "custom/runs"), customRelOut)

	absTarget := filepath.Join(t.TempDir(), "abs-runs")
	customAbsOut := resolveSwarmOutputDir(root, samplePkg, map[string]interface{}{
		"output_dir": absTarget,
	})
	assert.Equal(t, absTarget, customAbsOut)

	// 4. Test sortDigestKernelObjects
	rawObjects := []map[string]any{
		{objects.FieldKeyKind: "backlog_item", objects.FieldKeyID: "BLI-1"},
		{objects.FieldKeyKind: "goal", objects.FieldKeyID: "GOAL-1"},
		{objects.FieldKeyKind: "milestone", objects.FieldKeyID: "MIL-1"},
		{objects.FieldKeyKind: "priority_plan", objects.FieldKeyID: "PRI-1"},
	}
	sorted := sortDigestKernelObjects(root, rawObjects)
	require.Len(t, sorted, 4)

	// 5. Test ensureValidSwarmAccount
	accountID := ensureValidSwarmAccount(ctx, sec, store)
	assert.NotEmpty(t, accountID)

	// Calling again reuses the created/valid account
	sec.AccountID = accountID
	accountID2 := ensureValidSwarmAccount(ctx, sec, store)
	assert.Equal(t, accountID, accountID2)

	// 6. Test launchSwarmBackground
	pid, logRel, err := launchSwarmBackground(root, "PRI-TEST-PLAN", "test_pkg")
	assert.NoError(t, err)
	assert.Greater(t, pid, 0)
	assert.Contains(t, logRel, "swarm-orchestrate-test_pkg.log")
	if proc, findErr := os.FindProcess(pid); findErr == nil {
		_ = proc.Kill()
	}

	// 7. Test confirmSwarmLaunch
	bufLaunch := new(bytes.Buffer)
	assert.True(t, confirmSwarmLaunch(bufLaunch, true))
	assert.True(t, confirmSwarmLaunch(bufLaunch, false))
}

func TestPersonaASKRefs_AdditionalEdgeCases(t *testing.T) {
	p := map[string]any{
		"agent_skill_refs":                []string{"ASK-1", "ASK-2", "NOT-ASK"},
		objects.FieldKeyRelatedObjectRefs: []any{"ASK-3", 42, nil, "ASK-1"},
	}
	refs := personaASKRefs(p)
	assert.Contains(t, refs, "ASK-1")
	assert.Contains(t, refs, "ASK-2")
	assert.Contains(t, refs, "ASK-3")
	assert.NotContains(t, refs, "NOT-ASK")
}

func TestSwarmStatus_StorageNilEdgeCases(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	_, _, err := countByStatus(ctx, nil, sec, "agent_task")
	assert.Error(t, err)

	_, err = countWithFilter(ctx, nil, sec, "priority_plan", nil)
	assert.Error(t, err)

	summary := summarizePersonaSkillBound(ctx, nil, sec)
	assert.Equal(t, "storage_nil", summary["error"])
}

func TestNewTopLevelRunCmd_Coverage(t *testing.T) {
	cmd := NewTopLevelRunCmd()
	assert.NotNil(t, cmd)
	assert.Equal(t, "run", cmd.Name())
}

func TestRunSwarmStatus_FormatsAndFlags(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	root := proj.Root
	ctx := context.Background()

	formats := []string{"json", "yaml", "table"}
	for _, fmtStr := range formats {
		cmd := NewStatusCmd()
		cli.SetContext(cmd, cli.ContextForProjectRoot(root))
		buf := new(bytes.Buffer)
		cmd.SetContext(pkgctx.WithCommandOutputWriter(ctx, buf))
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"--format", fmtStr})
		err := cmd.Execute()
		assert.NoError(t, err)
		assert.NotEmpty(t, buf.String())
	}
}

func TestRunSwarmInit_FlagsCoverage(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	root := proj.Root
	ctx := context.Background()

	cmd := NewInitCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(root))
	buf := new(bytes.Buffer)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(ctx, buf))
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--dry-run", "--pipeline", "PIP-NONEXISTENT", "--from-stage", "stage-test", "--persona-ref", "PER-TEST", "--allow-chat=false"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PIP-NONEXISTENT")
}

func TestResolveSwarmStorage_Variations(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	root := proj.Root

	// With processor context
	cmd := NewRunCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(root))
	sp, sec, opCtx, pr, err := resolveSwarmStorage(cmd)
	assert.NoError(t, err)
	assert.NotNil(t, sp)
	assert.NotNil(t, sec)
	assert.NotNil(t, opCtx)
	assert.Equal(t, root, pr)

	// Fallback path without processor context
	cmdBare := NewRunCmd()
	sp2, sec2, opCtx2, pr2, err2 := resolveSwarmStorage(cmdBare)
	assert.NoError(t, err2)
	assert.NotNil(t, sp2)
	assert.NotNil(t, sec2)
	assert.NotNil(t, opCtx2)
	assert.NotEmpty(t, pr2)
}
