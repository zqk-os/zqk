package releasegate

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestUpdateHomebrewFormulaScript_DryRunAndFailClosed(t *testing.T) {
	scriptPath := filepath.Clean(filepath.Join("..", "..", "scripts", "open-core", "update-homebrew-formula.sh"))
	require.FileExists(t, scriptPath)

	t.Run("dry_run_renders_all_four_platforms", func(t *testing.T) {
		tmpDir := t.TempDir()
		checksumsPath := filepath.Join(tmpDir, "zqk_0.8.0_checksums.txt")
		checksums := strings.Join([]string{
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  zqk_0.8.0_darwin_arm64.tar.gz",
			"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  zqk_0.8.0_darwin_amd64.tar.gz",
			"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc  zqk_0.8.0_linux_arm64.tar.gz",
			"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd  zqk_0.8.0_linux_amd64.tar.gz",
		}, "\n") + "\n"
		require.NoError(t, fileutil.WriteStandardFile(checksumsPath, []byte(checksums)))

		cmd := testkit.ManagedCommand(t, context.Background(), "bash", scriptPath, "v0.8.0", checksumsPath, "--dry-run")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "dry-run failed: %s", string(out))

		formula := string(out)
		assert.Contains(t, formula, `version "0.8.0"`)
		assert.Contains(t, formula, `sha256 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`)
		assert.Contains(t, formula, `sha256 "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`)
		assert.Contains(t, formula, `sha256 "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"`)
		assert.Contains(t, formula, `sha256 "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"`)
	})

	t.Run("missing_checksum_fails_closed", func(t *testing.T) {
		tmpDir := t.TempDir()
		checksumsPath := filepath.Join(tmpDir, "incomplete_checksums.txt")
		checksums := strings.Join([]string{
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  zqk_0.8.0_darwin_arm64.tar.gz",
			"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  zqk_0.8.0_darwin_amd64.tar.gz",
			"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc  zqk_0.8.0_linux_arm64.tar.gz",
		}, "\n") + "\n"
		require.NoError(t, fileutil.WriteStandardFile(checksumsPath, []byte(checksums)))

		cmd := testkit.ManagedCommand(t, context.Background(), "bash", scriptPath, "v0.8.0", checksumsPath, "--dry-run")
		out, err := cmd.CombinedOutput()
		require.Error(t, err, "expected failure when linux_amd64 checksum is missing")
		assert.Contains(t, string(out), "missing SHA_LINUX_AMD64")
	})
}
