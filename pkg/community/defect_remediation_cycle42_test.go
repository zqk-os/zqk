package community_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	newcmd "github.com/zqk-os/zqk/cmd/zqk/new"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestDefectRemediationCycle42_StaticFloorInvariant verifies:
// 1. GH-948: cmd/zqk/new/new.go registers 'tst' and 'tc' aliases for test_case.
// 2. GH-949: cmd/zqk/new/new.go defaults --category to 'functional' for criteria.
// 3. GH-952: id_prefixes_config.yaml and id_prefixes_config.go map WRK- to workstream.
// 4. GH-957: cas_one_blob.go uses defer lock cleanup and handles stale locks with 15s timeout.
func TestDefectRemediationCycle42_StaticFloorInvariant(t *testing.T) {
	repoRoot := findRepoRoot(t)

	t.Run("GH948_TestCaseAliases", func(t *testing.T) {
		verifyGH948StaticAliases(t, repoRoot)
	})
	t.Run("GH949_CriteriaDefaultCategory", func(t *testing.T) {
		verifyGH949StaticCriteriaCategory(t, repoRoot)
	})
	t.Run("GH952_WorkstreamPrefixMapping", func(t *testing.T) {
		verifyGH952StaticWorkstreamPrefix(t, repoRoot)
	})
	t.Run("GH957_FileCASLockTimeoutAndRemoval", func(t *testing.T) {
		verifyGH957StaticFileCASLock(t, repoRoot)
	})
}

func verifyGH948StaticAliases(t *testing.T, repoRoot string) {
	newGoPath := filepath.Join(repoRoot, "cmd", "zqk", "new", "new.go")
	contentBytes, err := fileutil.ReadFile(newGoPath)
	require.NoError(t, err)
	content := string(contentBytes)

	assert.Contains(t, content, `newKindConvenienceCmd(objects.KindTestCase, "tst", "tc")`,
		"new.go must register 'tst' and 'tc' aliases for test_case")

	rootCmd := newcmd.NewNewCmd()
	var tcCmd *cobra.Command
	for _, child := range rootCmd.Commands() {
		if child.Name() == objects.KindTestCase {
			tcCmd = child
			break
		}
	}
	require.NotNil(t, tcCmd, "test_case command must be registered under new")
	assert.Contains(t, tcCmd.Aliases, "tst")
	assert.Contains(t, tcCmd.Aliases, "tc")
}

func verifyGH949StaticCriteriaCategory(t *testing.T, repoRoot string) {
	newGoPath := filepath.Join(repoRoot, "cmd", "zqk", "new", "new.go")
	contentBytes, err := fileutil.ReadFile(newGoPath)
	require.NoError(t, err)
	content := string(contentBytes)

	assert.Contains(t, content, `if kind == objects.KindCriteria {`)
	assert.Contains(t, content, `cmd.Flags().String("category", "functional", "Criteria category (default: functional)")`)

	rootCmd := newcmd.NewNewCmd()
	var critCmd *cobra.Command
	for _, child := range rootCmd.Commands() {
		if child.Name() == objects.KindCriteria {
			critCmd = child
			break
		}
	}
	require.NotNil(t, critCmd, "criteria command must be registered under new")
	catFlag := critCmd.Flags().Lookup("category")
	require.NotNil(t, catFlag, "category flag must exist on criteria command")
	assert.Equal(t, "functional", catFlag.DefValue, "category flag default must be functional")
}

func verifyGH952StaticWorkstreamPrefix(t *testing.T, repoRoot string) {
	yamlPath := filepath.Join(repoRoot, ".zqk", "specs", "configs", "id_prefixes_config.yaml")
	yamlBytes, err := fileutil.ReadFile(yamlPath)
	require.NoError(t, err)
	yamlContent := string(yamlBytes)
	assert.Contains(t, yamlContent, "workstream:\n        - WS-\n        - WRK-")

	goConfigPath := filepath.Join(repoRoot, "pkg", "validation", "id_prefixes_config.go")
	goConfigBytes, err := fileutil.ReadFile(goConfigPath)
	require.NoError(t, err)
	assert.Contains(t, string(goConfigBytes), `kindnames.Workstream:          {"WS-", "WRK-"},`)
}

