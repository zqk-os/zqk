package handslapper

import (
	"context"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/logging"
)

// DriftType defines the classification of agent drift
type DriftType string

const (
	DriftTypeToolBypass         DriftType = "TOOL_BYPASS"
	DriftTypeNamespaceViolation DriftType = "NAMESPACE_VIOLATION"
	DriftTypeHallucination      DriftType = "HALLUCINATION"
)

// DriftEvent represents an instance where an agent violated kernel constraints
type DriftEvent struct {
	AgentID  string
	Type     DriftType
	Command  string
	Severity int
}

// Service is the HandSlapper drift monitoring service.
type Service struct {
	// strictMode enforces instant termination of offending agents
	strictMode bool
}

// NewService creates a new HandSlapper service
func NewService(strictMode bool) *Service {
	return &Service{
		strictMode: strictMode,
	}
}

// MonitorCommand inspects a command requested by an agent before it hits the OS.
// If the command violates ZQK constraints (e.g., bypassing the kernel with raw unix tools),
// the HandSlapper rejects the command.
func (s *Service) MonitorCommand(ctx context.Context, agentID string, cmd string) error {
	// 1. Check for Legitimate Entitlement
	// If the workflow or engineer explicitly authorizes primitive usage for a specific script
	if strings.Contains(cmd, "ZQK_BYPASS_HANDSLAPPER=1") {
		return nil
	}

	forbiddenTools := []string{"grep", "awk", "sed", "cat", "ls", "find"}

	// 2. Tokenize command to catch wrapped execution (e.g. bash -c 'grep foo')
	cmdTokens := strings.Fields(cmd)
	if len(cmdTokens) == 0 {
		return nil
	}

	// 3. Scan all tokens. If any token is exactly the forbidden tool, reject.
	for _, token := range cmdTokens {
		// Clean the token of quotes for evaluation
		cleanToken := strings.Trim(token, `"'`)

		// Check for Namespace Violations (e.g., .zqk folder)
		if strings.Contains(cleanToken, paths.ProjectDataDir) {
			event := DriftEvent{
				AgentID:  agentID,
				Type:     DriftTypeNamespaceViolation,
				Command:  cmd,
				Severity: 10,
			}
			s.LogDrift(event)
			if s.strictMode {
				return Electrocute(ctx, "NAMESPACE_VIOLATION", fmt.Sprintf("Agent %s attempted to violate namespace by interacting with ephemeral '.zqk' directory. Command rejected", agentID))
			}
		}

		// Check for Tool Bypass
		for _, tool := range forbiddenTools {
			if cleanToken == tool {
				event := DriftEvent{
					AgentID:  agentID,
					Type:     DriftTypeToolBypass,
					Command:  cmd,
					Severity: 10,
				}
				s.LogDrift(event)
				if s.strictMode {
					return Electrocute(ctx, "TOOL_BYPASS", fmt.Sprintf("Agent %s attempted to bypass the Knowledge Kernel using forbidden primitive '%s'. Command rejected. Use the ZQK CLI", agentID, tool))
				}
			}
		}
	}

	return nil
}

// LogDrift records the violation into the telemetry/logging system.
func (s *Service) LogDrift(event DriftEvent) {
	logger := logging.GetLogger()
	logging.FluentEvent(logger).
		Error("agent_drift_detected", fmt.Errorf("%s", string(event.Type))).
		Command(event.Command).
		Log()
}
