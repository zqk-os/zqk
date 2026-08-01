package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/llm"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

var globalActiveMCPToolCalls atomic.Int32

// Engine defines the cognitive run loop bridging LLM and MCP Executor.
type Engine struct {
	client            llm.Client
	executor          Executor
	maxSteps          int
	maxToolSchemaSize int
	engineID          string
	OnStep            func(ctx context.Context, stepLog string) error
}

func NewEngine(client llm.Client, executor Executor, maxToolSchemaSize int, engineID string) *Engine {
	maxSteps := 30
	if envMax := os.Getenv(zqkenv.SwarmMaxSteps()); envMax != "" {
		if parsed, err := strconv.Atoi(envMax); err == nil && parsed > 0 {
			maxSteps = parsed
		}
	}
	return &Engine{
		client:            client,
		executor:          executor,
		maxSteps:          maxSteps,
		maxToolSchemaSize: maxToolSchemaSize,
		engineID:          engineID,
	}
}

// Run executes the cognitive loop until the LLM produces a final string result without tool calls.
func (e *Engine) Run(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	logger := logging.GetLogger()
	logging.FluentEvent(logger).Info("Swarm engine run loop started").
		WithFields(
			logging.Int("maxSteps", e.maxSteps),
			logging.Int("userPromptLength", len(userPrompt)),
		).
		Log()

	tools, err := e.executor.GetTools(ctx)
	if err != nil {
		return "", errfmt.Newf("failed to get tools initially").Wrap(err)
	}

	var toolLines []string
	for _, t := range tools {
		toolLines = append(toolLines, fmt.Sprintf("- %s: %s", t.Name, t.Description))
	}

	tp := DefaultToolPrefix()
	injectedSystemPrompt := systemPrompt + "\n\n## Available Tools\nYou MUST use these exact tool names when making function calls. Tool names use underscores, not spaces (e.g. " + tp + "new_object, not \"" + strings.TrimSuffix(tp, "_") + " new object\"):\n" + strings.Join(toolLines, "\n") + "\n\nRULES:\n1. Use ONLY the function calling API to invoke tools — never write tool calls as JSON in your response.\n2. Wait for tool results before proceeding.\n3. Object Creation Workflow:\n   a. Call `" + tp + "new_object` with the specific `kind` you are creating (e.g., `technical_spec`, `criteria`).\n   b. It will return the file path of the draft. Read and edit this file using file tools (`" + tp + "read_file`, `" + tp + "write_file`, etc).\n   c. Call `" + tp + "object_create` with the same `kind` to persist it.\n4. If you were NOT explicitly assigned a task_id, DO NOT call agent_next. When finished, output text saying you are done and make NO tool calls."

	messages := []llm.Message{
		{Role: "system", Content: injectedSystemPrompt},
		{Role: "user", Content: userPrompt},
	}

	var toolCallHistory []ToolCallRecord

	for i := 0; i < e.maxSteps; i++ {
		tools, err := e.executor.GetTools(ctx)
		if err != nil {
			return "", errfmt.Newf("failed to get tools").Wrap(err)
		}

		logging.FluentEvent(logger).Debug(fmt.Sprintf("Swarm step %d: Requesting LLM completion", i)).
			WithFields(logging.Int("toolCount", len(tools))).
			Log()

		// Token Budgeting / Context Compaction
		contextWindow := zqkenv.Get(zqkenv.LLMContextWindowSize()).IntOrDefault(32768)
		tracker := NewTokenTracker(contextWindow, 0.9)

		promptTokens, historyTokens, totalTokens := tracker.EstimateTokens(messages, tools)
		logging.FluentEvent(logger).Info("Swarm step token budget check").
			WithFields(
				logging.Int("step", i),
				logging.Int("promptTokens", promptTokens),
				logging.Int("historyTokens", historyTokens),
				logging.Int("totalTokens", totalTokens),
				logging.Int("contextWindowLimit", contextWindow),
			).
			Log()

		if !tracker.ValidateBudget(totalTokens) {
			logging.FluentEvent(logger).Info("Token budget exceeded limit, compacting history").
				WithFields(
					logging.Int("oldTotalTokens", totalTokens),
					logging.Int("messageCount", len(messages)),
				).
				Log()

			var ok bool
			messages, ok = tracker.CompactHistory(messages, tools)
			if !ok {
				logging.FluentEvent(logger).Error("Swarm run loop aborted: prompt size exceeds context budget limit", nil).Log()
				return "", errfmt.Errorf("prompt size exceeds context budget limit: %d tokens", totalTokens)
			}

			// Recalculate after compaction
			_, _, totalTokens = tracker.EstimateTokens(messages, tools)
			logging.FluentEvent(logger).Info("Token budget after history compaction").
				WithFields(
					logging.Int("newTotalTokens", totalTokens),
					logging.Int("messageCount", len(messages)),
				).
				Log()
		}

		// Provider Profile Schema Compression
		if e.maxToolSchemaSize > 0 {
			tb, _ := json.Marshal(tools)
			if len(tb) > e.maxToolSchemaSize {
				logging.FluentEvent(logger).Warn("Tool schema exceeded provider limit; compressing schema.").
					WithFields(logging.Int("originalSize", len(tb)), logging.Int("maxSize", e.maxToolSchemaSize)).Log()

				// Strip descriptions to compress
				compressed := make([]llm.ToolDefinition, len(tools))
				for ti, t := range tools {
					compressed[ti] = t
					compressed[ti].Description = ""
				}
				tools = compressed
			}
		}

		tb, _ := json.Marshal(tools)
		logging.FluentEvent(logger).Debug("Total LLM Tools and JSON length").
			WithFields(logging.Int("toolCount", len(tools)), logging.Int("jsonLength", len(tb))).
			Log()

		resp, err := e.client.GenerateStructuredCompletion(ctx, messages, tools)
		if err != nil {
			logging.FluentEvent(logger).Error("LLM generation failed", err).WithFields(logging.Int("step", i)).Log()
			return "", errfmt.Newf("generation failed").Wrap(err)
		}

		logging.FluentEvent(logger).Info(fmt.Sprintf("Swarm step %d: LLM response received", i)).
			WithFields(
				logging.Int("contentLength", len(resp.Content)),
				logging.Int("toolCalls", len(resp.ToolCalls)),
			).
			Log()

		// Append the assistant's message.
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		diagDir := os.Getenv(zqkenv.DiagnosticsDir())
		if diagDir != "" {
			// Write to a session-specific file to prevent interleaved data during highly concurrent multi-agent workflows
			logPath := filepath.Join(diagDir, fmt.Sprintf("llm-monologue-%s.log", e.engineID))
			if err := os.MkdirAll(filepath.Dir(logPath), paths.DirPerm755); err == nil {
				if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, paths.FilePerm644); err == nil {
					var toolOutput string
					if len(resp.ToolCalls) > 0 {
						if b, jErr := json.MarshalIndent(resp.ToolCalls, "", "  "); jErr == nil {
							toolOutput = fmt.Sprintf("\n[Tool Calls]:\n%s", string(b))
						}
					}

					// Format the step header and content into a single block to ensure atomic appends
					logBlock := fmt.Sprintf("\n\n========== [Session: %s] LLM STEP %d ==========\n%s%s\n",
						e.engineID, i, resp.Content, toolOutput)

					f.WriteString(logBlock)
					f.Close()
				}
			}
		}

		if e.OnStep != nil {
			_ = e.OnStep(ctx, fmt.Sprintf("Step %d:\n%s", i, resp.Content))
		}
		logging.FluentEvent(logger).Debug(fmt.Sprintf("LLM STEP %d", i)).
			WithFields(logging.String("content", resp.Content)).
			Log()
		for _, call := range resp.ToolCalls {
			logging.FluentEvent(logger).Debug(fmt.Sprintf("TOOL CALL: %s", call.Name)).Log()
		}

		if len(resp.ToolCalls) == 0 {
			// FALLBACK: Try to parse a hallucinated tool call from a markdown JSON block
			// if the model ignored the native API
			parsedToolCalls, err := extractHallucinatedToolCalls(resp.Content)
			if err == nil && len(parsedToolCalls) > 0 {
				resp.ToolCalls = parsedToolCalls
				logging.FluentEvent(logger).Warn("Intercepted hallucinated tool calls from markdown content").WithFields(logging.Int("count", len(parsedToolCalls))).Log()
			} else {
				// No more tool calls; the model provided a final answer.
				logging.FluentEvent(logger).Info("Swarm run loop completed successfully (no tool calls).").Log()
				return resp.Content, nil
			}
		}

		// Record tool calls and check for repeating sequence pattern (circuit-breaker)
		for _, call := range resp.ToolCalls {
			toolCallHistory = append(toolCallHistory, ToolCallRecord{
				Name:      call.Name,
				Arguments: call.Arguments,
			})
		}
		if tripped, reason := detectRepeatingPattern(toolCallHistory); tripped {
			logging.FluentEvent(logger).Warn("Swarm run loop aborted: circuit breaker tripped").
				WithFields(logging.String("reason", reason)).Log()
			return "", errfmt.Errorf("circuit breaker tripped: %s", reason)
		}

		// Execute tools
		for callIdx, call := range resp.ToolCalls {
			originalName := call.Name
			// Normalization
			call.Name = strings.ReplaceAll(strings.TrimSpace(call.Name), " ", "_")

			activeCnt := globalActiveMCPToolCalls.Add(1)
			var result string
			var err error

			// Pre-circuit-breaker: detect consecutive duplicate calls and inject
			// guidance instead of executing. This gives the LLM one chance to
			// self-correct before the circuit breaker trips on the next iteration.
			if DetectConsecutiveDuplicate(toolCallHistory, ToolCallRecord{Name: call.Name, Arguments: call.Arguments}) {
				result = DuplicateCallGuidanceMessage(call.Name)
				logging.FluentEvent(logger).Warn("Consecutive duplicate tool call detected (pre-circuit-breaker guidance injected)").
					WithFields(
						logging.String("toolName", call.Name),
						logging.String("engineID", e.engineID),
					).
					Log()

				historyIdx := len(toolCallHistory) - len(resp.ToolCalls) + callIdx
				if historyIdx >= 0 && historyIdx < len(toolCallHistory) {
					toolCallHistory[historyIdx].SoftBlocked = true
				}

				globalActiveMCPToolCalls.Add(-1)
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: call.ID,
					Name:       call.Name,
				})
				continue
			}

			// Context-gathering budget check: if model is in a loop of status checks,
			// intercept read-only status calls and inject guidance instead of executing.
			if IsContextGatheringTool(call.Name) && DetectContextGatheringLoop(toolCallHistory) {
				result = ContextGatheringLoopGuidance()
				logging.FluentEvent(logger).Warn("Context-gathering loop detected (guidance injected)").
					WithFields(
						logging.String("toolName", call.Name),
						logging.String("engineID", e.engineID),
					).
					Log()

				historyIdx := len(toolCallHistory) - len(resp.ToolCalls) + callIdx
				if historyIdx >= 0 && historyIdx < len(toolCallHistory) {
					toolCallHistory[historyIdx].SoftBlocked = true
				}

				globalActiveMCPToolCalls.Add(-1)
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: call.ID,
					Name:       call.Name,
				})
				continue
			}

			// Detect repeating multi-tool cycle (by name)
			if tripped, period := DetectRepeatingCycle(toolCallHistory); tripped {
				// Check if the previous iteration of this cycle was already soft-blocked
				hasSoftBlockedCycle := false
				n := len(toolCallHistory)
				for i := 0; i < period; i++ {
					prevIdx := n - 2*period + i
					if prevIdx >= 0 && prevIdx < len(toolCallHistory) && toolCallHistory[prevIdx].SoftBlocked {
						hasSoftBlockedCycle = true
						break
					}
				}

				if hasSoftBlockedCycle {
					logging.FluentEvent(logger).Warn("Swarm run loop aborted: cycle circuit breaker tripped").
						WithFields(
							logging.Int("period", period),
							logging.String("engineID", e.engineID),
						).Log()
					return "", errfmt.Errorf("cycle circuit breaker tripped: repeating cycle of period %d detected after guidance", period)
				}

				result = RepeatingCycleGuidance(period)
				logging.FluentEvent(logger).Warn("Repeating multi-tool cycle detected (guidance injected)").
					WithFields(
						logging.String("toolName", call.Name),
						logging.Int("period", period),
						logging.String("engineID", e.engineID),
					).
					Log()

				historyIdx := len(toolCallHistory) - len(resp.ToolCalls) + callIdx
				if historyIdx >= 0 && historyIdx < len(toolCallHistory) {
					toolCallHistory[historyIdx].SoftBlocked = true
				}

				globalActiveMCPToolCalls.Add(-1)
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: call.ID,
					Name:       call.Name,
				})
				continue
			}

			toolExists := false
			var closestMatch string
			var minDist int = 9999

			for _, t := range tools {
				if t.Name == call.Name {
					toolExists = true
					break
				}
				dist := levenshtein(call.Name, t.Name)
				if dist < minDist {
					minDist = dist
					closestMatch = t.Name
				}
			}

			if !toolExists && minDist > 3 {
				err = fmt.Errorf("tool '%s' not found", originalName)
				if closestMatch != "" {
					err = fmt.Errorf("tool '%s' not found. Did you mean: %s? Use exact names", originalName, closestMatch)
				}
				result = err.Error()
				logging.FluentEvent(logger).Warn("Tool not found (feedback loop injected)").
					WithFields(
						logging.String("toolName", originalName),
						logging.String("normalizedName", call.Name),
					).
					Log()
			} else {
				// R2: Soft-block execute_bash for non-test/non-build commands
				if call.Name == ExecuteBashToolName() && ShouldSoftBlockBashTool(call.Arguments) {
					result = BashSoftBlockGuidanceMessage()
					logging.FluentEvent(logger).Warn("Soft-blocked execute_bash call (steering toward specific tools)").
						WithFields(
							logging.String("toolName", call.Name),
							logging.String("arguments", call.Arguments),
							logging.String("engineID", e.engineID),
						).
						Log()
				} else {
					logging.FluentEvent(logger).Info("Executing MCP Tool").
						WithFields(
							logging.String("toolName", call.Name),
							logging.String("toolCallID", call.ID),
							logging.String("engineID", e.engineID),
							logging.Int("active_instances", int(activeCnt)),
						).
						Log()

					result, err = e.executor.ExecuteToolCall(ctx, call)

					if err != nil {
						// We pass the error back to the LLM.
						result = fmt.Sprintf("Error executing tool %s: %v", call.Name, err)
						logging.FluentEvent(logger).Warn("MCP Tool execution returned error").
							WithFields(
								logging.String("toolName", call.Name),
								logging.String("error", err.Error()),
							).
							Log()
					} else {
						logging.FluentEvent(logger).Debug("MCP Tool execution successful").
							WithFields(
								logging.String("toolName", call.Name),
								logging.Int("resultLength", len(result)),
							).
							Log()
					}
				}
			}

			globalActiveMCPToolCalls.Add(-1)

			// Truncate extremely long tool responses to protect context window
			if len(result) > 25000 {
				logging.FluentEvent(logger).Warn("Truncating massive tool response").
					WithFields(
						logging.String("toolName", call.Name),
						logging.Int("originalLength", len(result)),
					).Log()
				result = result[:25000] + "\n\n... [OUTPUT TRUNCATED BY KERNEL: Exceeds 25KB limit] ..."
			}

			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: call.ID,
				Name:       call.Name,
			})
		}
	}

	logging.FluentEvent(logger).Warn("Swarm run loop aborted: exceeded max steps").Log()
	return "", errfmt.Errorf("exceeded max steps (%d)", e.maxSteps)
}

