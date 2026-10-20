package reports

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReportsElevation_ReportsCmd_FlagBindingsAndInit(t *testing.T) {
	t.Parallel()

	cmd := NewReportsCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "reports", cmd.Name())

	subcommands := cmd.Commands()
	subNames := make(map[string]*cobra.Command)
	for _, sub := range subcommands {
		subNames[sub.Name()] = sub
	}

	for _, name := range []string{"pcs", "edd", "blockers"} {
		sub, exists := subNames[name]
		require.True(t, exists, "reports command missing %s subcommand", name)

		flag := sub.Flags().Lookup("include-commit-data")
		require.NotNil(t, flag, "subcommand %s missing include-commit-data flag", name)
		assert.Equal(t, "false", flag.DefValue)
	}

	quickSub, exists := subNames["quick"]
	require.True(t, exists, "reports command missing quick subcommand")
	require.NotNil(t, quickSub.Flags().Lookup("preset"), "quick subcommand missing preset flag")
	require.NotNil(t, quickSub.Flags().Lookup("strict"), "quick subcommand missing strict flag")

	// Test bindReportCommand directly
	dummyCmd := &cobra.Command{Use: "dummy"}
	hb := clipkg.DynamicHelpBuilder("Dummy Short", "Dummy Long")
	bound := bindReportCommand(dummyCmd, hb, func(cmd *cobra.Command, args []string) error { return nil })
	require.NotNil(t, bound)
	assert.NotNil(t, bound.Flags().Lookup("include-commit-data"))
}

func TestReportsElevation_CalculateReportMetrics_ErrorPaths(t *testing.T) {
	// Case 1: ctx == nil
	t.Run("NilContext", func(t *testing.T) {
		cmd := NewPCSCmd()
		cmd.SetContext(context.Background())
		cli.SetContext(cmd, nil)

		_, err := calculateReportMetrics(cmd, "PCS")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get context")
	})

	// Case 2: projectRoot == "" and ResolveProjectRoot(".") == ""
	t.Run("EmptyProjectRootNotFound", func(t *testing.T) {
		tempDir := t.TempDir()
		origWd, err := fileutil.Getwd()
		require.NoError(t, err)
		defer func() { _ = os.Chdir(origWd) }()
		require.NoError(t, os.Chdir(tempDir))

		t.Setenv("ZQK_PROJECT_ROOT", "")
		t.Setenv("ZQK_TEST_ROOT", "")

		cmd := NewPCSCmd()
		cmd.SetContext(context.Background())
		cli.SetContext(cmd, cli.ContextForProjectRoot(""))

		_, err = calculateReportMetrics(cmd, "PCS")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "project root not found")
	})

	// Case 3: Storage initialization failure
	t.Run("StorageInitFailure", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "not-a-directory")
		require.NoError(t, fileutil.WriteFile(tmpFile, []byte("invalid"), paths.FilePerm644))
		cmd := NewPCSCmd()
		cmd.SetContext(context.Background())
		cli.SetContext(cmd, cli.ContextForProjectRoot(tmpFile))

		_, err := calculateReportMetrics(cmd, "PCS")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize storage")
	})
}

