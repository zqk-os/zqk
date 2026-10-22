package system

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestCanSkipAutofixRun_NilOrNoFlag(t *testing.T) {
	t.Parallel()
	require.False(t, canSkipAutofixRun(nil, nil))

	cmd := &cobra.Command{}
	cmd.Flags().Bool("auto-fix", false, "")
	require.False(t, canSkipAutofixRun(cmd, nil))
}

func TestCanSkipAutofixRun_TargetedArgs(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().Bool("auto-fix", true, "")
	cmd.Flags().Bool("auto-fix-scheduler", true, "")
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("clear-cache", false, "")
	cmd.Flags().Bool("refresh-cache", false, "")

	require.False(t, canSkipAutofixRun(cmd, []string{"GOAL-001"}))
}

func TestCanSkipAutofixRun_BypassFlags(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().Bool("auto-fix", true, "")
	cmd.Flags().Bool("auto-fix-scheduler", true, "")
	cmd.Flags().Bool("force", true, "")
	cmd.Flags().Bool("clear-cache", false, "")
	cmd.Flags().Bool("refresh-cache", false, "")

	require.False(t, canSkipAutofixRun(cmd, nil))
}

func TestCanSkipAutofixRun_CleanKernel(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), tempRoot)

	// Create directory structure
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tempRoot, paths.ProjectDataDir, paths.CacheDir), paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tempRoot, paths.ProjectDataDir, paths.WalDir), paths.DirPerm755))

	cache := validation.SharedValidationStateCache(tempRoot)
	require.NotNil(t, cache)

	cache.Set(&validation.ValidationState{
		ObjectID:      "GOAL-1",
		ObjectKind:    "goal",
		LastValidated: time.Now(),
		Issues:        nil,
	})
	require.NoError(t, cache.Save())

	cmd := &cobra.Command{}
	cmd.Flags().Bool("auto-fix", true, "")
	cmd.Flags().Bool("auto-fix-scheduler", true, "")
	cmd.Flags().Bool("skip-if-locked", false, "")
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("clear-cache", false, "")
	cmd.Flags().Bool("refresh-cache", false, "")

	// Clean cache + no pending batches + no uncommitted drafts -> true
	require.True(t, canSkipAutofixRun(cmd, nil))

	// Introduce a pending autofix batch -> false
	autofixDir := filepath.Join(tempRoot, paths.ProjectDataDir, "autofix")
	require.NoError(t, fileutil.MkdirAll(autofixDir, paths.DirPerm755))
	batchFile := filepath.Join(autofixDir, "AUTOFIX-12345.json")
	require.NoError(t, fileutil.WriteFile(batchFile, []byte("{}"), paths.FilePerm644))

	require.False(t, canSkipAutofixRun(cmd, nil))
	require.NoError(t, fileutil.Remove(batchFile))

	// Introduce an auto-fixable issue in the cache -> false
	cache.Set(&validation.ValidationState{
		ObjectID:      "GOAL-2",
		ObjectKind:    "goal",
		LastValidated: time.Now(),
		Issues: []validation.ValidationIssue{
			{Tier: 1, Message: "Missing title", AutoFixable: true},
		},
	})
	require.NoError(t, cache.Save())

	require.False(t, canSkipAutofixRun(cmd, nil))
}