type ToolCallRecord struct {
	Name        string
	Arguments   string
	SoftBlocked bool
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func levenshtein(s, t string) int {
	if len(s) == 0 {
		return len(t)
	}
	if len(t) == 0 {
		return len(s)
	}

	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := 0; j <= len(t); j++ {
		d[0][j] = j
	}

	for i := 1; i <= len(s); i++ {
		for j := 1; j <= len(t); j++ {
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}
			d[i][j] = min(min(d[i-1][j]+1, d[i][j-1]+1), d[i-1][j-1]+cost)
		}
	}
	return d[len(s)][len(t)]
}

func detectRepeatingPattern(history []ToolCallRecord) (bool, string) {
	n := len(history)
	// We check periods P from 1 to 4
	for p := 1; p <= 4; p++ {
		// To detect 3 consecutive repetitions of period P, we need at least 3 * P elements
		minLen := 3 * p
		if n < minLen {
			continue
		}

		// Check if the last 3 chunks of length P are identical
		match := true
		hasSoftBlocked := false
		for i := 0; i < p; i++ {
			idx1 := n - p + i
			idx2 := n - 2*p + i
			idx3 := n - 3*p + i
			if history[idx1].Name != history[idx2].Name || history[idx1].Arguments != history[idx2].Arguments ||
				history[idx1].Name != history[idx3].Name || history[idx1].Arguments != history[idx3].Arguments {
				match = false
				break
			}
			if history[idx2].SoftBlocked || history[idx3].SoftBlocked {
				hasSoftBlocked = true
			}
		}
		if match && hasSoftBlocked {
			patternDesc := ""
			for i := 0; i < p; i++ {
				patternDesc += history[n-3*p+i].Name + " "
			}
			return true, fmt.Sprintf("detected repeating pattern of period %d: [%s] repeating 3 times consecutively after guidance", p, strings.TrimSpace(patternDesc))
		}
	}
	return false, ""
}

