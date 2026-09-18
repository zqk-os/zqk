package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/process"
)

var globalActiveMCPToolCalls atomic.Int32

// PreserveToolSchemas disables provider-profile schema compression.
// A 4KiB cap exceeded a 13-tool allowlist (~4.5KiB) and stripped every
// description, after which AgentX invented CLI names (zqk_agent_orchestrate,
// zqk_feed_status) that are not MCP tools and aborted at max steps.
const PreserveToolSchemas = 0

// maxMutationNoToolCorrections is how many empty completions replay
// write-now guidance before the run parks. Models often need a second
// or third nudge after an initial status-only turn or dropped invalid probe.
// TRACK: BLI-COMMS-ORCH-EXECUTE-NOT-ACK-001
const maxMutationNoToolCorrections = 3

// Engine defines the cognitive run loop bridging LLM and MCP Executor.
type Engine struct {
	client                     llm.Client
	executor                   Executor
	maxSteps                   int
	maxToolSchemaSize          int
	engineID                   string
	sessionID                  string
	taskID                     string
	minSuccessfulToolCalls     int
	requiredAnySuccessfulTools []string
	completionVerifier         CompletionVerifier
	hooks                      []AgentHook
	maxCompletionRepairs       int
	toolAllowlist              []string
	extraSystem                string
	OnStep                     func(ctx context.Context, stepLog string) error
}

// CompletionVerifier inspects a run's tool calls when the model stops calling
// tools. A non-empty feedback string rejects the completion and is replayed to
// the model as another turn, so it can repair work that a tool-name check
// cannot judge — code that was written but does not compile.
type CompletionVerifier func(ctx context.Context, history []ToolCallRecord) (feedback string, err error)

// DefaultHooks returns the baseline supervisory hooks for swarm engine runs.
func DefaultHooks() []AgentHook {
	return []AgentHook{
		NewToolDenialGuard(),
		NewHallucinationCircuitBreaker(),
	}
}

func NewEngine(client llm.Client, executor Executor, maxToolSchemaSize int, engineID string) *Engine {
	maxSteps := 30
	if envMax := config.SwarmMaxSteps().OrDefault(0); envMax > 0 {
		maxSteps = envMax
	}
	return &Engine{
		client:            client,
		executor:          executor,
		maxSteps:          maxSteps,
		maxToolSchemaSize: maxToolSchemaSize,
		engineID:          engineID,
		hooks:             DefaultHooks(),
	}
}

// WithHooks appends hooks to the engine.
func (e *Engine) WithHooks(hooks ...AgentHook) *Engine {
	e.hooks = append(e.hooks, hooks...)
	return e
}

// SetHooks overrides hooks on the engine.
func (e *Engine) SetHooks(hooks ...AgentHook) *Engine {
	e.hooks = hooks
	return e
}

// Hooks returns the engine's current hooks.
func (e *Engine) Hooks() []AgentHook {
	return e.hooks
}

// RequireSuccessfulToolCalls makes narrative-only completion fail closed until
// the run has executed at least min MCP calls successfully.
func (e *Engine) RequireSuccessfulToolCalls(min int) *Engine {
	if min > 0 {
		e.minSuccessfulToolCalls = min
	}
	return e
}

// RequireAnySuccessfulTools fail-closes completion until at least one successful
// MCP call matches any of the listed tool names (exact match after normalization).
// Use for coding seats where status/list probes must not count as work evidence.
func (e *Engine) RequireAnySuccessfulTools(names ...string) *Engine {
	for _, name := range names {
		name = strings.ReplaceAll(strings.TrimSpace(name), " ", "_")
		if name == "" {
			continue
		}
		e.requiredAnySuccessfulTools = append(e.requiredAnySuccessfulTools, name)
	}
	return e
}

