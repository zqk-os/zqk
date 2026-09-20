package bridge

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Channel identifies an enterprise messaging ingress surface.
type Channel string

const (
	ChannelSlack    Channel = "slack"
	ChannelTeams    Channel = "teams"
	ChannelTelegram Channel = "telegram"
	ChannelSignal   Channel = "signal"
	ChannelGeneric  Channel = "generic"
)

// IngressMessage is the normalized shape all adapters produce before steering append.
type IngressMessage struct {
	Channel    Channel `json:"channel,omitempty"`
	ExternalID string  `json:"external_id,omitempty"` // provider message / event id
	Text       string  `json:"text"`
	FromUser   string  `json:"from_user,omitempty"`
	ThreadID   string  `json:"thread_id,omitempty"`
	// AgentID is the swarm seat to attribute as sender agent (optional).
	AgentID string `json:"agent_id,omitempty"`
	// ToAgentID optionally routes the steering event to a peer seat.
	ToAgentID string `json:"to_agent_id,omitempty"`
}

// Adapter parses provider-specific payloads into IngressMessage.
type Adapter interface {
	Channel() Channel
	Parse(payload []byte) (IngressMessage, error)
}

// ToSteerInput maps a normalized ingress message onto AppendEventInput (same schema as feed steer).
func ToSteerInput(projectRoot string, msg IngressMessage) (agentfeed.AppendEventInput, error) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return agentfeed.AppendEventInput{}, errfmt.Errorf("bridge ingress missing text")
	}
	ch := msg.Channel
	if ch == "" {
		ch = ChannelGeneric
	}
	sender := agentfeed.FeedSenderMessagingBridge
	if suffix := strings.TrimSpace(string(ch)); suffix != "" && suffix != string(ChannelGeneric) {
		sender = agentfeed.FeedSenderMessagingBridge + "_" + suffix
	}
	in := agentfeed.AppendEventInput{
		ProjectRoot: projectRoot,
		Message:     text,
		AgentID:     strings.TrimSpace(msg.AgentID),
		ToAgentID:   strings.TrimSpace(msg.ToAgentID),
		Sender:      sender,
		EventType:   agentfeed.FeedEventTypeSteering,
		SelfACK:     true,
		ACKMessage:  "bridge ingress accepted",
	}
	if id := strings.TrimSpace(msg.ExternalID); id != "" {
		// Correlate kernel_ack / peer_ack to provider id via in_reply_to when no feed parent.
		in.InReplyTo = "ext:" + string(ch) + ":" + id
	}
	return in, nil
}
