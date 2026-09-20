package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// MessageDeliveryAdapter defines the interface for delivering messages back to the user environment.
type MessageDeliveryAdapter interface {
	Deliver(ctx context.Context, message string, msgID string, renderDetails map[string]string) error
}

// AgentMessageAdapter delivers messages natively to the Agent IDE framework.
type AgentMessageAdapter struct {
	TranscriptPath string
}

func (a *AgentMessageAdapter) Deliver(ctx context.Context, message string, msgID string, renderDetails map[string]string) error {
	brainDir := filepath.Dir(filepath.Dir(filepath.Dir(a.TranscriptPath)))
	convID := filepath.Base(brainDir)

	msg := map[string]any{
		"ID":            msgID,
		"Recipient":     convID,
		"Sender":        "ZQK-Chat-Responder",
		"Priority":      "MESSAGE_PRIORITY_HIGH",
		"Message":       message,
		"renderDetails": renderDetails,
	}

	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	undeliveredDir := filepath.Join(brainDir, ".system_generated", "messages", "undelivered")
	if err := fileutil.EnsureDir(undeliveredDir); err != nil {
		return err
	}

	// Write to messages directory
	messagesDir := filepath.Dir(undeliveredDir)
	msgPath := filepath.Join(messagesDir, msgID+".json")
	if err := fileutil.WriteSecureFile(msgPath, b); err != nil {
		return err
	}

	// Write pointer to undelivered directory
	ptrPath := filepath.Join(undeliveredDir, msgID)
	if err := fileutil.WriteSecureFile(ptrPath, []byte("")); err != nil {
		return err
	}

	return nil
}

// LegacyIDEMessageAdapter delivers messages via pbcopy and osascript keystrokes (macOS only).
type LegacyIDEMessageAdapter struct{}

func (a *LegacyIDEMessageAdapter) Deliver(ctx context.Context, message string, msgID string, renderDetails map[string]string) error {
	pbCmd := execwrap.Command("pbcopy")
	pbCmd.Stdin = strings.NewReader(message)
	if err := pbCmd.Run(); err != nil {
		return fmt.Errorf("pbcopy failed: %w", err)
	}

	osaCmd := execwrap.Command("osascript", "-e", "tell application \"System Events\" to keystroke \"v\" using command down", "-e", "delay 0.5", "-e", "tell application \"System Events\" to keystroke return")
	if err := osaCmd.Run(); err != nil {
		return fmt.Errorf("osascript failed: %w", err)
	}

	return nil
}

// GetDeliveryAdapter resolves the appropriate adapter based on the environment.
func GetDeliveryAdapter(transcriptPath string) MessageDeliveryAdapter {
	if transcriptPath != "" && strings.Contains(transcriptPath, ".system_generated") {
		return &AgentMessageAdapter{
			TranscriptPath: transcriptPath,
		}
	}
	return &LegacyIDEMessageAdapter{}
}
