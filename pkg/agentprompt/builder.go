package agentprompt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/paths"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/storage"
)

// OnboardingPromptTemplateID is the canonical ID for the onboarding prompt
const OnboardingPromptTemplateID = "PROMPT-1775443238169278000-3db7d1ea"

// BuildOnboardingPrompt builds a vectorized, topologically sorted onboarding prompt.
func BuildOnboardingPrompt(ctx context.Context, sp storage.ObjectStorageProvider, budget int) (string, error) {
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Fetch the Prompt Template
	templateObj, err := sp.Read(ctx, secCtx, OnboardingPromptTemplateID)
	if err != nil {
		return "", fmt.Errorf("failed to read onboarding prompt template: %w", err)
	}

	promptBody, _ := templateObj[objects.FieldKeyPromptBody].(string)
	if promptBody == "" {
		promptBody = "Initialize agent session."
	}

	// 2. Fetch Active Policies
	policiesEnv, err := LoadActivePolicies(ctx, sp, secCtx)
	if err != nil {
		return "", fmt.Errorf("failed to load policies: %w", err)
	}

	// 3. Assemble candidates for the pipeline allocator
	var candidates []pipeline.ContextNode

	// The prompt template is CORE and must be included.
	candidates = append(candidates, pipeline.ContextNode{
		ID:      OnboardingPromptTemplateID,
		Tokens:  len(promptBody) / 4,
		Weight:  1.0,
		IsCore:  true,
		Payload: promptBody,
	})

	// Add policies as candidates
	for _, policy := range policiesEnv.ActivePolicies {
		id, _ := policy[objects.FieldKeyID].(string)
		title, _ := policy[objects.FieldKeyTitle].(string)
		desc, _ := policy[objects.FieldKeyDescription].(string)

		content := fmt.Sprintf("### %s (%s)\n%s\n", title, id, desc)
		tokens := len(content) / 4
		if tokens == 0 {
			tokens = 1
		}

		candidates = append(candidates, pipeline.ContextNode{
			ID:      id,
			Tokens:  tokens,
			Weight:  0.8, // Default weight for policies
			IsCore:  false,
			Payload: content,
			PartOf:  []string{OnboardingPromptTemplateID}, // Policies attach to the main prompt
		})
	}

	// 4. Allocate budget
	allocator := pipeline.NewAllocator(budget)
	selected := allocator.Allocate(candidates)

	// 5. Construct the final markdown string
	var sb strings.Builder
	var policiesSb strings.Builder

	for _, node := range selected {
		content, _ := node.Payload.(string)
		if node.ID == OnboardingPromptTemplateID {
			sb.WriteString(content)
			sb.WriteString("\n\n")
		} else {
			policiesSb.WriteString(content)
			policiesSb.WriteString("\n")
		}
	}

	if policiesSb.Len() > 0 {
		sb.WriteString("## Active Policies\n")
		sb.WriteString("You are bound by the following active project policies:\n\n")
		sb.WriteString(policiesSb.String())
	}

	return sb.String(), nil
}

// TaskPromptOptions encapsulates all context needed to build a unified agent prompt.
type TaskPromptOptions struct {
	PlanTitle     string
	PlanID        string
	SessionID     string
	TargetAgent   string
	PersonaID     string // PER-* when known; used for bound policy/skill refs
	Capability    string
	TaskTitle     string
	TaskContext   string // Additional Context (like Subgraph JSON)
	ValidationDSL string // The user wanted this loaded in!
	IncludeTDD    bool   // Whether to include the Mandatory TDD Paradigm
	TokenBudget   int    // Maximum tokens for the prompt, defaults to 32768
	// Layer Persist writes the compact envelope only. Execute (default) still uses refs, not bodies.
	Layer PromptLayer
	// IncludeObserver adds a live, task-scoped AST hint. Default is the tool-access
	// pointer only. Never persist the hint on agent_task.
	IncludeObserver bool
	TaskSteps       string // Pre-formatted task steps section (or synthesized mutation steps)
}

