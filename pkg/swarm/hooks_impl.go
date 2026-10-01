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

	lower := strings.ToLower(combined)
	if strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "system_managed_field") ||
		strings.Contains(lower, "read-only") ||
		strings.Contains(lower, "unauthorized") {
		// Provide ambient coaching prompt instead of tripping consecutive denial kill
		g.consecutiveDenials = 0
		return PermissionDenialGuidance(combined), nil
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
	if toolSuffixIs(call.Name, "write_code") || toolSuffixIs(call.Name, "execute_bash") {
		goroutinelabels.NewGoroutine("ambient_success_trigger", "ambient workspace check").StartSimple(func() {
			// Minimal placeholder logic for ambient warming
		})
	}
}

// CatastrophicErrorGuard aborts execution immediately when unrecoverable or catastrophic
// failures occur (e.g. fatal authentication/authorization denials, missing accounts, or
// persistent fatal infrastructure errors), preventing wasteful CPU and token burn.
type CatastrophicErrorGuard struct {
	consecutiveFailures     int
	lastErrorSummary        string
	maxConsecutiveIdentical int
}

// NewCatastrophicErrorGuard creates a new CatastrophicErrorGuard.
func NewCatastrophicErrorGuard() *CatastrophicErrorGuard {
	return &CatastrophicErrorGuard{
		maxConsecutiveIdentical: 3,
	}
}

func (g *CatastrophicErrorGuard) PreTool(ctx context.Context, call llm.ToolCall) error {
	return nil
}

func (g *CatastrophicErrorGuard) PostTool(ctx context.Context, call llm.ToolCall, result string, err error) (string, error) {
	combined := result
	if err != nil {
		if combined != "" {
			combined += ": " + err.Error()
		} else {
			combined = err.Error()
		}
	}

	lower := strings.ToLower(combined)

	// Immediate fatal catastrophic errors: Authentication / Authorization policy rejection
	// Once an account index or auth policy fails (e.g. POL-AGENT-ACCOUNT-LOGIN-001 or missing worker account),
	// further execution turns will NEVER succeed.
	if strings.Contains(combined, "POL-AGENT-ACCOUNT-LOGIN") ||
		strings.Contains(lower, "not found in account index") ||
		strings.Contains(lower, "unauthorized: account") ||
		strings.Contains(lower, "unauthorized: persona") ||
		strings.Contains(lower, "unrecoverable security failure") {
		return "", fmt.Errorf("catastrophic_error_guard: unrecoverable auth/identity failure (%s). Aborting swarm to prevent thrashing", strings.TrimSpace(combined))
	}

	// Repeated identical execution failures:
	if err != nil || strings.Contains(lower, "error:") || strings.Contains(lower, "tool execution failed") {
		currErr := strings.TrimSpace(combined)
		if len(currErr) > 200 {
			currErr = currErr[:200]
		}
		if currErr != "" && currErr == g.lastErrorSummary {
			g.consecutiveFailures++
			if g.consecutiveFailures >= g.maxConsecutiveIdentical {
				return "", fmt.Errorf("catastrophic_error_guard: %d consecutive identical execution failures (%s). Aborting swarm", g.consecutiveFailures, g.lastErrorSummary)
			}
		} else {
			g.lastErrorSummary = currErr
			g.consecutiveFailures = 1
		}
	} else {
		// Clean success resets consecutive failure counter
		g.consecutiveFailures = 0
		g.lastErrorSummary = ""
	}

	return "", nil
}

func (g *CatastrophicErrorGuard) OnSuccess(ctx context.Context, call llm.ToolCall, result string) {
	g.consecutiveFailures = 0
	g.lastErrorSummary = ""
}
