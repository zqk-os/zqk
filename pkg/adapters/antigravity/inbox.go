package antigravity

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// InboxAdapter writes into Gemini Antigravity CLI's per-conversation
// ".system_generated/messages" inbox. This is not kernel-native delivery.
type InboxAdapter struct {
	TranscriptPath string
}

// Vendor implements adapters.MessageDelivery.
func (a *InboxAdapter) Vendor() string { return VendorID }

// Deliver implements adapters.MessageDelivery using Antigravity's inbox JSON
// schema (ID/Recipient/Sender/Priority/Message/renderDetails).
func (a *InboxAdapter) Deliver(_ context.Context, message string, msgID string, renderDetails map[string]string) error {
	convRoot, err := ConversationRootFromTranscript(a.TranscriptPath)
	if err != nil {
		return err
	}

	// Field names and MESSAGE_PRIORITY_HIGH are Antigravity inbox protocol, not kernel objects.
	payload := map[string]any{
		"ID":            msgID,
		"Recipient":     ConversationID(convRoot),
		"Sender":        "ZQK-Chat-Responder",
		"Priority":      "MESSAGE_PRIORITY_HIGH",
		"Message":       message,
		"renderDetails": renderDetails,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	undelivered := undeliveredInboxDir(convRoot)
	if err := fileutil.EnsureDir(undelivered); err != nil {
		return err
	}

	messagesDir := filepath.Dir(undelivered)
	if err := fileutil.WriteSecureFile(filepath.Join(messagesDir, msgID+".json"), b); err != nil {
		return err
	}
	return fileutil.WriteSecureFile(filepath.Join(undelivered, msgID), []byte(""))
}
