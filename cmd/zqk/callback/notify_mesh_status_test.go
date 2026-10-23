package callback

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSummarizeCallbackMeshMessage(t *testing.T) {
	got := summarizeCallbackMeshMessage(map[string]any{
		"job_id":  "SCH-1",
		"outcome": "success",
		"error":   "boom",
	})
	want := "scheduler callback SCH-1 success error=boom"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestStampSchedulerCallbackMeshStatus_doesNotFillInbox(t *testing.T) {
	root := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(root, paths.ProjectDataDir), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		FeedID:        "AGF-cb-mesh",
	}); err != nil {
		t.Fatal(err)
	}
	if err := stampSchedulerCallbackMeshStatus(root, map[string]any{
		"job_id":  "SCH-mesh-1",
		"outcome": "success",
	}); err != nil {
		t.Fatal(err)
	}
	seat := agentfeed.Seat{AgentID: "peer-agent-2", RoleHints: []string{"peer-agent-2"}}
	snap, err := agentfeed.LoadCorrespondence(root, seat, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.InboxUnacked) != 0 {
		t.Fatalf("mesh_status stamp must not fill inbox: %+v", snap.InboxUnacked)
	}
	if snap.NextActionHint != agentfeed.HintContinue {
		t.Fatalf("hint=%s want continue", snap.NextActionHint)
	}
}
