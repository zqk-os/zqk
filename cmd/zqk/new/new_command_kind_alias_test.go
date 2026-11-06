package newcmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cli "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestNewCmd_KindAliases verifies kind alias parity across zqk new and object new CLI entrypoints.
// Covers:
// - CRIT-1793968574418837000-610b10ad (Static Doc Invariant: docs/guides/CLI_KIND_ALIAS_PARITY.md)
// - CRIT-1793968577752769000-e83e756c (Operational Dynamic Proof: resolution of tst, tc, crit, req, pri, bli, mil, goal)
// - CRIT-1793968580513447000-d6c49c73 (Negative Adversarial Boundary: helpful error suggesting valid aliases & canonical kinds)
func TestNewCmd_KindAliases(t *testing.T) {
	t.Run("StaticDocInvariant_CLI_KIND_ALIAS_PARITY", func(t *testing.T) {
		repoRoot := cli.ResolveProjectRoot(".")
		docPath := filepath.Join(repoRoot, "docs/guides/CLI_KIND_ALIAS_PARITY.md")
		data, err := fileutil.ReadFile(docPath)
		require.NoError(t, err, "docs/guides/CLI_KIND_ALIAS_PARITY.md must exist")
		content := string(data)

		for _, requiredToken := range []string{"tst", "tc", "crit", "req", "pri", "bli", "mil", "goal"} {
			assert.True(t, strings.Contains(content, requiredToken),
				"CLI_KIND_ALIAS_PARITY.md must define kind alias %q", requiredToken)
		}
	})

	t.Run("OperationalDynamicProof_CanonicalResolution", func(t *testing.T) {
		expectedMappings := map[string]string{
			"tst":  objects.KindTestCase,
			"tc":   objects.KindTestCase,
			"pri":  objects.KindPriorityPlan,
			"plan": objects.KindPriorityPlan,
			"crit": objects.KindCriteria,
			"req":  objects.KindRequirement,
			"bli":  objects.KindBacklogItem,
			"mil":  objects.KindMilestone,
			"goal": objects.KindGoal,
		}

		for alias, expectedKind := range expectedMappings {
			resolved := objects.GetCanonicalKind(alias)
			assert.Equal(t, expectedKind, resolved, "GetCanonicalKind(%q) should resolve to %q", alias, expectedKind)
		}
	})

	t.Run("OperationalDynamicProof_DirectConvenienceRouting", func(t *testing.T) {
		root := NewNewCmd()

		aliasToExpectedCommand := map[string]string{
			"tst":  objects.KindTestCase,
			"tc":   objects.KindTestCase,
			"pri":  objects.KindPriorityPlan,
			"plan": objects.KindPriorityPlan,
			"crit": objects.KindCriteria,
			"req":  objects.KindRequirement,
			"bli":  objects.KindBacklogItem,
			"mil":  objects.KindMilestone,
			"goal": objects.KindGoal,
		}

		for alias, expectedCmdName := range aliasToExpectedCommand {
			cmd, _, err := root.Find([]string{alias})
			require.NoError(t, err, "root.Find([]string{%q}) should succeed", alias)
			require.NotNil(t, cmd, "command for %q should not be nil", alias)
			assert.Equal(t, expectedCmdName, cmd.Name(), "alias %q should route to %q command", alias, expectedCmdName)
		}
	})

	t.Run("OperationalDynamicProof_MintDraftViaAliases", func(t *testing.T) {
		testRoot := setupTestProjectRoot(t)
		require.NotEmpty(t, testRoot)

		testCases := []struct {
			subcommand   string
			expectedKind string
			title        string
		}{
			{"tst", objects.KindTestCase, "Verification Case Via Tst"},
			{"tc", objects.KindTestCase, "Verification Case Via Tc"},
			{"pri", objects.KindPriorityPlan, "Cycle Plan Via Pri"},
			{"req", objects.KindRequirement, "Contract Via Req"},
			{"crit", objects.KindCriteria, "Proof Via Crit"},
			{"bli", objects.KindBacklogItem, "Effort Unit Via Bli"},
			{"mil", objects.KindMilestone, "Boundary Via Mil"},
			{"goal", objects.KindGoal, "Strategic Direction Via Goal"},
		}

		for _, tc := range testCases {
			var outBuf bytes.Buffer
			ctx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)

			cmd := NewNewCmd()
			cmd.SetContext(ctx)
			cmd.SetArgs([]string{tc.subcommand, "--title", tc.title})

			err := cmd.Execute()
			require.NoError(t, err, "zqk new %s should succeed", tc.subcommand)

			draft := readLatestDraft(t, testRoot, tc.expectedKind)
			require.NotNil(t, draft, "draft for %s should exist on draft plane", tc.expectedKind)
			assert.Equal(t, tc.expectedKind, draft[objects.FieldKeyKind])
			assert.Equal(t, tc.title, draft[objects.FieldKeyTitle])
		}
	})

	t.Run("OperationalDynamicProof_NewObjectAliasParity", func(t *testing.T) {
		testRoot := setupTestProjectRoot(t)
		require.NotEmpty(t, testRoot)

		aliasCases := []struct {
			alias        string
			expectedKind string
			title        string
		}{
			{"tst", objects.KindTestCase, "Object Test Via Tst"},
			{"pri", objects.KindPriorityPlan, "Object Plan Via Pri"},
			{"crit", objects.KindCriteria, "Object Crit Via Crit"},
			{"req", objects.KindRequirement, "Object Req Via Req"},
			{"bli", objects.KindBacklogItem, "Object BLI Via Bli"},
			{"mil", objects.KindMilestone, "Object Mil Via Mil"},
		}

		for _, tc := range aliasCases {
			var outBuf bytes.Buffer
			ctx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)

			cmd := NewNewCmd()
			cmd.SetContext(ctx)
			cmd.SetArgs([]string{"object", tc.alias, "--title", tc.title})

			err := cmd.Execute()
			require.NoError(t, err, "zqk new object %s should succeed", tc.alias)

			draft := readLatestDraft(t, testRoot, tc.expectedKind)
			require.NotNil(t, draft, "draft for %s should exist on draft plane", tc.expectedKind)
			assert.Equal(t, tc.expectedKind, draft[objects.FieldKeyKind])
			assert.Equal(t, tc.title, draft[objects.FieldKeyTitle])
		}
	})

	t.Run("NegativeBoundary_UnrecognizedKindAlias", func(t *testing.T) {
		testRoot := setupTestProjectRoot(t)
		require.NotEmpty(t, testRoot)

		// 1. Calling objects.ResolveAndValidateKindForProject directly
		_, err := objects.ResolveAndValidateKindForProject(testRoot, "unrecognized_kind_xyz")
		require.Error(t, err, "unrecognized kind alias should be rejected")
		errMsg := err.Error()
		assert.Contains(t, errMsg, "unknown kind \"unrecognized_kind_xyz\"")
		assert.Contains(t, errMsg, "valid canonical kinds include")
		assert.Contains(t, errMsg, "valid aliases include")

		// 2. Calling zqk new object with unrecognized kind alias
		var outBuf bytes.Buffer
		ctx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)

		cmd := NewNewCmd()
		cmd.SetContext(ctx)
		cmd.SetArgs([]string{"object", "unrecognized_kind_xyz", "--title", "Should Fail"})

		execErr := cmd.Execute()
		require.Error(t, execErr, "zqk new object unrecognized_kind_xyz should fail")
		execErrMsg := execErr.Error()
		assert.Contains(t, execErrMsg, "unknown kind \"unrecognized_kind_xyz\"")
		assert.Contains(t, execErrMsg, "valid canonical kinds include")
		assert.Contains(t, execErrMsg, "valid aliases include")
	})
}
