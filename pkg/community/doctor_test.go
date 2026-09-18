package community_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/community"
)

// CRIT-1789715923297491000-72ded0bd: Functional Acceptance
// Verifies comprehensive diagnostic health check suite covering git, go, workspace writable, process storage, and resources.
func TestDoctor_FunctionalAcceptance(t *testing.T) {
	tempDir := t.TempDir()
	// Create mock .zqk directory
	err := os.Mkdir(filepath.Join(tempDir, ".zqk"), 0755)
	require.NoError(t, err)

	doctor := community.NewSystemDoctor(tempDir)
	report := doctor.RunDiagnostics(context.Background())

	assert.Equal(t, 5, report.TotalChecks, "Default doctor should run 5 standard checks")
	assert.NotEmpty(t, report.GoVersion)
	assert.NotEmpty(t, report.OS)
	assert.NotEmpty(t, report.Arch)
	assert.True(t, report.Passed >= 3, "At least git, writable, and resources should pass in this environment")

	// Verify JSON serialization
	jsonOut, err := community.FormatReportJSON(report)
	require.NoError(t, err)
	assert.Contains(t, jsonOut, "\"total_checks\": 5")
	assert.Contains(t, jsonOut, "\"workspace_writable\"")
	assert.Contains(t, jsonOut, "\"git_available\"")
}

// CRIT-1789715923297492000-aae99082: Boundary & Error Handling
// Verifies boundary handling: non-existent directories, canceled contexts, custom checkers, and formatting.
func TestDoctor_BoundaryAndErrorHandling(t *testing.T) {
	t.Run("non_existent_workspace_root", func(t *testing.T) {
		nonExistent := filepath.Join(os.TempDir(), "non_existent_zqk_path_probe_12345")
		checker := &community.FilesystemPermissionsChecker{}
		res := checker.Check(context.Background(), nonExistent)
		assert.Equal(t, community.SeverityFail, res.Severity)
		assert.Contains(t, res.Message, "cannot stat workspace root")
		assert.NotEmpty(t, res.Remediation)
	})

	t.Run("file_as_workspace_root", func(t *testing.T) {
		tempFile := filepath.Join(t.TempDir(), "dummy.txt")
		err := os.WriteFile(tempFile, []byte("probe"), 0644)
		require.NoError(t, err)

		checker := &community.FilesystemPermissionsChecker{}
		res := checker.Check(context.Background(), tempFile)
		assert.Equal(t, community.SeverityFail, res.Severity)
		assert.Contains(t, res.Message, "is not a directory")
	})

	t.Run("missing_process_storage_warns", func(t *testing.T) {
		emptyDir := t.TempDir()
		checker := &community.ProcessDirectoryChecker{}
		res := checker.Check(context.Background(), emptyDir)
		assert.Equal(t, community.SeverityWarn, res.Severity)
		assert.Contains(t, res.Message, "no .zqk metadata directory found")
		assert.NotEmpty(t, res.Remediation)
	})

	t.Run("context_timeout_aborts_cleanly", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		doctor := community.NewSystemDoctor(t.TempDir())
		report := doctor.RunDiagnostics(ctx)

		assert.Equal(t, 5, report.TotalChecks)
		assert.Equal(t, 5, report.Failures)
		assert.Equal(t, 0, report.Passed)
		for _, res := range report.Results {
			assert.Equal(t, community.SeverityFail, res.Severity)
			assert.Contains(t, res.Message, "check aborted")
		}
	})

	t.Run("unknown_severity_falls_back_to_fail", func(t *testing.T) {
		doctor := community.NewSystemDoctor(t.TempDir())
		doctor.Register(&mockCustomChecker{
			name:     "custom_weird_severity",
			category: "custom",
			result: community.DiagnosticResult{
				Name:     "custom_weird_severity",
				Category: "custom",
				Severity: community.CheckSeverity("UNKNOWN_STR"),
				Message:  "strange outcome",
			},
		})
		report := doctor.RunDiagnostics(context.Background())
		assert.Equal(t, 6, report.TotalChecks)
		var customRes *community.DiagnosticResult
		for i := range report.Results {
			if report.Results[i].Name == "custom_weird_severity" {
				customRes = &report.Results[i]
				break
			}
		}
		require.NotNil(t, customRes)
		assert.Equal(t, community.SeverityFail, customRes.Severity)
	})
}

// CRIT-1789715923297493000-c5da9b3f: Integration & Conformance
// Verifies custom check registration, execution timing, and structured reporting.
func TestDoctor_IntegrationAndConformance(t *testing.T) {
	tempDir := t.TempDir()
	doctor := community.NewSystemDoctor(tempDir)

	mockCheck := &mockCustomChecker{
		name:     "sample_plugin_check",
		category: "plugins",
		result: community.DiagnosticResult{
			Name:        "sample_plugin_check",
			Category:    "plugins",
			Severity:    community.SeverityPass,
			Message:     "all plugins validated",
			Remediation: "",
		},
	}
	doctor.Register(mockCheck)

	start := time.Now()
	report := doctor.RunDiagnostics(context.Background())
	duration := time.Since(start)

	assert.Equal(t, 6, report.TotalChecks)
	assert.True(t, duration >= 0)

	// Verify categories present in results
	categories := make(map[string]bool)
	for _, res := range report.Results {
		categories[res.Category] = true
		assert.True(t, res.DurationMs >= 0)
	}
	assert.True(t, categories["toolchain"])
	assert.True(t, categories["filesystem"])
	assert.True(t, categories["plugins"])

	jsonReport, err := community.FormatReportJSON(report)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(jsonReport, "{\n"))
}

// Mock checker for tests.
type mockCustomChecker struct {
	name     string
	category string
	result   community.DiagnosticResult
}

func (m *mockCustomChecker) Name() string     { return m.name }
func (m *mockCustomChecker) Category() string { return m.category }
func (m *mockCustomChecker) Check(ctx context.Context, rootDir string) community.DiagnosticResult {
	return m.result
}
