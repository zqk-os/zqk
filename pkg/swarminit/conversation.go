package swarminit

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
)

// ProbeConversationViaAgentAPI calls agentapi get-conversation-metadata.
// Empty conversation or "trajectory not found" is a hard failure.
func ProbeConversationViaAgentAPI(ctx context.Context, bin, lsAddr, conversationID string) error {
	conv := strings.TrimSpace(conversationID)
	if conv == "" {
		return errfmt.Errorf("conversation id is empty")
	}
	bin = strings.TrimSpace(bin)
	if bin == "" {
		return errfmt.Errorf("agentapi binary not set")
	}
	cmd := execwrap.CommandContext(ctx, bin, "get-conversation-metadata", conv)
	cmd.Env = os.Environ()
	if ls := strings.TrimSpace(lsAddr); ls != "" {
		cmd.Env = append(cmd.Env, "ANTIGRAVITY_LS_ADDRESS="+ls)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		blob := stdout.String() + stderr.String() + err.Error()
		if strings.Contains(strings.ToLower(blob), "trajectory not found") {
			return errfmt.Errorf("trajectory not found: %s", conv)
		}
		return errfmt.Errorf("agentapi metadata: %s", strings.TrimSpace(blob))
	}
	blob := stdout.String() + stderr.String()
	if strings.Contains(strings.ToLower(blob), "trajectory not found") {
		return errfmt.Errorf("trajectory not found: %s", conv)
	}
	return nil
}

// NewConversationProbe returns a probe that uses agentapi when bin is non-empty.
func NewConversationProbe(agentapiBin, lsAddr string) ConversationProbe {
	return func(ctx context.Context, seatID string, rec agentfeed.PeerSeatRecord) error {
		_ = seatID
		if strings.TrimSpace(rec.Conversation) == "" {
			return errfmt.Errorf("conversation id is empty")
		}
		if strings.TrimSpace(agentapiBin) == "" {
			return errfmt.Errorf("trajectory not found: %s (no agentapi probe configured)", rec.Conversation)
		}
		return ProbeConversationViaAgentAPI(ctx, agentapiBin, lsAddr, rec.Conversation)
	}
}

// LookPathAgentAPI locates the agentapi binary (optional).
func LookPathAgentAPI() string {
	p, err := exec.LookPath("agentapi")
	if err != nil {
		return ""
	}
	return p
}
