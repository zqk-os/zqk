package system

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func executeCycle120Command(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func setupCycle120ProjectEnv(t *testing.T) string {
	t.Helper()
	tempRoot := t.TempDir()
	dotZqk := filepath.Join(tempRoot, paths.ProjectDataDir)
	err := fileutil.MkdirAll(dotZqk, paths.DirPerm755)
	require.NoError(t, err)

	processDir := filepath.Join(dotZqk, "process")
	err = fileutil.MkdirAll(processDir, paths.DirPerm755)
	require.NoError(t, err)

	t.Setenv(zqkenv.ProjectRoot().Name(), tempRoot)
	t.Setenv(zqkenv.TestRoot().Name(), tempRoot)
	zqkenv.ProjectRoot().Set(tempRoot)
	zqkenv.TestRoot().Set(tempRoot)

	return tempRoot
}

func TestCycle120_AlignHelpers_CalculationsAndFormatting(t *testing.T) {
	t.Parallel()

	// GoalRefsFromObject tests
	objWithRefs := map[string]any{
		objects.FieldKeyGoalRefs: []any{"GOAL-001", "GOAL-002"},
	}
	refs := GoalRefsFromObject(objWithRefs)
	assert.Len(t, refs, 2)
	assert.True(t, refs["GOAL-001"])
	assert.True(t, refs["GOAL-002"])

	objWithNil := map[string]any{
		objects.FieldKeyGoalRefs: nil,
	}
	assert.Empty(t, GoalRefsFromObject(objWithNil))

	// RoundTwo tests
	assert.Equal(t, 85.55, RoundTwo(85.546))
	assert.Equal(t, 0.0, RoundTwo(0.0))
	assert.Equal(t, 100.0, RoundTwo(100.0))

	// TruncateGaps tests
	gaps := []map[string]any{
		{"id": "BLI-1"}, {"id": "BLI-2"}, {"id": "BLI-3"},
	}
	assert.Len(t, TruncateGaps(gaps, 2), 2)
	assert.Len(t, TruncateGaps(gaps, 5), 3)

	// OutputAlignDashboard table output
	cmd := NewAlignCmd()
	res := map[string]any{
		"alignment_score": 85.0,
	}
	err := OutputAlignDashboard(cmd, res, 85.0, 10, 8, 2, 3)
	require.NoError(t, err)

	// OutputAlignTable table output
	alignData := map[string]any{
		"alignment": map[string]any{
			"goal_work": map[string]any{
				"total_work_items":    10,
				"items_with_goals":    8,
				"items_without_goals": 2,
				"goal_coverage_pct":   80.0,
			},
			"alignment_score": 80.0,
		},
		"alignment_score": 80.0,
		"goals_count":     2,
	}
	err = OutputAlignTable(cmd, alignData, false, "")
	require.NoError(t, err)
}

func TestCycle120_AlignCommand_FlagsAndHelp(t *testing.T) {
	cmd := NewAlignCmd()
	require.NotNil(t, cmd)

	assert.Equal(t, "align", cmd.Use)
	assert.NotNil(t, cmd.Flags().Lookup("gaps"))
	assert.NotNil(t, cmd.Flags().Lookup(objects.KindGoal))
	assert.NotNil(t, cmd.Flags().Lookup("score"))
	assert.NotNil(t, cmd.Flags().Lookup("dashboard"))

	out, err := executeCycle120Command(t, cmd, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "Validates that work items")
}

func TestCycle120_AlignCommand_ExecutionModes(t *testing.T) {
	tempDir := setupCycle120ProjectEnv(t)
	testkit.RegisterTempProjectTeardown(t, tempDir, nil)

	cmd := NewAlignCmd()
	_, err := executeCycle120Command(t, cmd, "--score")
	assert.NoError(t, err)

	dashCmd := NewAlignCmd()
	_, err = executeCycle120Command(t, dashCmd, "--dashboard")
	assert.NoError(t, err)

	gapsCmd := NewAlignCmd()
	_, err = executeCycle120Command(t, gapsCmd, "--gaps")
	assert.NoError(t, err)
}

func TestCycle120_Dashboard_MetricsAndStatus(t *testing.T) {
	t.Parallel()

	// getPCSStatus boundary tests
	assert.Equal(t, "Excellent", getPCSStatus(85.0))
	assert.Equal(t, "Good", getPCSStatus(65.0))
	assert.Equal(t, "Fair", getPCSStatus(45.0))
	assert.Equal(t, "Poor", getPCSStatus(25.0))
	assert.Equal(t, "Critical", getPCSStatus(10.0))

	// getHistoricalMetric with missing directory
	val, err := getHistoricalMetric("pcs")
	assert.Error(t, err)
	assert.Equal(t, 0.0, val)

	// getHistoricalMetric with temp directory and historical files
	tempDir := t.TempDir()
	metricsDir := filepath.Join(tempDir, "docs", "reports", "metrics")
	err = fileutil.MkdirAll(metricsDir, paths.DirPerm755)
	require.NoError(t, err)

	f1 := filepath.Join(metricsDir, "pcs-2026-01-01.json")
	f2 := filepath.Join(metricsDir, "pcs-2026-01-02.json")
	payload1, err := json.Marshal(map[string]float64{"pcs": 75.0, "edd": 5.0})
	require.NoError(t, err)
	payload2, err := json.Marshal(map[string]float64{"pcs": 80.0, "edd": 4.5})
	require.NoError(t, err)

	err = fileutil.WriteStandardFile(f1, payload1)
	require.NoError(t, err)
	err = fileutil.WriteStandardFile(f2, payload2)
	require.NoError(t, err)

	cmd := NewDashboardCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "dashboard", cmd.Name())
}

