package observer

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	observerpkg "github.com/zqk-os/zqk/pkg/observer"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	labelObserverTestGoroutine = "observer_test_subscriber"
	descObserverTestGoroutine  = "listening for observer lifecycle events"
)

func executeObserverCmdWithCtx(ctx context.Context, cmd *cobra.Command, args []string) (string, error) {
	root := &cobra.Command{Use: "zqk"}
	root.PersistentFlags().StringP("format", "f", "", "Output format")
	root.AddCommand(cmd)

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	baseCtx := pkgctx.WithCommandOutputWriter(ctx, &buf)
	root.SetContext(baseCtx)
	cmd.SetContext(baseCtx)

	root.SetArgs(append([]string{cmd.Name()}, args...))
	err := root.Execute()
	return buf.String(), err
}

func TestObserverExtract_SyntaxWarning(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Valid Go file
	validCode := `package sample
type SafeEntity struct{}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(proj.Root, "safe.go"), []byte(validCode), paths.FilePerm644))

	// Invalid Go file that yields a syntax warning in result.Errors
	brokenCode := `package sample
func Unfinished(
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(proj.Root, "broken.go"), []byte(brokenCode), paths.FilePerm644))

	out, err := executeObserverCmd(NewObserverCmd(), []string{"extract", "--dir", proj.Root, "--format", "json"})
	require.NoError(t, err)
	assert.Contains(t, out, "SafeEntity")
}

func TestObserverExtract_OmittedDirAndFormats(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	sampleCode := `package sample
type Widget struct {
	ID string
}
func (w *Widget) Inspect() string { return w.ID }
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(proj.Root, "widget.go"), []byte(sampleCode), paths.FilePerm644))

	t.Chdir(proj.Root)

	// Omit --dir so dir == emptyValue branch takes effect
	outYAML, err := executeObserverCmd(NewObserverCmd(), []string{"extract", "--format", "yaml"})
	require.NoError(t, err)
	assert.Contains(t, outYAML, "Widget")

	outTable, err := executeObserverCmd(NewObserverCmd(), []string{"extract", "--format", "table"})
	require.NoError(t, err)
	assert.NotEmpty(t, outTable)
}

func TestObserverExtract_CanceledContext(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := executeObserverCmdWithCtx(ctx, NewObserverCmd(), []string{"extract", "--dir", proj.Root})
	require.Error(t, err)
}

func TestObserverPopulate_SyntaxWarning(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv("ZQK_GRAPH_ENABLED", "true")
	t.Setenv("ZQK_MOCK_GRAPH", "true")
	t.Setenv("GRAPH_ENABLED", "true")

	validCode := `package sample
type Resource struct{}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(proj.Root, "resource.go"), []byte(validCode), paths.FilePerm644))

	brokenCode := `package sample
func Incomplete(
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(proj.Root, "broken.go"), []byte(brokenCode), paths.FilePerm644))

	out, err := executeObserverCmd(NewObserverCmd(), []string{"populate", "--dir", proj.Root, "--extract-id", "syntax-warn-1", "--format", "json"})
	require.NoError(t, err)
	assert.Contains(t, out, "NodesCreated")
}

func TestObserverPopulate_InvalidDir(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv("ZQK_GRAPH_ENABLED", "true")
	t.Setenv("ZQK_MOCK_GRAPH", "true")
	t.Setenv("GRAPH_ENABLED", "true")

	_, err := executeObserverCmd(NewObserverCmd(), []string{"populate", "--dir", "/path/that/does/not/exist/99999"})
	require.Error(t, err)
}

func TestObserverPopulate_Semantic_NoEligibleEntities(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv("ZQK_GRAPH_ENABLED", "true")
	t.Setenv("ZQK_MOCK_GRAPH", "true")
	t.Setenv("GRAPH_ENABLED", "true")

	// Only test files: no production entities eligible for semantic enhancement
	testCode := `package sample_test
import "testing"
func TestSample(t *testing.T) {}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(proj.Root, "sample_test.go"), []byte(testCode), paths.FilePerm644))

	out, err := executeObserverCmd(NewObserverCmd(), []string{"populate", "--dir", proj.Root, "--semantic", "--extract-id", "sem-empty-1", "--format", "json"})
	require.NoError(t, err)
	assert.Contains(t, out, "NodesCreated")
}

func TestObserverPopulate_Semantic_WithEntities(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv("ZQK_GRAPH_ENABLED", "true")
	t.Setenv("ZQK_MOCK_GRAPH", "true")
	t.Setenv("GRAPH_ENABLED", "true")

	// Production function entity with semantic enhancement enabled
	code := `package sample
func ComputeAction() int {
	return 100
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(proj.Root, "action.go"), []byte(code), paths.FilePerm644))

	out, err := executeObserverCmd(NewObserverCmd(), []string{"populate", "--dir", proj.Root, "--semantic", "--extract-id", "sem-action-1", "--format", "json"})
	require.NoError(t, err)
	assert.Contains(t, out, "NodesCreated")
}

func TestObserverPopulate_OmittedDirAndExtractID(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv("ZQK_GRAPH_ENABLED", "true")
	t.Setenv("ZQK_MOCK_GRAPH", "true")
	t.Setenv("GRAPH_ENABLED", "true")

	sampleCode := `package sample
type Record struct {
	Name string
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(proj.Root, "record.go"), []byte(sampleCode), paths.FilePerm644))

	t.Chdir(proj.Root)

	// Omit --dir and --extract-id to hit default dir and timestamp generation
	outYAML, err := executeObserverCmd(NewObserverCmd(), []string{"populate", "--format", "yaml"})
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(outYAML), "nodescreated")

	outTable, err := executeObserverCmd(NewObserverCmd(), []string{"populate", "--format", "table"})
	require.NoError(t, err)
	assert.NotEmpty(t, outTable)
}

func TestObserverEvents_SubscriberStreaming(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	sampleCode := `package sample
type Token struct{}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(proj.Root, "token.go"), []byte(sampleCode), paths.FilePerm644))

	eventChan := make(chan observerpkg.ObserverEvent, 10)
	observerpkg.DefaultEmitter.Subscribe(func(ctx context.Context, ev observerpkg.ObserverEvent) {
		select {
		case eventChan <- ev:
		default:
		}
	})

	listenerCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	receivedTypes := make(chan string, 10)
	goroutinelabels.NewGoroutine(labelObserverTestGoroutine, descObserverTestGoroutine).
		WithContext(listenerCtx).
		StartSimple(func() {
			for {
				select {
				case <-listenerCtx.Done():
					return
				case ev := <-eventChan:
					select {
					case receivedTypes <- ev.Type:
					case <-listenerCtx.Done():
						return
					}
				}
			}
		})

	out, err := executeObserverCmd(NewObserverCmd(), []string{"extract", "--dir", proj.Root, "--format", "json"})
	require.NoError(t, err)
	assert.Contains(t, out, "Token")

	// Verify that at least EventExtractStarted or EventExtractCompleted was received
	found := false
	timer := time.After(2 * time.Second)
	for !found {
		select {
		case evtType := <-receivedTypes:
			if evtType == observerpkg.EventExtractStarted || evtType == observerpkg.EventExtractCompleted {
				found = true
			}
		case <-timer:
			t.Fatal("timed out waiting for observer extract event")
		}
	}
	assert.True(t, found)
}