// VerifyCompletionWith fail-closes completion on an outcome check rather than a
// tool-name check. The model gets up to maxRepairs extra turns to act on the
// verifier's feedback before the run fails.
// RestrictTools keeps only the named MCP tools on the wire. Empty means
// the executor's full eager set. Used for code-draft models.
func (e *Engine) RestrictTools(names ...string) *Engine {
	e.toolAllowlist = e.toolAllowlist[:0]
	for _, name := range names {
		name = strings.ReplaceAll(strings.TrimSpace(name), " ", "_")
		if name != "" {
			e.toolAllowlist = append(e.toolAllowlist, name)
		}
	}
	return e
}

// WithExtraSystem appends guidance after the caller system prompt (and
// before the Available Tools suffix).
func (e *Engine) WithExtraSystem(s string) *Engine {
	e.extraSystem = strings.TrimSpace(s)
	return e
}

func (e *Engine) VerifyCompletionWith(verify CompletionVerifier, maxRepairs int) *Engine {
	if verify == nil {
		return e
	}
	e.completionVerifier = verify
	if maxRepairs < 0 {
		maxRepairs = 0
	}
	e.maxCompletionRepairs = maxRepairs
	return e
}

// MutationEvidenceTools returns write_code / write_file names under the brand
// tool prefix — interim ATK completion evidence for seat-worker coding seats.
// A successful write is not proof of work: it accepts code that does not
// compile, and seats have looped rewriting a broken file to satisfy it.
// TRACK: BLI-1786948736717976000-a0522aac — remove when the compile + test
// completion gate replaces tool-name evidence (POL-AGENT-COMMS-CHECK-001).
func MutationEvidenceTools() []string {
	return mutationEvidenceNames(DefaultToolPrefix(), brand.ProductNamespacePrefix(brand.NamespacePrefix())+"_")
}

// IsMutationEvidenceTool reports whether name is a write_code / write_file tool
// under any brand or channel prefix (zqk_write_file and zqk-stable_write_file
// both count). MCP strips -stable from the child process; seat-workers that
// still namespace as zqk-stable must not drop a successful write.
func IsMutationEvidenceTool(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(n, "_write_code") || strings.HasSuffix(n, "_write_file") ||
		n == "write_code" || n == "write_file"
}

