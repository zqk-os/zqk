package explain

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/acronyms"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// mockErrorStorage wraps storage.ObjectStorageProvider to simulate write/update errors
type mockErrorStorage struct {
	storage.ObjectStorageProvider
}

func (m *mockErrorStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return errors.New("simulated write fault on storage")
}

func (m *mockErrorStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	return errors.New("simulated update fault on storage")
}

// TestExplainCmd_CoverageElevation_SyncToKernelError tests error path when storage sync fails
func TestExplainCmd_CoverageElevation_SyncToKernelError(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})

	cmd := NewExplainCmd()
	var outBuf bytes.Buffer
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	errStore := &mockErrorStorage{ObjectStorageProvider: proj.FileStorage}
	cmd.SetContext(cli.WithStorageProvider(baseCtx, errStore))

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmd, cctx)

	cmd.SetArgs([]string{"--sync"})

	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to synchronize acronyms to kernel")
}

// TestExplainCmd_CoverageElevation_EmptySchemeFallback tests fallback to static ListAll when scheme has no stored terms
func TestExplainCmd_CoverageElevation_EmptySchemeFallback(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})

	cmd := NewExplainCmd()
	var outBuf bytes.Buffer
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmd, cctx)

	// List all under an empty scheme that does not exist in store -> fallback to ListAll
	cmd.SetArgs([]string{"--scheme", "VOC-NONEXISTENT-EMPTY"})

	err = cmd.Execute()
	require.NoError(t, err)
	output := outBuf.String()
	assert.Contains(t, output, "BLI")
	assert.Contains(t, output, "Backlog Item")
	assert.Contains(t, output, "PRI")
}

// TestExplainCmd_CoverageElevation_EmptySchemeFlag tests scheme flag provided as explicit empty string
func TestExplainCmd_CoverageElevation_EmptySchemeFlag(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})

	cmd := NewExplainCmd()
	var outBuf bytes.Buffer
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmd, cctx)

	// Passing --scheme "" triggers `if schemeFlag == ""` defaulting logic
	cmd.SetArgs([]string{"--scheme", "", "BLI"})

	err = cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "Backlog Item")
}

// TestExplainCmd_CoverageElevation_SingleItemWithoutRelatedRefs tests formatting when item has empty related refs
func TestExplainCmd_CoverageElevation_SingleItemWithoutRelatedRefs(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create parent scheme
	schemeObj := map[string]any{
		"id":             "VOC-STANDALONE-TERMS",
		"kind":           "vocabulary_scheme",
		"title":          "Standalone Terms",
		"status":         "active",
		"context_scope":  "operational",
		"purpose":        "mixed",
		"summary":        "Vocabulary terms without relations.",
		"namespace_id":   "zqk:kernel",
		"schema_version": "2.0.0",
		"source_type":    "internal",
	}
	require.NoError(t, proj.FileStorage.Create(t.Context(), secCtx, schemeObj))

	// Create term with NO related refs
	customTerm := acronyms.Acronym{
		Code:        "ISOLATED",
		FullName:    "Isolated Capability Term",
		SchemeRef:   "VOC-STANDALONE-TERMS",
		Category:    "concept",
		Definition:  "A term that has zero related references.",
		Context:     "architecture",
		RelatedRefs: nil,
	}
	require.NoError(t, proj.FileStorage.Create(t.Context(), secCtx, customTerm.ToGlossaryTerm()))

	cmd := NewExplainCmd()
	var outBuf bytes.Buffer
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, secCtx)
	cmd.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmd, cctx)

	cmd.SetArgs([]string{"--scheme", "VOC-STANDALONE-TERMS", "ISOLATED"})
	require.NoError(t, cmd.Execute())

	output := outBuf.String()
	assert.Contains(t, output, "ACRONYM    : ISOLATED")
	assert.Contains(t, output, "FULL NAME  : Isolated Capability Term")
	assert.NotContains(t, output, "RELATED    :")
}

// TestExplainCmd_CoverageElevation_UnknownAcronym_NoFuzzySuggestions tests error message when query has no close matches
func TestExplainCmd_CoverageElevation_UnknownAcronym_NoFuzzySuggestions(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})

	cmd := NewExplainCmd()
	var outBuf bytes.Buffer
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmd.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmd, cctx)

	// An utterly distinct string with zero Levenshtein proximity to existing acronyms
	cmd.SetArgs([]string{"ZZZZZZZZ_NO_MATCH_99999"})

	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown kernel acronym \"ZZZZZZZZ_NO_MATCH_99999\"")
	assert.NotContains(t, err.Error(), "Did you mean:")
}

