package semantic

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clictx "github.com/zqk-os/zqk/pkg/cliapp/context"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func executeSemanticWithFormat(t *testing.T, projectRoot, format string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("ZQK_PROJECT_ROOT", projectRoot)
	cmd := NewSemanticCmd()
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cliCtx := &cli.Context{
		Format: cli.OutputFormat(format),
		Context: &clictx.Context{
			ProjectRoot: projectRoot,
			Profile:     "debug",
			Format:      format,
		},
	}
	cli.SetContext(cmd, cliCtx)
	for _, sub := range cmd.Commands() {
		cli.SetContext(sub, cliCtx)
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func executeSemantic(t *testing.T, projectRoot string, args ...string) (string, error) {
	t.Helper()
	return executeSemanticWithFormat(t, projectRoot, "", args...)
}

func seedSemanticArtifacts(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, fileutil.WriteFile(filepath.Join(root, "sample.yaml"), []byte("key: value\n"), 0644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(root, "schema.json"), []byte(`{"$schema": "http://json-schema.org/draft-07/schema#"}`), 0644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(root, "ontology.owl"), []byte("<rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\"></rdf:RDF>"), 0644))
}

func seedConvergenceDrift(t *testing.T, store storage.ObjectStorageProvider) {
	t.Helper()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	session := map[string]any{
		objects.FieldKeyID:              "CVS-DRIFT-001",
		objects.FieldKeyKind:            objects.KindConvergenceSession,
		objects.FieldKeyTitle:           "Failing Drift Session",
		objects.FieldKeyStatus:          "active",
		objects.FieldKeyDeltaAssessment: "trending_away",
	}
	if err := store.Create(ctx, secCtx, session); err != nil {
		t.Fatalf("failed to seed convergence session: %v", err)
	}
}

func TestNewSemanticCmd_Hierarchy(t *testing.T) {
	cmd := NewSemanticCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "semantic", cmd.Name())

	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["assess"], "semantic must have assess subcommand")
	assert.True(t, subNames["recommend"], "semantic must have recommend subcommand")
	assert.True(t, subNames["infer"], "semantic must have infer subcommand")
}

func TestSemantic_Assess(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	seedSemanticArtifacts(t, proj.Root)

	// Table output with populated indicators
	out, err := executeSemantic(t, proj.Root, "assess")
	require.NoError(t, err)
	assert.Contains(t, out, "Semantic Maturity Assessment")
	assert.Contains(t, out, "Level:")
	assert.Contains(t, out, "Indicators:")

	// JSON output
	jsonOut, err := executeSemanticWithFormat(t, proj.Root, "json", "assess")
	require.NoError(t, err)
	assert.Contains(t, jsonOut, "level")

	// YAML output
	yamlOut, err := executeSemanticWithFormat(t, proj.Root, "yaml", "assess")
	require.NoError(t, err)
	assert.Contains(t, yamlOut, "level:")
}

func TestSemantic_Recommend(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	seedSemanticArtifacts(t, proj.Root)

	// Default output
	out, err := executeSemantic(t, proj.Root, "recommend")
	require.NoError(t, err)
	assert.Contains(t, out, "Semantic Maturity Recommendations")

	// JSON output
	jsonOut, err := executeSemanticWithFormat(t, proj.Root, "json", "recommend")
	require.NoError(t, err)
	assert.Contains(t, jsonOut, "level")
}

func TestSemantic_Infer(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	seedConvergenceDrift(t, proj.FileStorage)

	// Dry run table output with matching convergence drift inferences
	out, err := executeSemantic(t, proj.Root, "infer")
	require.NoError(t, err)
	assert.Contains(t, out, "Strategic Inferences & Proposed Backlog Items")
	assert.Contains(t, out, "Remediate Failing Drift Session")

	// Dry run JSON output via format
	jsonOut, err := executeSemanticWithFormat(t, proj.Root, "json", "infer")
	require.NoError(t, err)
	assert.Contains(t, jsonOut, "Remediate Failing Drift Session")

	// Filter with matching session-id
	sessionOut, err := executeSemantic(t, proj.Root, "infer", "--session-id", "CVS-DRIFT-001")
	require.NoError(t, err)
	assert.Contains(t, sessionOut, "Remediate Failing Drift Session")

	// Filter with non-matching session-id
	noMatchOut, err := executeSemantic(t, proj.Root, "infer", "--session-id", "non-existent-session")
	require.NoError(t, err)
	assert.Contains(t, noMatchOut, "No strategic inferences identified")

	// Apply inferences to backlog
	applyOut, err := executeSemantic(t, proj.Root, "infer", "--apply")
	require.NoError(t, err)
	assert.Contains(t, applyOut, "Successfully applied 1 inferences to the backlog")

	// Re-apply to exercise existing-object skip branch
	reapplyOut, err := executeSemantic(t, proj.Root, "infer", "--apply")
	require.NoError(t, err)
	assert.Contains(t, reapplyOut, "Successfully applied 0 inferences to the backlog")
}

func TestSemantic_Infer_Empty(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	// Empty project without matching heuristics
	out, err := executeSemantic(t, proj.Root, "infer")
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	// Apply on empty inferences
	applyOut, err := executeSemantic(t, proj.Root, "infer", "--apply")
	require.NoError(t, err)
	assert.Contains(t, applyOut, "No inferences to apply")
}
