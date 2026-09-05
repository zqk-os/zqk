package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// EscalationSeverity indicates the urgency of an escalation.
type EscalationSeverity string

const (
	EscalationSeverityWarning  EscalationSeverity = "warning"
	EscalationSeverityCritical EscalationSeverity = "critical"
)

// EscalationNotice is the vendor-neutral data structure passed to any escalation provider.
// It contains all context needed for diagnosis without coupling to any specific
// agent, model provider, or delivery mechanism.
type EscalationNotice struct {
	Title            string             `json:"title"`
	Severity         EscalationSeverity `json:"severity"`
	Timestamp        string             `json:"timestamp,omitempty"`
	Issues           []string           `json:"issues"`
	Context          map[string]any     `json:"context,omitempty"`
	SuggestedActions []string           `json:"suggested_actions,omitempty"`
}

// EscalationProvider is the interface for delivering escalation notices.
// Implementations handle the specifics of how to reach the target:
//   - InboxEscalationProvider: writes JSON to a filesystem inbox directory
//   - CommandEscalationProvider: runs an arbitrary command (osascript, curl, custom CLI)
//
// Future implementations could include:
//   - HTTPEscalationProvider: POST to a model API endpoint
//   - MCPEscalationProvider: send via MCP protocol
//   - WebhookEscalationProvider: POST to Slack, Discord, etc.
type EscalationProvider interface {
	Escalate(ctx context.Context, notice EscalationNotice) error
}

// EscalationTier pairs a failure threshold with a provider.
type EscalationTier struct {
	Threshold int
	Provider  EscalationProvider
	Name      string // For logging: "agent", "human", etc.
}

// EscalationChain evaluates failure counts against an ordered list of tiers
// and routes to the highest matching provider. Tiers must be ordered by
// threshold ascending.
type EscalationChain struct {
	Tiers []EscalationTier
}

// Evaluate checks the failure count against tiers and calls the highest
// matching provider. Only one provider is called per evaluation.
func (c *EscalationChain) Evaluate(ctx context.Context, failures int, notice EscalationNotice) error {
	var matched *EscalationTier
	for i := range c.Tiers {
		if failures >= c.Tiers[i].Threshold {
			matched = &c.Tiers[i]
		}
	}
	if matched == nil {
		return nil
	}
	return matched.Provider.Escalate(ctx, notice)
}

// --- Concrete Providers ---

// InboxEscalationProvider writes escalation notices as JSON files to a directory.
// Works for both human and agent inboxes. Vendor-neutral: any consumer can
// read JSON from a directory.
type InboxEscalationProvider struct {
	InboxDir string
}

func (p *InboxEscalationProvider) Escalate(_ context.Context, notice EscalationNotice) error {
	if notice.Timestamp == "" {
		notice.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	fileutil.EnsureDir(p.InboxDir) //nolint:errcheck

	data, err := json.MarshalIndent(notice, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal escalation notice: %w", err)
	}

	filename := fmt.Sprintf("CAP-ESCALATION-%s.json", time.Now().UTC().Format("20060102-150405"))
	return fileutil.WriteSecureFile(filepath.Join(p.InboxDir, filename), data)
}

// CommandEscalationProvider runs an arbitrary command to deliver the escalation.
// The prompt is written to a temp file and the special token {prompt_file} in
// Args is replaced with its path. This supports:
//   - osascript (AppleScript for macOS chat injection)
//   - curl (HTTP POST to any API)
//   - custom CLIs (zqk agent prompt, model-specific CLIs)
//   - any future delivery mechanism
type CommandEscalationProvider struct {
	Command     string   // The command to run (e.g., "osascript", "curl", "zqk")
	Args        []string // Arguments, may contain {prompt_file} for substitution
	ProjectRoot string   // Used for temp file location
}

func (p *CommandEscalationProvider) Escalate(ctx context.Context, notice EscalationNotice) error {
	// Build the prompt text from the notice
	prompt := notice.buildPrompt()

	// Write prompt to temp file for command to read
	promptDir := p.ProjectRoot
	if promptDir == "" {
		promptDir = fileutil.TempDir()
	}
	stateDir := filepath.Join(promptDir, paths.ProjectDataDir, "state")
	fileutil.EnsureDir(stateDir) //nolint:errcheck

	promptFile := filepath.Join(stateDir, "cap_escalation_prompt.txt")
	if err := fileutil.WriteSecureFile(promptFile, []byte(prompt)); err != nil {
		return fmt.Errorf("failed to write prompt file: %w", err)
	}

	// Substitute {prompt_file} in args
	args := make([]string, len(p.Args))
	for i, arg := range p.Args {
		args[i] = strings.ReplaceAll(arg, "{prompt_file}", promptFile)
	}

	cmd := execwrap.CommandContext(ctx, p.Command, args...)
	cmd.Dir = p.ProjectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("escalation command failed: %s: %w", string(out), err)
	}

	return nil
}

// buildPrompt creates a human/agent-readable prompt from the escalation notice.
func (n *EscalationNotice) buildPrompt() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s\n\n", n.Title))

	if len(n.Issues) > 0 {
		sb.WriteString("Issues:\n")
		for _, issue := range n.Issues {
			sb.WriteString(fmt.Sprintf("- %s\n", issue))
		}
		sb.WriteString("\n")
	}

	if len(n.Context) > 0 {
		sb.WriteString("Context:\n")
		for k, v := range n.Context {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
		sb.WriteString("\n")
	}

	if len(n.SuggestedActions) > 0 {
		sb.WriteString("Suggested actions:\n")
		for i, action := range n.SuggestedActions {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, action))
		}
	}

	return sb.String()
}