// TestExplainCmd_CoverageElevation_FormatOptionsAndAliases tests format options and command aliases
func TestExplainCmd_CoverageElevation_FormatOptionsAndAliases(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})

	// 1. Verify aliases on root command
	cmd := NewExplainCmd()
	assert.Equal(t, "explain", cmd.Use)
	assert.Contains(t, cmd.Aliases, "glossary")
	assert.Contains(t, cmd.Aliases, "acronym")
	assert.Contains(t, cmd.Aliases, "acronyms")

	// 2. Format yaml fallback for list
	var yamlListBuf bytes.Buffer
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &yamlListBuf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmdYamlList := NewExplainCmd()
	cmdYamlList.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))
	cmdYamlList.SetArgs([]string{"-f", "yaml"})
	require.NoError(t, cmdYamlList.Execute())
	assert.Contains(t, yamlListBuf.String(), "BLI")

	// 3. Format yaml fallback for single item
	var yamlItemBuf bytes.Buffer
	baseCtx = pkgctx.WithCommandOutputWriter(context.Background(), &yamlItemBuf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	cmdYamlItem := NewExplainCmd()
	cmdYamlItem.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))
	cmdYamlItem.SetArgs([]string{"BLI", "-f", "yaml"})
	require.NoError(t, cmdYamlItem.Execute())
	assert.Contains(t, yamlItemBuf.String(), "ACRONYM    : BLI")

	// 4. Argument count constraint validation (maximum 1 arg allowed)
	cmdExtraArgs := NewExplainCmd()
	cmdExtraArgs.SetArgs([]string{"ARG1", "ARG2"})
	err := cmdExtraArgs.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts at most 1 arg(s)")
}

// TestExplainCmd_CoverageElevation_DynamicCustomSchemeJSON tests dynamic term lookup with JSON format
func TestExplainCmd_CoverageElevation_DynamicCustomSchemeJSON(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	secCtx := pkgctx.NewSystemSecurityContext()

	// Ensure parent vocabulary scheme
	schemeObj := map[string]any{
		"id":             "VOC-DYNAMIC-CATALOG",
		"kind":           "vocabulary_scheme",
		"title":          "Dynamic Catalog Taxonomy",
		"status":         "active",
		"context_scope":  "operational",
		"purpose":        "mixed",
		"summary":        "Catalog taxonomy for dynamic terms.",
		"namespace_id":   "zqk:kernel",
		"schema_version": "2.0.0",
		"source_type":    "internal",
	}
	require.NoError(t, proj.FileStorage.Create(t.Context(), secCtx, schemeObj))

	// Register two dynamic glossary terms
	termA := acronyms.Acronym{
		Code:        "DYNA",
		FullName:    "Dynamic Term Alpha",
		SchemeRef:   "VOC-DYNAMIC-CATALOG",
		Category:    "dynamic",
		Definition:  "First dynamically registered term.",
		Context:     "catalog",
		RelatedRefs: []string{"DYNB"},
	}
	termB := acronyms.Acronym{
		Code:        "DYNB",
		FullName:    "Dynamic Term Beta",
		SchemeRef:   "VOC-DYNAMIC-CATALOG",
		Category:    "dynamic",
		Definition:  "Second dynamically registered term.",
		Context:     "catalog",
		RelatedRefs: []string{"DYNA"},
	}
	require.NoError(t, proj.FileStorage.Create(t.Context(), secCtx, termA.ToGlossaryTerm()))
	require.NoError(t, proj.FileStorage.Create(t.Context(), secCtx, termB.ToGlossaryTerm()))

	// List in JSON with custom scheme
	cmdJSONList := NewExplainCmd()
	var listJSONBuf bytes.Buffer
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &listJSONBuf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, secCtx)
	cmdJSONList.SetContext(cli.WithStorageProvider(baseCtx, proj.FileStorage))

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmdJSONList, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = proj.Root
	cli.SetContext(cmdJSONList, cctx)

	cmdJSONList.SetArgs([]string{"--scheme", "VOC-DYNAMIC-CATALOG", "-f", "json"})
	require.NoError(t, cmdJSONList.Execute())

	jsonOut := listJSONBuf.String()
	assert.Contains(t, jsonOut, `"code": "DYNA"`)
	assert.Contains(t, jsonOut, `"full_name": "Dynamic Term Alpha"`)
	assert.Contains(t, jsonOut, `"code": "DYNB"`)
	assert.Contains(t, jsonOut, `"full_name": "Dynamic Term Beta"`)

	// Item lookup with custom scheme
	cmdJSONItem := NewExplainCmd()
	var itemJSONBuf bytes.Buffer
	baseCtxItem := pkgctx.WithCommandOutputWriter(context.Background(), &itemJSONBuf)
	baseCtxItem = pkgctx.WithSecurityContext(baseCtxItem, secCtx)
	cmdJSONItem.SetContext(cli.WithStorageProvider(baseCtxItem, proj.FileStorage))
	cli.SetContext(cmdJSONItem, cctx)

	cmdJSONItem.SetArgs([]string{"--scheme", "VOC-DYNAMIC-CATALOG", "DYNA", "-f", "json"})
	require.NoError(t, cmdJSONItem.Execute())

	itemOut := itemJSONBuf.String()
	assert.Contains(t, itemOut, `"code": "DYNA"`)
	assert.Contains(t, itemOut, `"full_name": "Dynamic Term Alpha"`)
}
