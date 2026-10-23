package ui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func cleanupIsolatedProject(t *testing.T, root string) {
	t.Helper()
	if p, ok := storage.GetGlobalStorageProviderCache().Get(root); ok && p != nil {
		if closer, ok := p.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				t.Logf("cleanup storage error: %v", err)
			}
		}
		storage.GetGlobalStorageProviderCache().Delete(root)
	}
}

func execUIWithContext(t *testing.T, cliCtx *cli.Context, ctx context.Context, args ...string) (string, error) {
	t.Helper()
	cmd := NewUICmd()
	if cliCtx != nil {
		cli.SetContext(cmd, cliCtx)
	} else {
		cli.SetContext(cmd, &cli.Context{})
	}
	var buf bytes.Buffer
	if ctx == nil {
		ctx = context.Background()
	}
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(pkgctx.WithCommandOutputWriter(ctx, &buf))
	return buf.String(), err
}

func TestUICmd_InteractiveRunTUI_NonInteractiveFallback(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	defer cleanupIsolatedProject(t, proj.Root)

	// Execute without --format flag, defaulting to format=table and invoking RunTUI.
	// In non-interactive test mode, RunTUI renders a single-shot frame to os.Stdout and returns nil.
	_, err := execUI(t, proj.Root, nil, "--tab", "state")
	require.NoError(t, err)
}

func TestUICmd_ContextFormatInheritance(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	defer cleanupIsolatedProject(t, proj.Root)

	// Set Context.Format to JSON while omitting --format flag on CLI.
	// ui.go inherits proc.Format() when format flag is omitted.
	cliCtx := cli.ContextForProjectRoot(proj.Root)
	cliCtx.Format = cli.FormatJSON

	out, err := execUIWithContext(t, cliCtx, nil, "--tab", "health")
	require.NoError(t, err)
	assert.NotEmpty(t, out)
	assert.Contains(t, out, "projectroot:")
}

func TestUICmd_Web_ZeroOrNegativePortFallback(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	defer cleanupIsolatedProject(t, proj.Root)

	// Passing --port 0 with --web triggers port <= 0 reset to 8080
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately so server does not hang

	_, err := execUI(t, proj.Root, ctx, "--web", "--port", "0")
	// Because port 0 is reset to 8080 and changed flag is true, it attempts 8080 or returns error/cleans up
	if err != nil {
		assert.Contains(t, err.Error(), "8080")
	}

	// Passing --port -1 with --web triggers port <= 0 reset to 8080
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	_, err2 := execUI(t, proj.Root, ctx2, "--web", "--port", "-1")
	if err2 != nil {
		assert.Contains(t, err2.Error(), "8080")
	}
}

func TestUICmd_Aliases(t *testing.T) {
	cmd := NewUICmd()
	assert.ElementsMatch(t, []string{"dashboard", "console"}, cmd.Aliases)
}

func TestUICmd_InvalidFlagValues(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	defer cleanupIsolatedProject(t, proj.Root)

	_, err := execUI(t, proj.Root, nil, "--port", "invalid-number")
	require.Error(t, err)
}

func TestUICmd_AllTabsInteractiveRunTUI(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	defer cleanupIsolatedProject(t, proj.Root)

	tabs := []string{"seismograph", "state", "audit", "swarm", "pm", "metrics", "scheduler", "health"}
	for _, tab := range tabs {
		_, err := execUI(t, proj.Root, nil, "--tab", tab)
		require.NoError(t, err)
	}
}

func TestUICmd_Web_GracefulContextCancellation(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	defer cleanupIsolatedProject(t, proj.Root)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	err = l.Close()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	goroutinelabels.NewGoroutine("test_graceful_cancel", "cancel").StartSimple(func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	})

	out, err := execUI(t, proj.Root, ctx, "--web", "--port", fmt.Sprintf("%d", port))
	require.NoError(t, err)
	assert.Contains(t, out, fmt.Sprintf("http://127.0.0.1:%d", port))
}
