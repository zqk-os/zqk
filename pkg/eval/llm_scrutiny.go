package eval

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// InteractionOutcome classifies the final state of an LLM turn.
type InteractionOutcome string

const (
	OutcomeSuccess   InteractionOutcome = "success"
	OutcomeFailure   InteractionOutcome = "failure"
	OutcomeSyntaxErr InteractionOutcome = "syntax_error"
	OutcomeTimeout   InteractionOutcome = "timeout"
	OutcomeUnknown   InteractionOutcome = "unknown"

	initialLogScanBufferSize = 1024 * 1024
	maxLogScanBufferSize     = 10 * 1024 * 1024
)

// ParsedToolCall represents an extracted tool invocation within an LLM interaction.
type ParsedToolCall struct {
	Name       string         `json:"name"`
	Args       map[string]any `json:"args,omitempty"`
	ExitCode   int            `json:"exit_code"`
	ErrorMsg   string         `json:"error_msg,omitempty"`
	Syntactic  bool           `json:"syntactic"`
	Successful bool           `json:"successful"`
}

// LLMInteraction represents a single atomic model interaction cycle.
type LLMInteraction struct {
	ID           string             `json:"id"`
	Timestamp    time.Time          `json:"timestamp"`
	SourceLog    string             `json:"source_log"`
	SessionID    string             `json:"session_id,omitempty"`
	Model        string             `json:"model"`
	Provider     string             `json:"provider,omitempty"`
	StepIndex    int                `json:"step_index"`
	InputTokens  int                `json:"input_tokens"`
	OutputTokens int                `json:"output_tokens"`
	DurationMs   int64              `json:"duration_ms,omitempty"`
	ToolCalls    []ParsedToolCall   `json:"tool_calls,omitempty"`
	Outcome      InteractionOutcome `json:"outcome"`
	Errors       []string           `json:"errors,omitempty"`
	Violations   []string           `json:"violations,omitempty"`
	RawSnippet   string             `json:"raw_snippet,omitempty"`
}

// ModelRankingMetrics aggregates quality and performance telemetry by model.
type ModelRankingMetrics struct {
	Model              string   `json:"model"`
	Provider           string   `json:"provider"`
	TotalInteractions  int      `json:"total_interactions"`
	SuccessfulCount    int      `json:"successful_count"`
	FailedCount        int      `json:"failed_count"`
	SyntaxErrors       int      `json:"syntax_errors"`
	SuccessRate        float64  `json:"success_rate"`
	SyntacticValidRate float64  `json:"syntactic_valid_rate"`
	ToolExecutionRate  float64  `json:"tool_execution_rate"`
	AvgInputTokens     float64  `json:"avg_input_tokens"`
	AvgOutputTokens    float64  `json:"avg_output_tokens"`
	CompositeScore     float64  `json:"composite_score"`
	Rank               int      `json:"rank"`
	TopFailureModes    []string `json:"top_failure_modes"`
	TuningFeedback     []string `json:"tuning_feedback"`
}

// ScrutinyReport encapsulates the complete scrutiny ranking outcome.
type ScrutinyReport struct {
	GeneratedAt       time.Time             `json:"generated_at"`
	LogsParsed        int                   `json:"logs_parsed"`
	TotalInteractions int                   `json:"total_interactions"`
	ModelsScrutinized int                   `json:"models_scrutinized"`
	Rankings          []ModelRankingMetrics `json:"rankings"`
	Summary           string                `json:"summary"`
}

// ScrutinyOptions controls log discovery, filtering, and report constraints.
type ScrutinyOptions struct {
	LogPaths []string
	Model    string
	Since    time.Duration
	Top      int
	Format   string
}

// LLMScrutinyParser parses diverse log formats into LLMInteractions.
type LLMScrutinyParser struct{}

// NewLLMScrutinyParser returns a new log parser.
func NewLLMScrutinyParser() *LLMScrutinyParser {
	return &LLMScrutinyParser{}
}

type transcriptStepRaw struct {
	StepIndex    int             `json:"step_index"`
	Source       string          `json:"source"`
	Type         string          `json:"type"`
	Status       string          `json:"status"`
	CreatedAt    string          `json:"created_at"`
	InputTokens  *int            `json:"input_tokens"`
	OutputTokens *int            `json:"output_tokens"`
	Content      string          `json:"content"`
	Thinking     string          `json:"thinking"`
	ToolCalls    json.RawMessage `json:"tool_calls"`
}

type transcriptToolCallRaw struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

func newLogScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, initialLogScanBufferSize)
	scanner.Buffer(buf, maxLogScanBufferSize)
	return scanner
}