func TestReportsElevation_PCS_BoundaryStatusAndOutput(t *testing.T) {
	t.Parallel()

	// Boundary coverage for getPCSStatus
	tests := []struct {
		score    float64
		expected string
	}{
		{100.0, "excellent"},
		{85.0, "excellent"},
		{80.0, "excellent"},
		{79.99, "good"},
		{65.0, "good"},
		{60.0, "good"},
		{59.99, "fair"},
		{45.0, "fair"},
		{40.0, "fair"},
		{39.99, "poor"},
		{25.0, "poor"},
		{20.0, "poor"},
		{19.99, "critical"},
		{5.0, "critical"},
		{0.0, "critical"},
		{-10.0, "critical"},
	}

	for _, tt := range tests {
		status := getPCSStatus(tt.score)
		assert.Equal(t, tt.expected, status, "score: %.2f", tt.score)
	}

	// Boundary coverage for outputPCSTable
	tableTiers := []struct {
		score            float64
		expectedFragment string
	}{
		{85.0, "Status: Excellent - High confidence in project delivery"},
		{65.0, "Status: Good - Project on track with minor concerns"},
		{45.0, "Status: Fair - Some concerns, monitor closely"},
		{25.0, "Status: Poor - Significant concerns, action needed"},
		{10.0, "Status: Critical - Immediate attention required"},
	}

	for _, tt := range tableTiers {
		cmd := NewPCSCmd()
		var buf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

		err := outputPCSTable(cmd, tt.score)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Project Confidence Score (PCS)")
		assert.Contains(t, buf.String(), tt.expectedFragment)
	}

	// outputPCSJSON & outputPCSYAML
	t.Run("JSON_YAML_Output", func(t *testing.T) {
		cmd := NewPCSCmd()
		var jsonBuf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &jsonBuf))
		err := outputPCSJSON(cmd, 75.5)
		require.NoError(t, err)
		assert.Contains(t, jsonBuf.String(), `"pcs": 75.5`)
		assert.Contains(t, jsonBuf.String(), `"status": "good"`)

		var yamlBuf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &yamlBuf))
		err = outputPCSYAML(cmd, 85.0)
		require.NoError(t, err)
		assert.Contains(t, yamlBuf.String(), "pcs: 85")
		assert.Contains(t, yamlBuf.String(), "status: excellent")
	})
}

func TestReportsElevation_EDD_BoundaryStatusAndOutput(t *testing.T) {
	t.Parallel()

	// Boundary coverage for getEDDStatus
	tests := []struct {
		edd      float64
		expected string
	}{
		{35.0, "significant_underestimation"},
		{20.01, "significant_underestimation"},
		{20.0, "moderate_underestimation"},
		{15.0, "moderate_underestimation"},
		{5.01, "moderate_underestimation"},
		{5.0, "good_accuracy"},
		{0.0, "good_accuracy"},
		{-4.99, "good_accuracy"},
		{-5.0, "moderate_overestimation"},
		{-10.0, "moderate_overestimation"},
		{-19.99, "moderate_overestimation"},
		{-20.0, "significant_overestimation"},
		{-35.0, "significant_overestimation"},
	}

	for _, tt := range tests {
		status := getEDDStatus(tt.edd)
		assert.Equal(t, tt.expected, status, "edd: %.2f", tt.edd)
	}

	// Boundary coverage for outputEDDTable
	tableTiers := []struct {
		edd              float64
		expectedFragment string
	}{
		{25.0, "Status: Significant Underestimation"},
		{10.0, "Status: Moderate Underestimation"},
		{0.0, "Status: Good Estimation Accuracy"},
		{-10.0, "Status: Moderate Overestimation"},
		{-25.0, "Status: Significant Overestimation"},
	}

	for _, tt := range tableTiers {
		cmd := NewEDDCmd()
		var buf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

		err := outputEDDTable(cmd, tt.edd)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Effort Distribution Discrepancy (EDD)")
		assert.Contains(t, buf.String(), tt.expectedFragment)
	}
}