func TestCycle120_GitCommand_HierarchyAndValidation(t *testing.T) {
	cmd := NewGitCmd()
	require.NotNil(t, cmd)

	assert.Equal(t, "git", cmd.Use)
	assert.True(t, cmd.HasSubCommands())

	analyzeCmd, _, err := cmd.Find([]string{"analyze"})
	require.NoError(t, err)
	require.NotNil(t, analyzeCmd)
	assert.NotNil(t, analyzeCmd.Flags().Lookup("hash"))
	assert.NotNil(t, analyzeCmd.Flags().Lookup("recent"))
	assert.NotNil(t, analyzeCmd.Flags().Lookup("link"))

	queryCmd, _, err := cmd.Find([]string{"query"})
	require.NoError(t, err)
	require.NotNil(t, queryCmd)

	out, err := executeCycle120Command(t, cmd, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "Analyze Git commits")

	_, err = executeCycle120Command(t, cmd, "query")
	assert.Error(t, err, "query should require exactly 1 argument")
}

func TestCycle120_MetricsHelpers_FilteringAndFormatting(t *testing.T) {
	t.Parallel()

	// formatDuration tests
	assert.Equal(t, "500ms", formatDuration(500*time.Millisecond))
	assert.Equal(t, "5.0s", formatDuration(5*time.Second))
	assert.Equal(t, "2.5m", formatDuration(150*time.Second))

	// truncate tests
	assert.Equal(t, "short", truncate("short", 10))
	assert.Equal(t, "very lo...", truncate("very long string", 10))

	// contains tests
	assert.True(t, contains("system check", "check"))
	assert.True(t, contains("system check", "system"))
	assert.False(t, contains("system check", "status"))

	// filterMetrics tests
	testMetrics := map[string]*clipkg.CommandMetrics{
		"m1": {
			NormalizedCmd:   "system check",
			FailureCount:    1,
			InvocationCount: 10,
			AvgDuration:     2 * time.Second,
		},
		"m2": {
			NormalizedCmd:   "system status",
			TimeoutCount:    2,
			InvocationCount: 5,
			AvgDuration:     12 * time.Second,
		},
		"m3": {
			NormalizedCmd:   "system align",
			InvocationCount: 20,
			AvgDuration:     1 * time.Second,
		},
	}

	filteredFail := filterMetrics(testMetrics, "", "failures")
	assert.Len(t, filteredFail, 1)
	assert.Equal(t, "system check", filteredFail[0].NormalizedCmd)

	filteredTimeout := filterMetrics(testMetrics, "", "timeouts")
	assert.Len(t, filteredTimeout, 1)
	assert.Equal(t, "system status", filteredTimeout[0].NormalizedCmd)

	filteredSlow := filterMetrics(testMetrics, "", "slow")
	assert.Len(t, filteredSlow, 1)
	assert.Equal(t, "system status", filteredSlow[0].NormalizedCmd)

	filteredCmd := filterMetrics(testMetrics, "align", "")
	assert.Len(t, filteredCmd, 1)
	assert.Equal(t, "system align", filteredCmd[0].NormalizedCmd)

	// sortMetrics tests
	sorted := sortMetrics(filteredFail)
	assert.NotEmpty(t, sorted)

	// outputMetricsTable tests
	cmd := NewMetricsCmd()
	err := outputMetricsTable(cmd, nil)
	require.NoError(t, err)

	err = outputMetricsTable(cmd, []*clipkg.CommandMetrics{
		{
			NormalizedCmd:   "system check",
			InvocationCount: 10,
			SuccessCount:    9,
			FailureCount:    1,
			AvgDuration:     500 * time.Millisecond,
		},
	})
	require.NoError(t, err)
}