func stringVal(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// ParseTranscriptJSONL extracts interactions from Antigravity/agentic transcript JSONL streams.
func (p *LLMScrutinyParser) ParseTranscriptJSONL(r io.Reader, sourceName string) ([]LLMInteraction, error) {
	scanner := newLogScanner(r)

	var (
		interactions []LLMInteraction
		pending      *LLMInteraction
		currentModel = "default-assistant"
	)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var step transcriptStepRaw
		if err := json.Unmarshal(line, &step); err != nil {
			if pending != nil {
				pending.Errors = append(pending.Errors, "corrupted_log_line")
				pending.Outcome = OutcomeSyntaxErr
			}
			continue
		}

		currentModel = extractSubagentModel(step, currentModel)

		if step.Type == "PLANNER_RESPONSE" {
			if pending != nil {
				interactions = append(interactions, *pending)
			}
			cur := parsePlannerStep(step, sourceName, currentModel)
			pending = &cur
			continue
		}

		if pending != nil && (step.Type == "GENERIC" || step.Type == "TOOL_RESULT" || step.Source == "SYSTEM") {
			applyToolResult(pending, step)
			interactions = append(interactions, *pending)
			pending = nil
		}
	}

	if pending != nil {
		interactions = append(interactions, *pending)
	}

	return interactions, scanner.Err()
}

func extractSubagentModel(step transcriptStepRaw, fallback string) string {
	if step.Type != "PLANNER_RESPONSE" || len(step.ToolCalls) == 0 {
		return fallback
	}
	var tcalls []transcriptToolCallRaw
	if err := json.Unmarshal(step.ToolCalls, &tcalls); err == nil {
		for _, tc := range tcalls {
			if tc.Name == "invoke_subagent" {
				var args map[string]any
				if err := json.Unmarshal(tc.Args, &args); err == nil {
					m := stringVal(args["Model"])
					if m != "" && m != "inherit" {
						return m
					}
				}
			}
		}
	}
	return fallback
}

func parsePlannerStep(step transcriptStepRaw, sourceName, model string) LLMInteraction {
	ts, err := time.Parse(time.RFC3339, step.CreatedAt)
	if err != nil || ts.IsZero() {
		ts = time.Now()
	}

	inTokens := 0
	if step.InputTokens != nil {
		inTokens = *step.InputTokens
	}
	outTokens := 0
	if step.OutputTokens != nil {
		outTokens = *step.OutputTokens
	}

	cur := LLMInteraction{
		ID:           fmt.Sprintf("%s-step-%d", filepath.Base(sourceName), step.StepIndex),
		Timestamp:    ts,
		SourceLog:    sourceName,
		Model:        model,
		StepIndex:    step.StepIndex,
		InputTokens:  inTokens,
		OutputTokens: outTokens,
		Outcome:      OutcomeSuccess,
	}

	if step.Status == "ERROR" {
		cur.Outcome = OutcomeFailure
		cur.Errors = append(cur.Errors, "step_status_error")
	}

	if len(step.ToolCalls) > 0 && string(step.ToolCalls) != "null" {
		var tcalls []transcriptToolCallRaw
		if err := json.Unmarshal(step.ToolCalls, &tcalls); err != nil {
			cur.Outcome = OutcomeSyntaxErr
			cur.Errors = append(cur.Errors, "malformed_tool_call_json")
		} else {
			for _, tc := range tcalls {
				parsedTC := ParsedToolCall{Name: tc.Name, Syntactic: true, Successful: true}
				if len(tc.Args) > 0 {
					var argsMap map[string]any
					if err := json.Unmarshal(tc.Args, &argsMap); err != nil {
						parsedTC.Syntactic = false
						cur.Outcome = OutcomeSyntaxErr
						cur.Errors = append(cur.Errors, fmt.Sprintf("invalid_args_json:%s", tc.Name))
					} else {
						parsedTC.Args = argsMap
					}
				}
				cur.ToolCalls = append(cur.ToolCalls, parsedTC)
			}
		}
	}
	return cur
}

