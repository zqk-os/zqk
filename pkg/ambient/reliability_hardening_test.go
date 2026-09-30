package ambient_test

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/ambient"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/healthcheck"
	_ "github.com/zqk-os/zqk/pkg/healthcheck/monitors"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/walutil"
)

// CRIT-CEF-RELIABILITY-PANIC-FREE-001 (Static Floor)
// Static AST audit proves zero raw panic calls exist in production runtime paths:
// pkg/ambient/inbox.go, pkg/coordination/cli_notifier_helper.go, and pkg/logging/fluent_builder.go.
func TestReliabilityHardening_PanicFree_StaticFloor(t *testing.T) {
	filesToAudit := []string{
		"../ambient/inbox.go",
		"../coordination/cli_notifier_helper.go",
		"../logging/fluent_builder.go",
	}

	for _, fileRel := range filesToAudit {
		absPath, err := filepath.Abs(fileRel)
		require.NoError(t, err, "failed to resolve path for %s", fileRel)
		content, err := os.ReadFile(absPath)
		require.NoError(t, err, "failed to read file %s", absPath)

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, absPath, content, parser.ParseComments)
		require.NoError(t, err, "failed to parse %s", absPath)

		var panicCalls []string
		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "panic" {
				pos := fset.Position(call.Pos())
				panicCalls = append(panicCalls, fmt.Sprintf("%s:%d", pos.Filename, pos.Line))
			}
			return true
		})

		require.Empty(t, panicCalls, "raw panic call(s) detected in production file %s: %v", fileRel, panicCalls)
	}

	// Operational check: initializing AutonomyInbox below minimum capacity does not panic, clamps gracefully.
	inbox := ambient.NewAutonomyInbox(1)
	require.NotNil(t, inbox)
	require.Equal(t, ambient.MinInboxCapacity, inbox.Cap())

	// Operational check: NewCLINotifierWithCoordinator with empty args returns safe NoopOperationNotifier rather than panicking.
	notifier := coordination.NewCLINotifierWithCoordinator(false, false, "", nil, "op-1", "test", "default")
	require.NotNil(t, notifier)
	require.IsType(t, storage.NoopOperationNotifier{}, notifier)
}

// CRIT-CEF-RELIABILITY-WAL-POISON-001 (Operational Proof)
// Replay cursor quarantines oversized and unparseable records to disk with alert logging rather than silently advancing offset.
// Also proves that uninitialized WAL and metrics directories report degraded rather than false-green ok.
func TestReliabilityHardening_WALPoisonQuarantine_OperationalProof(t *testing.T) {
	tempDir := t.TempDir()
	walPath := filepath.Join(tempDir, "test.wal")

	// Write valid record, corrupt record, oversized record, and another valid record
	lines := [][]byte{
		[]byte(`{"seq":1,"payload":"valid_1"}` + "\n"),
		[]byte(`THIS_IS_CORRUPT_JSON_DATA` + "\n"),
		[]byte(`{"seq":2,"payload":"` + strings.Repeat("A", 1000) + `"}` + "\n"),
		[]byte(`{"seq":3,"payload":"valid_3"}` + "\n"),
	}
	f, err := fileutil.OpenFile(walPath, os.O_CREATE|os.O_RDWR, fileutil.StandardFilePerm)
	require.NoError(t, err)
	for _, l := range lines {
		_, err := f.Write(l)
		require.NoError(t, err)
	}
	_ = f.Close()

	type sampleRec struct {
		Seq     int64  `json:"seq"`
		Payload string `json:"payload"`
	}

	var delivered []int64
	parseFn := func(b []byte) (*sampleRec, error) {
		var r sampleRec
		if err := json.Unmarshal(b, &r); err != nil {
			return nil, err
		}
		if r.Seq == 0 {
			return nil, fmt.Errorf("missing seq")
		}
		return &r, nil
	}
	extractSeqFn := func(r *sampleRec) int64 {
		return r.Seq
	}
	callback := func(r *sampleRec) error {
		delivered = append(delivered, r.Seq)
		return nil
	}

	stats, err := walutil.ReplayFromCursor(walPath, 500, walutil.ReplayCursor{}, parseFn, extractSeqFn, callback)
	require.NoError(t, err)
	require.Equal(t, 2, stats.Delivered)
	require.Equal(t, int64(3), stats.Cursor.Seq)
	require.Equal(t, 1, stats.Corrupted)
	require.Equal(t, 1, stats.Oversized)
	require.Equal(t, []int64{1, 3}, delivered)

	// Verify quarantine directory was populated with corrupt records
	quarantineDir := filepath.Join(tempDir, "quarantine")
	entries, err := os.ReadDir(quarantineDir)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "quarantine directory must contain poisoned WAL lines")

	// Operational check for false-green elimination in monitors
	res, err := healthcheck.DefaultRegistry.Run(context.Background(), filepath.Join(tempDir, "nonexistent"), "wal_backlog")
	require.NoError(t, err)
	require.Equal(t, "degraded", res.Status, "missing WAL directory must report degraded, not ok")

	resObj, err := healthcheck.DefaultRegistry.Run(context.Background(), filepath.Join(tempDir, "nonexistent"), "object_volume")
	require.NoError(t, err)
	require.Equal(t, "degraded", resObj.Status, "missing object_volume directory must report degraded, not ok")

	resStream, err := healthcheck.DefaultRegistry.Run(context.Background(), filepath.Join(tempDir, "nonexistent"), "stream_volume")
	require.NoError(t, err)
	require.Equal(t, "degraded", resStream.Status, "missing stream_volume directory must report degraded, not ok")

	// Darwin CAS strict sync verification
	filecas.SetDarwinStrictSync(true)
	defer filecas.SetDarwinStrictSync(false)
	require.True(t, filecas.IsDarwinStrictSync())
}

