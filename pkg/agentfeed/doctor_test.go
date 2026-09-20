package agentfeed_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/stretchr/testify/require"
)

func TestInspectFeed_Doctor(t *testing.T) {
	tempDir := t.TempDir()

	feedPath := datacell.AgentChatChannelEventsJSONLPath(tempDir)
	policyPath := datacell.AgentChatChannelConfigPath(tempDir)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(feedPath)))
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(policyPath)))
	require.NoError(t, fileutil.WriteStandardFile(feedPath, []byte("{\"event_id\":\"123\"}\n")))
	require.NoError(t, fileutil.WriteStandardFile(policyPath, []byte("{\"enabled\":true}")))

	t.Run("Healthy paths", func(t *testing.T) {
		res := agentfeed.InspectFeed(agentfeed.DoctorOptions{
			ProjectRoot: tempDir,
			MCPQueryErr: errors.New("mcp connection refused"),
		})
		require.Equal(t, "healthy", res.FeedHealth)
		require.True(t, res.ContractPathsOK)
		require.Contains(t, res.Issues[0], "mcp_query_failed")
	})

	t.Run("Vendor path leak via policy marker", func(t *testing.T) {
		policy := agentfeed.VendorPathPolicy{ForbiddenMarkers: []string{".gemini", ".claude"}}
		require.NoError(t, fileutil.WriteStandardFile(policyPath, []byte("{\"enabled\":true, \"path\": \".claude/some/path\"}")))
		res := agentfeed.InspectFeed(agentfeed.DoctorOptions{
			ProjectRoot:    tempDir,
			MCPSubscribers: 1,
			VendorPolicy:   &policy,
		})
		require.False(t, res.ContractPathsOK)
		require.True(t, res.PeerWakeLive)
		require.Contains(t, res.Issues, "vendor_path_leak: lite policy contains .claude")
	})
}

func TestVendorPathPolicy_FindLeak(t *testing.T) {
	t.Parallel()
	p := agentfeed.VendorPathPolicy{ForbiddenMarkers: []string{".gemini", ".windsurf"}}
	if m, ok := p.FindLeak("/tmp/proj/.windsurf/brain"); !ok || m != ".windsurf" {
		t.Fatalf("got marker=%q ok=%v", m, ok)
	}
	if _, ok := p.FindLeak("/tmp/proj/.zqk/config"); ok {
		t.Fatal("project data path must not leak")
	}
}
