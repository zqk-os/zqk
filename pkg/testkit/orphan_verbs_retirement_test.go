package testkit_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/app"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var approvedShortcuts = []string{
	"do",
	"inspect",
	"mutate",
	"query",
	"validate",
	"completion",
	"rollback",
	"sync",
	"pre-commit",
	"learn",
	"new",
	"reports",
	"tray",
	"join",
}

// TestOrphanVerbsRetirement_StaticFloor verifies CRIT-1790814952688118000-9460bd8d.
// Asserts that all 14 root commands are registered on the root command and formally specified in
// docs/architecture/CLI_COMMAND_TAXONOMY_STANDARDS.md.
func TestOrphanVerbsRetirement_StaticFloor(t *testing.T) {
	t.Parallel()

	rootCmd := app.NewRootCommand()
	commands := rootCmd.Commands()
	cmdMap := make(map[string]bool)
	for _, c := range commands {
		cmdMap[c.Name()] = true
	}

	for _, shortcut := range approvedShortcuts {
		assert.True(t, cmdMap[shortcut], "root command must register approved shortcut: %s", shortcut)
	}

	// Verify taxonomy documentation
	projectRoot := paths.ResolveProjectRoot(".")
	docPath := filepath.Join(projectRoot, "docs", "architecture", "CLI_COMMAND_TAXONOMY_STANDARDS.md")
	require.True(t, fileutil.Exists(docPath), "CLI_COMMAND_TAXONOMY_STANDARDS.md must exist")

	docBytes, err := fileutil.ReadFile(docPath)
	require.NoError(t, err)
	docText := string(docBytes)

	for _, shortcut := range approvedShortcuts {
		expectedPattern := "`zqk " + shortcut + "`"
		assert.Contains(t, docText, expectedPattern, "documentation must document shortcut: %s", shortcut)
	}
}

// TestOrphanVerbsRetirement_OperationalProof verifies CRIT-1790814952688120000-6931bd7c.
// Asserts that executing the approved ergonomics shortcuts with help flag executes cleanly.
func TestOrphanVerbsRetirement_OperationalProof(t *testing.T) {
	t.Parallel()

	for _, shortcut := range approvedShortcuts {
		rootCmd := app.NewRootCommand()
		var outBuf, errBuf bytes.Buffer
		rootCmd.SetOut(&outBuf)
		rootCmd.SetErr(&errBuf)
		rootCmd.SetArgs([]string{shortcut, "--help"})

		err := rootCmd.Execute()
		assert.NoError(t, err, "shortcut '%s --help' must execute without error", shortcut)
		assert.NotEmpty(t, outBuf.String(), "shortcut '%s --help' must print usage help", shortcut)
	}
}

// TestOrphanVerbsRetirement_NegativeBoundary verifies CRIT-1790814952688119000-2c817e43.
// Asserts that unrecognized commands fail closed with clear error messaging and do not panic.
func TestOrphanVerbsRetirement_NegativeBoundary(t *testing.T) {
	t.Parallel()

	rootCmd := app.NewRootCommand()
	var outBuf, errBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)
	rootCmd.SetArgs([]string{"unrecognized-orphan-verb-xyz"})

	err := rootCmd.Execute()
	assert.Error(t, err, "unrecognized root verb must return an error")
	combined := errBuf.String() + outBuf.String() + err.Error()
	assert.True(t, strings.Contains(combined, "unknown command") || strings.Contains(combined, "unrecognized"),
		"error message should indicate unknown command: %s", combined)
}
