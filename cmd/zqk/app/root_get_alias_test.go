package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/cmd/zqk/app"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestRootGetAlias_Registration(t *testing.T) {
	root := app.NewRootCommand()
	require.NotNil(t, root)

	getCmd, _, err := root.Find([]string{"get"})
	require.NoError(t, err)
	require.NotNil(t, getCmd)

	assert.Equal(t, "get", getCmd.Name())
	assert.Equal(t, "everyday", getCmd.GroupID)
	assert.Contains(t, getCmd.Aliases, "show")
	assert.Contains(t, getCmd.Aliases, "view")

	// Verify supported flags exist on the get alias (local or inherited)
	assert.NotNil(t, getCmd.Flag("format"))
	assert.NotNil(t, getCmd.Flag("fields"))
	assert.NotNil(t, getCmd.Flag("view"))
	assert.NotNil(t, getCmd.Flag("link-hydration"))
	assert.NotNil(t, getCmd.Flag("kind"))
	assert.NotNil(t, getCmd.Flag("write-resolved-sidecar"))
}

func TestRootGetAlias_ResolvesObjectIdenticallyToObjectGet(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-root-get-test-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = fileutil.RemoveAll(tmpDir)
	})

	projectRoot, err := setupAppTestEnvironmentRoot(tmpDir)
	require.NoError(t, err)

	err = copySpecsToTestRootApp(projectRoot)
	require.NoError(t, err)

	processBase := paths.ResolvePathFromCacheOrConstant(projectRoot, "process", paths.ProcessDir)
	backlogDir := filepath.Join(processBase, objects.GetDirectoryFromKind(objects.KindBacklogItem))
	err = fileutil.MkdirAll(backlogDir, paths.DirPerm755)
	require.NoError(t, err)

	originalTestRoot := zqkenv.TestRoot().Get()
	_ = zqkenv.TestRoot().Set(projectRoot)
	t.Cleanup(func() {
		if originalTestRoot != app.EmptyValue {
			_ = zqkenv.TestRoot().Set(originalTestRoot)
		} else {
			_ = zqkenv.TestRoot().Unset()
		}
	})

	objectID := fmt.Sprintf("BLI-%d", time.Now().UnixNano()%1000000)
	obj := map[string]any{
		objects.FieldKeyID:            objectID,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "Root Get Alias Equivalence Verification",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	data, err := yaml.Marshal(obj)
	require.NoError(t, err)

	objFile := filepath.Join(tmpDir, objectID+".yaml")
	err = fileutil.WriteFile(objFile, data, paths.FilePerm644)
	require.NoError(t, err)

	// Create object in CAS
	root := app.NewRootCommand()
	ctx := pkgctx.WithCacheMode(context.Background(), pkgctx.CacheModeTestSync)
	root.SetContext(ctx)
	root.SetArgs([]string{"object", "create", objects.KindBacklogItem, "--file", objFile})
	err = root.Execute()
	require.NoError(t, err)

	st, err := storage.NewFileObjectStorage(projectRoot)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, projectRoot, st)
	testkit.RegisterTempProjectTeardown(t, projectRoot, st)

	// 1. Compare JSON output between `zqk get <ID>` and `zqk object get <ID>`
	var bufRootJSON bytes.Buffer
	rootJSONCmd := app.NewRootCommand()
	rootJSONCmd.SetContext(pkgctx.WithCommandOutputWriter(ctx, &bufRootJSON))
	rootJSONCmd.SetOut(&bufRootJSON)
	rootJSONCmd.SetErr(&bufRootJSON)
	rootJSONCmd.SetArgs([]string{"get", objectID, "--format", "json"})
	err = rootJSONCmd.Execute()
	require.NoError(t, err, "zqk get failed: %s", bufRootJSON.String())

	var bufObjectJSON bytes.Buffer
	objectJSONCmd := app.NewRootCommand()
	objectJSONCmd.SetContext(pkgctx.WithCommandOutputWriter(ctx, &bufObjectJSON))
	objectJSONCmd.SetOut(&bufObjectJSON)
	objectJSONCmd.SetErr(&bufObjectJSON)
	objectJSONCmd.SetArgs([]string{"object", "get", objectID, "--format", "json"})
	err = objectJSONCmd.Execute()
	require.NoError(t, err, "zqk object get failed: %s", bufObjectJSON.String())

	var parsedRootJSON, parsedObjectJSON map[string]any
	err = json.Unmarshal(bufRootJSON.Bytes(), &parsedRootJSON)
	require.NoError(t, err, "unmarshal root JSON: %s", bufRootJSON.String())
	err = json.Unmarshal(bufObjectJSON.Bytes(), &parsedObjectJSON)
	require.NoError(t, err, "unmarshal object JSON: %s", bufObjectJSON.String())

	assert.Equal(t, objectID, parsedRootJSON[objects.FieldKeyID])
	assert.Equal(t, parsedObjectJSON, parsedRootJSON, "zqk get and zqk object get should return identical JSON")

	// 2. Compare YAML output between `zqk get <ID>` and `zqk object get <ID>`
	var bufRootYAML bytes.Buffer
	rootYAMLCmd := app.NewRootCommand()
	rootYAMLCmd.SetContext(pkgctx.WithCommandOutputWriter(ctx, &bufRootYAML))
	rootYAMLCmd.SetOut(&bufRootYAML)
	rootYAMLCmd.SetErr(&bufRootYAML)
	rootYAMLCmd.SetArgs([]string{"get", objectID, "--format", "yaml"})
	err = rootYAMLCmd.Execute()
	require.NoError(t, err, "zqk get yaml failed: %s", bufRootYAML.String())

	var bufObjectYAML bytes.Buffer
	objectYAMLCmd := app.NewRootCommand()
	objectYAMLCmd.SetContext(pkgctx.WithCommandOutputWriter(ctx, &bufObjectYAML))
	objectYAMLCmd.SetOut(&bufObjectYAML)
	objectYAMLCmd.SetErr(&bufObjectYAML)
	objectYAMLCmd.SetArgs([]string{"object", "get", objectID, "--format", "yaml"})
	err = objectYAMLCmd.Execute()
	require.NoError(t, err, "zqk object get yaml failed: %s", bufObjectYAML.String())

	var parsedRootYAML, parsedObjectYAML map[string]any
	err = yaml.Unmarshal(bufRootYAML.Bytes(), &parsedRootYAML)
	require.NoError(t, err, "unmarshal root YAML: %s", bufRootYAML.String())
	err = yaml.Unmarshal(bufObjectYAML.Bytes(), &parsedObjectYAML)
	require.NoError(t, err, "unmarshal object YAML: %s", bufObjectYAML.String())

	assert.Equal(t, parsedObjectYAML, parsedRootYAML, "zqk get and zqk object get should return identical YAML")
}
