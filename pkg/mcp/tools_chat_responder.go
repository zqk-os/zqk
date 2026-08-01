package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
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
		cwd, err := os.Getwd()
		if err != nil {
			return nil, errfmt.Errorf("failed to get working directory: %w", err)
		}
		projectRoot = cwd
	}

	// Resolve agent identity. Prefer explicit agent_id from the caller (the
	// IDE agent knows its own name), fall back to the MCP session's AccountID.
	agentID, _ := args["agent_id"].(string)
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

	eventPath := datacell.AgentChatChannelEventsJSONLPath(projectRoot)

	if err := fileutil.EnsureDir(filepath.Dir(eventPath)); err != nil {
		return nil, errfmt.Errorf("failed to create agent chat directory: %w", err)
	}

	// 1. Get initial file size to know where to start reading from
	var startSize int64
	stat, err := os.Stat(eventPath)
	if err == nil {
		startSize = stat.Size()
	}

	// 2. Append the new message with agent identity
	f, err := os.OpenFile(eventPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, errfmt.Errorf("failed to open chat events file: %w", err)
	}

	timestamp := time.Now().UTC()
	event := map[string]any{
		"timestamp":               timestamp.Format(time.RFC3339),
		"sender":                  "ide_chat",
		"agent_id":                agentID,
		objects.FieldKeySessionID: sessionID,
		"message":                 message,
	}

	b, err := json.Marshal(event)
	if err != nil {
		f.Close()
		return nil, errfmt.Errorf("failed to marshal event: %w", err)
	}

	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return nil, errfmt.Errorf("failed to write event: %w", err)
	}

	// Self-ACK: when no external chat responder daemon is running (common in
	// demo/standalone projects), the MCP server writes its own lightweight ACK.
	// This closes the mesh gap where pull-only JSONL channels can't wake idle
	// agents (ITEM-EXAMPLE). The poll loop below will find this immediately.
	ack := map[string]any{
		"timestamp":               timestamp.Add(time.Millisecond).Format(time.RFC3339Nano),
		"sender":                  "chat_responder",
		"agent_id":                "kernel",
		objects.FieldKeySessionID: sessionID,
		"message":                 "ZQK_KERNEL_ACK — message received by kernel. Processing: " + message,
	}
	if ab, err := json.Marshal(ack); err == nil {
		_, _ = f.Write(append(ab, '\n'))
	}

	f.Close()

	// Broadcast via MCP notification to wake up idle agents
	if eventEmitter := server.GetEventEmitter(); eventEmitter != nil {
		_ = eventEmitter.Emit(&Event{
			Type:      EventTypeActionRequired,
			Timestamp: timestamp,
			Message:   fmt.Sprintf("Incoming chat message from %s: %s", agentID, message),
			Severity:  "info",
			Priority:  "high",
			Fields:    event,
		})
	}

	// 3. Tail the file for a response (up to 30 seconds)
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
				objects.FieldKeyStatus:    "timeout",
				"message":                 "Message injected, but timed out waiting for a response from the native swarm.",
				"agent_id":                agentID,
				objects.FieldKeySessionID: sessionID,
				"event_path":              eventPath,
			}, nil
		case <-ticker.C:
			rf, err := os.Open(eventPath)
			if err != nil {
				continue
			}

			stat, err = rf.Stat()
			if err != nil {
				rf.Close()
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
						if sender == "chat_responder" && msg != "" {
							responseMessage = msg
							found = true
							break
						}
					}
				}
				startSize = stat.Size()
			}
			rf.Close()
		}
	}

	result := map[string]any{
		objects.FieldKeyStatus:    "success",
		"message":                 "Received response from ZQK native swarm.",
		"response":                responseMessage,
		"agent_id":                agentID,
		objects.FieldKeySessionID: sessionID,
		"event_path":              eventPath,
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
				"agent_id": map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Optional identifier for the calling agent (e.g. 'cursor-claude', 'agy-gemini'). If omitted, derived from the MCP session's account ID.",
				},
			},
			"required": []string{"message"},
		},
		nil,
	)
}
