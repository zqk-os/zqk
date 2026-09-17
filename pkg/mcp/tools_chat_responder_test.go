package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/agentfeed"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestHandleChatInject_usesSharedWriter(t *testing.T) {
	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion:         datacell.AgentChatChannelSchemaVersion,
		Enabled:               true,
		DeliveryMode:          datacell.DeliveryModeNotify,
		FeedID:                "AGF-chat-test",
		ContractSchemaVersion: "1",
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := HandleChatInject(ctx, nil, map[string]any{
		"message":               "hello mesh",
		objects.FieldKeyAgentID: "ide-test",
	}, root)
	if err != nil {
		t.Fatalf("HandleChatInject: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("result type %T", out)
	}
	if m[objects.FieldKeyStatus] != "success" {
		t.Fatalf("status=%v result=%v", m[objects.FieldKeyStatus], m)
	}
	if m["feed_id"] != "AGF-chat-test" {
		t.Fatalf("feed_id=%v", m["feed_id"])
	}
	eventPath, _ := m["event_path"].(string)
	f, err := fileutil.Open(eventPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	foundSteer := false
	for sc.Scan() {
		var ev map[string]any
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatalf("json: %v", err)
		}
		if ev["sender"] == agentfeed.FeedSenderIDEChat && ev["message"] == "hello mesh" {
			foundSteer = true
			if ev[objects.FieldKeyEventType] != agentfeed.FeedEventTypeChat {
				t.Fatalf("event_type=%v", ev[objects.FieldKeyEventType])
			}
		}
	}
	if !foundSteer {
		t.Fatal("expected ide_chat event in JSONL")
	}
}

func TestHandleChatInject_disabledFeed(t *testing.T) {
	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       false,
		DeliveryMode:  datacell.DeliveryModeOff,
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := HandleChatInject(context.Background(), nil, map[string]any{
		"message": "x",
	}, root)
	if err == nil {
		t.Fatal("expected disabled error")
	}
}
// tdd refresh