// BuildTaskPrompt creates a unified markdown prompt for an agent task, incorporating
// all context layers: policies, feedback, skills, structural AST graph, and task specifics.
func BuildTaskPrompt(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, projectRoot string, opts TaskPromptOptions) (string, error) {
	if opts.Layer == PromptLayerPersist {
		env, err := BuildTaskEnvelope(ctx, sp, secCtx, projectRoot, opts)
		if err != nil {
			return "", err
		}
		return env.Description, nil
	}

	personaID := resolvePersonaID(opts)

	// 1. Bound policies (standing + persona) — refs, not the full catalog bodies.
	policiesEnv, err := LoadBoundPolicies(ctx, sp, secCtx, personaID)
	var policySection string
	if err == nil && policiesEnv != nil {
		policySection = policiesEnv.GeneratePromptSection()
	}

	// 2. Fetch Autonomous Feedback
	feedbackEnv, err := LoadAutonomousFeedback(ctx, sp, secCtx)
	var feedbackSection string
	if err == nil && feedbackEnv != nil {
		feedbackSection = feedbackEnv.GeneratePromptSection()
	}

	// 3. Fetch Relevant Skills (persona-linked ASK-* + orchestration-boot + keyword match)
	skillEnforcement, err := LoadRelevantSkillsOpts(ctx, sp, secCtx, skillLoadOpts(opts, personaID, projectRoot))
	if err != nil {
		return "", fmt.Errorf("skill seal verification failed (fail-closed): %w", err)
	}
	var skillSection string
	if skillEnforcement != nil {
		skillSection = skillEnforcement.GeneratePromptSection()
	}

	// 4. Observer: tool access always. Task-scoped hits are opt-in and never persisted.
	var ambientSection string
	if opts.IncludeObserver && projectRoot != "" {
		ambientSection = RelevantObserverSection(ctx, projectRoot, opts.TaskTitle, opts.PlanTitle)
	} else {
		ambientSection = ObserverToolAccessSection()
	}

	budget := opts.TokenBudget
	if budget <= 0 {
		if envVal := config.LLMContextWindowSize().OrDefault(0); envVal > 0 {
			budget = envVal
		} else if envVal <= 0 {
			budget = 32768
		} else {
			budget = 32768
		}
	}

	var candidates []pipeline.ContextNode

	// Core task specifics are always included
	var coreSb strings.Builder
	coreSb.WriteString("# Orchestration Context\n")
	if opts.PlanTitle != "" {
		coreSb.WriteString(fmt.Sprintf("Plan: %s (%s)\n", opts.PlanTitle, opts.PlanID))
	}
	if opts.SessionID != "" {
		coreSb.WriteString(fmt.Sprintf("Session: %s\n", opts.SessionID))
	}
	if opts.TargetAgent != "" {
		coreSb.WriteString(fmt.Sprintf("Target Agent/Pod: %s (Capability: %s)\n", opts.TargetAgent, opts.Capability))
	}
	coreSb.WriteString("\n")

	candidates = append(candidates, pipeline.ContextNode{
		ID:      "core_header",
		Tokens:  len(coreSb.String()) / 4,
		Weight:  1.0,
		IsCore:  true,
		Payload: coreSb.String(),
	})

	if policySection != "" {
		candidates = append(candidates, pipeline.ContextNode{
			ID:      "policies",
			Tokens:  len(policySection) / 4,
			Weight:  0.8,
			Payload: policySection + "\n",
		})
	}
	if feedbackSection != "" {
		candidates = append(candidates, pipeline.ContextNode{
			ID:      "feedback",
			Tokens:  len(feedbackSection) / 4,
			Weight:  0.9,
			Payload: feedbackSection + "\n",
		})
	}
	if skillSection != "" {
		candidates = append(candidates, pipeline.ContextNode{
			ID:      "skills",
			Tokens:  len(skillSection) / 4,
			Weight:  0.85,
			Payload: skillSection + "\n",
		})
	}
	if ambientSection != "" {
		candidates = append(candidates, pipeline.ContextNode{
			ID:      "ambient",
			Tokens:  len(ambientSection) / 4,
			Weight:  0.7,
			Payload: ambientSection + "\n",
		})
	}

	var footerSb strings.Builder
	footerSb.WriteString("## Standing mandates\n")
	footerSb.WriteString(paths.RewriteCanonicalCLIInvocations("Resolve bodies with `zqk object get`. Do not copy them into the task object.\n"))
	if opts.IncludeTDD {
		footerSb.WriteString(fmt.Sprintf("- TDD: `%s`\n", StandingPolicyTDD))
	}
	footerSb.WriteString(fmt.Sprintf("- Flywheel (Anti-Idleness Protocol): `%s` — summary/merge is a milestone transition, never a stopping condition; advance autonomously without yielding to idle.\n\n", StandingPolicyFlywheel))
	if opts.ValidationDSL != "" {
		footerSb.WriteString("## Validation DSL (Verification Steps)\n")
		footerSb.WriteString(opts.ValidationDSL)
		footerSb.WriteString("\n\n")
	}

	footerSb.WriteString("## Delegated Task\n")
	footerSb.WriteString(fmt.Sprintf("- Process task: %s\n", opts.TaskTitle))
	if opts.TaskContext != "" {
		footerSb.WriteString("\n### Context\n")
		footerSb.WriteString(opts.TaskContext)
		footerSb.WriteString("\n")
	}
	if opts.TaskSteps != "" {
		footerSb.WriteString("\n")
		footerSb.WriteString(opts.TaskSteps)
		footerSb.WriteString("\n")
	}

	candidates = append(candidates, pipeline.ContextNode{
		ID:      "core_footer",
		Tokens:  len(footerSb.String()) / 4,
		Weight:  1.0,
		IsCore:  true,
		Payload: footerSb.String(),
	})

	allocator := pipeline.NewAllocator(budget)
	selected := allocator.Allocate(candidates)

	var sb strings.Builder
	for _, node := range selected {
		content, _ := node.Payload.(string)
		sb.WriteString(content)
	}

	return sb.String(), nil
}

