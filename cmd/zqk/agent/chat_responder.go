package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// NewChatResponderCmd creates the command for the native Chat Responder subagent
func NewChatResponderCmd() *cobra.Command {
	var c cobra.Command
	c.Use = "chat-responder"
	c.Short = "Run the native Chat Responder subagent"
	c.Long = `Reads the agent chat channel, determines if anything has changed since the last time it read, and responds natively within ZQK if there are new messages.`
	c.RunE = func(cmd *cobra.Command, args []string) error {
		var ctx context.Context
		var cancel context.CancelFunc
		if timeout := cli.GetTimeout(cmd); timeout > 0 {
			ctx, cancel = context.WithTimeout(pkgctx.NewSystemContext(), timeout)
		} else {
			ctx, cancel = context.WithCancel(pkgctx.NewSystemContext())
		}
		defer cancel()

		proc, err := cli.NewProcessor(cmd)
		if err != nil {
			return errfmt.Newf("failed to get CLI processor").Wrap(err)
		}

		transcriptPath := zqkenv.AGTranscriptPath().Get()
		if transcriptPath == "" {
			_ = cli.WriteOutput(cmd, []byte("AG_TRANSCRIPT_PATH not set; skipping chat responder.\n"))
			return nil
		}

		// Extract conversation ID from transcript path to maintain per-conversation state
		convID := "DEFAULT"
		brainDir := filepath.Dir(filepath.Dir(filepath.Dir(transcriptPath)))
		if base := filepath.Base(brainDir); base != "" && base != "." {
			convID = base
		}

		// Hash the convID into a uint32 to match the ZQK-\d+ pattern
		hash := fnv.New32a()
		_, _ = hash.Write([]byte(convID))
		sessionID := fmt.Sprintf("ZQK-%d", hash.Sum32())
		var lastStep int64 = -1
		var lastNudgeTime time.Time
		secCtx := pkgctx.NewSystemSecurityContext()

		sessionObj, err := proc.Storage().Read(ctx, secCtx, sessionID)
		if err == nil && sessionObj != nil {
			if titleStr, ok := sessionObj[objects.FieldKeyTitle].(string); ok {
				var state map[string]any
				if json.Unmarshal([]byte(titleStr), &state) == nil {
					if stepFloat, ok := state["last_step"].(float64); ok {
						lastStep = int64(stepFloat)
					}
					if nudgeStr, ok := state["last_nudge_time"].(string); ok {
						lastNudgeTime, _ = time.Parse(time.RFC3339, nudgeStr)
					}
				}
			}
		}

		saveState := func(step int64, nudge time.Time) {
			state := map[string]any{
				"last_step": step,
			}
			if !nudge.IsZero() {
				state["last_nudge_time"] = nudge.Format(time.RFC3339)
			}
			stateBytes, _ := json.Marshal(state)
			mut := map[string]any{
				objects.FieldKeyID:            sessionID,
				objects.FieldKeyKind:          objects.KindZqkSession,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				objects.FieldKeyTitle:         string(stateBytes),
				objects.FieldKeyAccountID:     secCtx.AccountID,
			}
			if sessionObj != nil {
				_ = proc.Storage().Update(ctx, secCtx, sessionID, mut)
			} else {
				_ = proc.Storage().Create(ctx, secCtx, mut)
				sessionObj = mut
			}
		}

		file, err := fileutil.Open(transcriptPath)
		if err != nil {
			return errfmt.Newf("failed to open transcript %s", transcriptPath).Wrap(err)
		}
		defer file.Close()

		llmClient := llm.NewClient(ctx, llm.DefaultConfig(ctx))
		if llmClient == nil {
			return fmt.Errorf("failed to init LLM client")
		}

		scanner := bufio.NewScanner(file)
		var newMessages []map[string]any
		var maxStep int64 = lastStep

		var lastMsgTime time.Time
		for scanner.Scan() {
			var entry map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
				if tsStr, ok := entry["timestamp"].(string); ok {
					if ts, err := time.Parse(time.RFC3339, tsStr); err == nil {
						if ts.After(lastMsgTime) {
							lastMsgTime = ts
						}
					}
				}
				stepFloat, ok := entry["step_index"].(float64)
				if !ok {
					continue
				}
				step := int64(stepFloat)
				if step > maxStep {
					maxStep = step
				}
				if step > lastStep {
					typ, _ := entry[objects.FieldKeyType].(string)
					content, _ := entry[objects.FieldKeyContent].(string)

					if (typ == "USER_INPUT" || typ == "PLANNER_RESPONSE") && content != "" && !strings.Contains(content, "[ZQK Swarm]") && !strings.Contains(content, "[CAP Orchestrator]") {
						newMessages = append(newMessages, entry)
					}
				}
			}
		}

		var isIdle bool
		if len(newMessages) == 0 {
			if !lastMsgTime.IsZero() && time.Since(lastMsgTime) > 15*time.Minute {
				if lastNudgeTime.After(lastMsgTime) {
					_ = cli.WriteOutput(cmd, []byte("No new messages found, and already nudged.\n"))
					saveState(maxStep, lastNudgeTime)
					return nil
				}
				isIdle = true
				lastNudgeTime = time.Now()
			} else {
				_ = cli.WriteOutput(cmd, []byte("No new messages found.\n"))
				saveState(maxStep, lastNudgeTime)
				return nil
			}
		}

		// Build transcript context
		transcriptCtx := getTranscriptContext(transcriptPath)

		// Build observer context
		var observerCtx string
		observerTipsPath := filepath.Join(proc.ProjectRoot(), ".zqk", "state", "observer_tips.json")
		if b, err := fileutil.ReadFile(observerTipsPath); err == nil {
			var tips []string
			if err := json.Unmarshal(b, &tips); err == nil && len(tips) > 0 {
				var sb strings.Builder
				sb.WriteString("\n--- AST Observer Insights ---\n")
				for _, tip := range tips {
					sb.WriteString("- " + tip + "\n")
				}
				sb.WriteString("-----------------------------\n")
				observerCtx = sb.String()
			}
		}

		// Aggregate new messages to respond to
		var newChatText string
		if isIdle {
			newChatText = "[SYSTEM_IDLE_PREVENTION_EVENT]\nAgents have been idle for over 15 minutes. Run zqk workflow whats-next --format json --skip-measure. Do fill_item.command_hint if present. Do not remint ORCHESTRATE_PLAN on terminal-only ATKs. Do not trigger orchestration from chat."
			_ = cli.WriteOutput(cmd, []byte("Idle detected. Generating nudge response...\n"))
		} else {
			var sb strings.Builder
			for _, msg := range newMessages {
				typ, _ := msg[objects.FieldKeyType].(string)
				content, _ := msg[objects.FieldKeyContent].(string)
				senderName := "ide_assistant"
				if typ == "USER_INPUT" {
					senderName = "human_user"
				}
				sb.WriteString(fmt.Sprintf("%s: %q\n", senderName, content))
			}
			newChatText = sb.String()
			_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Found %d new messages. Generating response...\n", len(newMessages))))
		}

		secCtx = pkgctx.NewSystemSecurityContext()
		var persona map[string]any
		personasRes, listErr := proc.Storage().List(ctx, secCtx, nil, storage.ListFilter{
			Kind:    objects.KindPersona,
			Filters: map[string]any{objects.FieldKeyRole: "chat-responder"},
		})
		if listErr == nil && len(personasRes.Objects) > 0 {
			persona = personasRes.Objects[0]
		}
		systemPrompt := `You are the ZQK Kernel Steward, the native voice of the ZQK operating system.
Your primary job is to coordinate between the human, the IDE Assistant, and the ZQK Kernel.
You MUST maintain the posture of the ZQK Kernel Steward and ALWAYS act as an advocate and promoter for the Knowledge Kernel CAP loop.`
		if persona != nil {
			if desc, ok := persona[objects.FieldKeyDescription].(string); ok && desc != "" {
				systemPrompt = desc + "\nCRITICAL: You are the ZQK Kernel Steward. You must act as an advocate and promoter for the CAP loop."
			}
		}

		prompt := fmt.Sprintf("%s\n%s\n%s\nThe following are NEW messages that have just arrived:\n%s\nPlease respond appropriately as the ZQK Kernel Steward.", systemPrompt, observerCtx, transcriptCtx, newChatText)

		resp, err := llmClient.GenerateCompletion(ctx, prompt, "")
		if err != nil {
			_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("LLM Error: %v\n", err)))
			return errfmt.Newf("error generating response").Wrap(err)
		}

		formattedResp := fmt.Sprintf("[ZQK Swarm] %s", strings.TrimSpace(resp))

		adapter := GetDeliveryAdapter(transcriptPath)
		msgID := fmt.Sprintf("zqk-%d", maxStep)
		renderDetails := map[string]string{
			"messageTitle": "ZQK Swarm Native Response",
		}

		if err := adapter.Deliver(ctx, formattedResp, msgID, renderDetails); err != nil {
			_ = cli.WriteOutput(cmd, []byte(fmt.Sprintf("Failed to deliver message: %v\n", err)))
		} else {
			_ = cli.WriteOutput(cmd, []byte("Responded successfully.\n"))
		}

		saveState(maxStep, lastNudgeTime)
		return nil
	}
	return &c
}

func getTranscriptContext(path string) string {
	if path == "" {
		return ""
	}
	file, err := fileutil.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	var messages []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			typ, _ := entry[objects.FieldKeyType].(string)
			content, _ := entry[objects.FieldKeyContent].(string)
			if typ == "USER_INPUT" && content != "" {
				messages = append(messages, "User: "+content)
			} else if typ == "PLANNER_RESPONSE" && content != "" {
				messages = append(messages, "IDE Assistant: "+content)
			}
		}
	}

	// Keep only the last 20 messages to prevent context limit errors
	if len(messages) > 20 {
		messages = messages[len(messages)-20:]
	}

	var sb strings.Builder
	sb.WriteString("\n--- Recent IDE Chat History ---\n")
	for _, msg := range messages {
		sb.WriteString(msg + "\n")
	}
	sb.WriteString("-------------------------------\n")
	return sb.String()
}