func mutationEvidenceNames(prefixes ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, p := range prefixes {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.HasSuffix(p, "_") {
			p += "_"
		}
		for _, suffix := range []string{"write_code", "write_file"} {
			name := p + suffix
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	return out
}

func toolNameAllowed(name string, allow []string) bool {
	if IsMutationEvidenceTool(name) {
		for _, want := range allow {
			if IsMutationEvidenceTool(want) {
				return true
			}
		}
	}
	for _, want := range allow {
		if name == want {
			return true
		}
	}
	return false
}

// Run executes the cognitive loop until the LLM produces a final string result without tool calls.
func (e *Engine) Run(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	logger := logging.GetLogger()
	logging.FluentEvent(logger).Info("Swarm engine run loop started").
		WithFields(e.fields(ctx,
			logging.Int("maxSteps", e.maxSteps),
			logging.Int("userPromptLength", len(userPrompt)),
		)...).
		Log()

	tools, err := e.executor.GetTools(ctx)
	if err != nil {
		return "", errfmt.Newf("failed to get tools initially").Wrap(err)
	}
	tools = filterToolsByAllowlist(tools, e.toolAllowlist)

	var toolNames []string
	for _, t := range tools {
		toolNames = append(toolNames, t.Name)
	}
	injectedSystemPrompt := systemPrompt
	if e.extraSystem != "" {
		injectedSystemPrompt += "\n\n" + e.extraSystem
	}
	injectedSystemPrompt += FormatAvailableToolsSuffix(toolNames, DefaultToolPrefix())

	messages := []llm.Message{
		{Role: "system", Content: injectedSystemPrompt},
		{Role: "user", Content: userPrompt},
	}

	var toolCallHistory []ToolCallRecord
	successfulToolCalls := 0
	matchedRequiredToolCalls := 0
	noToolCorrections := 0
	completionRepairs := 0
	markdownRecoveries := 0
	lookupNudgeSent := false

	for i := 0; i < e.maxSteps; i++ {
		tools, err := e.executor.GetTools(ctx)
		if err != nil {
			return "", errfmt.Newf("failed to get tools").Wrap(err)
		}
		tools = filterToolsByAllowlist(tools, e.toolAllowlist)

		needEvidence := (e.minSuccessfulToolCalls > 0 && successfulToolCalls < e.minSuccessfulToolCalls) ||
			(len(e.requiredAnySuccessfulTools) > 0 && matchedRequiredToolCalls == 0)
		wroteEvidence := len(e.requiredAnySuccessfulTools) > 0 && matchedRequiredToolCalls > 0
		if needEvidence {
			switch {
			case ShouldForceWriteOnly(true, toolCallHistory):
				tools = filterToolsByAllowlist(tools, MutationEvidenceTools())
			case ShouldNarrowToWriteMenu(true, toolCallHistory):
				tools = filterToolsByAllowlist(tools, CodeDraftToolNames(DefaultToolPrefix()))
			}
			if ShouldNarrowToWriteMenu(true, toolCallHistory) && !lookupNudgeSent {
				lookupNudgeSent = true
				messages = append(messages, llm.Message{Role: "user", Content: unpaidLookupNudge()})
				logging.FluentEvent(logger).Warn("Unpaid lookup budget spent; narrowing tool menu to write").
					WithFields(e.fields(ctx,
						logging.Int("unpaidLookupStreak", UnpaidLookupStreak(toolCallHistory)),
						logging.Int("toolCount", len(tools)),
					)...).Log()
			}
		} else if wroteEvidence {
			// Live 3.6 paid the write pin then got the full eager menu back
			// and spent the remaining timeout on object_get plus rewrites.
			// Keep write/read/test only so the next turn is vet, not another
			// kernel list. TRACK: BLI-SWM-002
			tools = filterToolsByAllowlist(tools, CodeDraftToolNames(DefaultToolPrefix()))
		}

		logging.FluentEvent(logger).Debug(fmt.Sprintf("Swarm step %d: Requesting LLM completion", i)).
			WithFields(e.fields(ctx, logging.Int("toolCount", len(tools)))...).
			Log()

		// Token Budgeting / Context Compaction
		contextWindow := zqkenv.Get(zqkenv.LLMContextWindowSize().Name()).IntOrDefault(32768)
		tracker := NewTokenTracker(contextWindow, 0.9)

		promptTokens, historyTokens, totalTokens := tracker.EstimateTokens(messages, tools)
		logging.FluentEvent(logger).Info("Swarm step token budget check").
			WithFields(e.fields(ctx,
				logging.Int("step", i),
				logging.Int("promptTokens", promptTokens),
				logging.Int("historyTokens", historyTokens),
				logging.Int("totalTokens", totalTokens),
				logging.Int("contextWindowLimit", contextWindow),
			)...).
			Log()

		if !tracker.ValidateBudget(totalTokens) {
			logging.FluentEvent(logger).Info("Token budget exceeded limit, compacting history").
				WithFields(e.fields(ctx,
					logging.Int("oldTotalTokens", totalTokens),
					logging.Int("messageCount", len(messages)),
				)...).
				Log()

			var ok bool
			messages, ok = tracker.CompactHistory(messages, tools)
			if !ok {
				logging.FluentEvent(logger).Error("Swarm run loop aborted: prompt size exceeds context budget limit", nil).
					WithFields(e.fields(ctx)...).Log()
				return "", errfmt.Errorf("prompt size exceeds context budget limit: %d tokens", totalTokens)
			}

			// Recalculate after compaction
			_, _, totalTokens = tracker.EstimateTokens(messages, tools)
			logging.FluentEvent(logger).Info("Token budget after history compaction").
				WithFields(e.fields(ctx,
					logging.Int("newTotalTokens", totalTokens),
					logging.Int("messageCount", len(messages)),
				)...).
				Log()
		}

		// Provider Profile Schema Compression
		if e.maxToolSchemaSize > 0 {
			tb, _ := json.Marshal(tools)
			if len(tb) > e.maxToolSchemaSize {
				logging.FluentEvent(logger).Warn("Tool schema exceeded provider limit; compressing schema.").
					WithFields(e.fields(ctx, logging.Int("originalSize", len(tb)), logging.Int("maxSize", e.maxToolSchemaSize))...).Log()

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
			WithFields(e.fields(ctx, logging.Int("toolCount", len(tools)), logging.Int("jsonLength", len(tb)))...).
			Log()

		pinTool := ""
		if needEvidence && len(e.requiredAnySuccessfulTools) > 0 {
			pinTool = e.requiredAnySuccessfulTools[0]
		}
		choiceState := toolChoiceState{
			ToolsPresent:          len(tools) > 0,
			NeedExecutionEvidence: needEvidence,
			MarkdownRecoveries:    markdownRecoveries,
			NoToolCorrections:     noToolCorrections,
			PinTool:               pinTool,
		}
		choice := nextToolChoice(choiceState)
		choiceFn := nextToolChoiceFunction(choiceState)
		prompt := messages
		resp, err := llm.StructuredCompletion(ctx, e.client, messages, tools, llm.StructuredCompletionOptions{
			ToolChoice:         choice,
			ToolChoiceFunction: choiceFn,
		})
		if err != nil {
			logging.FluentEvent(logger).Error("LLM generation failed", err).WithFields(e.fields(ctx, logging.Int("step", i))...).Log()
			return "", errfmt.Newf("generation failed").Wrap(err)
		}
		process.TouchMeaningfulActivity()

		logging.FluentEvent(logger).Info(fmt.Sprintf("Swarm step %d: LLM response received", i)).
			WithFields(e.fields(ctx,
				logging.Int("contentLength", len(resp.Content)),
				logging.Int("toolCalls", len(resp.ToolCalls)),
				logging.String("toolChoice", choice),
				logging.String("toolChoiceFunction", choiceFn),
			)...).
			Log()

		nativeCalls := append([]llm.ToolCall(nil), resp.ToolCalls...)
		stepContent := resp.Content
		recoveredMarkdown := recoverMarkdownToolCalls(&resp)
		var droppedRecovered []string
		if recoveredMarkdown {
			markdownRecoveries++
			resp.ToolCalls, droppedRecovered = filterRecoveredToolCalls(true, e.requiredAnySuccessfulTools, resp.ToolCalls)
			logging.FluentEvent(logger).Warn("Intercepted hallucinated tool calls from markdown content").
				WithFields(e.fields(ctx,
					logging.CountField(len(resp.ToolCalls)),
					logging.Int("markdownRecoveries", markdownRecoveries),
					logging.Int("droppedRecovered", len(droppedRecovered)),
					logging.String("toolChoice", choice),
				)...).Log()
		}
		if needEvidence {
			var droppedNative []string
			filtered, dropped := filterUnpaidLookupCalls(true, toolCallHistory, resp.ToolCalls)
			if len(filtered) > 0 {
				resp.ToolCalls = filtered
				droppedNative = dropped
			}
			if len(droppedNative) > 0 {
				droppedRecovered = append(droppedRecovered, droppedNative...)
				logging.FluentEvent(logger).Warn("Dropped native lookup tools while write pin is unpaid").
					WithFields(e.fields(ctx,
						logging.Int("droppedNative", len(droppedNative)),
						logging.Int("unpaidLookupStreak", UnpaidLookupStreak(toolCallHistory)),
					)...).Log()
			}
		}

		writeLLMTrace(llmTraceRecord{
			EngineID:           e.engineID,
			Step:               i,
			ToolChoice:         choice,
			ToolChoiceFunction: choiceFn,
			ToolNames:          toolDefinitionNames(tools),
			Prompt:             prompt,
			RawContent:         stepContent,
			NativeToolCalls:    nativeCalls,
			RecoveredMarkdown:  recoveredMarkdown,
			ExecutedToolCalls:  resp.ToolCalls,
			DroppedToolNames:   droppedRecovered,
		})

		// Append after recovery so the transcript carries native tool_calls,
		// not the prose JSON that taught the 7B the wrong convention.
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		if e.OnStep != nil {
			_ = e.OnStep(ctx, fmt.Sprintf("Step %d:\n%s", i, stepContent))
		}
		logging.FluentEvent(logger).Debug(fmt.Sprintf("LLM STEP %d", i)).
			WithFields(e.fields(ctx, logging.String("content", stepContent))...).
			Log()
		for _, call := range resp.ToolCalls {
			logging.FluentEvent(logger).Debug(fmt.Sprintf("TOOL CALL: %s", call.Name)).
				WithFields(e.fields(ctx)...).Log()
		}

		if len(resp.ToolCalls) == 0 {
			needCount := successfulToolCalls < e.minSuccessfulToolCalls
			needNamed := len(e.requiredAnySuccessfulTools) > 0 && matchedRequiredToolCalls == 0
			if needCount || needNamed {
				if noToolCorrections >= maxMutationNoToolCorrections {
					if needNamed {
						return "", errfmt.Errorf(
							"required mutation tool evidence not met: need one of %v (matched %d)",
							e.requiredAnySuccessfulTools,
							matchedRequiredToolCalls,
						)
					}
					return "", errfmt.Errorf(
						"minimum successful tool calls not met: got %d, want %d",
						successfulToolCalls,
						e.minSuccessfulToolCalls,
					)
				}
				noToolCorrections++
				guidance := fmt.Sprintf(
					"Your narrative is not execution evidence. You have completed %d successful tool calls; at least %d are required.",
					successfulToolCalls,
					e.minSuccessfulToolCalls,
				)
				if needNamed {
					guidance = fmt.Sprintf(
						"Status/list probes are not task evidence. You must successfully invoke one of these mutation tools before completing: %s. Do not claim files, tests, commits, or completion without those tool results.",
						strings.Join(e.requiredAnySuccessfulTools, ", "),
					)
				} else {
					guidance += " Invoke an exact tool from Available Tools now via the native function-calling API. Do not write JSON in your message."
				}
				messages = append(messages, llm.Message{Role: "user", Content: guidance})
				continue
			}
			if e.completionVerifier != nil {
				feedback, verifyErr := e.completionVerifier(ctx, toolCallHistory)
				if verifyErr != nil {
					return "", errfmt.Newf("completion verification failed").Wrap(verifyErr)
				}
				if feedback != "" {
					if completionRepairs >= e.maxCompletionRepairs {
						return "", errfmt.Errorf(
							"completion rejected after %d repair attempt(s): %s",
							completionRepairs,
							feedback,
						)
					}
					completionRepairs++
					logging.FluentEvent(logger).Warn("Swarm completion rejected; replaying verifier feedback").
						WithFields(e.fields(ctx, logging.Int("repair", completionRepairs))...).Log()
					messages = append(messages, llm.Message{Role: "user", Content: feedback})
					continue
				}
			}
			// No more tool calls; the model provided a final answer after
			// satisfying any caller-required execution evidence.
			logging.FluentEvent(logger).Info("Swarm run loop completed successfully (no tool calls).").
				WithFields(e.fields(ctx)...).Log()
			return stepContent, nil
		}

		// Record tool calls and check for repeating sequence pattern (circuit-breaker)
		for _, call := range resp.ToolCalls {
			toolCallHistory = append(toolCallHistory, ToolCallRecord{
				Turn:      i,
				Name:      call.Name,
				Arguments: call.Arguments,
			})
		}
		if tripped, reason := detectRepeatingPattern(toolCallHistory); tripped {
			logging.FluentEvent(logger).Warn("Swarm run loop aborted: circuit breaker tripped").
				WithFields(e.fields(ctx, logging.String("reason", reason))...).Log()
			return "", errfmt.Errorf("circuit breaker tripped: %s", reason)
		}

		// Evaluate multi-tool cycle breaker once per turn
		cycleTripped, cyclePeriod := DetectRepeatingCycle(toolCallHistory)
		if cycleTripped {
			var hardAbort bool
			for i := 0; i < len(toolCallHistory)-len(resp.ToolCalls); i++ {
				if toolCallHistory[i].SoftBlocked {
					hardAbort = true
					break
				}
			}
			if hardAbort {
				logging.FluentEvent(logger).Warn("Swarm run loop aborted: cycle circuit breaker tripped").
					WithFields(e.fields(ctx, logging.PeriodField(cyclePeriod))...).Log()
				return "", errfmt.Errorf("cycle circuit breaker tripped: repeating cycle detected after guidance")
			}
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
					WithFields(e.fields(ctx, logging.ToolNameField(call.Name))...).
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
					WithFields(e.fields(ctx, logging.ToolNameField(call.Name))...).
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

			// Inject guidance if the cycle breaker tripped for this turn
			if cycleTripped {
				result = RepeatingCycleGuidance(cyclePeriod)
				logging.FluentEvent(logger).Warn("Repeating multi-tool cycle detected (guidance injected)").
					WithFields(e.fields(ctx, logging.ToolNameField(call.Name), logging.PeriodField(cyclePeriod))...).
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

			// Force-write guidance: if code exploration budget is spent and the call is not mutation evidence
			if ShouldForceWriteOnly(needEvidence, toolCallHistory) && !IsMutationEvidenceTool(call.Name) {
				if tripped, reason := CheckReadLoopWatchdog(toolCallHistory); tripped && reason != "" {
					result = reason
				} else {
					result = "Code exploration budget spent. You must invoke write_code or write_file to implement the solution."
				}
				logging.FluentEvent(logger).Warn("Non-mutation tool called during force-write mode (guidance injected)").
					WithFields(e.fields(ctx, logging.ToolNameField(call.Name))...).
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

			if steerMsg, steered := guardSwarmToolCall(call); steered {
				result = steerMsg
				logging.FluentEvent(logger).Info("Steered invented or restricted swarm tool").
					WithFields(e.fields(ctx,
						logging.ToolNameField(originalName),
						logging.NormalizedNameField(call.Name),
					)...).
					Log()
			} else if !toolExists && minDist > 3 {
				err = fmt.Errorf("tool '%s' not found", originalName)
				if closestMatch != "" {
					err = fmt.Errorf("tool '%s' not found. Did you mean: %s? Use exact names", originalName, closestMatch)
				}
				result = err.Error()
				logging.FluentEvent(logger).Warn("Tool not found (feedback loop injected)").
					WithFields(e.fields(ctx,
						logging.ToolNameField(originalName),
						logging.NormalizedNameField(call.Name),
					)...).
					Log()
			} else {
				// R2: Soft-block execute_bash for non-test/non-build commands
				if call.Name == ExecuteBashToolName() && ShouldSoftBlockBashTool(call.Arguments) {
					result = "Soft-blocked: " + BashSoftBlockGuidanceMessage()
					logging.FluentEvent(logger).Warn("Soft-blocked execute_bash call (steering toward specific tools)").
						WithFields(e.fields(ctx,
							logging.ToolNameField(call.Name),
							logging.ArgumentsField(call.Arguments),
						)...).
						Log()

					historyIdx := len(toolCallHistory) - len(resp.ToolCalls) + callIdx
					if historyIdx >= 0 && historyIdx < len(toolCallHistory) {
						toolCallHistory[historyIdx].SoftBlocked = true
					}

					var hookAbort error
					for _, hook := range e.hooks {
						_, hErr := hook.PostTool(ctx, call, result, nil)
						if hErr != nil {
							hookAbort = hErr
							break
						}
					}
					if hookAbort != nil {
						logging.FluentEvent(logger).Error("Swarm loop aborted by PostTool hook", hookAbort).WithFields(e.fields(ctx)...).Log()
						return "", hookAbort
					}
				} else {
					// 1. PreTool Hooks
					var hookAbort error
					for _, hook := range e.hooks {
						if hErr := hook.PreTool(ctx, call); hErr != nil {
							hookAbort = hErr
							break
						}
					}
					if hookAbort != nil {
						logging.FluentEvent(logger).Error("Swarm loop aborted by PreTool hook", hookAbort).WithFields(e.fields(ctx)...).Log()
						return "", hookAbort
					}

					logging.FluentEvent(logger).Info("Executing MCP Tool").
						WithFields(e.fields(ctx,
							logging.ToolNameField(call.Name),
							logging.ToolCallIDField(call.ID),
							logging.ActiveInstancesField(int(activeCnt)),
						)...).
						Log()

					result, err = e.executor.ExecuteToolCall(ctx, call)
					process.TouchMeaningfulActivity()

					// 2. PostTool Hooks
					var steeringPrompt string
					for _, hook := range e.hooks {
						steer, hErr := hook.PostTool(ctx, call, result, err)
						if hErr != nil {
							hookAbort = hErr
							break
						}
						if steer != "" {
							steeringPrompt = steer
						}
					}
					if hookAbort != nil {
						logging.FluentEvent(logger).Error("Swarm loop aborted by PostTool hook", hookAbort).WithFields(e.fields(ctx)...).Log()
						return "", hookAbort
					}

					if err != nil {
						// We pass the error back to the LLM.
						result = fmt.Sprintf("Error executing tool %s: %v", call.Name, err)
					}
					if steeringPrompt != "" {
						result = result + "\n\n" + steeringPrompt
					}
					if err != nil {
						logging.FluentEvent(logger).Warn("MCP Tool execution returned error").
							WithFields(e.fields(ctx,
								logging.ToolNameField(call.Name),
								logging.ErrorTextField(err.Error()),
							)...).
							Log()
					} else {
						successfulToolCalls++
						if toolNameAllowed(call.Name, e.requiredAnySuccessfulTools) {
							matchedRequiredToolCalls++
						}
						logging.FluentEvent(logger).Debug("MCP Tool execution successful").
							WithFields(e.fields(ctx,
								logging.ToolNameField(call.Name),
								logging.Int("resultLength", len(result)),
							)...).
							Log()

						// 3. OnSuccess Hooks (async)
						for _, hook := range e.hooks {
							h := hook
							c := call
							r := result
							goroutinelabels.NewGoroutine("ambient_success_trigger", "ambient workspace check").StartSimple(func() {
								h.OnSuccess(context.Background(), c, r)
							})
						}
					}
				}
			}

			globalActiveMCPToolCalls.Add(-1)

			clipped, truncated := clipSwarmToolResult(call.Name, result)
			if truncated {
				logging.FluentEvent(logger).Warn("Truncating massive tool response").
					WithFields(e.fields(ctx,
						logging.ToolNameField(call.Name),
						logging.Int("originalLength", len(result)),
					)...).Log()
				result = clipped
				historyIdx := len(toolCallHistory) - len(resp.ToolCalls) + callIdx
				if historyIdx >= 0 && historyIdx < len(toolCallHistory) {
					toolCallHistory[historyIdx].SoftBlocked = true
				}
			}

			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: call.ID,
				Name:       call.Name,
			})
		}
		if recoveredMarkdown {
			messages = append(messages, llm.Message{Role: "user", Content: MarkdownToolCallConventionGuidance()})
		}
	}

	if (e.minSuccessfulToolCalls == 0 || successfulToolCalls >= e.minSuccessfulToolCalls) &&
		(len(e.requiredAnySuccessfulTools) == 0 || matchedRequiredToolCalls > 0) {
		logging.FluentEvent(logger).Info("Swarm run loop completed at max steps with satisfied evidence").
			WithFields(e.fields(ctx, logging.Int("successfulToolCalls", successfulToolCalls), logging.Int("matchedRequired", matchedRequiredToolCalls))...).Log()
		return "Execution completed: required mutation evidence satisfied.", nil
	}

	logging.FluentEvent(logger).Warn("Swarm run loop aborted: exceeded max steps").
		WithFields(e.fields(ctx)...).Log()
	return "", errfmt.Errorf("exceeded max steps (%d)", e.maxSteps)
}

type ToolCallRecord struct {
	Turn        int
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

// recoverMarkdownToolCalls fills ToolCalls from assistant prose when the model
// skipped the native tools API. Clears Content so the transcript does not teach
// JSON-in-markdown as the calling convention.
func recoverMarkdownToolCalls(resp *llm.StructuredCompletionResponse) bool {
	if resp == nil || len(resp.ToolCalls) > 0 || strings.TrimSpace(resp.Content) == "" {
		return false
	}
	parsed, err := extractHallucinatedToolCalls(resp.Content)
	if err != nil || len(parsed) == 0 {
		return false
	}
	resp.ToolCalls = parsed
	resp.Content = ""
	return true
}

func extractHermesToolCalls(content string) []llm.ToolCall {
	const open = "<tool_call>"
	const close = "</tool_call>"
	var calls []llm.ToolCall
	rest := content
	for {
		start := strings.Index(rest, open)
		if start == -1 {
			return calls
		}
		rest = rest[start+len(open):]
		end := strings.Index(rest, close)
		if end == -1 {
			return calls
		}
		inner := strings.TrimSpace(rest[:end])
		rest = rest[end+len(close):]
		parsed, err := extractHallucinatedToolCalls(inner)
		if err != nil || len(parsed) == 0 {
			continue
		}
		calls = append(calls, parsed...)
	}
}

func extractHallucinatedToolCalls(content string) ([]llm.ToolCall, error) {
	if hermes := extractHermesToolCalls(content); len(hermes) > 0 {
		return hermes, nil
	}

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
		// Some local models explain the next action and then emit an unfenced,
		// pretty-printed tool object. Decode from the first object boundary so
		// prose does not turn an executable request into a false final answer.
		objectStart := strings.Index(jsonContent, "{")
		if objectStart == -1 {
			if bashCalls := extractBashCodeBlockToolCalls(content); len(bashCalls) > 0 {
				return bashCalls, nil
			}
			return nil, err
		}
		if decodeErr := json.NewDecoder(strings.NewReader(jsonContent[objectStart:])).Decode(&parsed); decodeErr != nil {
			if bashCalls := extractBashCodeBlockToolCalls(content); len(bashCalls) > 0 {
				return bashCalls, nil
			}
			return nil, decodeErr
		}
	}
	if parsed.Name == "" {
		return nil, errfmt.Errorf("tool call name is empty")
	}

	argsBytes, _ := json.Marshal(parsed.Arguments)

	toolCalls = append(toolCalls, llm.ToolCall{
		ID:        fmt.Sprintf("call_%d", time.Now().UnixNano()),
		Name:      parsed.Name,
		Arguments: string(argsBytes),
	})

	return toolCalls, nil
}

func extractBashCodeBlockToolCalls(content string) []llm.ToolCall {
	startIdx := strings.Index(content, "```bash")
	prefixLen := 7
	if startIdx == -1 {
		startIdx = strings.Index(content, "```sh")
		prefixLen = 5
	}
	if startIdx == -1 {
		return nil
	}
	endIdx := strings.Index(content[startIdx+prefixLen:], "```")
	if endIdx == -1 {
		return nil
	}
	script := strings.TrimSpace(content[startIdx+prefixLen : startIdx+prefixLen+endIdx])
	if script == "" {
		return nil
	}
	argsBytes, _ := json.Marshal(map[string]any{
		"command": script,
	})
	return []llm.ToolCall{
		{
			ID:        fmt.Sprintf("call_bash_%d", time.Now().UnixNano()),
			Name:      DefaultToolPrefix() + "execute_bash",
			Arguments: string(argsBytes),
		},
	}
}

// WithMaxSteps overrides the default or configured maximum steps for this engine run.
// Use this to right-size the run duration based on task complexity (e.g., estimated effort or step count).
func (e *Engine) WithMaxSteps(steps int) *Engine {
	if steps > 0 {
		e.maxSteps = steps
	}
	return e
}