func TestCycle120_MetricsCommand_HierarchyAndSubcommands(t *testing.T) {
	cmd := NewMetricsCmd()
	require.NotNil(t, cmd)

	assert.Equal(t, "metrics", cmd.Name())
	assert.NotNil(t, cmd.Flags().Lookup("command"))
	assert.NotNil(t, cmd.Flags().Lookup("filter"))
	assert.NotNil(t, cmd.Flags().Lookup("limit"))
	assert.NotNil(t, cmd.Flags().Lookup("summary"))
	assert.NotNil(t, cmd.Flags().Lookup("all-time"))
	assert.NotNil(t, cmd.Flags().Lookup("window"))
	assert.NotNil(t, cmd.Flags().Lookup("day"))

	out, err := executeCycle120Command(t, cmd, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "command execution metrics")
}

func TestCycle120_ResourceHygiene_FlagsAndExecution(t *testing.T) {
	cmd := NewResourceHygieneCmd()
	require.NotNil(t, cmd)

	assert.Equal(t, "resource-hygiene", cmd.Name())
	assert.NotNil(t, cmd.Flags().Lookup("reap-processes"))
	assert.NotNil(t, cmd.Flags().Lookup("reap-scorecards"))
	assert.NotNil(t, cmd.Flags().Lookup("reap-metrics"))
	assert.NotNil(t, cmd.Flags().Lookup("metrics-max-age"))

	out, err := executeCycle120Command(t, cmd, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "resource-hygiene")

	tempDir := setupCycle120ProjectEnv(t)
	runCmd := NewResourceHygieneCmd()
	out, err = executeCycle120Command(t, runCmd, "--dry-run", "--project-root", tempDir)
	require.NoError(t, err)
	assert.Contains(t, out, "Resource Hygiene Report")
}

func TestCycle120_ServiceManager_ConfigAuthAndLifecycle(t *testing.T) {
	sm := NewServiceManager()
	require.NotNil(t, sm)
	assert.NotNil(t, sm.HostManager())

	// getServiceConfig tests
	cfg, err := sm.getServiceConfig("memgraph")
	require.NoError(t, err)
	assert.Equal(t, "memgraph", cfg.Name)
	assert.Equal(t, "zqk-memgraph", cfg.ContainerName)

	_, err = sm.getServiceConfig("non-existent-service")
	assert.Error(t, err)

	// ListServices test
	services := sm.ListServices()
	assert.Contains(t, services, "memgraph")

	// getServiceAuth test
	auth := sm.getServiceAuth("memgraph", cfg)
	assert.Nil(t, auth)

	t.Setenv("ZQK_MEMGRAPH_USERNAME", "admin")
	t.Setenv("ZQK_MEMGRAPH_PASSWORD", "secret")
	authWithEnv := sm.getServiceAuth("memgraph", cfg)
	require.NotNil(t, authWithEnv)
	assert.Equal(t, "admin", authWithEnv.Username)
	assert.Equal(t, "secret", authWithEnv.Password)

	// Pure ServiceManager without hostManager to trigger unknown service error
	pureSM := &ServiceManager{}
	ctx := context.Background()
	assert.Error(t, pureSM.Start(ctx, "invalid_service_test", nil))
	assert.Error(t, pureSM.Stop(ctx, "invalid_service_test"))
	running, err := pureSM.Status(ctx, "invalid_service_test")
	assert.Error(t, err)
	assert.False(t, running)

	// SetDefaultServiceManagerFactory
	SetDefaultServiceManagerFactory(func() *ServiceManager {
		return sm
	})
	assert.Equal(t, sm, getActiveServiceManager())
	SetDefaultServiceManagerFactory(nil)
	assert.NotNil(t, getActiveServiceManager())
}

