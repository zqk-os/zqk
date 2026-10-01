package integrity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLegacyCustomRulesGate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-gate-test-*")
	require.NoError(t, err)
	defer fileutil.RemoveAll(tmpDir)

	// Clean workspace without customRuleValidators
	ok, note := LegacyCustomRulesGate(tmpDir)
	assert.True(t, ok)
	assert.NotEmpty(t, note)

	// Forbidden file pkg/validation/go_validator_custom_rules.go exists
	forbidden := filepath.Join(tmpDir, "pkg", "validation", "go_validator_custom_rules.go")
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(forbidden), 0o755))
	require.NoError(t, fileutil.WriteFile(forbidden, []byte("package validation"), 0o644))

	okFail, noteFail := LegacyCustomRulesGate(tmpDir)
	assert.False(t, okFail)
	assert.Contains(t, noteFail, "must stay deleted")
}

func TestMembraneCoverageGate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zqk-membrane-test-*")
	require.NoError(t, err)
	defer fileutil.RemoveAll(tmpDir)

	// In empty workspace where first file doesn't exist, gate passes (binary dist)
	ok, gaps := MembraneCoverageGate(tmpDir)
	assert.True(t, ok)
	assert.Empty(t, gaps)
}