func verifyGH957StaticFileCASLock(t *testing.T, repoRoot string) {
	casOneBlobPath := filepath.Join(repoRoot, "pkg", "storage", "filecas", "cas_one_blob.go")
	casBytes, err := fileutil.ReadFile(casOneBlobPath)
	require.NoError(t, err)
	casContent := string(casBytes)

	assert.Contains(t, casContent, "15 * time.Second", "filecas must define 15s timeout for sweep lock TTL")
	assert.Contains(t, casContent, "defer func()", "cas_one_blob.go must use defer block for lock cleanup")
	assert.True(t, strings.Contains(casContent, "fileutil.Remove(lockPath)") || strings.Contains(casContent, "os.Remove(lockPath)"),
		"cas_one_blob.go must clean up lockPath")
}

// TestDefectRemediationCycle42_OperationalDynamicProof verifies:
// 1. GH-944: Template generator omits empty optional references (priority_plan_ref: "", milestone_ref: "").
// 2. GH-957: Sweep lock executes cleanly, cleans up lock file, and non-concurrent invocation succeeds.
// 3. GH-948/GH-952: Alias resolution and WRK- prefix recognition work dynamically.
func TestDefectRemediationCycle42_OperationalDynamicProof(t *testing.T) {
	repoRoot := findRepoRoot(t)

	t.Run("GH944_TemplateReferenceOmission", func(t *testing.T) {
		verifyGH944OperationalTemplateOmission(t, repoRoot)
	})
	t.Run("GH957_SweepLockExecutionAndCleanup", func(t *testing.T) {
		verifyGH957OperationalSweepLock(t)
	})
	t.Run("GH948_GH952_DynamicResolution", func(t *testing.T) {
		verifyGH948GH952OperationalResolution(t)
	})
}

func verifyGH944OperationalTemplateOmission(t *testing.T, repoRoot string) {
	binZqk := filepath.Join(repoRoot, "bin", "zqk")
	if !fileutil.IsRegularFile(binZqk) {
		buildCmd := testkit.ManagedCommand(t, t.Context(), "go", "build", "-o", binZqk, "./cmd/zqk")
		buildCmd.Dir = repoRoot
		buildOut, buildErr := buildCmd.CombinedOutput()
		require.NoError(t, buildErr, "failed to build bin/zqk: %s", string(buildOut))
	}

	cmd := testkit.ManagedCommand(t, t.Context(), binZqk, "object", "template", "backlog_item")
	zqkenv.WireExecForIsolatedProject(cmd, repoRoot)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "object template backlog_item must succeed: %s", string(out))

	tmpl := string(out)
	assert.Contains(t, tmpl, "kind: backlog_item")
	assert.NotContains(t, tmpl, `priority_plan_ref: ""`, "generated template must omit empty priority_plan_ref")
	assert.NotContains(t, tmpl, `priority_plan_ref: # ""`, "generated template must omit commented empty priority_plan_ref")
	assert.NotContains(t, tmpl, `milestone_ref: ""`, "generated template must omit empty milestone_ref")
	assert.NotContains(t, tmpl, `milestone_ref: # ""`, "generated template must omit commented empty milestone_ref")
}

func verifyGH957OperationalSweepLock(t *testing.T) {
	dir := t.TempDir()
	cas := filecas.NewContentAddressableStorage(dir, "backlog_item")
	lockPath := filepath.Join(dir, ".cas_sweep.lock")

	ranFirst := false
	errFirst := cas.WithSweepLock(func() error {
		ranFirst = true
		assert.True(t, fileutil.Exists(lockPath), "lock file must exist while sweep lock is held")
		return nil
	})
	require.NoError(t, errFirst)
	assert.True(t, ranFirst, "first sweep lock callback must execute")
	assert.False(t, fileutil.Exists(lockPath), "lock file must be unlinked upon return")

	ranSecond := false
	errSecond := cas.WithSweepLock(func() error {
		ranSecond = true
		return nil
	})
	require.NoError(t, errSecond)
	assert.True(t, ranSecond, "second sweep lock invocation must succeed cleanly")
	assert.False(t, fileutil.Exists(lockPath), "lock file must remain unlinked")
}