// CRIT-CEF-RELIABILITY-FEED-BACKLOG-001 (Negative Boundary)
// Feed doctor detects stalled unacked awaits older than 10 minutes and flags feed as degraded.
func TestReliabilityHardening_FeedBacklogDegraded_NegativeBoundary(t *testing.T) {
	tempDir := t.TempDir()

	feedPath := datacell.AgentChatChannelEventsJSONLPath(tempDir)
	policyPath := datacell.AgentChatChannelConfigPath(tempDir)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(feedPath)))
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(policyPath)))
	require.NoError(t, fileutil.WriteStandardFile(feedPath, []byte("{\"event_id\":\"123\"}\n")))
	require.NoError(t, fileutil.WriteStandardFile(policyPath, []byte("{\"enabled\":true}")))

	// When awaits are clean and fresh, feed is healthy
	cleanRes := agentfeed.InspectFeed(agentfeed.DoctorOptions{
		ProjectRoot: tempDir,
	})
	require.Equal(t, "healthy", cleanRes.FeedHealth)

	// When an unacked await exceeds the 10-minute threshold, doctor detects backlog and reports degraded
	awaitsPath := paths.PeerAckAwaitsPath(tempDir)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(awaitsPath)))
	staleTime := time.Now().UTC().Add(-15 * time.Minute).Format(time.RFC3339)
	awaitsContent := fmt.Sprintf(`{"awaits":[{"id":"AWAIT-999","event_id":"AFE-999","from_agent_id":"agent-sender","to_agent_id":"agent-receiver","action":"wake","status":"open","created_at":"%s"}]}`, staleTime)
	require.NoError(t, fileutil.WriteStandardFile(awaitsPath, []byte(awaitsContent)))

	stalledRes := agentfeed.InspectFeed(agentfeed.DoctorOptions{
		ProjectRoot: tempDir,
	})
	require.Equal(t, "degraded", stalledRes.FeedHealth)
	foundIssue := false
	for _, issue := range stalledRes.Issues {
		if strings.Contains(issue, "feed_unacked_awaits_stalled") {
			foundIssue = true
			break
		}
	}
	require.True(t, foundIssue, "expected feed_unacked_awaits_stalled in doctor issues")
}