func applyToolResult(pending *LLMInteraction, step transcriptStepRaw) {
	contentLower := strings.ToLower(step.Content)
	switch {
	case strings.Contains(contentLower, "command exited with code") && !strings.Contains(contentLower, "command exited with code 0"):
		pending.Outcome = OutcomeFailure
		pending.Errors = append(pending.Errors, "nonzero_command_exit")
		if len(pending.ToolCalls) > 0 {
			pending.ToolCalls[len(pending.ToolCalls)-1].Successful = false
			pending.ToolCalls[len(pending.ToolCalls)-1].ExitCode = 1
		}
	case strings.Contains(contentLower, "error:") || strings.Contains(contentLower, "failed with error"):
		pending.Outcome = OutcomeFailure
		pending.Errors = append(pending.Errors, "execution_error")
		if len(pending.ToolCalls) > 0 {
			pending.ToolCalls[len(pending.ToolCalls)-1].Successful = false
		}
	case strings.Contains(contentLower, "command exited with code 0"):
		if len(pending.ToolCalls) > 0 {
			pending.ToolCalls[len(pending.ToolCalls)-1].Successful = true
			pending.ToolCalls[len(pending.ToolCalls)-1].ExitCode = 0
		}
	}
}

// ParseLLMTraceLog extracts interactions from Swarm / AgentX trace log files.
func (p *LLMScrutinyParser) ParseLLMTraceLog(r io.Reader, sourceName string) ([]LLMInteraction, error) {
	scanner := newLogScanner(r)
	var (
		interactions []LLMInteraction
		cur          *LLMInteraction
		sessionID    string
		currentModel = "qwen2.5-coder"
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "========== [Session: ") {
			if cur != nil {
				interactions = append(interactions, *cur)
			}
			parts := strings.Split(line, "]")
			if len(parts) >= 2 {
				sessPart := strings.TrimPrefix(parts[0], "========== [Session: ")
				sessionID = strings.TrimSpace(sessPart)
			}
			cur = &LLMInteraction{
				ID:        fmt.Sprintf("%s-%d", sessionID, len(interactions)+1),
				Timestamp: time.Now(),
				SourceLog: sourceName,
				SessionID: sessionID,
				Model:     currentModel,
				Outcome:   OutcomeSuccess,
			}
			continue
		}

		if cur == nil {
			continue
		}

		switch {
		case strings.HasPrefix(line, "channel: "):
			channel := strings.TrimPrefix(line, "channel: ")
			switch channel {
			case "native":
				cur.Outcome = OutcomeSuccess
			case "markdown-recovered":
				cur.Violations = append(cur.Violations, "markdown_recovered_tool_call")
			case "markdown-recovered-dropped":
				cur.Outcome = OutcomeFailure
				cur.Errors = append(cur.Errors, "markdown_tool_call_dropped")
			case "empty":
				cur.Outcome = OutcomeFailure
				cur.Errors = append(cur.Errors, "empty_completion")
			}
		case strings.HasPrefix(line, "server_hint: "):
			cur.Violations = append(cur.Violations, strings.TrimPrefix(line, "server_hint: "))
		case strings.HasPrefix(line, "tools: "):
			tlist := strings.Split(strings.TrimPrefix(line, "tools: "), ",")
			for _, tname := range tlist {
				tname = strings.TrimSpace(tname)
				if tname != "" {
					cur.ToolCalls = append(cur.ToolCalls, ParsedToolCall{
						Name:       tname,
						Syntactic:  true,
						Successful: true,
					})
				}
			}
		}
	}

	if cur != nil {
		interactions = append(interactions, *cur)
	}

	return interactions, scanner.Err()
}

// DiscoverLogs finds all relevant transcript, trace, and telemetry log files.
func DiscoverLogs(baseDir string, since time.Duration, includeHarness bool) ([]string, error) {
	var candidates []string
	cutoff := time.Now().Add(-since)

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if since > 0 && info.ModTime().Before(cutoff) {
			return nil
		}
		name := info.Name()
		if strings.HasPrefix(name, "transcript") && strings.HasSuffix(name, ".jsonl") {
			candidates = append(candidates, path)
		} else if strings.HasPrefix(name, "llm-trace-") && strings.HasSuffix(name, ".log") {
			candidates = append(candidates, path)
		}
		return nil
	}

	if baseDir != "" {
		if walkErr := filepath.Walk(baseDir, walkFn); walkErr != nil {
			logging.LogSwallowedError(walkErr)
		}
	}

	// Also discover standard Antigravity harness logs if requested
	if includeHarness {
		if home, homeErr := fileutil.UserHomeDir(); homeErr == nil && home != "" {
			harnessBrain := filepath.Join(home, ".gemini", "antigravity-cli", "brain")
			if _, err := fileutil.Stat(harnessBrain); err == nil {
				if walkErr := filepath.Walk(harnessBrain, walkFn); walkErr != nil {
					logging.LogSwallowedError(walkErr)
				}
			}
		}
	}

	return candidates, nil
}

// ScrutinyEvaluator aggregates interactions and calculates multi-model correctness rankings.
type ScrutinyEvaluator struct{}

