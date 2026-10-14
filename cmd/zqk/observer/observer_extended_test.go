package observer

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func executeObserverCmd(cmd *cobra.Command, args []string) (string, error) {
	root := &cobra.Command{Use: "zqk"}
	root.PersistentFlags().StringP("format", "f", "", "Output format")
	root.AddCommand(cmd)

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	baseCtx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	root.SetContext(baseCtx)
	cmd.SetContext(baseCtx)

	root.SetArgs(append([]string{cmd.Name()}, args...))
	err := root.Execute()
	return buf.String(), err
}

func TestObserverExtract_Success(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Create sample Go file
	srcDir := filepath.Join(proj.Root, "sample")
	require.NoError(t, fileutil.MkdirAll(srcDir, paths.DirPerm755))
	sampleCode := `package sample

// Calculator calculates things
type Calculator struct {
	Value int
}

func (c *Calculator) Add(n int) int {
	return c.Value + n
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(srcDir, "sample.go"), []byte(sampleCode), paths.FilePerm644))

	out, err := executeObserverCmd(NewObserverCmd(), []string{"extract", "--dir", srcDir, "--format", "json"})
	require.NoError(t, err)

	assert.Contains(t, out, "Calculator")
	assert.Contains(t, out, "Add")
}

func TestObserverExtract_DefaultDir(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	sampleCode := `package rootpkg
type Item struct{}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(proj.Root, "item.go"), []byte(sampleCode), paths.FilePerm644))

	out, err := executeObserverCmd(newExtractCmd(), []string{"--dir", proj.Root, "--format", "json"})
	require.NoError(t, err)
	assert.Contains(t, out, "Item")
}

func TestObserverExtract_InvalidDir(t *testing.T) {
	_, err := executeObserverCmd(NewObserverCmd(), []string{"extract", "--dir", "/path/that/does/not/exist/12345"})
	require.Error(t, err)
}

func TestObserverPopulate_DisabledGraph(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.RawGraphEnabled().Name(), "false")

	_, err := executeObserverCmd(NewObserverCmd(), []string{"populate", "--dir", proj.Root})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "graph backend not enabled")
}

func TestObserverPopulate_MockGraph(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv("ZQK_GRAPH_ENABLED", "true")
	t.Setenv("ZQK_MOCK_GRAPH", "true")
	t.Setenv("GRAPH_ENABLED", "true")

	sampleCode := `package rootpkg
type Item struct{}
func (i *Item) Do() {}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(proj.Root, "item.go"), []byte(sampleCode), paths.FilePerm644))

	out, err := executeObserverCmd(NewObserverCmd(), []string{"populate", "--dir", proj.Root, "--extract-id", "test-extract-1", "--format", "json"})
	require.NoError(t, err)
	assert.Contains(t, out, "NodesCreated")
}
