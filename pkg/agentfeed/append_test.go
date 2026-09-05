package agentfeed

import (
	"bufio"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestAppendEvent_writesJSONLAndACK(t *testing.T) {
	root := t.TempDir()
	cfg := datacell.AgentChatChannelConfig{
		SchemaVersion:         datacell.AgentChatChannelSchemaVersion,
		Enabled:               true,
		DeliveryMode:          datacell.DeliveryModeNotify,
		FeedID:                "AGF-test",
		ContractSchemaVersion: "1",
	}
	if err := datacell.WriteAgentChatChannelConfig(root, cfg); err != nil {
		t.Fatalf("WriteAgentChatChannelConfig: %v", err)
	}

	res, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "steer me",
		AgentID:     "tpm",
		SessionID:   "SESS-1",
		Sender:      FeedSenderHumanSteer,
		EventType:   FeedEventTypeSteering,
		SelfACK:     true,
	})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if res.EventPath == "" {
		t.Fatal("expected event path")
	}
	if res.DeliveryMode != datacell.DeliveryModeNotify {
		t.Fatalf("delivery_mode=%q", res.DeliveryMode)
	}
	if res.FeedID != "AGF-test" {
		t.Fatalf("feed_id=%q", res.FeedID)
	}

	f, err := fileutil.Open(res.EventPath)
	if err != nil {
		t.Fatalf("open events: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var lines []map[string]any
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		lines = append(lines, m)
	}
	if len(lines) != 2 {
		t.Fatalf("want 2 lines (event+ack), got %d", len(lines))
	}
	if lines[0]["message"] != "steer me" || lines[0]["sender"] != FeedSenderHumanSteer {
		t.Fatalf("event=%v", lines[0])
	}
	if lines[0][objects.FieldKeyEventType] != FeedEventTypeSteering {
		t.Fatalf("event_type=%v", lines[0][objects.FieldKeyEventType])
	}
	if lines[0][objects.FieldKeyAgentID] != "tpm" {
		t.Fatalf("agent_id=%v", lines[0][objects.FieldKeyAgentID])
	}
	if lines[1]["sender"] != FeedSenderChatResponder {
		t.Fatalf("ack sender=%v", lines[1]["sender"])
	}
	if lines[0][JSONFieldEventID] == nil || lines[0][JSONFieldEventID] == "" {
		t.Fatalf("missing event_id on event: %v", lines[0])
	}
	if lines[1][objects.FieldKeyEventType] != FeedEventTypeKernelAck {
		t.Fatalf("kernel ack event_type=%v", lines[1][objects.FieldKeyEventType])
	}
	if lines[1][JSONFieldInReplyTo] != lines[0][JSONFieldEventID] {
		t.Fatalf("ack in_reply_to=%v parent=%v", lines[1][JSONFieldInReplyTo], lines[0][JSONFieldEventID])
	}
}

func TestAppendEvent_writesRoleWhenSet(t *testing.T) {
	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeNotify,
		FeedID:        "AGF-role",
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	res, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "PR merged",
		AgentID:     "peer-agent-01",
		PersonaRef:  "PER-1785098293551936000",
		Role:        "coder", // from persona object, not inventing peer|coordinator
		Sender:      FeedSenderMeshStatus,
		EventType:   FeedEventTypeMeshStatus,
		SelfACK:     false,
	})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if res.Event[objects.FieldKeyPersonaRef] != "PER-1785098293551936000" {
		t.Fatalf("persona_ref=%v", res.Event[objects.FieldKeyPersonaRef])
	}
	if res.Event[objects.FieldKeyRole] != "coder" {
		t.Fatalf("role=%v", res.Event[objects.FieldKeyRole])
	}
	if res.Event[objects.FieldKeyAgentID] != "peer-agent-01" {
		t.Fatalf("agent_id=%v", res.Event[objects.FieldKeyAgentID])
	}
}

func TestAppendEvent_disabledFails(t *testing.T) {
	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       false,
		DeliveryMode:  datacell.DeliveryModeOff,
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "nope",
		AgentID:     "tpm",
		Sender:      FeedSenderHumanSteer,
	})
	if err == nil {
		t.Fatal("expected error when feed disabled")
	}
}

func TestAppendEvent_respectsEventsPathOverride(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join(".zqk", "logs", "custom", "events.jsonl")
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion:           datacell.AgentChatChannelSchemaVersion,
		Enabled:                 true,
		DeliveryMode:            datacell.DeliveryModeLog,
		EventsJSONLPathOverride: rel,
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	res, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Message:     "override path",
		AgentID:     "agy",
		Sender:      FeedSenderIDEChat,
		SelfACK:     false,
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	want := filepath.Join(root, rel)
	if res.EventPath != want {
		t.Fatalf("path=%q want %q", res.EventPath, want)
	}
	if _, err := fileutil.Stat(want); err != nil {
		t.Fatalf("stat: %v", err)
	}
}

func TestAppendEvent_missingMessage(t *testing.T) {
	root := t.TempDir()
	_, err := AppendEvent(AppendEventInput{
		ProjectRoot: root,
		Sender:      FeedSenderIDEChat,
	})
	if err == nil {
		t.Fatal("expected missing message error")
	}
}

type mockEmitter struct {
	emitted []any
}

func (m *mockEmitter) Emit(event any) int {
	m.emitted = append(m.emitted, event)
	return 1
}

func TestAppendEvent_invokesEventEmitter(t *testing.T) {
	root := t.TempDir()
	if err := datacell.WriteAgentChatChannelConfig(root, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeNotify,
		FeedID:        "AGF-emitter",
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	emitter := &mockEmitter{}
	_, err := AppendEvent(AppendEventInput{
		ProjectRoot:  root,
		Message:      "steer with emitter",
		AgentID:      "tpm",
		Sender:       FeedSenderHumanSteer,
		EventType:    FeedEventTypeSteering,
		EventEmitter: emitter,
	})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if len(emitter.emitted) != 1 {
		t.Fatalf("expected 1 event emitted, got %d", len(emitter.emitted))
	}
	m, ok := emitter.emitted[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", emitter.emitted[0])
	}
	if m[objects.FieldKeyType] != "action.required" {
		t.Errorf("expected type action.required, got %v", m[objects.FieldKeyType])
	}
}
