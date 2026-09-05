package bridge

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/errfmt"
)

var (
	registryMu sync.RWMutex
	registry   = map[Channel]Adapter{}
)

func init() {
	MustRegister(NewSlackAdapter())
	MustRegister(NewTeamsAdapter())
	MustRegister(NewTelegramAdapter())
	MustRegister(NewSignalAdapter())
	MustRegister(NewGenericAdapter())
}

// MustRegister panics on duplicate/empty channel (package init only).
func MustRegister(a Adapter) {
	if a == nil {
		// TRACK: [Bridge registry initialization violation]
		panic("bridge: nil adapter")
	}
	ch := a.Channel()
	if ch == "" {
		// TRACK: [Bridge registry initialization violation]
		panic("bridge: empty channel")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, ok := registry[ch]; ok {
		// TRACK: [Bridge registry initialization violation]
		panic("bridge: duplicate adapter for " + string(ch))
	}
	registry[ch] = a
}

// Lookup returns a registered adapter by channel name.
func Lookup(channel string) (Adapter, error) {
	ch := Channel(strings.ToLower(strings.TrimSpace(channel)))
	registryMu.RLock()
	a, ok := registry[ch]
	registryMu.RUnlock()
	if !ok {
		return nil, errfmt.Errorf("unknown messaging bridge channel %q", channel)
	}
	return a, nil
}

// Parse looks up channel and parses payload.
func Parse(channel string, payload []byte) (IngressMessage, error) {
	a, err := Lookup(channel)
	if err != nil {
		return IngressMessage{}, err
	}
	return a.Parse(payload)
}

// GenericAdapter accepts the normalized IngressMessage JSON directly (tests / webhooks that
// already mapped at the edge).
type GenericAdapter struct{}

func NewGenericAdapter() *GenericAdapter { return &GenericAdapter{} }

func (a *GenericAdapter) Channel() Channel { return ChannelGeneric }

func (a *GenericAdapter) Parse(payload []byte) (IngressMessage, error) {
	var msg IngressMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return IngressMessage{}, errfmt.Newf("generic bridge payload").Wrap(err)
	}
	if msg.Channel == "" {
		msg.Channel = ChannelGeneric
	}
	if strings.TrimSpace(msg.Text) == "" {
		return IngressMessage{}, errfmt.Errorf("generic bridge payload missing text")
	}
	return msg, nil
}

// SlackAdapter parses a minimal Slack Events API / slash-command JSON shape.
// Not a full Slack SDK — Phase C ingress contract only.
type SlackAdapter struct{}

func NewSlackAdapter() *SlackAdapter { return &SlackAdapter{} }

func (a *SlackAdapter) Channel() Channel { return ChannelSlack }

type slackPayload struct {
	Type    string `json:"type"`
	EventID string `json:"event_id"`
	Event   *struct {
		Type    string `json:"type"`
		User    string `json:"user"`
		Text    string `json:"text"`
		Channel string `json:"channel"`
		TS      string `json:"ts"`
		Thread  string `json:"thread_ts"`
	} `json:"event"`
	// Slash command fields
	UserID  string `json:"user_id"`
	Text    string `json:"text"`
	Channel string `json:"channel_id"`
	Cmd     string `json:"command"`
}

func (a *SlackAdapter) Parse(payload []byte) (IngressMessage, error) {
	var p slackPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return IngressMessage{}, errfmt.Newf("slack bridge payload").Wrap(err)
	}
	msg := IngressMessage{Channel: ChannelSlack}

	if p.Event != nil {
		msg.Text = strings.TrimSpace(p.Event.Text)
		msg.FromUser = strings.TrimSpace(p.Event.User)
		msg.ExternalID = firstNonEmpty(p.EventID, p.Event.TS)
		msg.ThreadID = firstNonEmpty(p.Event.Thread, p.Event.Channel)
	} else {
		msg.Text = strings.TrimSpace(p.Text)
		msg.FromUser = strings.TrimSpace(p.UserID)
		msg.ExternalID = firstNonEmpty(p.EventID, p.Channel)
		msg.ThreadID = strings.TrimSpace(p.Channel)
	}
	if msg.Text == "" {
		return IngressMessage{}, errfmt.Errorf("slack bridge payload missing text")
	}
	return msg, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
