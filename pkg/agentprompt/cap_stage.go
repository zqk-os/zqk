package agentprompt

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

// Cap stage prompt_template ids (kernel objects). Shared preamble is prepended
// on every lap so agents re-imprint CAP posture — TRACK: /
// REQ-CAP-STAGE-PROMPTS-001 / CRIT-CAP-STAGE-PROMPTS-001.
const (
	CapStagePreambleTemplateID        = "PROMPT-CAP-STAGE-PREAMBLE"
	CapStagePlanningTemplateID        = "PROMPT-CAP-STAGE-PLANNING"
	CapStageDesignTemplateID          = "PROMPT-CAP-STAGE-DESIGN"
	CapStageGroomingTemplateID        = "PROMPT-CAP-STAGE-GROOMING"
	CapStageOrchestratingTemplateID   = "PROMPT-CAP-STAGE-ORCHESTRATING"
	CapStageReviewTemplateID          = "PROMPT-CAP-STAGE-REVIEW"
	CapStageMetricsTemplateID         = "PROMPT-CAP-STAGE-METRICS"
	CapStageSelfImprovementTemplateID = "PROMPT-CAP-STAGE-SELF-IMPROVEMENT"
	CapStageSentinelStageTemplateID   = "PROMPT-CAP-STAGE-SENTINEL"
)

// CapStagePromptTemplateIDs maps CapStages labels → prompt_template ids.
var CapStagePromptTemplateIDs = map[string]string{
	"cap_stage_planning":         CapStagePlanningTemplateID,
	"cap_stage_design":           CapStageDesignTemplateID,
	"cap_stage_grooming":         CapStageGroomingTemplateID,
	"cap_stage_orchestrating":    CapStageOrchestratingTemplateID,
	"cap_stage_review":           CapStageReviewTemplateID,
	"cap_stage_metrics":          CapStageMetricsTemplateID,
	"cap_stage_self_improvement": CapStageSelfImprovementTemplateID,
	"cap_stage_sentinel":         CapStageSentinelStageTemplateID,
}

// CapStageWakeOptions optional substitutions for stage AGI bodies.
type CapStageWakeOptions struct {
	PlanID string
	CvsID  string
}

// BuildCapStageWakePrompt loads shared preamble + stage template bodies from the kernel.
// Falls back to a minimal inline contract if objects are missing (fail-open for CAP ticks).
func BuildCapStageWakePrompt(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, stage string, opts CapStageWakeOptions) (string, error) {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		return "", fmt.Errorf("cap stage required")
	}
	preamble := readPromptBody(ctx, sp, secCtx, CapStagePreambleTemplateID)
	if preamble == "" {
		preamble = fallbackCapPreamble()
	}
	stageID := CapStagePromptTemplateIDs[stage]
	stageBody := ""
	if stageID != "" {
		stageBody = readPromptBody(ctx, sp, secCtx, stageID)
	}
	if stageBody == "" {
		stageBody = fallbackCapStageBody(stage)
	}

	var sb strings.Builder
	sb.WriteString(preamble)
	sb.WriteString("\n\n---\n")
	sb.WriteString(stageBody)
	if opts.PlanID != "" {
		sb.WriteString("\n\nplan_id: ")
		sb.WriteString(opts.PlanID)
	}
	if opts.CvsID != "" {
		sb.WriteString("\nparent_cvs_id: ")
		sb.WriteString(opts.CvsID)
	}
	sb.WriteString("\ncap_stage: ")
	sb.WriteString(stage)
	return sb.String(), nil
}

// CapStageTemplateCoverage reports missing CapStages map entries (for tests / audit).
func CapStageTemplateCoverage() (missing []string) {
	for _, stage := range whatsnext.CapStages {
		if CapStagePromptTemplateIDs[stage] == "" {
			missing = append(missing, stage)
		}
	}
	return missing
}

func readPromptBody(ctx context.Context, sp storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, id string) string {
	if sp == nil || id == "" {
		return ""
	}
	obj, err := sp.Read(ctx, secCtx, id)
	if err != nil || obj == nil {
		return ""
	}
	body, _ := obj[objects.FieldKeyPromptBody].(string)
	return strings.TrimSpace(body)
}

func fallbackCapPreamble() string {
	return fmt.Sprintf("CAP loop posture: honor %s + CAP_LOOP_CONTRACT; no forged cap_cycle; "+
		"zero-trust completes; notify hourglass; TPM does not cut peer ATKs.", objects.JobIDCapOrchestrator)
}

func fallbackCapStageBody(stage string) string {
	return "STAGE: " + stage + "\nExecute this CAP stage per CAP_LOOP_CONTRACT delivery evidence; do not advance without artifacts."
}