// SentinelPromptTemplateID is the canonical ID for the CAP Sentinel prompt template.
const SentinelPromptTemplateID = "PROMPT-1783091834904015000-066e0f7d"

// defaultSentinelTokenBudget is a conservative budget for small local LLMs (Qwen, Ollama 7B, etc.).
// Larger models (Llama 70B, hosted APIs) should pass a higher TokenBudget via SentinelPromptOptions.
const defaultSentinelTokenBudget = 2048

// KernelHealthSignals captures observable health metrics from the ZQK kernel.
// These are injected into the sentinel prompt so the local LLM can reason about drift.
type KernelHealthSignals struct {
	FailingTestCount   int
	PolicyViolations   int
	SchedulerErrorRate float64
	DriftIndicators    []string
}

// SentinelPromptOptions encapsulates all context needed to build the sentinel prompt.
type SentinelPromptOptions struct {
	StateJSON     string // JSON output of zqk workflow whats-next
	HealthSignals KernelHealthSignals
	// TokenBudget caps total prompt tokens. 0 uses defaultSentinelTokenBudget (2048).
	// Set higher (e.g. 8192) for larger local models or hosted APIs.
	TokenBudget int
}

// BuildSentinelPrompt builds a context-window-aware prompt for the CAP Kernel Steward
// using pipeline.Allocator — the same mechanism as BuildOnboardingPrompt. Kernel objects
// (mission, milestones, goals, health signals, policies) are ranked by weight so that
// small local LLMs (Qwen 7B, Ollama) always get the most actionable context first, and
// verbose lower-priority sections are dropped when budget is tight.
func BuildSentinelPrompt(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, _ string, opts SentinelPromptOptions) (string, error) {
	budget := opts.TokenBudget
	if budget <= 0 {
		budget = defaultSentinelTokenBudget
	}

	// 1. Fetch and render the prompt template (CORE — always included).
	var promptBody string
	templateObj, err := sp.Read(ctx, secCtx, SentinelPromptTemplateID)
	if err != nil {
		// Fallback to default sentinel prompt body if template object is not found
		promptBody = "You are the CAP Kernel Steward. State: {{.StateJSON}}"
	} else {
		promptBody, _ = templateObj[objects.FieldKeyPromptBody].(string)
		if promptBody == "" {
			promptBody = "You are the CAP Kernel Steward. State: {{.StateJSON}}"
		}
	}
	// Strip goal/milestone details from StateJSON to avoid duplication with kernel sections below.
	// Keep only the high-signal fields: priority_plan, backlog_counts, convergence_sessions, agent_instruction.
	compactState := compactStateJSON(opts.StateJSON)
	promptBody = strings.ReplaceAll(promptBody, "{{.StateJSON}}", compactState)

	directiveFooter := "\n---\n## DIRECTIVE REQUIRED\n" +
		"Output EXACTLY this format (2 sentences max, no markdown headers, no lists, no planning essay):\n" +
		"AGENT DIRECTIVE: <one sentence: the single highest-priority action right now>. " +
		"REASON: <one sentence: which kernel signal or drift indicator drove this>.\n"
	coreContent := promptBody + "\n\n" + directiveFooter

	// 2. Assemble candidates for the allocator.
	// Ranking: core > health signals > mission > milestones > goals > policies.
	// This ensures small-context models still get the most actionable data.
	var candidates []pipeline.ContextNode

	candidates = append(candidates, pipeline.ContextNode{
		ID:      SentinelPromptTemplateID,
		Tokens:  len(coreContent) / 4,
		Weight:  1.0,
		IsCore:  true,
		Payload: coreContent,
	})

	if health := buildHealthSection(opts.HealthSignals); health != "" {
		candidates = append(candidates, pipeline.ContextNode{
			ID:      "sentinel:health",
			Tokens:  len(health) / 4,
			Weight:  0.95,
			Payload: health,
			PartOf:  []string{SentinelPromptTemplateID},
		})
	}

	if mission := loadKernelSection(ctx, sp, secCtx, objects.KindMission, "active", "## Mission\n", formatMissionObj); mission != "" {
		candidates = append(candidates, pipeline.ContextNode{
			ID:      "sentinel:mission",
			Tokens:  len(mission) / 4,
			Weight:  0.9,
			Payload: mission,
			PartOf:  []string{SentinelPromptTemplateID},
		})
	}

	if milestones := loadKernelSection(ctx, sp, secCtx, objects.KindMilestone, "in_progress", "## In-Progress Milestones\n", formatListObj); milestones != "" {
		candidates = append(candidates, pipeline.ContextNode{
			ID:      "sentinel:milestones",
			Tokens:  len(milestones) / 4,
			Weight:  0.85,
			Payload: milestones,
			PartOf:  []string{SentinelPromptTemplateID},
		})
	}

	// Goals are intentionally excluded: goal IDs/titles are already implicit in the
	// priority plan + backlog summary injected via StateJSON. Adding them again bloats
	// small-context models and causes repetition in the directive output.

	if policiesEnv, _ := LoadActivePolicies(ctx, sp, secCtx); policiesEnv != nil {
		if policyContent := policiesEnv.GeneratePromptSection(); policyContent != "" {
			candidates = append(candidates, pipeline.ContextNode{
				ID:      "sentinel:policies",
				Tokens:  len(policyContent) / 4,
				Weight:  0.5,
				Payload: policyContent,
				PartOf:  []string{SentinelPromptTemplateID},
			})
		}
	}

	// 3. Allocate within budget and assemble in topological order.
	selected := pipeline.NewAllocator(budget).Allocate(candidates)

	var sb strings.Builder
	for _, node := range selected {
		content, _ := node.Payload.(string)
		sb.WriteString(content)
	}
	return sb.String(), nil
}

