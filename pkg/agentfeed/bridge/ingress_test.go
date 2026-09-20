package bridge

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/agentfeed"
)

func TestSlackAdapter_ParseEvent(t *testing.T) {
	t.Parallel()
	payload := []byte(`{
		"event_id": "Ev123",
		"event": {"type":"message","user":"U1","text":"ship Phase C","ts":"1.2","channel":"C9"}
	}`)
	msg, err := Parse("slack", payload)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Channel != ChannelSlack || msg.Text != "ship Phase C" || msg.FromUser != "U1" || msg.ExternalID != "Ev123" {
		t.Fatalf("unexpected msg: %+v", msg)
	}
}

func TestSlackAdapter_ParseSlash(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"user_id":"U2","text":"steer now","channel_id":"C1","command":"/zqk"}`)
	msg, err := NewSlackAdapter().Parse(payload)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "steer now" || msg.FromUser != "U2" {
		t.Fatalf("unexpected msg: %+v", msg)
	}
}

func TestToSteerInput_mapsSteeringSchema(t *testing.T) {
	t.Parallel()
	in, err := ToSteerInput("/repo", IngressMessage{
		Channel:    ChannelSlack,
		ExternalID: "Ev9",
		Text:       "hello from slack",
		ToAgentID:  "agy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if in.EventType != agentfeed.FeedEventTypeSteering {
		t.Fatalf("event_type=%q", in.EventType)
	}
	if in.Sender != agentfeed.FeedSenderMessagingBridge+"_slack" {
		t.Fatalf("sender=%q", in.Sender)
	}
	if in.Message != "hello from slack" || in.ToAgentID != "agy" {
		t.Fatalf("unexpected input: %+v", in)
	}
	if in.InReplyTo != "ext:slack:Ev9" {
		t.Fatalf("in_reply_to=%q", in.InReplyTo)
	}
}

func TestGenericAdapter_andLookup(t *testing.T) {
	t.Parallel()
	msg, err := Parse("generic", []byte(`{"text":"via edge","agent_id":"tpm","channel":"generic"}`))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "via edge" || msg.AgentID != "tpm" {
		t.Fatalf("unexpected: %+v", msg)
	}
	if _, err := Lookup("nope"); err == nil {
		t.Fatal("expected unknown channel error")
	}
}

func TestTeamsAdapter_Parse(t *testing.T) {
	t.Parallel()
	payload := []byte(`{
		"type":"message","id":"m-1","text":"ship from Teams",
		"from":{"id":"29:user","name":"Ada"},
		"conversation":{"id":"19:conv"}
	}`)
	msg, err := Parse("teams", payload)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Channel != ChannelTeams || msg.Text != "ship from Teams" || msg.FromUser != "Ada" || msg.ExternalID != "m-1" {
		t.Fatalf("unexpected: %+v", msg)
	}
	in, err := ToSteerInput("/repo", msg)
	if err != nil {
		t.Fatal(err)
	}
	if in.Sender != agentfeed.FeedSenderMessagingBridge+"_teams" {
		t.Fatalf("sender=%q", in.Sender)
	}
}

func TestTelegramAdapter_ParseUpdate(t *testing.T) {
	t.Parallel()
	payload := []byte(`{
		"update_id": 99,
		"message": {
			"message_id": 7,
			"text": "ship from TG",
			"from": {"id": 42, "username": "bob"},
			"chat": {"id": -100}
		}
	}`)
	msg, err := Parse("telegram", payload)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "ship from TG" || msg.FromUser != "bob" || msg.ExternalID != "7" || msg.ThreadID != "-100" {
		t.Fatalf("unexpected: %+v", msg)
	}
}

func TestSignalAdapter_ParseEnvelope(t *testing.T) {
	t.Parallel()
	payload := []byte(`{
		"envelope": {
			"source": "+15551212",
			"timestamp": 99,
			"dataMessage": {"message": "ship from Signal", "timestamp": 100}
		}
	}`)
	msg, err := Parse("signal", payload)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Channel != ChannelSignal || msg.Text != "ship from Signal" || msg.FromUser != "+15551212" || msg.ExternalID != "99" {
		t.Fatalf("unexpected: %+v", msg)
	}
}

func TestToSteerInput_requiresText(t *testing.T) {
	t.Parallel()
	if _, err := ToSteerInput("/repo", IngressMessage{}); err == nil {
		t.Fatal("expected error")
	}
}