func TestCycle120_ServiceSubcommands_HierarchyAndOutput(t *testing.T) {
	cmd := NewServiceCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "service", cmd.Name())

	subCmds := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subCmds[sub.Name()] = true
	}
	assert.True(t, subCmds["start"])
	assert.True(t, subCmds["stop"])
	assert.True(t, subCmds["status"])
	assert.True(t, subCmds["list"])

	// formatServiceResult tests
	testCmd := NewServiceCmd()
	err := formatServiceResult(testCmd, "memgraph", "started", "started")
	require.NoError(t, err)

	listCmd := NewServiceListCmd()
	_, err = executeCycle120Command(t, listCmd)
	require.NoError(t, err)

	startCmd := NewServiceStartCmd()
	_, err = executeCycle120Command(t, startCmd, "invalid-svc")
	assert.Error(t, err)

	statusCmd := NewServiceStatusCmd()
	_, err = executeCycle120Command(t, statusCmd, "invalid-svc")
	assert.NoError(t, err)
}

func TestCycle120_StartHereCommand_Execution(t *testing.T) {
	cmd := NewStartHereCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "start-here", cmd.Name())

	err := runStartHere(cmd, nil)
	require.NoError(t, err)
}

func TestCycle120_StatusExtractProjectName(t *testing.T) {
	t.Parallel()

	validConfig := `
project:
  name: my-sample-project
  version: 1.0.0
`
	assert.Equal(t, "my-sample-project", extractProjectName(validConfig))

	quotedConfig := `
project:
  name: "quoted-name"
`
	assert.Equal(t, "quoted-name", extractProjectName(quotedConfig))

	noProject := `
storage:
  backend: memory
`
	assert.Empty(t, extractProjectName(noProject))
	assert.Empty(t, extractProjectName(""))
}

func TestCycle120_StatusHelpers_TableFormatting(t *testing.T) {
	t.Parallel()

	assert.Contains(t, formatStatusTableHeader(), "ZQK Project Status")

	projOut := formatStatusTableProject(map[string]any{
		"project": "zen-test",
		"root":    "/tmp/test",
	})
	assert.Contains(t, projOut, "zen-test")
	assert.Contains(t, projOut, "/tmp/test")

	// formatStatusTableStatus branches
	assert.Contains(t, formatStatusTableStatus(map[string]any{
		objects.FieldKeyStatus: projectStatusIncomplete,
	}), "Incomplete")

	assert.Contains(t, formatStatusTableStatus(map[string]any{
		objects.FieldKeyStatus: projectStatusInitialized,
		"system_health": map[string]any{
			objects.FieldKeyStatus: "critical",
			"blocking_issues":      3,
		},
	}), "Degraded / Critical")

	assert.Contains(t, formatStatusTableStatus(map[string]any{
		objects.FieldKeyStatus: projectStatusInitialized,
		"system_health": map[string]any{
			objects.FieldKeyStatus: "warning",
			"warnings":             2,
		},
	}), "Initialized (2 warnings)")

	// formatStatusTableHealth
	healthOut := formatStatusTableHealth(map[string]any{
		"system_health": map[string]any{
			"scheduler": map[string]any{
				"running": true,
			},
			objects.FieldKeyStatus: "healthy",
			"blocking_issues":      0,
		},
	})
	assert.Contains(t, healthOut, "running")

	// formatStatusTablePlan
	planOut := formatStatusTablePlan(map[string]any{
		"current_priority_plan": map[string]any{
			objects.FieldKeyID:    "PLAN-001",
			objects.FieldKeyTitle: "Foundational Plan",
			"backlog_items":       5,
		},
	})
	assert.Contains(t, planOut, "PLAN-001 - Foundational Plan (5 backlog items)")

	// formatStatusTableCapReview
	capOut := formatStatusTableCapReview(map[string]any{
		"cap_review": map[string]any{
			"timestamp":    "2026-10-23T00:00:00Z",
			"system_check": true,
			"freshness":    "pristine",
		},
	})
	assert.Contains(t, capOut, "CAP Review Gate Status")

	// formatStatusTableStorage
	storageOut := formatStatusTableStorage(map[string]any{
		objects.FieldKeyStorage: map[string]any{
			"backend":  "file-based",
			"data_dir": "/tmp/.zqk/process",
			"object_counts": map[string]int{
				objects.KindGoal: 3,
			},
		},
	})
	assert.Contains(t, storageOut, "Storage:")
	assert.Contains(t, storageOut, "file-based")

	// formatStatusTableActivity
	activityOut := formatStatusTableActivity(map[string]any{
		"recent_activity": []string{"commit 1", "commit 2"},
	})
	assert.Contains(t, activityOut, "Recent Activity:")
	assert.Contains(t, activityOut, "commit 1")
}