// compactStateJSON strips verbose, low-signal fields from the whats-next JSON before
// injecting into the sentinel prompt. Goal IDs/titles are already provided by the
// kernel sections (mission, milestones) so keeping them in the state blob wastes tokens
// on small-context local models. Keeps: priority_plan, backlog_counts_by_status,
// agent_instruction, convergence_sessions_active (count only), measure_skip_reason.
func compactStateJSON(raw string) string {
	if raw == "" {
		return "{}"
	}
	// Best-effort JSON parse; fall back to raw if it fails.
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw
	}
	keep := map[string]any{}
	for _, k := range []string{
		"agent_instruction",
		"priority_plan",
		"backlog_counts_by_status",
		"measure_skip_reason",
	} {
		if v, ok := m[k]; ok {
			keep[k] = v
		}
	}
	// Include convergence session count but not full objects (too verbose).
	if sessions, ok := m["convergence_sessions_active"].([]any); ok {
		keep["convergence_sessions_active_count"] = len(sessions)
	}
	b, err := json.Marshal(keep)
	if err != nil {
		return raw
	}
	return string(b)
}

// loadKernelSection lists kernel objects of kind/status and formats them using formatter.
// Returns "" on error so the caller degrades gracefully (budget allocator skips empty nodes).
func loadKernelSection(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext,
	kind, status, header string, formatter func(map[string]any) string) string {

	result, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), storage.ListFilter{
		Kind:    kind,
		Filters: map[string]any{objects.FieldKeyStatus: status},
		Limit:   10,
	})
	if err != nil || result == nil || len(result.Objects) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(header)
	for _, obj := range result.Objects {
		if line := formatter(obj); line != "" {
			sb.WriteString(line)
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// formatMissionObj formats a mission object for the prompt.
func formatMissionObj(obj map[string]any) string {
	id, _ := obj[objects.FieldKeyID].(string)
	stmt, _ := obj[objects.FieldKeyMissionStatement].(string)
	if stmt == "" {
		stmt, _ = obj[objects.FieldKeyTitle].(string)
	}
	if id == "" && stmt == "" {
		return ""
	}
	return fmt.Sprintf("**%s**: %s\n", id, stmt)
}

// formatListObj formats a generic kernel object as a bullet list item.
func formatListObj(obj map[string]any) string {
	id, _ := obj[objects.FieldKeyID].(string)
	title, _ := obj[objects.FieldKeyTitle].(string)
	if id == "" && title == "" {
		return ""
	}
	return fmt.Sprintf("- **%s**: %s\n", id, title)
}

// buildHealthSection formats kernel health signals for the prompt.
func buildHealthSection(h KernelHealthSignals) string {
	if h.FailingTestCount == 0 && h.PolicyViolations == 0 && len(h.DriftIndicators) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Kernel Health Signals\n")
	if h.FailingTestCount > 0 {
		sb.WriteString(fmt.Sprintf("- Failing tests: %d\n", h.FailingTestCount))
	}
	if h.PolicyViolations > 0 {
		sb.WriteString(fmt.Sprintf("- Policy violations (POL-CODE-007): %d\n", h.PolicyViolations))
	}
	if h.SchedulerErrorRate > 0 {
		sb.WriteString(fmt.Sprintf("- Scheduler error rate: %.1f%%\n", h.SchedulerErrorRate*100))
	}
	for _, d := range h.DriftIndicators {
		sb.WriteString(fmt.Sprintf("- Drift indicator: %s\n", d))
	}
	sb.WriteString("\n")
	return sb.String()
}