// NewScrutinyEvaluator creates a new evaluator.
func NewScrutinyEvaluator() *ScrutinyEvaluator {
	return &ScrutinyEvaluator{}
}

// Evaluate processes interactions into a ranked ScrutinyReport.
func (e *ScrutinyEvaluator) Evaluate(interactions []LLMInteraction) ScrutinyReport {
	modelMap := aggregateModelMetrics(interactions)
	rankings := computeRankingsList(modelMap)

	summary := fmt.Sprintf("Scrutinized %d LLM interactions across %d distinct model(s).",
		len(interactions), len(rankings))

	return ScrutinyReport{
		GeneratedAt:       time.Now().UTC(),
		TotalInteractions: len(interactions),
		ModelsScrutinized: len(rankings),
		Rankings:          rankings,
		Summary:           summary,
	}
}

func aggregateModelMetrics(interactions []LLMInteraction) map[string]*ModelRankingMetrics {
	modelMap := make(map[string]*ModelRankingMetrics)
	for _, item := range interactions {
		m := item.Model
		if strings.TrimSpace(m) == "" {
			m = "unknown-model"
		}
		rec, exists := modelMap[m]
		if !exists {
			provider := inferProvider(m)
			rec = &ModelRankingMetrics{Model: m, Provider: provider}
			modelMap[m] = rec
		}

		rec.TotalInteractions++
		if item.Outcome == OutcomeSuccess {
			rec.SuccessfulCount++
		} else {
			rec.FailedCount++
		}
		if item.Outcome == OutcomeSyntaxErr {
			rec.SyntaxErrors++
		}
		for _, errStr := range item.Errors {
			rec.TopFailureModes = append(rec.TopFailureModes, errStr)
		}
		for _, vStr := range item.Violations {
			rec.TopFailureModes = append(rec.TopFailureModes, vStr)
		}
	}
	return modelMap
}

func inferProvider(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "claude"):
		return "anthropic"
	case strings.Contains(m, "gpt"):
		return "openai"
	case strings.Contains(m, "gemini") || strings.Contains(m, "pro") || strings.Contains(m, "flash"):
		return "google"
	case strings.Contains(m, "qwen"):
		return "alibaba"
	case strings.Contains(m, "deepseek"):
		return "deepseek"
	default:
		return "local/custom"
	}
}

func computeRankingsList(modelMap map[string]*ModelRankingMetrics) []ModelRankingMetrics {
	var rankings []ModelRankingMetrics
	for _, rec := range modelMap {
		if rec.TotalInteractions > 0 {
			rec.SuccessRate = float64(rec.SuccessfulCount) / float64(rec.TotalInteractions)
			rec.SyntacticValidRate = float64(rec.TotalInteractions-rec.SyntaxErrors) / float64(rec.TotalInteractions)
			rec.ToolExecutionRate = rec.SuccessRate
		}

		score := (rec.SuccessRate * 50.0) + (rec.SyntacticValidRate * 30.0) + (rec.ToolExecutionRate * 20.0)
		if score < 0 {
			score = 0
		} else if score > 100 {
			score = 100
		}
		rec.CompositeScore = score
		rec.TopFailureModes = topElements(rec.TopFailureModes, 3)
		rec.TuningFeedback = generateTuningFeedback(rec)
		rankings = append(rankings, *rec)
	}

	sort.Slice(rankings, func(i, j int) bool {
		if rankings[i].CompositeScore == rankings[j].CompositeScore {
			return rankings[i].TotalInteractions > rankings[j].TotalInteractions
		}
		return rankings[i].CompositeScore > rankings[j].CompositeScore
	})

	for i := range rankings {
		rankings[i].Rank = i + 1
	}
	return rankings
}

func generateTuningFeedback(rec *ModelRankingMetrics) []string {
	var feedback []string
	if rec.SyntacticValidRate < 0.90 {
		feedback = append(feedback, "Enforce strict schema validation reminder in system prompt; model produces malformed tool arguments.")
	}
	for _, mode := range rec.TopFailureModes {
		if strings.Contains(mode, "markdown") {
			feedback = append(feedback, "Model leaks Markdown JSON instead of native function calling; switch model role to 'code_draft' or inject auto-tool parser instructions.")
			break
		}
	}
	if rec.SuccessRate < 0.75 {
		feedback = append(feedback, "Execution failure rate exceeds 25%; reinforce fail-closed negative constraints and AST pre-checks.")
	}
	if len(feedback) == 0 {
		feedback = append(feedback, "Prompts well-calibrated; high syntactic and operational correctness.")
	}
	return feedback
}

