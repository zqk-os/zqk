package spec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestElevation_NewSpecCmd_Structure(t *testing.T) {
	cmd := NewSpecCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "spec", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)

	subcommands := cmd.Commands()
	subNames := make([]string, len(subcommands))
	for i, sc := range subcommands {
		subNames[i] = sc.Name()
	}
	assert.Contains(t, subNames, "list")
	assert.Contains(t, subNames, "get")
	assert.Contains(t, subNames, "fields")
}

func TestElevation_RunSpecFields_TableFormatAndAttributes(t *testing.T) {
	projectRoot, storageProvider := setupSpecTestProject(t)

	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))

	sampleSpec := `schema_version: "2.0.0"
ontology: "widget"
description: "A test widget object specification"
fields:
  name:
    type: "string"
    semantic_type: "title"
    permissions: "rwx"
    checklist:
      purpose: "Widget title description"
    validation:
      required: true
  description:
    type: "string"
    permissions: "rw"
    validation:
      required: false
  counter:
    type: "integer"
    validation:
      required: false
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "widget.yaml"), []byte(sampleSpec), paths.FilePerm644))

	// 1. Table format (default format = "")
	cmdTable := createSpecFieldsTestCommand(t, projectRoot, storageProvider, "")
	err := runSpecFields(cmdTable, []string{"SPEC-widget"})
	require.NoError(t, err)

	// 2. YAML format
	cmdYAML := createSpecFieldsTestCommand(t, projectRoot, storageProvider, cli.FormatYAML)
	err = runSpecFields(cmdYAML, []string{"widget"})
	require.NoError(t, err)

	// 3. Raw flag
	cmdRaw := createSpecFieldsTestCommand(t, projectRoot, storageProvider, cli.FormatJSON)
	_ = cmdRaw.Flags().Set("raw", "true")
	err = runSpecFields(cmdRaw, []string{"WIDGET"})
	require.NoError(t, err)

	// 4. Non-existent kind error
	cmdErr := createSpecFieldsTestCommand(t, projectRoot, storageProvider, cli.FormatTable)
	err = runSpecFields(cmdErr, []string{"non_existent_kind_abc"})
	require.Error(t, err)
}

func TestElevation_RunSpecGet_TableFormatAndFullMetadata(t *testing.T) {
	projectRoot, storageProvider := setupSpecTestProject(t)

	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))

	sampleSpec := `schema_version: "2.0.0"
ontology: "gadget"
extends: "base_object"
composes:
  - "auditable"
  - "traceable"
visibility: "public"
storage_profile: "standard"
id_prefixes:
  - "GDG"
id_synonyms:
  - "device"
description: "Comprehensive gadget specification for elevation testing"
traits:
  - "immutable_id"
  - "temporal_audit"
fields:
  id:
    type: "string"
    validation:
      required: true
  title:
    type: "string"
    validation:
      required: true
  notes:
    type: "string"
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "gadget.yaml"), []byte(sampleSpec), paths.FilePerm644))

	// 1. Table format (format = "")
	cmdTable := createSpecGetTestCommand(t, projectRoot, storageProvider, "")
	err := runSpecGet(cmdTable, []string{"SPEC-gadget"})
	require.NoError(t, err)

	// 2. YAML format
	cmdYAML := createSpecGetTestCommand(t, projectRoot, storageProvider, cli.FormatYAML)
	err = runSpecGet(cmdYAML, []string{"gadget"})
	require.NoError(t, err)

	// 3. Raw flag
	cmdRaw := createSpecGetTestCommand(t, projectRoot, storageProvider, cli.FormatJSON)
	_ = cmdRaw.Flags().Set("raw", "true")
	err = runSpecGet(cmdRaw, []string{"GADGET"})
	require.NoError(t, err)

	// 4. Non-existent kind
	cmdNotFound := createSpecGetTestCommand(t, projectRoot, storageProvider, cli.FormatTable)
	err = runSpecGet(cmdNotFound, []string{"unknown_kind_404"})
	require.Error(t, err)
}

func TestElevation_RunSpecList_FormatsAndFallback(t *testing.T) {
	projectRoot, storageProvider := setupSpecTestProject(t)

	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))

	sampleSpec := `schema_version: "2.0.0"
ontology: "device"
description: "A test device"
fields:
  name:
    type: "string"
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "device.yaml"), []byte(sampleSpec), paths.FilePerm644))

	// 1. Table format
	cmdTable := createSpecListTestCommand(t, projectRoot, storageProvider)
	ctxTable := cli.GetContext(cmdTable)
	ctxTable.Format = cli.FormatTable
	cli.SetContext(cmdTable, ctxTable)
	err := runSpecList(cmdTable, nil)
	require.NoError(t, err)

	// 2. YAML format
	cmdYAML := createSpecListTestCommand(t, projectRoot, storageProvider)
	ctxYAML := cli.GetContext(cmdYAML)
	ctxYAML.Format = cli.FormatYAML
	cli.SetContext(cmdYAML, ctxYAML)
	err = runSpecList(cmdYAML, nil)
	require.NoError(t, err)

	// 3. JSON format
	cmdJSON := createSpecListTestCommand(t, projectRoot, storageProvider)
	ctxJSON := cli.GetContext(cmdJSON)
	ctxJSON.Format = cli.FormatJSON
	cli.SetContext(cmdJSON, ctxJSON)
	err = runSpecList(cmdJSON, nil)
	require.NoError(t, err)
}

func TestElevation_ResolveSpecAndFields_Normalizations(t *testing.T) {
	projectRoot, storageProvider := setupSpecTestProject(t)

	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))

	sampleSpec := `schema_version: "2.0.0"
ontology: "component"
description: "Component specification"
fields:
  id:
    type: "string"
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "component.yaml"), []byte(sampleSpec), paths.FilePerm644))

	cmd := createSpecGetTestCommand(t, projectRoot, storageProvider, cli.FormatJSON)
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	// Normal prefix stripping and case folding
	kind, spec, fields, err := resolveSpecAndFields(cmd, proc, "  SPEC-Component  ")
	require.NoError(t, err)
	assert.Equal(t, "component", kind)
	require.NotNil(t, spec)
	assert.Contains(t, fields, "id")

	// Missing spec returns error
	_, _, _, err = resolveSpecAndFields(cmd, proc, "missing_component_xyz")
	require.Error(t, err)
}