func TestReportsElevation_Blockers_OutputFormats(t *testing.T) {
	t.Parallel()

	t.Run("EmptyBlockersAndDependencies", func(t *testing.T) {
		cmd := NewBlockersCmd()
		var buf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

		db := &metrics.DependenciesBlockers{
			Summary:      "",
			Dependencies: nil,
			Blockers:     nil,
			ActionItems:  nil,
		}

		err := outputBlockersTable(cmd, db)
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "Dependencies & Blockers (D&B)")
		assert.Contains(t, output, "Dependencies: None identified")
		assert.Contains(t, output, "Blockers: None identified")
		assert.NotContains(t, output, "Action Items (")
	})

	t.Run("PopulatedBlockersDependenciesAndActionItems", func(t *testing.T) {
		cmd := NewBlockersCmd()
		var buf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

		db := &metrics.DependenciesBlockers{
			Summary: "Risk assessment summary overview",
			Dependencies: []metrics.DependencyItem{
				{Severity: "high", Type: "code", Description: "dependency on auth service"},
				{Severity: "low", Type: "external", Description: "external API spec"},
			},
			Blockers: []metrics.BlockerItem{
				{Severity: "critical", Type: "work_item", Description: "database migration blocked"},
			},
			ActionItems: []metrics.ActionItem{
				{Priority: "P0", Description: "Unblock migration run"},
			},
		}

		err := outputBlockersTable(cmd, db)
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "Summary: Risk assessment summary overview")
		assert.Contains(t, output, "Dependencies (2):")
		assert.Contains(t, output, "1. [high] code - dependency on auth service")
		assert.Contains(t, output, "2. [low] external - external API spec")
		assert.Contains(t, output, "Blockers (1):")
		assert.Contains(t, output, "1. [critical] work_item - database migration blocked")
		assert.Contains(t, output, "Action Items (1):")
		assert.Contains(t, output, "1. [P0] Unblock migration run")
	})

	t.Run("Blockers_JSONFormatting", func(t *testing.T) {
		db := &metrics.DependenciesBlockers{
			Summary: "Comprehensive risk check",
			Dependencies: []metrics.DependencyItem{
				{Severity: "medium", Type: "code", Description: "Dep 1"},
			},
			Blockers: []metrics.BlockerItem{
				{Severity: "high", Type: "code", Description: "Blocker 1"},
			},
			ActionItems: []metrics.ActionItem{
				{Priority: "P1", Description: "Action 1"},
			},
		}

		result := map[string]any{
			objects.FieldKeySummary:      db.Summary,
			objects.FieldKeyDependencies: db.Dependencies,
			objects.FieldKeyBlockers:     db.Blockers,
			"action_items":               db.ActionItems,
		}

		data, err := json.Marshal(result)
		require.NoError(t, err)

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "Comprehensive risk check", parsed[objects.FieldKeySummary])
		assert.Len(t, parsed[objects.FieldKeyDependencies].([]any), 1)
		assert.Len(t, parsed[objects.FieldKeyBlockers].([]any), 1)
		assert.Len(t, parsed["action_items"].([]any), 1)
	})
}

func TestReportsElevation_Quick_PresetsAndRendering(t *testing.T) {
	projectRoot, storageProvider := setupTestProject(t)

	t.Run("UnknownPresetError", func(t *testing.T) {
		cmd := createQuickTestCommand(t, projectRoot, storageProvider)
		_ = cmd.Flags().Set("preset", "invalid-preset-value")

		err := runQuick(cmd, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unknown preset "invalid-preset-value"`)
		assert.Contains(t, err.Error(), "use: questions, milestones-overdue")
	})

	t.Run("ErrExitCodeFormat", func(t *testing.T) {
		err := errExitCode(1)
		assert.Equal(t, "critical SLA exceeded", err.Error())
	})

	t.Run("OutputQuickAndExit", func(t *testing.T) {
		cmd := createQuickTestCommand(t, projectRoot, storageProvider)
		var buf bytes.Buffer
		cmd.SetContext(pkgctx.WithCommandOutputWriter(cmd.Context(), &buf))
		_ = cmd.Flags().Set("preset", "questions")

		payload := map[string]any{
			"preset": "questions",
			"rows":   []map[string]any{},
			"meta":   map[string]any{"total_count": 0, "critical_exceeded": true},
		}

		err := outputQuickAndExit(cmd, payload, 1)
		require.Error(t, err)
		assert.Equal(t, errExitCode(1), err)
	})
}
