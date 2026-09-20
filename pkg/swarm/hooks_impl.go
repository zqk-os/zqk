package swarm

import (
	"context"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/llm"
)

type ToolDenialGuard struct {
	consecutiveDenials int
}

// NewToolDenialGuard creates a new ToolDenialGuard.
func NewToolDenialGuard() *ToolDenialGuard {
	return &ToolDenialGuard{}
}

func (g *ToolDenialGuard) PreTool(ctx context.Context, call llm.ToolCall) error {
	return nil // Evaluated post-execution in reality for soft-blocks, or pre-execution if purely string matched.
	// We'll actually do the block checking in PostTool for "ALLOWLIST DENY" or "Soft-blocked".
}

func (g *ToolDenialGuard) PostTool(ctx context.Context, call llm.ToolCall, result string, err error) (string, error) {
	combined := result
	if err != nil {
		combined += " " + err.Error()
	}
	if strings.Contains(combined, "ALLOWLIST DENY") || strings.Contains(combined, "Soft-blocked") || strings.Contains(combined, "Soft-block") || strings.Contains(combined, "GUIDANCE: Do not use") {
		g.consecutiveDenials++
		if g.consecutiveDenials >= 3 {
			return "", fmt.Errorf("tool_denial_guard: 3 consecutive blocked tool calls. Aborting swarm to prevent hallucination loop")
		}
	} else {
		g.consecutiveDenials = 0
	}
	return "", nil
}

func (g *ToolDenialGuard) OnSuccess(ctx context.Context, call llm.ToolCall, result string) {}

type HallucinationCircuitBreaker struct {
	consecutiveFileNotFound int
}

// NewHallucinationCircuitBreaker creates a new HallucinationCircuitBreaker.
func NewHallucinationCircuitBreaker() *HallucinationCircuitBreaker {
	return &HallucinationCircuitBreaker{}
}

func (h *HallucinationCircuitBreaker) PreTool(ctx context.Context, call llm.ToolCall) error {
	return nil
}

func (h *HallucinationCircuitBreaker) PostTool(ctx context.Context, call llm.ToolCall, result string, err error) (string, error) {
	combined := result
	if err != nil {
		combined += " " + err.Error()
	}
	if strings.Contains(strings.ToLower(combined), "no such file or directory") {
		h.consecutiveFileNotFound++
		if h.consecutiveFileNotFound == 3 {
			return "Guidance: Stop guessing file paths. Use zqk_observer_search first.", nil
		} else if h.consecutiveFileNotFound >= 4 {
			return "", fmt.Errorf("hallucination_circuit_breaker: 4 consecutive file-not-found errors. Aborting swarm")
		}
	} else if err == nil && !strings.Contains(strings.ToLower(result), "error") {
		h.consecutiveFileNotFound = 0
	}
	return "", nil
}

func (h *HallucinationCircuitBreaker) OnSuccess(ctx context.Context, call llm.ToolCall, result string) {
}

type ProactiveWorkspaceSeeder struct{}

func (p *ProactiveWorkspaceSeeder) PreTool(ctx context.Context, call llm.ToolCall) error { return nil }
func (p *ProactiveWorkspaceSeeder) PostTool(ctx context.Context, call llm.ToolCall, result string, err error) (string, error) {
	return "", nil
}
func (p *ProactiveWorkspaceSeeder) OnSuccess(ctx context.Context, call llm.ToolCall, result string) {
	// Ambient trigger: kick off lints or builds in background to warm cache.
	if call.Name == "zqk_write_code" || call.Name == "zqk_execute_bash" {
		goroutinelabels.NewGoroutine("ambient_success_trigger", "ambient workspace check").StartSimple(func() {
			// Minimal placeholder logic for ambient warming
		})
	}
}