func extractHallucinatedToolCalls(content string) ([]llm.ToolCall, error) {
	var toolCalls []llm.ToolCall

	// First check if it's just raw JSON wrapped in extra braces or standard braces
	cleanContent := strings.TrimSpace(content)
	if strings.HasPrefix(cleanContent, "{{") && strings.HasSuffix(cleanContent, "}}") {
		cleanContent = cleanContent[1 : len(cleanContent)-1]
	}

	jsonContent := cleanContent

	// If it contains a markdown block, extract it
	startIdx := strings.Index(content, "```json")
	if startIdx == -1 {
		startIdx = strings.Index(content, "```")
		if startIdx != -1 {
			endIdx := strings.Index(content[startIdx+3:], "```")
			if endIdx != -1 {
				jsonContent = content[startIdx+3 : startIdx+3+endIdx]
			}
		}
	} else {
		endIdx := strings.Index(content[startIdx+7:], "```")
		if endIdx != -1 {
			jsonContent = content[startIdx+7 : startIdx+7+endIdx]
		}
	}

	type ToolCallParse struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}

	lines := strings.Split(jsonContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var parsed ToolCallParse
		if err := json.Unmarshal([]byte(line), &parsed); err == nil && parsed.Name != "" {
			argsBytes, _ := json.Marshal(parsed.Arguments)
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:        fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), len(toolCalls)),
				Name:      parsed.Name,
				Arguments: string(argsBytes),
			})
		}
	}

	if len(toolCalls) > 0 {
		return toolCalls, nil
	}

	// Fallback to single block unmarshal if JSON lines didn't work (e.g. pretty-printed JSON)
	var parsed ToolCallParse
	if err := json.Unmarshal([]byte(jsonContent), &parsed); err != nil {
		return nil, err
	}

	argsBytes, _ := json.Marshal(parsed.Arguments)

	toolCalls = append(toolCalls, llm.ToolCall{
		ID:        fmt.Sprintf("call_%d", time.Now().UnixNano()),
		Name:      parsed.Name,
		Arguments: string(argsBytes),
	})

	return toolCalls, nil
}