func verifyGH948GH952OperationalResolution(t *testing.T) {
	rootCmd := newcmd.NewNewCmd()
	tstCmd, _, errTst := rootCmd.Find([]string{"tst"})
	require.NoError(t, errTst)
	assert.Equal(t, objects.KindTestCase, tstCmd.Name())

	tcCmd, _, errTc := rootCmd.Find([]string{"tc"})
	require.NoError(t, errTc)
	assert.Equal(t, objects.KindTestCase, tcCmd.Name())

	cfg := validation.GetGlobalIDPrefixesConfig()
	prefixes := cfg.GetPrefixesForKind("workstream")
	assert.Contains(t, prefixes, "WRK-")
	assert.Contains(t, prefixes, "WS-")

	validator := validation.NewIDValidator("")
	valid, valErr := validator.ValidateID("WRK-101", "workstream")
	require.NoError(t, valErr)
	assert.True(t, valid, "WRK- prefix must validate for workstream")
}

// TestDefectRemediationCycle42_NegativeAdversarialBoundary verifies:
// 1. GH-957: Contention with simulated stale lock (>15s) is automatically detected, swept, and reclaimed.
// 2. GH-944: CoerceMutationFields strips empty string scalar references to prevent invalid draft states.
func TestDefectRemediationCycle42_NegativeAdversarialBoundary(t *testing.T) {
	t.Run("GH957_StaleLockContentionReconciliation", func(t *testing.T) {
		verifyGH957NegativeStaleLockContention(t)
	})
	t.Run("GH944_EmptyScalarReferenceCoercion", func(t *testing.T) {
		verifyGH944NegativeEmptyScalarRefCoercion(t)
	})
}

func verifyGH957NegativeStaleLockContention(t *testing.T) {
	dir := t.TempDir()
	cas := filecas.NewContentAddressableStorage(dir, "backlog_item")
	lockPath := filepath.Join(dir, ".cas_sweep.lock")

	writeErr := fileutil.WriteStandardFile(lockPath, []byte("99999999"))
	require.NoError(t, writeErr)

	staleTime := time.Now().Add(-30 * time.Second)
	chtimeErr := fileutil.Chtimes(lockPath, staleTime, staleTime)
	require.NoError(t, chtimeErr)

	executed := false
	err := cas.WithSweepLock(func() error {
		executed = true
		return nil
	})
	require.NoError(t, err, "WithSweepLock must succeed and reclaim stale lock")
	assert.True(t, executed, "callback must execute after clearing stale lock")
	assert.False(t, fileutil.Exists(lockPath), "stale lock file must be cleaned up on return")
}

func verifyGH944NegativeEmptyScalarRefCoercion(t *testing.T) {
	obj := map[string]any{
		"kind":              objects.KindBacklogItem,
		"title":             "Cycle 42 Test Item",
		"priority_plan_ref": "",
		"milestone_ref":     "   ",
		"parent_id":         "",
		"valid_ref":         "PRI-042",
		"criteria_refs":     []string{"CRIT-001"},
	}

	objects.CoerceMutationFields(objects.KindBacklogItem, obj)

	assert.NotContains(t, obj, "priority_plan_ref", "empty string priority_plan_ref must be stripped")
	assert.NotContains(t, obj, "milestone_ref", "whitespace-only milestone_ref must be stripped")
	assert.NotContains(t, obj, "parent_id", "empty parent_id scalar ref must be stripped")
	assert.Equal(t, "PRI-042", obj["valid_ref"], "non-empty reference must be preserved")
	assert.Equal(t, []string{"CRIT-001"}, obj["criteria_refs"], "non-scalar reference list must be preserved")
}