func TestCycle120_StatusExecution(t *testing.T) {
	tempDir := setupCycle120ProjectEnv(t)
	testkit.RegisterTempProjectTeardown(t, tempDir, nil)

	cmd := NewStatusCmd()
	_, err := executeCycle120Command(t, cmd, "-f", "json")
	assert.NoError(t, err)

	tableCmd := NewStatusCmd()
	_, err = executeCycle120Command(t, tableCmd)
	assert.NoError(t, err)
}

func TestCycle120_WhoamiHelpers_ParsingAndFormatting(t *testing.T) {
	t.Parallel()

	// parseMCPRoles tests
	assert.Nil(t, parseMCPRoles(""))
	roles := parseMCPRoles("admin, developer , qa")
	assert.Equal(t, []string{"admin", "developer", "qa"}, roles)

	// parseMCPPermissions tests
	assert.Nil(t, parseMCPPermissions(""))
	perms := parseMCPPermissions("read, write , execute")
	assert.Equal(t, []string{"read", "write", "execute"}, perms)

	data := map[string]any{
		objects.FieldKeyAccountID:   "ACC-999",
		"lane":                      "admin",
		objects.FieldKeyPersonaRef:  "PER-001",
		objects.FieldKeyDisplayName: "Alex Test",
		objects.FieldKeyUsername:    "alextest",
		objects.FieldKeyEmail:       "alex@example.com",
		objects.FieldKeyRoles:       []string{"lead", "admin"},
		objects.FieldKeyPermissions: []string{"all"},
		objects.FieldKeyContext: map[string]any{
			objects.FieldKeySource: "cli",
		},
	}

	assert.Contains(t, formatWhoamiTableAccount(data), "ACC-999")
	assert.Contains(t, formatWhoamiTableLane(data), "admin")
	assert.Contains(t, formatWhoamiTablePersona(data), "PER-001")
	assert.Contains(t, formatWhoamiTableName(data), "Alex Test")
	assert.Contains(t, formatWhoamiTableUserInfo(data), "alextest")
	assert.Contains(t, formatWhoamiTableRoles(data), "lead, admin")
	assert.Contains(t, formatWhoamiTablePermissions(data), "all")
	assert.Contains(t, formatWhoamiTableContext(data), "CLI")

	// buildWhoamiOutputData test
	info := &WhoamiAccountInfo{
		AccountID:   "ACC-001",
		Roles:       []string{"viewer"},
		Permissions: []string{"read"},
		PersonaID:   "PER-DEV",
		Lane:        "doer",
		AccountDetails: map[string]any{
			objects.FieldKeyDisplayName: "Dev User",
		},
	}
	outData := buildWhoamiOutputData(info)
	assert.Equal(t, "ACC-001", outData[objects.FieldKeyAccountID])
	assert.Equal(t, "Dev User", outData[objects.FieldKeyDisplayName])
	assert.Equal(t, "doer", outData["lane"])

	cmd := NewWhoamiCmd()
	err := outputWhoamiTable(cmd, outData)
	require.NoError(t, err)
}

func TestCycle120_WhoamiCommand_Execution(t *testing.T) {
	tempDir := setupCycle120ProjectEnv(t)
	require.NotEmpty(t, tempDir)

	cmd := NewWhoamiCmd()
	_, err := executeCycle120Command(t, cmd)
	// Without AuthMiddleware seated security context, expect unauthorized error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestCycle120_VetConfigCmd_FlagsAndCleanExecution(t *testing.T) {
	cmd := NewVetConfigCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "vet-config", cmd.Use)

	out, err := executeCycle120Command(t, cmd, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "vet-config")
}

func TestCycle120_MaintenanceCommands_Execution(t *testing.T) {
	tempDir := setupCycle120ProjectEnv(t)
	require.NotEmpty(t, tempDir)

	cbCmd := NewCleanBranchesCmd()
	require.NotNil(t, cbCmd)
	out, err := executeCycle120Command(t, cbCmd, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "clean-branches")

	streamCmd := NewStreamGCCmd()
	require.NotNil(t, streamCmd)
	out, err = executeCycle120Command(t, streamCmd)
	require.NoError(t, err)
	assert.Contains(t, out, "pruned")

	bashCmd := NewSubmitTDEBashCmd()
	require.NotNil(t, bashCmd)
	_, err = executeCycle120Command(t, bashCmd)
	assert.Error(t, err, "expected error when missing --command")
}
