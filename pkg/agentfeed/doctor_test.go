package agentfeed_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
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
		require.True(t, res.ContractPathsOK)
		foundMCP := false
		for _, issue := range res.Issues {
			if strings.Contains(issue, "mcp_query_failed") {
				foundMCP = true
				break
			}
		}
		require.True(t, foundMCP, "expected mcp_query_failed, issues=%v", res.Issues)
		// Feed file present → healthy unless sovereignty/stalled awaits degrade it
		// (global ~/.cursor/mcp.json pin can mark temp roots hijacked).
		if res.NodeSovereignty.Severity == "ok" {
			require.Equal(t, "healthy", res.FeedHealth)
		}
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

	t.Run("Stalled open awaits reports degraded feed health", func(t *testing.T) {
		awaitsPath := paths.PeerAckAwaitsPath(tempDir)
		require.NoError(t, fileutil.EnsureDir(filepath.Dir(awaitsPath)))
		staleTime := time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339)
		awaitsContent := fmt.Sprintf(`{"awaits":[{"id":"AWAIT-1","event_id":"AFE-1","from_agent_id":"agent-1","action":"wake","status":"open","created_at":"%s"}]}`, staleTime)
		require.NoError(t, fileutil.WriteStandardFile(awaitsPath, []byte(awaitsContent)))

		res := agentfeed.InspectFeed(agentfeed.DoctorOptions{
			ProjectRoot: tempDir,
		})
		require.Equal(t, "degraded", res.FeedHealth)
		foundIssue := false
		for _, issue := range res.Issues {
			if strings.Contains(issue, "feed_unacked_awaits_stalled") {
				foundIssue = true
				break
			}
		}
		require.True(t, foundIssue, "expected feed_unacked_awaits_stalled in issues")
	})
}

func TestInspectFeed_ReconcilesStaleSeatPIDFromHeartbeat(t *testing.T) {
	// Doctor must track seat-worker heartbeats.
	root := t.TempDir()
	feedPath := datacell.AgentChatChannelEventsJSONLPath(root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(feedPath)))
	require.NoError(t, fileutil.WriteStandardFile(feedPath, []byte("{\"event_id\":\"1\"}\n")))

	self := os.Getpid()
	deadPID := 999999
	require.NoError(t, agentfeed.SavePeerSeats(root, agentfeed.PeerSeatsFile{Seats: map[string]agentfeed.PeerSeatRecord{
		"peer-agent-1": {PID: deadPID, Note: "stale after emergency restart"},
	}}))
	_, err := agentfeed.WriteSeatWorkerAlive(root, "peer-agent-1", "sess-reconcile", time.Now().UTC())
	require.NoError(t, err)

	res := agentfeed.InspectFeed(agentfeed.DoctorOptions{ProjectRoot: root, MCPSubscribers: 1})
	require.GreaterOrEqual(t, res.PeerSeatsLive, 1, "issues=%v", res.Issues)
	loaded, err := agentfeed.LoadPeerSeats(root)
	require.NoError(t, err)
	require.Equal(t, self, loaded.Seats["peer-agent-1"].PID)
	for _, issue := range res.Issues {
		require.NotContains(t, issue, "peer_seat_stale_pid: peer-agent-1")
	}
}

func TestInspectFeed_PrivilegedWriterSocketMissing(t *testing.T) {
	root := t.TempDir()
	feedPath := datacell.AgentChatChannelEventsJSONLPath(root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(feedPath)))
	require.NoError(t, fileutil.WriteStandardFile(feedPath, []byte("{\"event_id\":\"1\"}\n")))

	res := agentfeed.InspectFeed(agentfeed.DoctorOptions{ProjectRoot: root})
	found := false
	for _, issue := range res.Issues {
		if strings.Contains(issue, "privileged_writer_socket_missing:") {
			found = true
			break
		}
	}
	require.True(t, found, "expected privileged_writer_socket_missing, issues=%v", res.Issues)
}

func TestVendorPathPolicy_FindLeak(t *testing.T) {
	t.Parallel()
	p := agentfeed.VendorPathPolicy{ForbiddenMarkers: []string{".gemini", ".windsurf"}}
	if m, ok := p.FindLeak("/tmp/proj/.windsurf/brain"); !ok || m != ".windsurf" {
		t.Fatalf("got marker=%q ok=%v", m, ok)
	}
	if _, ok := p.FindLeak(filepath.Join("/tmp/proj", paths.ProjectDataDir, paths.AgentRuntimeDir)); ok {
		t.Fatal("project data path must not leak")
	}
}
