package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/lanceman/zqk/pkg/agentfeed"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// HandleChatInject handles the chat injection tool.
// The server parameter provides session identity (GetCurrentSessionID) and
// security context (AccountID) so every JSONL event is stamped with
// agent_id and session_id — critical for multi-agent disambiguation.
func HandleChatInject(ctx context.Context, server *Server, args map[string]any, projectRoot string) (any, error) {
	message, ok := args["message"].(string)
	if !ok || message == "" {
		return nil, errfmt.Errorf("missing required parameter: message")
	}

	if projectRoot == "" {
		cwd, err := fileutil.Getwd()
		if err != nil {
			return nil, errfmt.Errorf("failed to get working directory: %w", err)
		}
		projectRoot = cwd
	}

	// Resolve agent identity. Prefer explicit agent_id from the caller (the
	// IDE agent knows its own name), fall back to the MCP session's AccountID.
	agentID, _ := args[objects.FieldKeyAgentID].(string)
	if agentID == "" && server != nil {
		agentID = server.GetCallerAccountID()
	}
	if agentID == "" {
		agentID = "unknown"
	}

	// Session ID from the MCP session object (e.g. MCP-001).
	var sessionID string
	if server != nil {
		sessionID = server.GetCurrentSessionID()
	}

	var emitterAdapter agentfeed.EventEmitter
	if server != nil {
		if ee := server.GetEventEmitter(); ee != nil {
			emitterAdapter = NewEventEmitterAdapter(ee)
		}
	}

	res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot:  projectRoot,
		Message:      message,
		AgentID:      agentID,
		SessionID:    sessionID,
		Sender:       agentfeed.FeedSenderIDEChat,
		EventType:    agentfeed.FeedEventTypeChat,
		SelfACK:      true,
		EventEmitter: emitterAdapter,
	})
	if err != nil {
		return nil, err
	}

	eventPath := res.EventPath
	startSize := res.StartSize

	// Tail the file for a response (up to 30 seconds)
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var responseMessage string
	var found bool

	for !found {
		select {
		case <-readCtx.Done():
			return map[string]any{
				objects.FieldKeyStatus:    objects.ObjectStatusTimeout,
				"message":                 "Message injected, but timed out waiting for a response from the native swarm.",
				objects.FieldKeyAgentID:   agentID,
				objects.FieldKeySessionID: sessionID,
				"event_path":              eventPath,
			}, nil
		case <-ticker.C:
			rf, err := fileutil.Open(eventPath)
			if err != nil {
				continue
			}

			stat, err := rf.Stat()
			if err != nil {
				_ = rf.Close()
				continue
			}

			if stat.Size() > startSize {
				_, _ = rf.Seek(startSize, os.SEEK_SET)
				scanner := bufio.NewScanner(rf)
				for scanner.Scan() {
					line := scanner.Text()
					var ev map[string]any
					if err := json.Unmarshal([]byte(line), &ev); err == nil {
						sender, _ := ev["sender"].(string)
						msg, _ := ev["message"].(string)
						if sender == agentfeed.FeedSenderChatResponder && msg != "" {
							responseMessage = msg
							found = true
							break
						}
					}
				}
				startSize = stat.Size()
			}
			_ = rf.Close()
		}
	}

	result := map[string]any{
		objects.FieldKeyStatus:       objects.ObjectStatusSuccess,
		"message":                    "Received response from ZQK native swarm.",
		"response":                   responseMessage,
		objects.FieldKeyAgentID:      agentID,
		objects.FieldKeySessionID:    sessionID,
		"event_path":                 eventPath,
		objects.FieldKeyDeliveryMode: res.DeliveryMode,
		"feed_id":                    res.FeedID,
	}

	return result, nil
}

// RegisterChatInjectTool registers the chat injection tool with the server
func RegisterChatInjectTool(server *Server) {
	server.RegisterTool(
		GetToolName("chat_send"),
		"Send a chat message to the native ZQK agent chat channel. This enables IDE agents to communicate with native ZQK subagents like the Chat Responder. Messages are stamped with agent_id and session_id for multi-agent disambiguation.",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				"message": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "The chat message or guidance to send to the ZQK native swarm.",
				},
				objects.FieldKeyAgentID: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Optional identifier for the calling agent (e.g. 'coordinator', 'peer-worker'). If omitted, derived from the MCP session's account ID.",
				},
			},
			"required": []string{"message"},
		},
		nil,
	)
}
