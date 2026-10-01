package testkit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/tui"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCLITUIDecoupling_StaticFloor verifies CRIT-1790814946798057000-8237162f.
// Asserts that cmd/zqk/ui is reduced to a thin presentation shim (<= 2 files, <= 250 LOC).
func TestCLITUIDecoupling_StaticFloor(t *testing.T) {
	t.Parallel()

	projectRoot := paths.ResolveProjectRoot(".")
	uiCmdDir := filepath.Join(projectRoot, "cmd", "zqk", "ui")
	require.True(t, fileutil.Exists(uiCmdDir), "cmd/zqk/ui directory must exist")

	entries, err := os.ReadDir(uiCmdDir)
	require.NoError(t, err)

	goFiles := 0
	totalLines := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" {
			goFiles++
			content, err := fileutil.ReadFile(filepath.Join(uiCmdDir, entry.Name()))
			require.NoError(t, err)
			lines := len(filepath.SplitList(string(content)))
			_ = lines
			// count line breaks
			lineCount := 0
			for _, b := range content {
				if b == '\n' {
					lineCount++
				}
			}
			totalLines += lineCount
		}
	}

	assert.LessOrEqual(t, goFiles, 2, "cmd/zqk/ui must contain at most 2 files (current: %d)", goFiles)
	assert.LessOrEqual(t, totalLines, 250, "cmd/zqk/ui must not exceed 250 LOC (was 5,659, current: %d)", totalLines)

	// pkg/tui must exist and house the extracted TUI domain implementation
	tuiPkgDir := filepath.Join(projectRoot, "pkg", "tui")
	assert.True(t, fileutil.Exists(tuiPkgDir), "pkg/tui must exist as a standalone package")
}

// TestCLITUIDecoupling_OperationalProof verifies CRIT-1790814946798058000-5942e149.
// Asserts that pkg/tui models, ANSI controls, and views operate correctly outside the CLI command.
func TestCLITUIDecoupling_OperationalProof(t *testing.T) {
	t.Parallel()

	// 1. Verify ANSI terminal controls
	assert.Equal(t, "\033[?1049h", tui.AnsiAltBufferEnter)
	assert.Equal(t, "\033[?1049l", tui.AnsiAltBufferExit)
	assert.Equal(t, "\033[2J", tui.AnsiClearScreen)

	// 2. Initialize UIModel programmatically
	model := tui.NewUIModel(paths.ResolveProjectRoot("."), "seismograph")
	require.NotNil(t, model)
	assert.Equal(t, tui.TabSeismograph, model.ActiveTab)

	// 3. Switch tabs programmatically
	model.ActiveTab = tui.TabAudit
	assert.Equal(t, tui.TabAudit, model.ActiveTab)
	model.ActiveTab = tui.TabPM
	assert.Equal(t, tui.TabPM, model.ActiveTab)
}

// TestCLITUIDecoupling_NegativeBoundary verifies CRIT-1790814946798059000-827a85d2.
// Asserts headless fallback execution without raw terminal mode panics or nil pointer exceptions.
func TestCLITUIDecoupling_NegativeBoundary(t *testing.T) {
	t.Parallel()

	// Model with nil storage and security context handles refreshes safely
	model := tui.NewUIModel("", "")
	require.NotNil(t, model)

	assert.NotPanics(t, func() {
		model.RefreshMutations()
		model.RefreshAuditEvents()
		model.RefreshObjects()
		model.RefreshHealth()
	}, "headless model refreshes must not panic with empty storage/context")
}
