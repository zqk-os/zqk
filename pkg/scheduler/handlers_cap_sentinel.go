package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/agentprompt"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const capMorningReportFile = "cap_morning_report.json"

type capMorningReport struct {
	GeneratedAt           string                          `json:"generated_at"`
	PriorityPlan          any                             `json:"priority_plan,omitempty"`
	BacklogCountsByStatus map[string]int                  `json:"backlog_counts_by_status,omitempty"`
	PendingStage          capStagePending                 `json:"pending_stage"`
	FailureTracker        capFailureTracker               `json:"failure_tracker"`
	HealthSignals         agentprompt.KernelHealthSignals `json:"health_signals"`
	AuditReport           any                             `json:"audit_report,omitempty"`
	Directive             string                          `json:"directive,omitempty"`
}

// executeSentinelStage runs the CAP Overseer local LLM prompt logic.
func (h *CapOrchestratorHandler) executeSentinelStage(ctx context.Context, exe string) error {
	h.logger.Info("cap_stage_sentinel_started")

	// 1. Gather kernel state natively
	whatsNextRes, whatsNextErr := whatsnext.Execute(ctx, h.storage, h.projectRoot, &whatsnext.QueryRequest{
		SkipMeasure: true, // peek-only CAP context; do not re-enter measure or mutate cycle
	})
	if whatsNextErr != nil {
		h.logger.Error("cap_sentinel_whats_next_failed", whatsNextErr)
		return fmt.Errorf("gather whats-next: %w", whatsNextErr)
	}

	auditCmd := execwrap.CommandContext(ctx, exe, "system", "audit-report", "--format", "json")
	auditCmd = h.prepareCmd(auditCmd)
	auditOut, auditErr := auditCmd.CombinedOutput()

	var state map[string]any

	// Convert structs to map[string]any for json injection
	b, err := json.Marshal(whatsNextRes)
	if err == nil {
		_ = json.Unmarshal(b, &state)
	} else {
		return fmt.Errorf("marshal whats-next struct: %w", err)
	}
	if auditErr == nil {
		var audit map[string]any
		if err := json.Unmarshal(auditOut, &audit); err == nil {
			state["audit_report"] = audit
		}
	}

	stateJSON, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	// 2. Build the LLM prompt using the builder
	secCtx := pkgctx.NewSystemSecurityContext()
	healthSignals := gatherHealthSignals(h.projectRoot)
	pending, _ := h.readPendingStage()
	report := capMorningReport{
		GeneratedAt:           zqktime.NowRFC3339UTC(),
		PriorityPlan:          whatsNextRes.PriorityPlan,
		BacklogCountsByStatus: whatsNextRes.BacklogCountsByStatus,
		PendingStage:          pending,
		FailureTracker:        h.readFailureTracker(),
		HealthSignals:         healthSignals,
		AuditReport:           state["audit_report"],
	}
	h.writeStateFile(capMorningReportFile, report)
	prompt, err := agentprompt.BuildSentinelPrompt(ctx, h.storage, secCtx, h.projectRoot, agentprompt.SentinelPromptOptions{
		StateJSON:     string(stateJSON),
		HealthSignals: healthSignals,
	})
	if err != nil {
		h.logger.Error("cap_sentinel_prompt_build_failed", err)
		return fmt.Errorf("build sentinel prompt: %w", err)
	}

	// 3. Prompt the LLM
	client := llm.NewClient(ctx, nil) // Uses environment-configured LLM provider (Ollama, Gemini, Qwen, etc.)

	// Create context with timeout for LLM
	llmCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	response, err := client.GenerateCompletion(llmCtx, prompt, "You are a pragmatic, decisive engineering sentinel.")
	if err != nil {
		h.logger.Error("cap_sentinel_llm_failed", err)
		return fmt.Errorf("llm generation: %w", err)
	}

	h.logger.Info("cap_sentinel_directive_generated", logging.String("directive", response))

	// Write the directive to the state for downstream processing
	h.writeStateFile("cap_sentinel_directive.txt", response)
	report.Directive = response
	h.writeStateFile(capMorningReportFile, report)

	return nil
}
