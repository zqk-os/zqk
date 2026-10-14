package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func execUI(t *testing.T, root string, ctx context.Context, args ...string) (string, error) {
	t.Helper()
	cmd := NewUICmd()
	if root != "" {
		cli.SetContext(cmd, cli.ContextForProjectRoot(root))
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

func TestNewUICmd_Metadata(t *testing.T) {
	cmd := NewUICmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "ui", cmd.Name())
	assert.Contains(t, cmd.Aliases, "dashboard")
	assert.Equal(t, "seismograph", cmd.Flag("tab").DefValue)
	assert.Equal(t, "false", cmd.Flag("web").DefValue)
	assert.Equal(t, "8080", cmd.Flag("port").DefValue)
}

func TestUICmd_StructuredFormats(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	for _, fmtName := range []string{"json", "yaml", "jsonl"} {
		out, err := execUI(t, proj.Root, nil, "--format", fmtName, "--tab", "state")
		require.NoError(t, err)
		assert.NotEmpty(t, out)
		if fmtName == "json" {
			var data map[string]any
			require.NoError(t, json.Unmarshal([]byte(out), &data))
		}
	}
}

func TestUICmd_TabVariations(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	for _, tab := range []string{"state", "audit", "swarm", "pm", "metrics", "scheduler", "health", "seismograph"} {
		out, err := execUI(t, proj.Root, nil, "--format", "json", "--tab", tab)
		require.NoError(t, err)
		assert.NotEmpty(t, out)
	}
}

func TestUICmd_Web_ExplicitPortInUse(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	_, err = execUI(t, proj.Root, nil, "--web", "--port", fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is already in use")
}

func TestUICmd_Web_AllFallbackPortsInUse(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	var listeners []net.Listener
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()
	for p := 8080; p <= 8095; p++ {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			t.Skipf("cannot bind all ports: %v", err)
			return
		}
		listeners = append(listeners, l)
	}
	_, err := execUI(t, proj.Root, nil, "--web")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ports 8080-8095 are already in use")
}

func TestUICmd_Web_ExplicitPortSuccess(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	ctx, cancel := context.WithCancel(context.Background())
	goroutinelabels.NewGoroutine("test_ui_cancel", "cancel").StartSimple(func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	})
	out, err := execUI(t, proj.Root, ctx, "--web", "--port", fmt.Sprintf("%d", port))
	require.NoError(t, err)
	assert.Contains(t, out, fmt.Sprintf("Visual Studio running at http://127.0.0.1:%d", port))
}

func TestUICmd_Web_AutoFallbackSuccess(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	l8080, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		t.Skipf("cannot bind 8080: %v", err)
		return
	}
	defer l8080.Close()

	ctx, cancel := context.WithCancel(context.Background())
	goroutinelabels.NewGoroutine("test_ui_fallback_cancel", "cancel").StartSimple(func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	})
	out, err := execUI(t, proj.Root, ctx, "--web")
	require.NoError(t, err)
	assert.Contains(t, out, "Port 8080 was in use; automatically selected port 8081")
}

func TestUICmd_ProjectRoot_Fallback(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{SeedSchemaPlane: true})
	t.Chdir(proj.Root)
	out, err := execUI(t, "", nil, "--format", "json", "--tab", "state")
	require.NoError(t, err)
	assert.NotEmpty(t, out)
}