func topElements(elements []string, limit int) []string {
	if len(elements) == 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, el := range elements {
		counts[el]++
	}
	type pair struct {
		name  string
		count int
	}
	var pairs []pair
	for k, v := range counts {
		pairs = append(pairs, pair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].count > pairs[j].count
	})
	var result []string
	for i := 0; i < len(pairs) && i < limit; i++ {
		result = append(result, pairs[i].name)
	}
	return result
}

// FormatTable renders the report as an aligned ASCII console table.
func FormatTable(rep ScrutinyReport) string {
	var b strings.Builder
	b.WriteString("========================================================================================\n")
	b.WriteString("                 LLM INTERACTION SCRUTINY & MODEL CORRECTNESS RANKING\n")
	b.WriteString("========================================================================================\n")
	fmt.Fprintf(&b, "Generated At       : %s\n", rep.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "Total Interactions : %d\n", rep.TotalInteractions)
	fmt.Fprintf(&b, "Models Evaluated   : %d\n\n", rep.ModelsScrutinized)

	b.WriteString(fmt.Sprintf("%-5s %-25s %-12s %-8s %-10s %-12s %-8s\n",
		"RANK", "MODEL", "PROVIDER", "TOTAL", "SUCCESS", "SYNTAX-OK", "SCORE"))
	b.WriteString(strings.Repeat("-", 88) + "\n")

	for _, r := range rep.Rankings {
		b.WriteString(fmt.Sprintf("#%-4d %-25s %-12s %-8d %-9.1f%% %-11.1f%% %-7.1f\n",
			r.Rank, truncateStr(r.Model, 24), r.Provider, r.TotalInteractions,
			r.SuccessRate*100, r.SyntacticValidRate*100, r.CompositeScore))
	}
	b.WriteString(strings.Repeat("-", 88) + "\n\n")

	b.WriteString("--- Model Prompt Honing & Tuning Recommendations ---\n")
	for _, r := range rep.Rankings {
		fmt.Fprintf(&b, "• [%s] (Rank #%d, Score: %0.1f):\n", r.Model, r.Rank, r.CompositeScore)
		for _, f := range r.TuningFeedback {
			fmt.Fprintf(&b, "    → %s\n", f)
		}
		if len(r.TopFailureModes) > 0 {
			fmt.Fprintf(&b, "    Frequent failure modes: %s\n", strings.Join(r.TopFailureModes, ", "))
		}
	}
	return b.String()
}

// FormatMarkdown renders the report as GitHub-flavored Markdown.
func FormatMarkdown(rep ScrutinyReport) string {
	var b strings.Builder
	b.WriteString("# LLM Interaction Scrutiny & Model Correctness Ranking\n\n")
	fmt.Fprintf(&b, "- **Generated At**: `%s`\n", rep.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- **Total Interactions**: `%d`\n", rep.TotalInteractions)
	fmt.Fprintf(&b, "- **Models Scrutinized**: `%d`\n\n", rep.ModelsScrutinized)

	b.WriteString("| Rank | Model | Provider | Interactions | Success Rate | Syntax Accuracy | Composite Score |\n")
	b.WriteString("|:----:|:------|:---------|:------------:|:------------:|:---------------:|:---------------:|\n")
	for _, r := range rep.Rankings {
		fmt.Fprintf(&b, "| #%d | `%s` | %s | %d | %0.1f%% | %0.1f%% | **%0.1f** |\n",
			r.Rank, r.Model, r.Provider, r.TotalInteractions,
			r.SuccessRate*100, r.SyntacticValidRate*100, r.CompositeScore)
	}

	b.WriteString("\n## Prompt Honing & Feedback Streams\n\n")
	for _, r := range rep.Rankings {
		fmt.Fprintf(&b, "### `%s` (Score: %0.1f)\n\n", r.Model, r.CompositeScore)
		for _, fb := range r.TuningFeedback {
			fmt.Fprintf(&b, "- %s\n", fb)
		}
		if len(r.TopFailureModes) > 0 {
			fmt.Fprintf(&b, "\n*Frequent failure modes: `%s`*\n", strings.Join(r.TopFailureModes, "`, `"))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// SaveMetrics writes the scrutiny report summary into .zqk/metrics/llm_scrutiny_metrics.json.
func SaveMetrics(projectRoot string, report ScrutinyReport) error {
	metricsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir)
	if err := fileutil.EnsureDir(metricsDir); err != nil {
		return err
	}
	outPath := filepath.Join(metricsDir, "llm_scrutiny_metrics.json")
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFile(outPath, data, paths.FilePerm644)
}
