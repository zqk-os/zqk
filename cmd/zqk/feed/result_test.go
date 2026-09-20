package feed

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestFeedResult_conciseByDefault(t *testing.T) {
	cmd := &cobra.Command{}
	res := agentfeed.AppendEventResult{
		FeedID:       "AGF-1",
		DeliveryMode: "notify",
		EventPath:    "/tmp/events.jsonl",
		Event:        map[string]any{"message": "hello world"},
	}
	wake := &agentfeed.PeerWakeResult{Attempted: true}
	out := feedResult(cmd, res, wake)
	if out[objects.FieldKeyStatus] != "success" {
		t.Fatalf("status=%v", out[objects.FieldKeyStatus])
	}
	if out["peer_wake"] != "attempted" {
		t.Fatalf("peer_wake=%v", out["peer_wake"])
	}
	if _, ok := out["event_path"]; ok {
		t.Fatal("event_path should be omitted unless verbose")
	}
	if _, ok := out["event"]; ok {
		t.Fatal("full event must not be in default result")
	}
}

func TestFeedResult_verboseAddsPath(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("verbose", false, "")
	_ = cmd.Flags().Set("verbose", "true")
	res := agentfeed.AppendEventResult{
		FeedID:       "AGF-1",
		DeliveryMode: "notify",
		EventPath:    "/tmp/events.jsonl",
		Event:        map[string]any{"message": "hello"},
	}
	out := feedResult(cmd, res, nil)
	if out["event_path"] != "/tmp/events.jsonl" {
		t.Fatalf("event_path=%v", out["event_path"])
	}
}

func TestFeedResult_mcpLiveWake(t *testing.T) {
	cmd := &cobra.Command{}
	res := agentfeed.AppendEventResult{FeedID: "AGF-1", DeliveryMode: "notify", EventID: "AFE-1"}
	wake := &agentfeed.PeerWakeResult{
		Attempted:       true,
		Live:            true,
		Transport:       agentfeed.TransportMCPActionRequired,
		IdeBridgeQueued: true,
	}
	out := feedResult(cmd, res, wake)
	if out["peer_wake_unrepaired"] == true {
		t.Fatal("live MCP wake must not set peer_wake_unrepaired")
	}
	if out["peer_wake_live"] != true {
		t.Fatalf("peer_wake_live=%v", out["peer_wake_live"])
	}
	if out["ide_bridge_queued"] != true {
		t.Fatalf("ide_bridge_queued=%v", out["ide_bridge_queued"])
	}
}

func TestFeedResult_unrepairedFlagOnFailedWake(t *testing.T) {
	cmd := &cobra.Command{}
	res := agentfeed.AppendEventResult{FeedID: "AGF-1", DeliveryMode: "notify", EventID: "AFE-1"}
	wake := &agentfeed.PeerWakeResult{Attempted: true, Error: "no peer"}
	out := feedResult(cmd, res, wake)
	if out["peer_wake"] != "failed" {
		t.Fatalf("peer_wake=%v", out["peer_wake"])
	}
	if out["peer_wake_unrepaired"] != true {
		t.Fatalf("peer_wake_unrepaired=%v", out["peer_wake_unrepaired"])
	}
}
