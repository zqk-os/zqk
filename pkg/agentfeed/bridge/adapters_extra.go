package bridge

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// TeamsAdapter parses a minimal Microsoft Teams Bot Framework / webhook JSON shape.
type TeamsAdapter struct{}

func NewTeamsAdapter() *TeamsAdapter { return &TeamsAdapter{} }

func (a *TeamsAdapter) Channel() Channel { return ChannelTeams }

type teamsPayload struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Text string `json:"text"`
	From *struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		AadObjectID string `json:"aadObjectId"`
	} `json:"from"`
	Conversation *struct {
		ID string `json:"id"`
	} `json:"conversation"`
	ChannelData *struct {
		TeamsChannelID string `json:"teamsChannelId"`
		TeamsTeamID    string `json:"teamsTeamId"`
	} `json:"channelData"`
}

func (a *TeamsAdapter) Parse(payload []byte) (IngressMessage, error) {
	var p teamsPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return IngressMessage{}, errfmt.Newf("teams bridge payload").Wrap(err)
	}
	msg := IngressMessage{
		Channel:    ChannelTeams,
		Text:       strings.TrimSpace(p.Text),
		ExternalID: strings.TrimSpace(p.ID),
	}
	if p.From != nil {
		msg.FromUser = firstNonEmpty(p.From.Name, p.From.ID, p.From.AadObjectID)
	}
	if p.Conversation != nil {
		msg.ThreadID = strings.TrimSpace(p.Conversation.ID)
	}
	if msg.ThreadID == "" && p.ChannelData != nil {
		msg.ThreadID = firstNonEmpty(p.ChannelData.TeamsChannelID, p.ChannelData.TeamsTeamID)
	}
	if msg.Text == "" {
		return IngressMessage{}, errfmt.Errorf("teams bridge payload missing text")
	}
	return msg, nil
}

// TelegramAdapter parses a minimal Telegram Bot API Update / message JSON shape.
type TelegramAdapter struct{}

func NewTelegramAdapter() *TelegramAdapter { return &TelegramAdapter{} }

func (a *TelegramAdapter) Channel() Channel { return ChannelTelegram }

type telegramPayload struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64  `json:"message_id"`
		Text      string `json:"text"`
		Caption   string `json:"caption"`
		From      *struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"from"`
		Chat *struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
	Text      string `json:"text"`
	MessageID int64  `json:"message_id"`
	From      *struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"from"`
	Chat *struct {
		ID int64 `json:"id"`
	} `json:"chat"`
}

func (a *TelegramAdapter) Parse(payload []byte) (IngressMessage, error) {
	var p telegramPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return IngressMessage{}, errfmt.Newf("telegram bridge payload").Wrap(err)
	}
	msg := IngressMessage{Channel: ChannelTelegram}

	if p.Message != nil {
		msg.Text = firstNonEmpty(p.Message.Text, p.Message.Caption)
		if p.Message.From != nil {
			msg.FromUser = firstNonEmpty(p.Message.From.Username, formatInt64(p.Message.From.ID))
		}
		msg.ExternalID = firstNonEmpty(formatInt64(p.Message.MessageID), formatInt64(p.UpdateID))
		if p.Message.Chat != nil {
			msg.ThreadID = formatInt64(p.Message.Chat.ID)
		}
	} else {
		msg.Text = strings.TrimSpace(p.Text)
		if p.From != nil {
			msg.FromUser = firstNonEmpty(p.From.Username, formatInt64(p.From.ID))
		}
		msg.ExternalID = formatInt64(p.MessageID)
		if p.Chat != nil {
			msg.ThreadID = formatInt64(p.Chat.ID)
		}
	}
	if msg.Text == "" {
		return IngressMessage{}, errfmt.Errorf("telegram bridge payload missing text")
	}
	return msg, nil
}

func formatInt64(n int64) string {
	if n == 0 {
		return ""
	}
	return strconv.FormatInt(n, 10)
}

// SignalAdapter parses a minimal Signal webhook / signal-cli JSON shape.
type SignalAdapter struct{}

func NewSignalAdapter() *SignalAdapter { return &SignalAdapter{} }

func (a *SignalAdapter) Channel() Channel { return ChannelSignal }

type signalPayload struct {
	Text      string `json:"text"`
	Message   string `json:"message"`
	Source    string `json:"source"`
	Timestamp int64  `json:"timestamp"`
	Envelope  *struct {
		Source      string `json:"source"`
		Timestamp   int64  `json:"timestamp"`
		DataMessage *struct {
			Message   string `json:"message"`
			Timestamp int64  `json:"timestamp"`
		} `json:"dataMessage"`
	} `json:"envelope"`
}

func (a *SignalAdapter) Parse(payload []byte) (IngressMessage, error) {
	var p signalPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return IngressMessage{}, errfmt.Newf("signal bridge payload").Wrap(err)
	}
	msg := IngressMessage{Channel: ChannelSignal}
	if p.Envelope != nil {
		msg.FromUser = strings.TrimSpace(p.Envelope.Source)
		msg.ExternalID = formatInt64(p.Envelope.Timestamp)
		if p.Envelope.DataMessage != nil {
			msg.Text = strings.TrimSpace(p.Envelope.DataMessage.Message)
			if msg.ExternalID == "" {
				msg.ExternalID = formatInt64(p.Envelope.DataMessage.Timestamp)
			}
		}
	}
	if msg.Text == "" {
		msg.Text = firstNonEmpty(p.Text, p.Message)
	}
	if msg.FromUser == "" {
		msg.FromUser = strings.TrimSpace(p.Source)
	}
	if msg.ExternalID == "" {
		msg.ExternalID = formatInt64(p.Timestamp)
	}
	if msg.Text == "" {
		return IngressMessage{}, errfmt.Errorf("signal bridge payload missing text")
	}
	return msg, nil
}
